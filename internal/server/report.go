package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/report"
	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/store"
)

// buildReport reads the filter from the query and aggregates the group.
//
//	from, to       YYYY-MM-DD (inclusive)
//	categories     comma-separated ids
//	member         member id (paid by or shares)
//
// Settle-up payments are never part of a report.
func (s *Server) buildReport(r *http.Request) (*report.Report, error) {
	g := groupFrom(r.Context())
	q := r.URL.Query()
	f := report.Filter{From: q.Get("from"), To: q.Get("to"), MemberID: queryInt(r, "member")}
	if f.From != "" && !validDate(f.From) || f.To != "" && !validDate(f.To) {
		return nil, fmt.Errorf("dates must be YYYY-MM-DD")
	}
	for _, c := range strings.Split(q.Get("categories"), ",") {
		if c = strings.TrimSpace(c); c != "" {
			f.Categories = append(f.Categories, c)
		}
	}
	f.Debts = queryBool(r, "debts")
	exps, _, err := s.store.ListExpenses(r.Context(), store.ExpenseFilter{GroupID: g.ID, From: f.From, To: f.To, Categories: f.Categories, MemberID: f.MemberID, Reimbursements: "exclude"})
	if err != nil {
		return nil, err
	}
	cats, _ := s.store.ListCategories(r.Context(), userFrom(r.Context()).ID)
	balances, err := s.store.Balances(r.Context(), g.ID)
	if err != nil {
		return nil, err
	}
	// The expense list is newest first; a report reads better oldest first.
	for i, j := 0, len(exps)-1; i < j; i, j = i+1, j-1 {
		exps[i], exps[j] = exps[j], exps[i]
	}
	return report.Build(g, cats, exps, balances, f, displayName(userFrom(r.Context()))), nil
}

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	rep, err := s.buildReport(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (s *Server) handleReportPDF(w http.ResponseWriter, r *http.Request) {
	rep, err := s.buildReport(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	st, _ := s.store.GetSettings(r.Context())
	b, err := report.PDF(rep, st.Title)
	if err != nil {
		s.fail(w, err, "pdf")
		return
	}
	name := safeName(rep.Group.Name) + "-report"
	if rep.Filter.From != "" || rep.Filter.To != "" {
		name += "-" + strings.Trim(rep.Filter.From+"_"+rep.Filter.To, "_")
	} else {
		name += "-" + time.Now().Format("2006-01-02")
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.pdf"`)
	_, _ = w.Write(b)
}
