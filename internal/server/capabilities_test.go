package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/config"
	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/store"
)

// capWorld is what the capability tests look at: a ski trip Ana and Ben
// share with Marko (who has no account), Ana's private flat with Sani, and
// Ben's flat with Cleo.
//
//	2025-03-14  Ski trip  Dinner 54.00  paid by Ana, split Ana/Ben/Marko
//	2025-03-14  Ski trip  Taxi 30.00    paid by Marko, split Ana/Ben
//	2025-03-14  Ski trip  Ben pays Ana back 10.00
//	2025-03-15  Ben's flat Groceries 20.00 paid by Ben, split Ben/Cleo
//	2025-03-16  Ski trip  Lunch 20.00   paid by Ana, for Ben only
//	today       Ana's flat Rent 500.00   paid by Ana, split Ana/Sani
//
// So in the ski trip Ana is owed 31.00 by Ben and Marko 12.00 by Ben.
type capWorld struct {
	ski, flat, benFlat              store.Group
	dinner, taxi, payment, groc, lu store.Expense
}

func newCapWorld(h *perUserHarness) capWorld {
	var w capWorld
	w.flat, _, _ = h.anaGroup()
	h.json(h.anaTok, "POST", "/groups", map[string]any{"name": "Ski trip", "currency": "EUR",
		"members": []map[string]any{{"me": true}, {"user_id": h.ben.ID}, {"name": "Marko"}}}, 201, &w.ski)
	ana, ben, marko := w.ski.Members[0].ID, w.ski.Members[1].ID, w.ski.Members[2].ID
	sp := fmt.Sprintf("/groups/%d/expenses", w.ski.ID)
	shares := func(ids ...int64) []map[string]any {
		out := []map[string]any{}
		for _, id := range ids {
			out = append(out, map[string]any{"member_id": id, "share": 1})
		}
		return out
	}
	h.json(h.anaTok, "POST", sp, map[string]any{"title": "Dinner", "amount": 5400, "paid_by": ana, "date": "2025-03-14", "category": "restaurant",
		"notes": "the pizza place", "shares": shares(ana, ben, marko)}, 201, &w.dinner)
	h.json(h.anaTok, "POST", sp, map[string]any{"title": "Taxi", "amount": 3000, "paid_by": marko, "date": "2025-03-14", "category": "transport",
		"shares": shares(ana, ben)}, 201, &w.taxi)
	h.json(h.benTok, "POST", sp, map[string]any{"amount": 1000, "paid_by": ben, "date": "2025-03-14", "reimbursement": true, "shares": shares(ana)}, 201, &w.payment)
	h.json(h.anaTok, "POST", sp, map[string]any{"title": "Lunch", "amount": 2000, "paid_by": ana, "date": "2025-03-16", "shares": shares(ben)}, 201, &w.lu)
	h.json(h.benTok, "POST", "/groups", map[string]any{"name": "Ben's flat", "currency": "EUR", "members": []map[string]any{{"me": true}, {"name": "Cleo"}}}, 201, &w.benFlat)
	h.json(h.benTok, "POST", fmt.Sprintf("/groups/%d/expenses", w.benFlat.ID), map[string]any{"title": "Groceries", "amount": 2000, "paid_by": w.benFlat.Members[0].ID,
		"date": "2025-03-15", "shares": shares(w.benFlat.Members[0].ID, w.benFlat.Members[1].ID)}, 201, &w.groc)
	return w
}

// capItems are the rows of a capability answer.
func capItems(t *testing.T, m map[string]any) []map[string]any {
	t.Helper()
	l, ok := m["items"].([]any)
	if !ok {
		t.Fatalf("no items list in %v", m)
	}
	out := []map[string]any{}
	for _, v := range l {
		out = append(out, v.(map[string]any))
	}
	return out
}

func capItem(t *testing.T, m map[string]any, title string) map[string]any {
	t.Helper()
	for _, it := range capItems(t, m) {
		if it["title"] == title {
			return it
		}
	}
	t.Fatalf("no %q in %v", title, m["items"])
	return nil
}

