package server

import (
	"context"
	"time"

	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/currency"
	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/split"
	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/store"
)

// seedDemo fills an empty database with a believable trip so screenshots and
// first impressions work. No network: exchange rates are written in.
func (s *Server) seedDemo(ctx context.Context) error {
	users, _ := s.store.ListUsers(ctx)
	if len(users) == 0 {
		u := &store.User{Email: "admin@localhost", Name: "Ana", Role: "admin"}
		if err := s.store.CreateUser(ctx, u); err != nil {
			return err
		}
		users = []store.User{*u}
	}
	me := users[0]
	if n, _ := s.store.CountGroups(ctx); n > 0 {
		return nil
	}
	uid := me.ID
	now := time.Now()
	day := func(d int) string { return now.AddDate(0, 0, -d).Format("2006-01-02") }

	// --- A week in Lisbon, in EUR, with a few payments in other currencies.
	g := &store.Group{Name: "Lisbon week", Description: "Flights, flat, food — the usual", Currency: "EUR", CreatedBy: uid}
	members := []store.Member{{Name: displayName(&me), UserID: &uid}, {Name: "Marko"}, {Name: "Jelena"}}
	if err := s.store.CreateGroup(ctx, g, members); err != nil {
		return err
	}
	ana, marko, jelena := members[0].ID, members[1].ID, members[2].ID
	all := []int64{ana, marko, jelena}
	type row struct {
		title, cur string
		amount     int64
		rate       float64
		paidBy     int64
		date       string
		cat        string
		mode       string
		for_       []int64
		shares     []int64
		notes      string
	}
	rows := []row{
		{"Flights BEG → LIS", "EUR", 38700, 1, ana, day(40), "travel", split.ModeEven, all, nil, "Booked together, Ana's card"},
		{"Flat in Alfama (6 nights)", "EUR", 84000, 1, marko, day(38), "accommodation", split.ModeShares, all, []int64{1, 1, 1}, ""},
		{"Taxi from the airport", "EUR", 2350, 1, jelena, day(9), "transport", split.ModeEven, all, nil, ""},
		{"Groceries, Pingo Doce", "EUR", 6412, 1, ana, day(9), "groceries", split.ModeEven, all, nil, ""},
		{"Dinner at Cervejaria Ramiro", "EUR", 12600, 1, marko, day(8), "restaurant", split.ModeAmount, all, []int64{3800, 4800, 4000}, "Marko had the lobster"},
		{"Pastéis de Belém", "EUR", 1380, 1, jelena, day(7), "restaurant", split.ModeEven, all, nil, ""},
		{"Day trip to Sintra, train", "EUR", 1560, 1, ana, day(6), "transport", split.ModeEven, []int64{ana, jelena}, nil, "Marko stayed in"},
		{"Pena Palace tickets", "EUR", 4000, 1, jelena, day(6), "entertainment", split.ModeEven, []int64{ana, jelena}, nil, ""},
		{"Drinks in Bairro Alto", "EUR", 4700, 1, marko, day(5), "drinks", split.ModePercent, all, []int64{2000, 5000, 3000}, ""},
		{"Souvenirs on Amazon.co.uk", "GBP", 4299, 1.1731, ana, day(4), "shopping", split.ModeEven, all, nil, "Paid in pounds, card statement"},
		{"Lunch, paid in dollars", "USD", 8250, 0.9218, jelena, day(3), "restaurant", split.ModeEven, all, nil, "Café only took USD cash"},
		{"Fuel for the rental, Spain", "EUR", 7120, 1, marko, day(2), "fuel", split.ModeEven, all, nil, ""},
	}
	for _, r := range rows {
		e := &store.Expense{GroupID: g.ID, CreatedBy: uid, Title: r.title, Amount: r.amount, Currency: r.cur, FxRate: r.rate, PaidBy: r.paidBy, Date: r.date, Category: r.cat, Notes: r.notes, SplitMode: r.mode}
		e.AmountGroup = currency.Convert(r.amount, r.cur, g.Currency, r.rate)
		for i, m := range r.for_ {
			sh := store.Share{MemberID: m, Share: 1}
			if r.shares != nil {
				sh.Share = r.shares[i]
			}
			e.Shares = append(e.Shares, sh)
		}
		if err := store.ComputeShares(e); err != nil {
			return err
		}
		if err := s.store.SaveExpense(ctx, e); err != nil {
			return err
		}
		s.store.LogActivity(ctx, store.Activity{GroupID: g.ID, UserID: uid, Kind: "expense.added", ExpenseID: e.ID, Summary: s.expenseSummaryFor(members, e)})
	}
	// One reimbursement so the feature shows.
	re := &store.Expense{GroupID: g.ID, CreatedBy: uid, Title: "Reimbursement", Amount: 10000, Currency: "EUR", FxRate: 1, AmountGroup: 10000, PaidBy: jelena, Date: day(1), SplitMode: split.ModeEven, Reimbursement: true, Shares: []store.Share{{MemberID: marko, Share: 1}}}
	if err := store.ComputeShares(re); err != nil {
		return err
	}
	if err := s.store.SaveExpense(ctx, re); err != nil {
		return err
	}
	s.store.LogActivity(ctx, store.Activity{GroupID: g.ID, UserID: uid, Kind: "reimbursement.added", ExpenseID: re.ID, Summary: s.expenseSummaryFor(members, re)})

	// --- A flat share in BAM.
	g2 := &store.Group{Name: "Flat on Kralja Petra", Description: "Rent, bills and the odd shared shop", Currency: "BAM", CreatedBy: uid}
	m2 := []store.Member{{Name: displayName(&me), UserID: &uid}, {Name: "Nikola"}}
	if err := s.store.CreateGroup(ctx, g2, m2); err != nil {
		return err
	}
	both := []int64{m2[0].ID, m2[1].ID}
	rows2 := []row{
		{"Rent, this month", "BAM", 90000, 1, m2[0].ID, now.Format("2006-01") + "-01", "utilities", split.ModeEven, both, nil, ""},
		{"Electricity", "BAM", 8740, 1, m2[1].ID, day(12), "utilities", split.ModeEven, both, nil, ""},
		{"Internet", "BAM", 4500, 1, m2[1].ID, day(12), "utilities", split.ModeEven, both, nil, ""},
		{"IKEA shelf, paid in euros", "EUR", 7999, 1.95583, m2[0].ID, day(15), "shopping", split.ModeEven, both, nil, "Fixed EUR peg"},
		{"Toilet paper & co.", "BAM", 2360, 1, m2[0].ID, day(3), "groceries", split.ModeEven, both, nil, ""},
	}
	for _, r := range rows2 {
		e := &store.Expense{GroupID: g2.ID, CreatedBy: uid, Title: r.title, Amount: r.amount, Currency: r.cur, FxRate: r.rate, PaidBy: r.paidBy, Date: r.date, Category: r.cat, Notes: r.notes, SplitMode: r.mode}
		e.AmountGroup = currency.Convert(r.amount, r.cur, g2.Currency, r.rate)
		for _, m := range r.for_ {
			e.Shares = append(e.Shares, store.Share{MemberID: m, Share: 1})
		}
		if err := store.ComputeShares(e); err != nil {
			return err
		}
		if err := s.store.SaveExpense(ctx, e); err != nil {
			return err
		}
		s.store.LogActivity(ctx, store.Activity{GroupID: g2.ID, UserID: uid, Kind: "expense.added", ExpenseID: e.ID, Summary: s.expenseSummaryFor(m2, e)})
	}
	return nil
}

func (s *Server) expenseSummaryFor(members []store.Member, e *store.Expense) string {
	return s.expenseSummary(&store.Group{Members: members}, e)
}
