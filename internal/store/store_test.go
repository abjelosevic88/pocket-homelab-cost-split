package store

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/split"
)

func openTest(t *testing.T) (*Store, *Group, []Member) {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "t.db"), slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	ctx := context.Background()
	u := &User{Email: "ana@example.com", Name: "Ana", Role: "admin"}
	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	g := &Group{Name: "Trip", Currency: "EUR", CreatedBy: u.ID}
	members := []Member{{Name: "Ana", UserID: &u.ID}, {Name: "Marko"}, {Name: "Jelena"}}
	if err := s.CreateGroup(ctx, g, members); err != nil {
		t.Fatal(err)
	}
	return s, g, members
}

func TestBalancesAddUpAndSettle(t *testing.T) {
	s, g, m := openTest(t)
	ctx := context.Background()
	// Ana pays 90 for everyone, Marko 30 for everyone, Jelena 15 in dollars (rate 0.8) for Ana and herself.
	exps := []*Expense{
		{GroupID: g.ID, Title: "Dinner", Amount: 9000, Currency: "EUR", FxRate: 1, AmountGroup: 9000, PaidBy: m[0].ID, Date: "2026-10-01", Category: "restaurant", SplitMode: split.ModeEven, Shares: []Share{{MemberID: m[0].ID}, {MemberID: m[1].ID}, {MemberID: m[2].ID}}},
		{GroupID: g.ID, Title: "Taxi", Amount: 3000, Currency: "EUR", FxRate: 1, AmountGroup: 3000, PaidBy: m[1].ID, Date: "2026-10-02", Category: "transport", SplitMode: split.ModeEven, Shares: []Share{{MemberID: m[0].ID}, {MemberID: m[1].ID}, {MemberID: m[2].ID}}},
		{GroupID: g.ID, Title: "Coffee", Amount: 1500, Currency: "USD", FxRate: 0.8, AmountGroup: 1200, PaidBy: m[2].ID, Date: "2026-10-03", Category: "drinks", SplitMode: split.ModeEven, Shares: []Share{{MemberID: m[0].ID}, {MemberID: m[2].ID}}},
	}
	for _, e := range exps {
		if err := ComputeShares(e); err != nil {
			t.Fatal(err)
		}
		if err := s.SaveExpense(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	// Shares of the dollar coffee: 750 + 750 USD, 600 + 600 EUR.
	coffee, _ := s.GetExpense(ctx, g.ID, exps[2].ID)
	if len(coffee.Shares) != 2 || coffee.Shares[0].Amount != 750 || coffee.Shares[0].AmountGroup != 600 {
		t.Fatalf("coffee shares: %+v", coffee.Shares)
	}
	bals, err := s.Balances(ctx, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	var sum int64
	byID := map[int64]split.Balance{}
	for _, b := range bals {
		sum += b.Balance
		byID[b.MemberID] = b
	}
	if sum != 0 {
		t.Fatalf("balances sum to %d: %+v", sum, bals)
	}
	// Ana: paid 9000, owes 3000 + 1000 + 600 = 4600 → +4400.
	if byID[m[0].ID].Balance != 4400 {
		t.Fatalf("ana: %+v", byID[m[0].ID])
	}
	tr := split.Settle(bals)
	var moved int64
	for _, x := range tr {
		moved += x.Amount
	}
	if len(tr) != 2 || moved != 4400 {
		t.Fatalf("settle: %+v", tr)
	}
	// A reimbursement from Jelena to Ana closes her side.
	j := byID[m[2].ID].Balance
	re := &Expense{GroupID: g.ID, Title: "Reimbursement", Amount: -j, Currency: "EUR", FxRate: 1, AmountGroup: -j, PaidBy: m[2].ID, Date: "2026-10-04", SplitMode: split.ModeEven, Reimbursement: true, Shares: []Share{{MemberID: m[0].ID}}}
	if err := ComputeShares(re); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveExpense(ctx, re); err != nil {
		t.Fatal(err)
	}
	bals, _ = s.Balances(ctx, g.ID)
	for _, b := range bals {
		if b.MemberID == m[2].ID && b.Balance != 0 {
			t.Fatalf("jelena after paying: %+v", b)
		}
	}
	st, err := s.Stats(ctx, g.ID, g.Currency)
	if err != nil {
		t.Fatal(err)
	}
	if st.Total != 13200 || st.Reimbursed != -j || st.Count != 3 || len(st.Currencies) != 2 {
		t.Fatalf("stats: %+v", st)
	}
	// Members in expenses cannot go; the group lists them with the viewer's position.
	if used, _ := s.MemberInUse(ctx, m[1].ID); !used {
		t.Fatal("marko should be in use")
	}
	gg, err := s.GetGroup(ctx, g.ID, *m[0].UserID)
	if err != nil {
		t.Fatal(err)
	}
	if gg.MyMemberID != m[0].ID || gg.MyBalance != 1000 || gg.ExpenseCount != 4 || gg.Total != 13200 {
		t.Fatalf("group view: %+v", gg)
	}
	// Editing replaces the shares.
	exps[0].SplitMode, exps[0].Shares = split.ModeShares, []Share{{MemberID: m[0].ID, Share: 2}, {MemberID: m[1].ID, Share: 1}}
	if err := ComputeShares(exps[0]); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveExpense(ctx, exps[0]); err != nil {
		t.Fatal(err)
	}
	e, _ := s.GetExpense(ctx, g.ID, exps[0].ID)
	if len(e.Shares) != 2 || e.Shares[0].Amount != 6000 || e.Shares[1].Amount != 3000 {
		t.Fatalf("edited shares: %+v", e.Shares)
	}
	list, total, err := s.ListExpenses(ctx, ExpenseFilter{GroupID: g.ID, MemberID: m[2].ID})
	if err != nil || total != 3 || len(list) != 3 {
		t.Fatalf("filter by jelena: %d %v %v", total, err, list)
	}
	if err := s.DeleteGroup(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	if n := s.Counts(ctx); n["expenses"] != 0 || n["members"] != 0 || n["expense_shares"] != 0 {
		t.Fatalf("cascade: %v", n)
	}
}

func TestComputeSharesModes(t *testing.T) {
	e := &Expense{Amount: 1000, AmountGroup: 1900, SplitMode: split.ModePercent, Shares: []Share{{MemberID: 2, Share: 2500}, {MemberID: 1, Share: 7500}}}
	if err := ComputeShares(e); err != nil {
		t.Fatal(err)
	}
	// Sorted by member id; group amounts proportional and adding up to 1900.
	if e.Shares[0].MemberID != 1 || e.Shares[0].Amount != 750 || e.Shares[0].AmountGroup != 1425 || e.Shares[1].AmountGroup != 475 {
		t.Fatalf("percent: %+v", e.Shares)
	}
	e = &Expense{Amount: 1000, AmountGroup: 1000, SplitMode: split.ModeAmount, Shares: []Share{{MemberID: 1, Share: 999}, {MemberID: 2, Share: 2}}}
	if err := ComputeShares(e); err != split.ErrAmounts {
		t.Fatalf("amount mismatch: %v", err)
	}
	e = &Expense{Amount: 1000, AmountGroup: 1000, SplitMode: "weird", Shares: []Share{{MemberID: 1}}}
	if err := ComputeShares(e); err == nil {
		t.Fatal("unknown mode accepted")
	}
}

func TestDeleteGeneralCategory(t *testing.T) {
	s, g, m := openTest(t)
	ctx := context.Background()
	e := &Expense{GroupID: g.ID, Title: "Misc", Amount: 1000, Currency: "EUR", FxRate: 1, AmountGroup: 1000, PaidBy: m[0].ID, Date: "2026-10-01", Category: "general", SplitMode: split.ModeEven, Shares: []Share{{MemberID: m[0].ID}, {MemberID: m[1].ID}}}
	if err := ComputeShares(e); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveExpense(ctx, e); err != nil {
		t.Fatal(err)
	}
	// In use: General stays.
	if err := s.DeleteCategory(ctx, "general"); err == nil {
		t.Fatal("deleted General while an expense uses it")
	}
	// Unused: it goes, and the next built-in category takes over as the fallback.
	e.Category = "groceries"
	if err := s.SaveExpense(ctx, e); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteCategory(ctx, "general"); err != nil {
		t.Fatal(err)
	}
	if fb := s.FallbackCategory(ctx); fb != "groceries" {
		t.Fatalf("fallback after General is gone: %q", fb)
	}
	// A removed category's expenses move to the fallback that is left.
	if err := s.DeleteCategory(ctx, "groceries"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetExpense(ctx, g.ID, e.ID)
	if got.Category != "restaurant" {
		t.Fatalf("expense moved to %q, want restaurant", got.Category)
	}
	// The last built-in category always stays.
	for _, id := range []string{"restaurant", "drinks", "transport", "fuel", "accommodation", "entertainment", "shopping", "health", "utilities", "gifts", "travel"} {
		if err := s.DeleteCategory(ctx, id); err != nil {
			t.Fatal(id, err)
		}
	}
	if err := s.DeleteCategory(ctx, "other"); err == nil {
		t.Fatal("deleted the last built-in category")
	}
}
