package server

import (
	"fmt"
	"testing"

	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/split"
	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/store"
)

type balancesAnswer struct {
	Balances    []split.Balance            `json:"balances"`
	Settlements []split.Transfer           `json:"settlements"`
	CostCenters []store.CostCenterPosition `json:"cost_centers"`
}

// The PSUP case: people front money for a project, someone buys into the
// project's debt, the project pays back. Personal balances never move.
func TestCostCenterFrontBuyInRepay(t *testing.T) {
	h := newPerUserHarness(t)
	var g store.Group
	h.json(h.anaTok, "POST", "/groups", map[string]any{"name": "PSUP", "currency": "EUR", "members": []map[string]any{{"me": true}, {"name": "Radoslav"}}}, 201, &g)
	gp := fmt.Sprintf("/groups/%d", g.ID)
	ana, rad := g.Members[0].ID, g.Members[1].ID
	var proj store.Member
	h.json(h.anaTok, "POST", gp+"/members", map[string]any{"name": "Project", "kind": "cost_center"}, 201, &proj)
	if !proj.IsCostCenter() {
		t.Fatalf("not a cost center: %+v", proj)
	}
	exp := func(body map[string]any, code int) {
		t.Helper()
		if c, out := h.do(h.anaTok, "POST", gp+"/expenses", body); c != code {
			t.Fatalf("POST expense %v: %d %s", body, c, out)
		}
	}
	bal := func() balancesAnswer {
		t.Helper()
		var b balancesAnswer
		h.json(h.anaTok, "GET", gp+"/balances", nil, 200, &b)
		for _, x := range b.Balances {
			if x.MemberID == proj.ID {
				t.Fatalf("cost center in the personal balances: %+v", b.Balances)
			}
		}
		return b
	}
	claims := func(b balancesAnswer) map[int64]int64 {
		out := map[int64]int64{}
		for _, cc := range b.CostCenters {
			for _, c := range cc.Claims {
				out[c.MemberID] = c.Amount
			}
		}
		return out
	}

	// Ana fronts 2000 for the project.
	exp(map[string]any{"title": "Servers", "amount": 200000, "paid_by": ana, "shares": []map[string]any{{"member_id": proj.ID}}}, 201)
	b := bal()
	if len(b.Settlements) != 0 || claims(b)[ana] != 200000 || b.CostCenters[0].Owes != 200000 {
		t.Fatalf("after fronting: %+v", b)
	}
	// A cost center is never split with people, and never pays for things.
	exp(map[string]any{"title": "x", "amount": 900, "paid_by": ana, "shares": []map[string]any{{"member_id": ana}, {"member_id": rad}, {"member_id": proj.ID}}}, 400)
	exp(map[string]any{"title": "x", "amount": 900, "paid_by": proj.ID, "shares": []map[string]any{{"member_id": ana}}}, 400)

	// Radoslav pays Ana 1000, buying into the project's debt to her.
	exp(map[string]any{"amount": 100000, "paid_by": rad, "reimbursement": true, "cost_center_id": proj.ID, "shares": []map[string]any{{"member_id": ana}}}, 201)
	b = bal()
	if len(b.Settlements) != 0 || claims(b)[ana] != 100000 || claims(b)[rad] != 100000 {
		t.Fatalf("after buying in: %+v", b)
	}
	for _, x := range b.Balances {
		if x.Balance != 0 {
			t.Fatalf("a personal balance moved: %+v", b.Balances)
		}
	}
	// No more than Ana's remaining claim can be taken over; only payments may.
	exp(map[string]any{"amount": 100001, "paid_by": rad, "reimbursement": true, "cost_center_id": proj.ID, "shares": []map[string]any{{"member_id": ana}}}, 400)
	exp(map[string]any{"title": "x", "amount": 100, "paid_by": rad, "cost_center_id": proj.ID, "shares": []map[string]any{{"member_id": ana}}}, 400)

	// An ordinary personal payment still is one.
	exp(map[string]any{"amount": 5000, "paid_by": rad, "reimbursement": true, "shares": []map[string]any{{"member_id": ana}}}, 201)
	b = bal()
	if len(b.Settlements) != 1 || b.Settlements[0].From != ana || b.Settlements[0].To != rad || b.Settlements[0].Amount != 5000 || claims(b)[ana] != 100000 {
		t.Fatalf("after a personal payment: %+v", b)
	}

	// The project pays Radoslav back.
	exp(map[string]any{"amount": 100000, "paid_by": proj.ID, "reimbursement": true, "shares": []map[string]any{{"member_id": rad}}}, 201)
	b = bal()
	if _, ok := claims(b)[rad]; ok || claims(b)[ana] != 100000 || b.CostCenters[0].Repaid != 100000 {
		t.Fatalf("after repaying: %+v", b)
	}

	// The dashboard keeps it apart: Ana's personal balance is the 50 she owes
	// Radoslav, the project's 1000 is a claim.
	var d Dashboard
	h.json(h.anaTok, "GET", "/dashboard", nil, 200, &d)
	if d.GroupLines[0].Balance != -5000 || d.GroupLines[0].Claims != 100000 || d.GroupLines[0].Members != 2 {
		t.Fatalf("dashboard: %+v", d.GroupLines[0])
	}

	// A cost center has no account and, once bought into, stays one.
	if c, _ := h.do(h.anaTok, "PUT", fmt.Sprintf("%s/members/%d", gp, proj.ID), map[string]any{"user_id": h.ben.ID}); c != 400 {
		t.Fatalf("linked a cost center: %d", c)
	}
	if c, _ := h.do(h.anaTok, "PUT", fmt.Sprintf("%s/members/%d", gp, proj.ID), map[string]any{"kind": "person"}); c != 400 {
		t.Fatalf("turned a bought-into cost center into a person: %d", c)
	}
}

