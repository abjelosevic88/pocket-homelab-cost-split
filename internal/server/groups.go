package server

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/currency"
	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/split"
	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/store"
)

const groupKey ctxKey = 2

func groupFrom(ctx context.Context) *store.Group {
	g, _ := ctx.Value(groupKey).(*store.Group)
	return g
}

// withGroup loads the group in the URL and checks the viewer may see it: a
// linked member, or an admin. Others get 404, not 403, so group ids leak nothing.
func (s *Server) withGroup(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := userFrom(r.Context())
		g, err := s.store.GetGroup(r.Context(), idParam(r, "gid"), u.ID)
		if err != nil {
			s.fail(w, err, "group")
			return
		}
		if g.MyMemberID == 0 && u.Role != "admin" {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), groupKey, g)))
	})
}

func (s *Server) handleListGroups(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	f := store.GroupFilter{UserID: u.ID, Q: r.URL.Query().Get("q")}
	if queryBool(r, "all") && u.Role == "admin" {
		f.UserID = 0
	}
	switch r.URL.Query().Get("archived") {
	case "1", "true":
		t := true
		f.Archived = &t
	case "0", "false":
		t := false
		f.Archived = &t
	}
	groups, err := s.store.ListGroups(r.Context(), f, u.ID)
	if err != nil {
		s.fail(w, err, "list groups")
		return
	}
	writeJSON(w, http.StatusOK, groups)
}

type groupInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Currency    string `json:"currency"`
	Archived    *bool  `json:"archived"`
	// Create only: the first members. One may be the creator ("me": true).
	Members []struct {
		Name   string `json:"name"`
		UserID int64  `json:"user_id"`
		Me     bool   `json:"me"`
	} `json:"members"`
}

func (s *Server) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	var in groupInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeErr(w, http.StatusBadRequest, "the group needs a name")
		return
	}
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	if in.Currency == "" {
		st, _ := s.store.GetSettings(r.Context())
		in.Currency = st.DefaultCurrency
	}
	if !currency.Valid(in.Currency) {
		writeErr(w, http.StatusBadRequest, "unknown currency "+in.Currency)
		return
	}
	g := &store.Group{Name: in.Name, Description: strings.TrimSpace(in.Description), Currency: in.Currency, CreatedBy: u.ID}
	var members []store.Member
	hasMe := false
	for _, m := range in.Members {
		mm := store.Member{Name: strings.TrimSpace(m.Name)}
		if m.Me {
			uid := u.ID
			mm.UserID = &uid
			hasMe = true
			if mm.Name == "" {
				mm.Name = displayName(u)
			}
		} else if m.UserID > 0 {
			other, err := s.store.GetUser(r.Context(), m.UserID)
			if err != nil {
				writeErr(w, http.StatusBadRequest, "unknown user")
				return
			}
			uid := other.ID
			mm.UserID = &uid
			if mm.Name == "" {
				mm.Name = displayName(other)
			}
		}
		if mm.Name == "" {
			continue
		}
		members = append(members, mm)
	}
	if !hasMe {
		// The creator always takes part; without that they could not even open it.
		uid := u.ID
		members = append([]store.Member{{Name: displayName(u), UserID: &uid}}, members...)
	}
	if err := s.store.CreateGroup(r.Context(), g, members); err != nil {
		s.fail(w, err, "create group")
		return
	}
	s.store.LogActivity(r.Context(), store.Activity{GroupID: g.ID, UserID: u.ID, Kind: "group.created", Summary: g.Name})
	out, err := s.store.GetGroup(r.Context(), g.ID, u.ID)
	if err != nil {
		s.fail(w, err, "group")
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func displayName(u *store.User) string {
	if u.Name != "" {
		return u.Name
	}
	return strings.SplitN(u.Email, "@", 2)[0]
}

func (s *Server) handleGetGroup(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, groupFrom(r.Context()))
}

func (s *Server) handleUpdateGroup(w http.ResponseWriter, r *http.Request) {
	g := groupFrom(r.Context())
	var in groupInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if n := strings.TrimSpace(in.Name); n != "" {
		g.Name = n
	}
	g.Description = strings.TrimSpace(in.Description)
	if c := strings.ToUpper(strings.TrimSpace(in.Currency)); c != "" && c != g.Currency {
		if !currency.Valid(c) {
			writeErr(w, http.StatusBadRequest, "unknown currency "+c)
			return
		}
		if g.ExpenseCount > 0 {
			writeErr(w, http.StatusBadRequest, "the main currency cannot change once the group has expenses")
			return
		}
		g.Currency = c
	}
	if in.Archived != nil {
		g.Archived = *in.Archived
	}
	if err := s.store.UpdateGroup(r.Context(), g); err != nil {
		s.fail(w, err, "update group")
		return
	}
	s.store.LogActivity(r.Context(), store.Activity{GroupID: g.ID, UserID: userFrom(r.Context()).ID, Kind: "group.updated", Summary: g.Name})
	out, _ := s.store.GetGroup(r.Context(), g.ID, userFrom(r.Context()).ID)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	g := groupFrom(r.Context())
	u := userFrom(r.Context())
	if u.Role != "admin" && g.CreatedBy != u.ID {
		writeErr(w, http.StatusForbidden, "only the person who created the group (or an admin) can delete it")
		return
	}
	if err := s.store.DeleteGroup(r.Context(), g.ID); err != nil {
		s.fail(w, err, "delete group")
		return
	}
	s.log.Info("group deleted", "id", g.ID, "name", g.Name, "by", u.Email)
	w.WriteHeader(http.StatusNoContent)
}

