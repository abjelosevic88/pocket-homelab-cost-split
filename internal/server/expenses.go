package server

import (
	"context"
	"errors"
	"fmt"
	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/currency"
	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/fx"
	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/split"
	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/store"
)

func (s *Server) handleListExpenses(w http.ResponseWriter, r *http.Request) {
	g := groupFrom(r.Context())
	q := r.URL.Query()
	f := store.ExpenseFilter{GroupID: g.ID, Q: q.Get("q"), MemberID: queryInt(r, "member"), Category: q.Get("category"), From: q.Get("from"), To: q.Get("to"), Limit: int(queryInt(r, "limit")), Offset: int(queryInt(r, "offset"))}
	list, total, err := s.store.ListExpenses(r.Context(), f)
	if err != nil {
		s.fail(w, err, "list expenses")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"expenses": list, "total": total})
}

func (s *Server) handleGetExpense(w http.ResponseWriter, r *http.Request) {
	g := groupFrom(r.Context())
	e, err := s.store.GetExpense(r.Context(), g.ID, idParam(r, "eid"))
	if err != nil {
		s.fail(w, err, "expense")
		return
	}
	writeJSON(w, http.StatusOK, e)
}

// expenseInput is the body of POST/PUT expenses. Money is in minor units of
// `currency` (cents): 12.50 EUR = 1250, 1500 JPY = 1500.
type expenseInput struct {
	Title         string   `json:"title"`
	Amount        int64    `json:"amount"`
	Currency      string   `json:"currency"`
	FxRate        *float64 `json:"fx_rate"` // 1 unit of currency in group currency; omit to look it up
	PaidBy        int64    `json:"paid_by"`
	Date          string   `json:"date"`
	Category      string   `json:"category"`
	Notes         string   `json:"notes"`
	SplitMode     string   `json:"split_mode"`
	Reimbursement bool     `json:"reimbursement"`
	Shares        []struct {
		MemberID int64 `json:"member_id"`
		Share    int64 `json:"share"`
	} `json:"shares"`
}

// buildExpense validates input against the group and computes every amount.
func (s *Server) buildExpense(ctx context.Context, g *store.Group, in expenseInput, e *store.Expense) error {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		if in.Reimbursement {
			in.Title = "Reimbursement"
		} else {
			return errors.New("a title is required")
		}
	}
	if in.Amount == 0 {
		return errors.New("the amount cannot be zero")
	}
	if in.Amount < 0 && !in.Reimbursement {
		return errors.New("the amount must be positive (use a reimbursement for money going back)")
	}
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	if in.Currency == "" {
		in.Currency = g.Currency
	}
	if !currency.Valid(in.Currency) {
		return errors.New("unknown currency " + in.Currency)
	}
	if in.Date == "" {
		in.Date = store.Today()
	}
	if !validDate(in.Date) {
		return errors.New("date must be YYYY-MM-DD")
	}
	// A payment just moves money between two people; it has no category.
	if in.Reimbursement {
		in.Category = ""
	} else {
		if in.Category == "" {
			in.Category = s.store.FallbackCategory(ctx)
		}
		if !s.store.CategoryExists(ctx, in.Category, actorID(ctx)) {
			return errors.New("unknown category " + in.Category)
		}
	}
	if in.SplitMode == "" {
		in.SplitMode = split.ModeEven
	}
	if !split.ValidMode(in.SplitMode) {
		return errors.New("unknown split mode " + in.SplitMode)
	}
	members := map[int64]store.Member{}
	for _, m := range g.Members {
		members[m.ID] = m
	}
	if _, ok := members[in.PaidBy]; !ok {
		return errors.New("choose who paid")
	}
	if len(in.Shares) == 0 {
		return split.ErrNoParticipants
	}
	seen := map[int64]bool{}
	e.Shares = e.Shares[:0]
	for _, sh := range in.Shares {
		if _, ok := members[sh.MemberID]; !ok {
			return fmt.Errorf("member %d is not in this group", sh.MemberID)
		}
		if seen[sh.MemberID] {
			return errors.New("a member is listed twice")
		}
		seen[sh.MemberID] = true
		e.Shares = append(e.Shares, store.Share{MemberID: sh.MemberID, Share: sh.Share})
	}
	if in.Reimbursement && (len(e.Shares) != 1 || e.Shares[0].MemberID == in.PaidBy) {
		return errors.New("a reimbursement goes from one member to one other member")
	}
	if in.Reimbursement {
		in.SplitMode = split.ModeEven
	}
	// Exchange rate: given, kept from the saved expense if nothing changed, or looked up.
	rate := 1.0
	if in.Currency != g.Currency {
		switch {
		case in.FxRate != nil && *in.FxRate > 0:
			rate = *in.FxRate
		case e.ID != 0 && e.Currency == in.Currency && e.FxRate > 0:
			rate = e.FxRate
		default:
			r, _, err := s.lookupRate(ctx, in.Currency, g.Currency, in.Date)
			if err != nil {
				return fmt.Errorf("no exchange rate for %s → %s: enter one by hand", in.Currency, g.Currency)
			}
			rate = r
		}
		if math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= 0 {
			return errors.New("the exchange rate must be a positive number")
		}
	}
	e.Title, e.Amount, e.Currency, e.FxRate = in.Title, in.Amount, in.Currency, rate
	e.AmountGroup = currency.Convert(in.Amount, in.Currency, g.Currency, rate)
	e.PaidBy, e.Date, e.Category, e.Notes, e.SplitMode, e.Reimbursement = in.PaidBy, in.Date, in.Category, strings.TrimSpace(in.Notes), in.SplitMode, in.Reimbursement
	return store.ComputeShares(e)
}