func (h *perUserHarness) get(tok, path string) map[string]any {
	h.t.Helper()
	var m map[string]any
	h.json(tok, "GET", path, nil, 200, &m)
	return m
}

func TestPocketSearch(t *testing.T) {
	h := newPerUserHarness(t)
	w := newCapWorld(h)

	// a group: its people and where the person stands, in words
	got := capItems(t, h.get(h.benTok, "/pocket/search?q=ski"))
	if len(got) != 1 || got[0]["kind"] != "group" || got[0]["title"] != "Ski trip" || got[0]["id"] != fmt.Sprintf("group:%d", w.ski.ID) ||
		got[0]["url"] != fmt.Sprintf("/groups/%d", w.ski.ID) || got[0]["snippet"] != "Ana, Marko and you · you owe Ana 31.00 EUR, you owe Marko 12.00 EUR" || got[0]["date"] == "" {
		t.Fatalf("Ben's ski trip: %v", got)
	}
	if got := capItems(t, h.get(h.anaTok, "/pocket/search?q=SKI+trip")); len(got) != 1 || got[0]["snippet"] != "Ben, Marko and you · Ben owes you 31.00 EUR" {
		t.Fatalf("Ana's ski trip: %v", got)
	}
	if got := capItems(t, h.get(h.benTok, "/pocket/search?q=flat")); len(got) != 1 || got[0]["title"] != "Ben's flat" || got[0]["snippet"] != "Cleo and you · Cleo owes you 10.00 EUR" {
		t.Fatalf("Ben's flat: %v", got)
	}

	// an expense: by its title, its notes or its category's name
	for _, q := range []string{"dinner", "pizza", "restaurants", "PIZZA+place"} {
		got := capItems(t, h.get(h.benTok, "/pocket/search?q="+q))
		if len(got) != 1 || got[0]["kind"] != "expense" || got[0]["title"] != "Dinner" || got[0]["id"] != fmt.Sprintf("expense:%d", w.dinner.ID) ||
			got[0]["url"] != fmt.Sprintf("/groups/%d/expenses/%d", w.ski.ID, w.dinner.ID) || got[0]["date"] != "2025-03-14" ||
			got[0]["snippet"] != "54.00 EUR · paid by Ana · Ski trip · 14 Mar 2025" {
			t.Fatalf("%q: %v", q, got)
		}
	}
	if got := capItems(t, h.get(h.anaTok, "/pocket/search?q=transport")); len(got) != 1 || got[0]["snippet"] != "30.00 EUR · paid by Marko · Ski trip · 14 Mar 2025" {
		t.Fatalf("by category: %v", got)
	}
	if got := capItems(t, h.get(h.anaTok, "/pocket/search?q=dinner")); got[0]["snippet"] != "54.00 EUR · paid by you · Ski trip · 14 Mar 2025" {
		t.Fatalf("Ana paid: %v", got)
	}
	// a settle-up
	for tok, want := range map[string]string{h.benTok: "10.00 EUR · You paid Ana · Ski trip · 14 Mar 2025", h.anaTok: "10.00 EUR · Ben paid you · Ski trip · 14 Mar 2025"} {
		if got := capItems(t, h.get(tok, "/pocket/search?q=reimbursement")); len(got) != 1 || got[0]["kind"] != "settle-up" || got[0]["snippet"] != want {
			t.Fatalf("payment: %v", got)
		}
	}
	// newest first, and the limit
	if got := capItems(t, h.get(h.anaTok, "/pocket/search?q=ski")); len(got) != 1 {
		t.Fatalf("ski for Ana: %v", got)
	}
	if got := capItems(t, h.get(h.benTok, "/pocket/search?q=e")); len(got) < 5 || got[0]["kind"] != "group" {
		t.Fatalf("groups lead: %v", got)
	}
	if got := capItems(t, h.get(h.benTok, "/pocket/search?q=e&limit=1")); len(got) != 1 {
		t.Fatalf("limit 1: %v", got)
	}
	// words without a letter or digit find nothing, and LIKE's wildcards are plain characters
	for _, q := range []string{"%3F%3F", "*", "%25", "_", "-"} {
		if got := capItems(t, h.get(h.benTok, "/pocket/search?q="+q)); len(got) != 0 {
			t.Fatalf("%q: %v", q, got)
		}
	}
	if got := capItems(t, h.get(h.benTok, "/pocket/search?q=din%25")); len(got) != 0 {
		t.Fatalf("%% is not a wildcard: %v", got)
	}
	// another person's groups and expenses never appear
	for _, q := range []string{"private", "rent", "sani", "secret"} {
		if got := capItems(t, h.get(h.benTok, "/pocket/search?q="+q)); len(got) != 0 {
			t.Fatalf("Ben finds Ana's %q: %v", q, got)
		}
	}
	for _, q := range []string{"groceries", "cleo"} {
		if got := capItems(t, h.get(h.anaTok, "/pocket/search?q="+q)); len(got) != 0 {
			t.Fatalf("Ana finds Ben's %q: %v", q, got)
		}
	}
	if code, _ := h.do("nobody", "GET", "/pocket/search?q=ski", nil); code != 401 {
		t.Fatalf("signed out: %d", code)
	}
}