// The old workaround: a ghost person "Project" with every share pointing at
// it. Turning it into a cost center moves its debt out of the personal book.
func TestCostCenterFromGhostMember(t *testing.T) {
	h := newPerUserHarness(t)
	var g store.Group
	h.json(h.anaTok, "POST", "/groups", map[string]any{"name": "PSUP", "currency": "EUR", "members": []map[string]any{{"me": true}, {"name": "Radoslav"}, {"name": "Project"}}}, 201, &g)
	gp := fmt.Sprintf("/groups/%d", g.ID)
	ana, rad, ghost := g.Members[0].ID, g.Members[1].ID, g.Members[2].ID
	h.json(h.anaTok, "POST", gp+"/expenses", map[string]any{"title": "Servers", "amount": 3000, "paid_by": ana, "shares": []map[string]any{{"member_id": ghost}}}, 201, nil)
	h.json(h.anaTok, "POST", gp+"/expenses", map[string]any{"title": "Dinner", "amount": 1000, "paid_by": rad, "shares": []map[string]any{{"member_id": ana}, {"member_id": rad}}}, 201, nil)
	var b balancesAnswer
	h.json(h.anaTok, "GET", gp+"/balances", nil, 200, &b)
	if len(b.Balances) != 3 {
		t.Fatalf("ghost not a person yet: %+v", b)
	}
	// Ana's account cannot become a cost center.
	if c, _ := h.do(h.anaTok, "PUT", fmt.Sprintf("%s/members/%d", gp, ana), map[string]any{"kind": "cost_center"}); c != 400 {
		t.Fatalf("linked person became a cost center: %d", c)
	}
	h.json(h.anaTok, "PUT", fmt.Sprintf("%s/members/%d", gp, ghost), map[string]any{"kind": "cost_center"}, 200, nil)
	h.json(h.anaTok, "GET", gp+"/balances", nil, 200, &b)
	if len(b.Balances) != 2 || len(b.CostCenters) != 1 || b.CostCenters[0].Owes != 3000 {
		t.Fatalf("after converting: %+v", b)
	}
	// Only the dinner is personal: Ana owes Radoslav 5.
	if len(b.Settlements) != 1 || b.Settlements[0].From != ana || b.Settlements[0].Amount != 500 {
		t.Fatalf("personal debts: %+v", b.Settlements)
	}
}
