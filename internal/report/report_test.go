package report

import (
	"testing"

	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/store"
)

// A settle-up payment is not spending: the per-person table must add up to
// the same total as "Total spent", and the payment must not show up at all.
func TestBuildLeavesPaymentsOut(t *testing.T) {
	g := &store.Group{ID: 1, Name: "Flat", Currency: "EUR", Members: []store.Member{{ID: 1, Name: "Ana"}, {ID: 2, Name: "Marko"}}}
	exps := []*store.Expense{
		{ID: 1, Title: "Rent", Date: "2026-10-01", Category: "utilities", Currency: "EUR", Amount: 210522, AmountGroup: 210522, PaidBy: 1,
			Shares: []store.Share{{MemberID: 1, AmountGroup: 105261}, {MemberID: 2, AmountGroup: 105261}}},
		{ID: 2, Title: "Reimbursement", Date: "2026-10-05", Currency: "EUR", Amount: 70000, AmountGroup: 70000, PaidBy: 2, Reimbursement: true,
			Shares: []store.Share{{MemberID: 1, AmountGroup: 70000}}},
	}
	r := Build(g, nil, exps, nil, Filter{}, "test")
	if r.Total != 210522 || r.Count != 1 || len(r.Expenses) != 1 {
		t.Fatalf("total %d count %d expenses %d", r.Total, r.Count, len(r.Expenses))
	}
	var paid, share int64
	for _, m := range r.Members {
		paid += m.Paid
		share += m.Share
	}
	if paid != r.Total || share != r.Total {
		t.Fatalf("per person paid %d share %d, want both %d", paid, share, r.Total)
	}
	if _, err := PDF(r, "Pocket Split"); err != nil {
		t.Fatal(err)
	}
}