func TestPocketDay(t *testing.T) {
	h := newPerUserHarness(t)
	w := newCapWorld(h)

	day := h.get(h.anaTok, "/pocket/day/2025-03-14?tz=Europe/Sarajevo")
	if day["date"] != "2025-03-14" || day["summary"] != "2 expenses, your share 33.00 EUR; 1 settle-up" || len(capItems(t, day)) != 3 {
		t.Fatalf("Ana's day: %v", day)
	}
	if ms := day["metrics"].([]any); len(ms) != 2 || ms[0].(map[string]any)["value"] != "33.00 EUR" || ms[1].(map[string]any)["label"] != "You paid" || ms[1].(map[string]any)["value"] != "54.00 EUR" {
		t.Fatalf("metrics: %v", ms)
	}
	dinner := capItem(t, day, "Dinner")
	if dinner["id"] != fmt.Sprint(w.dinner.ID) || dinner["kind"] != "expense" || dinner["amount"] != float64(1800) || dinner["currency"] != "EUR" ||
		dinner["sub"] != "Ski trip · you paid 54.00 EUR, split with Ben and Marko" || dinner["note"] != "the pizza place" ||
		dinner["url"] != fmt.Sprintf("/groups/%d/expenses/%d", w.ski.ID, w.dinner.ID) || dinner["time"] != nil {
		t.Fatalf("dinner: %v", dinner)
	}
	if taxi := capItem(t, day, "Taxi"); taxi["amount"] != float64(1500) || taxi["sub"] != "Ski trip · paid by Marko" {
		t.Fatalf("taxi: %v", taxi)
	}
	if pay := capItem(t, day, "Reimbursement"); pay["kind"] != "settle-up" || pay["amount"] != nil || pay["sub"] != "Ski trip · Ben paid you 10.00 EUR" ||
		pay["url"] != fmt.Sprintf("/groups/%d/expenses/%d", w.ski.ID, w.payment.ID) {
		t.Fatalf("payment: %v", pay)
	}

	// Ben sees the same day from his side
	day = h.get(h.benTok, "/pocket/day/2025-03-14")
	if day["summary"] != "2 expenses, your share 33.00 EUR; 1 settle-up" || capItem(t, day, "Dinner")["sub"] != "Ski trip · paid by Ana" ||
		capItem(t, day, "Reimbursement")["sub"] != "Ski trip · You paid Ana 10.00 EUR" {
		t.Fatalf("Ben's day: %v", day)
	}
	if ms := day["metrics"].([]any); len(ms) != 1 {
		t.Fatalf("Ben paid for nothing that day: %v", ms)
	}
	// one expense: the summary says it in a sentence
	if d := h.get(h.benTok, "/pocket/day/2025-03-15"); d["summary"] != "Groceries split with Cleo, your share 10.00 EUR" || capItem(t, d, "Groceries")["sub"] != "Ben's flat · you paid 20.00 EUR, split with Cleo" {
		t.Fatalf("groceries: %v", d)
	}
	// paid for somebody else: no share of one's own
	d := h.get(h.anaTok, "/pocket/day/2025-03-16")
	if lunch := capItem(t, d, "Lunch"); d["summary"] != "Lunch, you paid 20.00 EUR for Ben" || lunch["amount"] != nil || lunch["sub"] != "Ski trip · you paid 20.00 EUR for Ben" {
		t.Fatalf("Ana's lunch: %v", d)
	}
	if d := h.get(h.benTok, "/pocket/day/2025-03-16"); d["summary"] != "Lunch, your share 20.00 EUR" || capItem(t, d, "Lunch")["amount"] != float64(2000) {
		t.Fatalf("Ben's lunch: %v", d)
	}
	// nothing that day, somebody else's day, a bad day
	if d := h.get(h.anaTok, "/pocket/day/2019-03-04"); d["summary"] != nil || len(capItems(t, d)) != 0 {
		t.Fatalf("an empty day: %v", d)
	}
	if d := h.get(h.anaTok, "/pocket/day/2025-03-15"); len(capItems(t, d)) != 0 {
		t.Fatalf("Ana sees Ben's groceries: %v", d)
	}
	if d := h.get(h.benTok, "/pocket/day/"+store.Today()); len(capItems(t, d)) != 0 {
		t.Fatalf("Ben sees Ana's rent: %v", d)
	}
	if code, _ := h.do(h.anaTok, "GET", "/pocket/day/yesterday", nil); code != 400 {
		t.Fatalf("a bad day: %d", code)
	}
}