// ---- members ----

type memberInput struct {
	Name   string `json:"name"`
	UserID *int64 `json:"user_id"` // null = no account; 0 = unlink
}

func (s *Server) handleAddMember(w http.ResponseWriter, r *http.Request) {
	g := groupFrom(r.Context())
	var in memberInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	m := store.Member{GroupID: g.ID, Name: strings.TrimSpace(in.Name)}
	if in.UserID != nil && *in.UserID > 0 {
		other, err := s.store.GetUser(r.Context(), *in.UserID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "unknown user")
			return
		}
		m.UserID = in.UserID
		if m.Name == "" {
			m.Name = displayName(other)
		}
	}
	if m.Name == "" {
		writeErr(w, http.StatusBadRequest, "a name is required")
		return
	}
	for _, x := range g.Members {
		if strings.EqualFold(x.Name, m.Name) {
			writeErr(w, http.StatusConflict, "there is already a member called "+x.Name)
			return
		}
	}
	if err := s.store.AddMember(r.Context(), &m); err != nil {
		if err == store.ErrConflict {
			writeErr(w, http.StatusConflict, "that user is already in the group")
			return
		}
		s.fail(w, err, "add member")
		return
	}
	s.store.Touch(r.Context(), g.ID)
	s.store.LogActivity(r.Context(), store.Activity{GroupID: g.ID, UserID: userFrom(r.Context()).ID, Kind: "member.added", Summary: m.Name})
	writeJSON(w, http.StatusCreated, m)
}

func (s *Server) handleUpdateMember(w http.ResponseWriter, r *http.Request) {
	g := groupFrom(r.Context())
	m, err := s.store.GetMember(r.Context(), g.ID, idParam(r, "mid"))
	if err != nil {
		s.fail(w, err, "member")
		return
	}
	var in memberInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if n := strings.TrimSpace(in.Name); n != "" {
		m.Name = n
	}
	if in.UserID != nil {
		if *in.UserID > 0 {
			if _, err := s.store.GetUser(r.Context(), *in.UserID); err != nil {
				writeErr(w, http.StatusBadRequest, "unknown user")
				return
			}
			m.UserID = in.UserID
		} else {
			m.UserID = nil
		}
	}
	if err := s.store.UpdateMember(r.Context(), m); err != nil {
		if err == store.ErrConflict {
			writeErr(w, http.StatusConflict, "that user is already in the group")
			return
		}
		s.fail(w, err, "update member")
		return
	}
	s.store.Touch(r.Context(), g.ID)
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) handleDeleteMember(w http.ResponseWriter, r *http.Request) {
	g := groupFrom(r.Context())
	id := idParam(r, "mid")
	m, err := s.store.GetMember(r.Context(), g.ID, id)
	if err != nil {
		s.fail(w, err, "member")
		return
	}
	if used, _ := s.store.MemberInUse(r.Context(), id); used {
		writeErr(w, http.StatusBadRequest, m.Name+" is part of expenses and cannot be removed; edit those expenses first")
		return
	}
	if len(g.Members) <= 1 {
		writeErr(w, http.StatusBadRequest, "a group needs at least one member")
		return
	}
	if err := s.store.DeleteMember(r.Context(), g.ID, id); err != nil {
		s.fail(w, err, "remove member")
		return
	}
	s.store.Touch(r.Context(), g.ID)
	s.store.LogActivity(r.Context(), store.Activity{GroupID: g.ID, UserID: userFrom(r.Context()).ID, Kind: "member.removed", Summary: m.Name})
	w.WriteHeader(http.StatusNoContent)
}

// ---- balances / stats / activity ----