func (s *Server) handleCreateExpense(w http.ResponseWriter, r *http.Request) {
	g := groupFrom(r.Context())
	u := userFrom(r.Context())
	var in expenseInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	op, err := core.NewOperation(fmt.Sprintf("%d:%s:groups:%d:expenses", u.ID, linkApp(r.Context()), g.ID), r.Header.Get("Idempotency-Key"), in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	e := &store.Expense{GroupID: g.ID, CreatedBy: u.ID, Source: linkApp(r.Context())}
	// Resolve a replay before defaults or exchange rates can change.
	tx, err := s.store.DB.BeginTx(r.Context(), nil)
	if err != nil {
		s.fail(w, err, "read operation")
		return
	}
	found, err := op.Replay(r.Context(), tx, e)
	tx.Rollback()
	if err != nil {
		s.fail(w, err, "read operation")
		return
	}
	if found {
		current, err := s.store.GetExpense(r.Context(), g.ID, e.ID)
		if err != nil {
			s.fail(w, err, "expense")
			return
		}
		if !linkOwns(r.Context(), current) {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		writeJSON(w, http.StatusCreated, e)
		return
	}
	if err = s.buildExpense(r.Context(), g, in, e); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	kind, topic := "expense.added", "split.expense_created"
	if e.Reimbursement {
		kind, topic = "reimbursement.added", "split.settled"
	}
	replayed, err := s.store.SaveExpenseOperation(r.Context(), e, op, s.expenseEvent(r.Context(), u, g, e, topic))
	if err != nil {
		s.fail(w, err, "save expense")
		return
	}
	if !replayed {
		s.store.LogActivity(r.Context(), store.Activity{GroupID: g.ID, UserID: u.ID, Kind: kind, ExpenseID: e.ID, Summary: s.expenseSummary(g, e) + viaLink(r.Context())})
	}
	writeJSON(w, http.StatusCreated, e)
}

func (s *Server) handleUpdateExpense(w http.ResponseWriter, r *http.Request) {
	g := groupFrom(r.Context())
	u := userFrom(r.Context())
	e, err := s.store.GetExpense(r.Context(), g.ID, idParam(r, "eid"))
	if err != nil {
		s.fail(w, err, "expense")
		return
	}
	if !linkOwns(r.Context(), e) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	var in expenseInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.buildExpense(r.Context(), g, in, e); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.SaveExpense(r.Context(), e); err != nil {
		s.fail(w, err, "save expense")
		return
	}
	s.store.LogActivity(r.Context(), store.Activity{GroupID: g.ID, UserID: u.ID, Kind: "expense.updated", ExpenseID: e.ID, Summary: s.expenseSummary(g, e) + viaLink(r.Context())})
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) handleDeleteExpense(w http.ResponseWriter, r *http.Request) {
	g := groupFrom(r.Context())
	u := userFrom(r.Context())
	e, err := s.store.GetExpense(r.Context(), g.ID, idParam(r, "eid"))
	if err != nil {
		s.fail(w, err, "expense")
		return
	}
	if !linkOwns(r.Context(), e) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if err := s.store.DeleteExpense(r.Context(), g.ID, e.ID, s.expenseEvent(r.Context(), u, g, e, "split.expense_deleted")); err != nil {
		s.fail(w, err, "delete expense")
		return
	}
	s.store.LogActivity(r.Context(), store.Activity{GroupID: g.ID, UserID: u.ID, Kind: "expense.deleted", Summary: s.expenseSummary(g, e) + viaLink(r.Context())})
	w.WriteHeader(http.StatusNoContent)
}

// audienceEmails lists the accounts that may see the group (its creator and
// every linked member), so a consumer of the event bus can show it to exactly
// those people and nobody else.
func (s *Server) audienceEmails(ctx context.Context, g *store.Group) []string {
	ids := []int64{}
	seen := map[int64]bool{}
	add := func(id int64) {
		if id != 0 && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	add(g.CreatedBy)
	for _, m := range g.Members {
		if m.UserID != nil {
			add(*m.UserID)
		}
	}
	out := []string{}
	for _, id := range ids {
		if u, err := s.store.GetUser(ctx, id); err == nil {
			out = append(out, u.Email)
		}
	}
	return out
}

// expenseSummary is the one-line form for the activity feed: "Dinner · 48.00 EUR · paid by Ana".
func (s *Server) expenseSummary(g *store.Group, e *store.Expense) string {
	payer := ""
	for _, m := range g.Members {
		if m.ID == e.PaidBy {
			payer = m.Name
		}
	}
	if e.Reimbursement {
		to := ""
		for _, m := range g.Members {
			if len(e.Shares) == 1 && m.ID == e.Shares[0].MemberID {
				to = m.Name
			}
		}
		return fmt.Sprintf("%s → %s · %s", payer, to, FormatMoney(e.Amount, e.Currency))
	}
	return fmt.Sprintf("%s · %s · paid by %s", e.Title, FormatMoney(e.Amount, e.Currency), payer)
}

// ---- currencies / fx ----

func (s *Server) handleCurrencies(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, currency.All())
}

// lookupRate answers from the cache, else asks the providers and remembers.
// Today's rate is refreshed when the cached one is older than six hours.
func (s *Server) lookupRate(ctx context.Context, from, to, date string) (float64, string, error) {
	if from == to {
		return 1, "same", nil
	}
	if st, err := s.store.GetSettings(ctx); err == nil && !st.FxAuto {
		return 0, "", fx.ErrUnavailable
	}
	if !validDate(date) || date > store.Today() {
		date = store.Today()
	}
	if rate, source, fetched, err := s.store.GetFxRate(ctx, from, to, date); err == nil {
		fresh := true
		if date == store.Today() {
			if t, err := time.Parse(time.RFC3339Nano, fetched); err == nil && time.Since(t) > 6*time.Hour {
				fresh = false
			}
		}
		if fresh {
			return rate, source + " (cached)", nil
		}
	}
	// Try the inverse pair too: a cached EUR→USD gives USD→EUR for free.
	if rate, source, _, err := s.store.GetFxRate(ctx, to, from, date); err == nil && rate > 0 && date != store.Today() {
		return 1 / rate, source + " (cached)", nil
	}
	rate, source, err := s.fx.Rate(ctx, from, to, date)
	if err != nil {
		return 0, "", err
	}
	_ = s.store.PutFxRate(ctx, from, to, date, rate, source)
	return rate, source, nil
}

func (s *Server) handleFx(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, to, date := strings.ToUpper(q.Get("from")), strings.ToUpper(q.Get("to")), q.Get("date")
	if !currency.Valid(from) || !currency.Valid(to) {
		writeErr(w, http.StatusBadRequest, "from and to must be ISO 4217 codes")
		return
	}
	if date == "" || !validDate(date) {
		date = store.Today()
	}
	rate, source, err := s.lookupRate(r.Context(), from, to, date)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "no rate: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"from": from, "to": to, "date": date, "rate": rate, "source": source,
		"example": FormatMoney(currency.Convert(100*int64(math.Pow10(currency.Decimals(from))), from, to, rate), to)})
}