func TestPocketWidget(t *testing.T) {
	h := newPerUserHarness(t)
	// no group yet: nothing to show
	if wd := h.get(h.benTok, "/pocket/widget"); wd["line"] != nil || len(capItems(t, wd)) != 0 {
		t.Fatalf("without groups: %v", wd)
	}
	var g store.Group
	h.json(h.benTok, "POST", "/groups", map[string]any{"name": "Empty", "currency": "EUR", "members": []map[string]any{{"me": true}, {"name": "Dora"}}}, 201, &g)
	if wd := h.get(h.benTok, "/pocket/widget"); wd["line"] != "All settled" || len(capItems(t, wd)) != 0 || wd["metrics"] != nil {
		t.Fatalf("all settled: %v", wd)
	}
	w := newCapWorld(h)

	wd := h.get(h.benTok, "/pocket/widget")
	rows := capItems(t, wd)
	if wd["line"] != "You owe 43.00 EUR, you are owed 10.00 EUR" || wd["url"] != "/home" || len(rows) != 3 ||
		rows[0]["title"] != "You owe Ana 31.00 EUR" || rows[0]["detail"] != "Ski trip" || rows[0]["url"] != fmt.Sprintf("/groups/%d/balances", w.ski.ID) ||
		rows[1]["title"] != "You owe Marko 12.00 EUR" || rows[2]["title"] != "Cleo owes you 10.00 EUR" || rows[2]["detail"] != "Ben's flat" {
		t.Fatalf("Ben's balances, largest first: %v", wd)
	}
	if ms := wd["metrics"].([]any); len(ms) != 2 || ms[0].(map[string]any)["value"] != "43.00 EUR" || ms[1].(map[string]any)["value"] != "10.00 EUR" {
		t.Fatalf("metrics: %v", ms)
	}
	wd = h.get(h.anaTok, "/pocket/widget")
	if rows := capItems(t, wd); wd["line"] != "You are owed 281.00 EUR" || len(rows) != 2 || rows[0]["title"] != "Sani owes you 250.00 EUR" || rows[1]["title"] != "Ben owes you 31.00 EUR" {
		t.Fatalf("Ana's balances: %v", wd)
	}
	// an archived group is left out, as on the home page
	h.json(h.anaTok, "PUT", fmt.Sprintf("/groups/%d", w.flat.ID), map[string]any{"archived": true}, 200, nil)
	if wd := h.get(h.anaTok, "/pocket/widget"); wd["line"] != "You are owed 31.00 EUR" || len(capItems(t, wd)) != 1 {
		t.Fatalf("archived: %v", wd)
	}
}

