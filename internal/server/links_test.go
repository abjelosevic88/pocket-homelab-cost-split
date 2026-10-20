package server

import (
	"fmt"
	"strings"
	"testing"

	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/store"
)

// linkTestTokens are the pcx_ tokens the fake Core (peruser_test.go) answers
// as delegated for Ben, with these scopes. pcx_ben_day comes from Pocket Day,
// the others from Pocket Kitchen.
var linkTestTokens = map[string]string{
	"pcx_ben_full":  "groups:read expenses:write",
	"pcx_ben_read":  "groups:read",
	"pcx_ben_write": "expenses:write",
	"pcx_ben_day":   "groups:read expenses:write",
}

// A linked app (Pocket Kitchen) acting for Ben through Pocket Core sees only
// Ben's groups, adds expenses to them, changes only what it added, and gets
// nothing else in the app.
func TestCoreLinkScopes(t *testing.T) {
	h := newPerUserHarness(t)
	ana, _, anaCat := h.anaGroup()
	var g store.Group
	h.json(h.benTok, "POST", "/groups", map[string]any{"name": "Ben's flat", "currency": "EUR", "members": []map[string]any{{"me": true}, {"name": "Cleo"}}}, 201, &g)
	shares := []map[string]any{{"member_id": g.Members[0].ID, "share": 1}, {"member_id": g.Members[1].ID, "share": 1}}
	var own store.Expense
	h.json(h.benTok, "POST", fmt.Sprintf("/groups/%d/expenses", g.ID), map[string]any{"title": "Ben's own", "amount": 1000, "paid_by": g.Members[0].ID, "shares": shares}, 201, &own)
	gp := fmt.Sprintf("/groups/%d", g.ID)
	ap := fmt.Sprintf("/groups/%d", ana.ID)
	full, read, write, day := "pcx_ben_full", "pcx_ben_read", "pcx_ben_write", "pcx_ben_day"

	// auth/me works for any linked app and is Ben.
	for _, tok := range []string{full, read, write} {
		if code, out := h.do(tok, "GET", "/auth/me", nil); code != 200 || !strings.Contains(string(out), "ben@example.com") {
			t.Fatalf("auth/me with %s: %d %s", tok, code, out)
		}
	}

	// groups:read sees Ben's groups only.
	var list []store.Group
	h.json(read, "GET", "/groups", nil, 200, &list)
	if len(list) != 1 || list[0].ID != g.ID {
		t.Fatalf("linked group list: %+v", list)
	}
	var one store.Group
	h.json(read, "GET", gp, nil, 200, &one)
	if len(one.Members) != 2 {
		t.Fatalf("linked group: %+v", one)
	}
	if code, out := h.do(read, "GET", gp+"/expenses?from=2000-01-01", nil); code != 200 || !strings.Contains(string(out), "Ben's own") {
		t.Fatalf("linked expenses: %d %s", code, out)
	}
	if code, _ := h.do(read, "GET", fmt.Sprintf("%s/expenses/%d", gp, own.ID), nil); code != 200 {
		t.Fatalf("linked single expense: %d", code)
	}
	if code, out := h.do(read, "GET", "/categories", nil); code != 200 || !strings.Contains(string(out), "general") || strings.Contains(string(out), anaCat.ID) {
		t.Fatalf("linked categories: %d %s", code, out)
	}
	for _, p := range []string{ap, ap + "/expenses"} {
		if code, _ := h.do(full, "GET", p, nil); code != 404 {
			t.Fatalf("Ana's %s through Ben's link: %d, want 404", p, code)
		}
	}
	// A token without groups:read cannot read.
	for _, p := range []string{"/groups", gp, gp + "/expenses", "/categories"} {
		if code, _ := h.do(write, "GET", p, nil); code != 403 {
			t.Fatalf("GET %s with expenses:write only: %d, want 403", p, code)
		}
	}

	// expenses:write adds an expense that carries the app's name.
	in := map[string]any{"title": "Groceries", "amount": 2350, "paid_by": g.Members[0].ID, "category": "groceries", "shares": shares}
	if code, _ := h.do(read, "POST", gp+"/expenses", in); code != 403 {
		t.Fatalf("read-only link added an expense: %d", code)
	}
	var e store.Expense
	h.json(full, "POST", gp+"/expenses", in, 201, &e)
	if e.Source != "kitchen" || e.CreatedBy != h.ben.ID {
		t.Fatalf("linked expense source/creator: %+v", e)
	}
	if code, _ := h.do(full, "POST", ap+"/expenses", map[string]any{"title": "x", "amount": 1, "paid_by": ana.Members[0].ID, "shares": []map[string]any{{"member_id": ana.Members[0].ID, "share": 1}}}); code != 404 {
		t.Fatalf("expense in Ana's group through Ben's link: %d", code)
	}
	var act []store.Activity
	h.json(h.benTok, "GET", gp+"/activity", nil, 200, &act)
	if len(act) == 0 || !strings.Contains(act[0].Summary, "via Pocket Kitchen") {
		t.Fatalf("activity does not name the app: %+v", act)
	}

	// It changes and deletes only what it added; a person's expense and
	// another app's are invisible to those writes.
	in["amount"] = 2400
	var upd store.Expense
	h.json(full, "PUT", fmt.Sprintf("%s/expenses/%d", gp, e.ID), in, 200, &upd)
	if upd.Amount != 2400 || upd.Source != "kitchen" {
		t.Fatalf("linked update: %+v", upd)
	}
	if code, _ := h.do(full, "PUT", fmt.Sprintf("%s/expenses/%d", gp, own.ID), in); code != 404 {
		t.Fatalf("linked app changed Ben's own expense: %d", code)
	}
	if code, _ := h.do(full, "DELETE", fmt.Sprintf("%s/expenses/%d", gp, own.ID), nil); code != 404 {
		t.Fatalf("linked app deleted Ben's own expense: %d", code)
	}
	if code, _ := h.do(day, "DELETE", fmt.Sprintf("%s/expenses/%d", gp, e.ID), nil); code != 404 {
		t.Fatalf("another linked app deleted Kitchen's expense: %d", code)
	}
	if code, _ := h.do(read, "DELETE", fmt.Sprintf("%s/expenses/%d", gp, e.ID), nil); code != 403 {
		t.Fatalf("read-only link deleted: %d", code)
	}
	if code, _ := h.do(full, "DELETE", fmt.Sprintf("%s/expenses/%d", gp, e.ID), nil); code != 204 {
		t.Fatalf("linked delete of its own expense: %d", code)
	}
	// Ben himself still edits his expenses as before.
	if code, _ := h.do(h.benTok, "PUT", fmt.Sprintf("%s/expenses/%d", gp, own.ID), map[string]any{"title": "Ben's own", "amount": 1100, "paid_by": g.Members[0].ID, "shares": shares}); code != 200 {
		t.Fatalf("Ben edits his own expense: %d", code)
	}

	// Everything else stays closed, even with every scope.
	for _, c := range []struct{ m, p string }{
		{"PUT", "/auth/me"}, {"POST", "/auth/logout"}, {"GET", "/dashboard"}, {"GET", "/activity"}, {"GET", "/currencies"}, {"GET", "/fx"},
		{"POST", "/categories"}, {"PUT", "/categories/x"}, {"DELETE", "/categories/x"},
		{"POST", "/groups"}, {"POST", "/import"}, {"PUT", gp}, {"DELETE", gp},
		{"GET", gp + "/balances"}, {"GET", gp + "/stats"}, {"GET", gp + "/activity"}, {"GET", gp + "/export.json"}, {"GET", gp + "/export.csv"}, {"GET", gp + "/report"}, {"GET", gp + "/report.pdf"},
		{"POST", gp + "/members"}, {"PUT", gp + "/members/1"}, {"DELETE", gp + "/members/1"},
		{"GET", "/users"}, {"POST", "/users"}, {"PUT", "/users/1"}, {"DELETE", "/users/1"},
		{"GET", "/settings"}, {"PUT", "/settings"}, {"GET", "/settings/system"},
		{"GET", "/tokens"}, {"POST", "/tokens"}, {"DELETE", "/tokens/1"},
	} {
		if code, _ := h.do(full, c.m, c.p, map[string]any{}); code != 403 {
			t.Fatalf("%s %s with a link token: %d, want 403", c.m, c.p, code)
		}
	}
	// And a link token never counts as a signed-in session.
	if code, out := h.do(full, "GET", "/auth/status", nil); code != 200 || !strings.Contains(string(out), `"authenticated":false`) {
		t.Fatalf("auth/status with a link token: %d %s", code, out)
	}
}