func (s *Server) handleBalances(w http.ResponseWriter, r *http.Request) {
	g := groupFrom(r.Context())
	bals, err := s.store.Balances(r.Context(), g.ID)
	if err != nil {
		s.fail(w, err, "balances")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"currency": g.Currency, "balances": bals, "settlements": split.Settle(bals)})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	g := groupFrom(r.Context())
	st, err := s.store.Stats(r.Context(), g.ID, g.Currency)
	if err != nil {
		s.fail(w, err, "stats")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleGroupActivity(w http.ResponseWriter, r *http.Request) {
	g := groupFrom(r.Context())
	a, err := s.store.ListActivity(r.Context(), g.ID, 0, int(queryInt(r, "limit")))
	if err != nil {
		s.fail(w, err, "activity")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) handleAllActivity(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	a, err := s.store.ListActivity(r.Context(), 0, u.ID, int(queryInt(r, "limit")))
	if err != nil {
		s.fail(w, err, "activity")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// ---- dashboard ----

// CurrencyPosition is the viewer's standing in one currency across groups.
type CurrencyPosition struct {
	Currency   string `json:"currency"`
	YouOwe     int64  `json:"you_owe"`      // minor units
	YouAreOwed int64  `json:"you_are_owed"` // minor units
	Net        int64  `json:"net"`
}

// Dashboard is the compact status used by widgets and the home page.
type Dashboard struct {
	Groups         int                   `json:"groups"`
	ArchivedGroups int                   `json:"archived_groups"`
	Positions      []CurrencyPosition    `json:"positions"`
	YouOweText     string                `json:"you_owe_text"`
	YouAreOwedText string                `json:"you_are_owed_text"`
	NetText        string                `json:"net_text"`
	SpentThisMonth []store.CurrencyTotal `json:"spent_this_month"`
	SpentText      string                `json:"spent_text"`
	GroupLines     []GroupLine           `json:"group_lines"`
}

// GroupLine is one group on the home page.
type GroupLine struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Currency string `json:"currency"`
	Balance  int64  `json:"balance"`
	Members  int    `json:"members"`
	Expenses int    `json:"expenses"`
	Total    int64  `json:"total"`
	LastDate string `json:"last_date"`
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	groups, err := s.store.ListGroups(r.Context(), store.GroupFilter{UserID: u.ID}, u.ID)
	if err != nil {
		s.fail(w, err, "dashboard")
		return
	}
	d := Dashboard{Positions: []CurrencyPosition{}, GroupLines: []GroupLine{}}
	pos := map[string]*CurrencyPosition{}
	for _, g := range groups {
		if g.Archived {
			d.ArchivedGroups++
			continue
		}
		d.Groups++
		p := pos[g.Currency]
		if p == nil {
			p = &CurrencyPosition{Currency: g.Currency}
			pos[g.Currency] = p
		}
		if g.MyBalance < 0 {
			p.YouOwe += -g.MyBalance
		} else {
			p.YouAreOwed += g.MyBalance
		}
		p.Net += g.MyBalance
		d.GroupLines = append(d.GroupLines, GroupLine{ID: g.ID, Name: g.Name, Currency: g.Currency, Balance: g.MyBalance, Members: len(g.Members), Expenses: g.ExpenseCount, Total: g.Total, LastDate: g.LastActivity})
	}
	for _, p := range pos {
		d.Positions = append(d.Positions, *p)
	}
	sort.Slice(d.Positions, func(i, j int) bool { return d.Positions[i].Currency < d.Positions[j].Currency })
	d.YouOweText = joinMoney(d.Positions, func(p CurrencyPosition) int64 { return p.YouOwe })
	d.YouAreOwedText = joinMoney(d.Positions, func(p CurrencyPosition) int64 { return p.YouAreOwed })
	d.NetText = joinMoney(d.Positions, func(p CurrencyPosition) int64 { return p.Net })
	d.SpentThisMonth, _ = s.store.MonthSpend(r.Context(), u.ID, time.Now().Format("2006-01"))
	var parts []string
	for _, c := range d.SpentThisMonth {
		parts = append(parts, FormatMoney(c.Total, c.Currency))
	}
	d.SpentText = strings.Join(parts, " · ")
	if d.SpentText == "" {
		d.SpentText = "—"
	}
	writeJSON(w, http.StatusOK, d)
}

func joinMoney(ps []CurrencyPosition, pick func(CurrencyPosition) int64) string {
	var parts []string
	for _, p := range ps {
		if v := pick(p); v != 0 {
			parts = append(parts, FormatMoney(v, p.Currency))
		}
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, " · ")
}

// FormatMoney renders minor units as "12.50 EUR" / "-3 JPY" / "1.250 BHD".
func FormatMoney(minor int64, code string) string {
	d := currency.Decimals(code)
	sign := ""
	if minor < 0 {
		sign = "-"
		minor = -minor
	}
	if d == 0 {
		return fmt.Sprintf("%s%d %s", sign, minor, code)
	}
	p := int64(1)
	for i := 0; i < d; i++ {
		p *= 10
	}
	return fmt.Sprintf("%s%d.%0*d %s", sign, minor/p, d, minor%p, code)
}