// Nothing here adds an expense or settles up: there is no capture route.
func TestPocketNoCapture(t *testing.T) {
	h := newPerUserHarness(t)
	w := newCapWorld(h)
	body := map[string]any{"request_id": "req-0000000000000001", "text": "12.50 lunch"}
	for _, tok := range []string{h.benTok, "pcx_ben_capture", "pcx_ben_full"} {
		if code, _ := h.do(tok, "POST", "/pocket/capture", body); code < 400 {
			t.Fatalf("capture with %s: %d", tok, code)
		}
	}
	var list struct {
		Total int `json:"total"`
	}
	h.json(h.benTok, "GET", fmt.Sprintf("/groups/%d/expenses", w.ski.ID), nil, 200, &list)
	if list.Total != 4 {
		t.Fatalf("expenses in the ski trip: %d", list.Total)
	}
}

// A link token with one pocket:* scope reaches its one route and /auth/me,
// nothing else; the app's own scopes do not open the new routes.
func TestPocketLinkTokens(t *testing.T) {
	h := newPerUserHarness(t)
	w := newCapWorld(h)
	gp := fmt.Sprintf("/groups/%d", w.ski.ID)
	day := "/pocket/day/2025-03-14"
	search, onday, widget, capture := "pcx_ben_search", "pcx_ben_onday", "pcx_ben_widget", "pcx_ben_capture"
	for _, c := range []struct {
		tok, method, path string
		want              int
	}{
		{search, "GET", "/pocket/search?q=ski", 200},
		{onday, "GET", day, 200},
		{widget, "GET", "/pocket/widget", 200},
		{widget, "GET", "/auth/me", 200},
		{search, "GET", "/auth/me", 200},
		// another capability's route
		{search, "GET", day, 403},
		{search, "GET", "/pocket/widget", 403},
		{onday, "GET", "/pocket/search?q=ski", 403},
		{onday, "GET", "/pocket/widget", 403},
		{widget, "GET", "/pocket/search?q=ski", 403},
		{widget, "GET", day, 403},
		{capture, "GET", "/pocket/widget", 403},
		{capture, "GET", "/pocket/search?q=ski", 403},
		// no capture route at all, and search is read-only (the router answers before any token is looked at)
		{capture, "POST", "/pocket/capture", 404},
		{search, "POST", "/pocket/search?q=ski", 405},
		// the rest of the app
		{search, "GET", "/groups", 403},
		{search, "GET", gp + "/expenses", 403},
		{onday, "GET", gp + "/expenses?from=2025-03-14&to=2025-03-14", 403},
		{widget, "GET", "/dashboard", 403},
		{widget, "GET", gp + "/balances", 403},
		{widget, "GET", gp, 403},
		{capture, "POST", gp + "/expenses", 403},
		{search, "GET", "/export.json", 403},
		// the app's own scopes do not open the new routes
		{"pcx_ben_full", "GET", "/pocket/search?q=ski", 403},
		{"pcx_ben_read", "GET", day, 403},
		{"pcx_ben_read", "GET", "/pocket/widget", 403},
		{"pcx_ben_read", "GET", "/groups", 200},
		{"pcx_nobody", "GET", "/pocket/widget", 401},
	} {
		if code, out := h.do(c.tok, c.method, c.path, map[string]any{}); code != c.want {
			t.Errorf("%s %s %s → %d, want %d (%s)", c.tok, c.method, c.path, code, c.want, out)
		}
	}
	// the token acts for Ben, and sees what Ben sees
	if got := capItems(t, h.get(search, "/pocket/search?q=ski")); len(got) != 1 || !strings.Contains(got[0]["snippet"].(string), "you owe Ana") {
		t.Fatalf("search through the link: %v", got)
	}
	if got := capItems(t, h.get(search, "/pocket/search?q=rent")); len(got) != 0 {
		t.Fatalf("Ana's rent through Ben's link: %v", got)
	}
	if d := h.get(onday, day); d["summary"] != "2 expenses, your share 33.00 EUR; 1 settle-up" {
		t.Fatalf("day through the link: %v", d)
	}
	if wd := h.get(widget, "/pocket/widget"); wd["line"] != "You owe 43.00 EUR, you are owed 10.00 EUR" {
		t.Fatalf("widget through the link: %v", wd)
	}
}

func TestPocketLinkScopes(t *testing.T) {
	req := func(m, p string) *http.Request { return httptest.NewRequest(m, p, nil) }
	for _, c := range []struct {
		scope string
		r     *http.Request
		want  bool
	}{
		{core.ScopeSearch, req("GET", "/api/v1/pocket/search?q=x"), true},
		{core.ScopeDay, req("GET", "/api/v1/pocket/day/2026-01-02"), true},
		{core.ScopeWidget, req("GET", "/api/v1/pocket/widget"), true},
		{core.ScopeWidget, req("GET", "/api/v1/auth/me"), true},
		{core.ScopeCapture, req("POST", "/api/v1/pocket/capture"), false}, // not offered
		{core.ScopeSearch, req("POST", "/api/v1/pocket/search"), false},
		{core.ScopeDay, req("GET", "/api/v1/pocket/day/today"), false},
		{core.ScopeWidget, req("GET", "/api/v1/dashboard"), false},
		{core.ScopeSearch, req("GET", "/api/v1/groups"), false},
		{"groups:read", req("GET", "/api/v1/pocket/widget"), false},
		{"groups:read", req("GET", "/api/v1/pocket/search"), false},
		{"expenses:write", req("POST", "/api/v1/pocket/capture"), false},
	} {
		if got := linkAllows([]string{c.scope}, c.r); got != c.want {
			t.Errorf("%s %s %s = %v, want %v", c.scope, c.r.Method, c.r.URL.Path, got, c.want)
		}
	}
}

// Split tells Core what it can do, so Search, Day and Hub find it on their own.
func TestManifestCapabilities(t *testing.T) {
	var mu sync.Mutex
	var manifest map[string]any
	fc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/apps/register" {
			mu.Lock()
			_ = json.NewDecoder(r.Body).Decode(&manifest)
			mu.Unlock()
		}
		io.WriteString(w, `{}`)
	}))
	t.Cleanup(fc.Close)
	dir := t.TempDir()
	cfg := &config.Config{DataDir: dir, DBPath: dir + "/t.db", AuthMode: "local", DefaultCurrency: "EUR", FXProvider: "none", SessionTTL: time.Hour,
		CoreURL: fc.URL, CoreRegistrationKey: "key"}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.Open(cfg.DBPath, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	s := New(cfg, st, log, nil)
	if err := s.registerWithCore(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	m := manifest
	mu.Unlock()
	if m["slug"] != "split" || m["category"] != "Money" {
		t.Fatalf("manifest: %v %v", m["slug"], m["category"])
	}
	search, _ := m["search"].(map[string]any)
	day, _ := m["day"].(map[string]any)
	widget, _ := m["widget"].(map[string]any)
	if search == nil || fmt.Sprint(search["bangs"]) != "[split owe]" || fmt.Sprint(search["kinds"]) != "[group expense settle-up]" ||
		day == nil || day["label"] != "Split" || day["one"] != "expense" || day["many"] != "expenses" || day["unit"] != "amount" ||
		widget == nil || widget["title"] != "Balances" || m["capture"] != nil {
		t.Fatalf("capabilities: %v %v %v %v", search, day, widget, m["capture"])
	}
	offered := map[string]bool{}
	for _, o := range m["offers"].([]any) {
		offered[o.(map[string]any)["scope"].(string)] = true
	}
	for _, sc := range []string{"groups:read", "expenses:write", core.ScopeSearch, core.ScopeDay, core.ScopeWidget} {
		if !offered[sc] {
			t.Errorf("offer %s missing: %v", sc, m["offers"])
		}
	}
	if offered[core.ScopeCapture] {
		t.Errorf("capture is offered: %v", m["offers"])
	}
}
