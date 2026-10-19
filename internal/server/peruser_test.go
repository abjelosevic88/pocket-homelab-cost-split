package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/config"
	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/store"
)

// perUserHarness is a server with two people: Ana (the first admin, the owner)
// and Ben (a household member). Each has an API token; Ben also signs in with a
// token Pocket Core issued to his paired phone.
type perUserHarness struct {
	t        *testing.T
	srv      *httptest.Server
	st       *store.Store
	ana, ben *store.User
	anaTok   string
	benTok   string
	benCore  string
}

func newPerUserHarness(t *testing.T) *perUserHarness {
	t.Helper()
	h := &perUserHarness{t: t, benCore: "pcd_ben_phone"}
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Token string `json:"token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if r.URL.Path != "/api/v1/auth/introspect" || in.Token != h.benCore {
			_ = json.NewEncoder(w).Encode(map[string]any{"active": false})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"active": true, "kind": "device", "user": map[string]any{"id": 2, "email": "ben@example.com", "name": "Ben", "role": "member"}})
	}))
	t.Cleanup(core.Close)
	dir := t.TempDir()
	cfg := &config.Config{DataDir: dir, DBPath: dir + "/t.db", AuthMode: "local", DefaultCurrency: "EUR", FXProvider: "none", SessionTTL: time.Hour, CoreURL: core.URL}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.Open(cfg.DBPath, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	h.st = st
	h.ana = &store.User{Email: "ana@example.com", Name: "Ana", Role: "admin"}
	h.ben = &store.User{Email: "ben@example.com", Name: "Ben", Role: "member"}
	for _, u := range []*store.User{h.ana, h.ben} {
		if err := st.CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	h.anaTok, _, _ = st.CreateAPIToken(ctx, h.ana.ID, "ana")
	h.benTok, _, _ = st.CreateAPIToken(ctx, h.ben.ID, "ben")
	h.srv = httptest.NewServer(New(cfg, st, log, nil).Router())
	t.Cleanup(h.srv.Close)
	return h
}

func (h *perUserHarness) do(tok, method, path string, body any) (int, []byte) {
	h.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, h.srv.URL+"/api/v1"+path, rd)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	out, _ := io.ReadAll(res.Body)
	return res.StatusCode, out
}

func (h *perUserHarness) json(tok, method, path string, body any, want int, v any) {
	h.t.Helper()
	code, out := h.do(tok, method, path, body)
	if code != want {
		h.t.Fatalf("%s %s: status %d, want %d: %s", method, path, code, want, out)
	}
	if v != nil {
		if err := json.Unmarshal(out, v); err != nil {
			h.t.Fatalf("%s %s: %v: %s", method, path, err, out)
		}
	}
}

// anaGroup creates Ana's group with a name-only participant, an expense and a
// custom category, the way the app looked before groups were per person.
func (h *perUserHarness) anaGroup() (g store.Group, e store.Expense, cat store.Category) {
	h.json(h.anaTok, "POST", "/categories", map[string]any{"label": "Ana secret category"}, 201, &cat)
	h.json(h.anaTok, "POST", "/groups", map[string]any{"name": "Ana private flat", "currency": "EUR", "members": []map[string]any{{"me": true}, {"name": "Sani"}}}, 201, &g)
	h.json(h.anaTok, "POST", fmt.Sprintf("/groups/%d/expenses", g.ID), map[string]any{
		"title": "Rent", "amount": 50000, "currency": "EUR", "paid_by": g.Members[0].ID, "category": cat.ID,
		"shares": []map[string]any{{"member_id": g.Members[0].ID, "share": 1}, {"member_id": g.Members[1].ID, "share": 1}},
	}, 201, &e)
	return
}

func TestPerUserGroupsAreInvisibleToOthers(t *testing.T) {
	h := newPerUserHarness(t)
	g, e, cat := h.anaGroup()
	gp := fmt.Sprintf("/groups/%d", g.ID)

	for _, tok := range []string{h.benTok, h.benCore} {
		// List and search.
		var list []store.Group
		h.json(tok, "GET", "/groups", nil, 200, &list)
		h.json(tok, "GET", "/groups?all=1&q=Ana", nil, 200, &list)
		if len(list) != 0 {
			t.Fatalf("Ben lists Ana's groups: %+v", list)
		}
		// Every read under the group is 404, never 403.
		for _, p := range []string{"", "/expenses", fmt.Sprintf("/expenses/%d", e.ID), "/balances", "/stats", "/activity", "/export.json", "/export.csv", "/report", "/report.pdf"} {
			if code, out := h.do(tok, "GET", gp+p, nil); code != 404 {
				t.Fatalf("GET %s%s as Ben: %d %s", gp, p, code, out)
			}
		}
		// And every write.
		writes := []struct {
			method, path string
			body         any
		}{
			{"PUT", gp, map[string]any{"name": "pwned"}},
			{"DELETE", gp, nil},
			{"POST", gp + "/members", map[string]any{"user_id": h.ben.ID}},
			{"PUT", fmt.Sprintf("%s/members/%d", gp, g.Members[1].ID), map[string]any{"user_id": h.ben.ID}},
			{"DELETE", fmt.Sprintf("%s/members/%d", gp, g.Members[1].ID), nil},
			{"POST", gp + "/expenses", map[string]any{"title": "x", "amount": 100, "paid_by": g.Members[0].ID, "shares": []map[string]any{{"member_id": g.Members[0].ID}}}},
			{"PUT", fmt.Sprintf("%s/expenses/%d", gp, e.ID), map[string]any{"title": "x", "amount": 100, "paid_by": g.Members[0].ID, "shares": []map[string]any{{"member_id": g.Members[0].ID}}}},
			{"DELETE", fmt.Sprintf("%s/expenses/%d", gp, e.ID), nil},
			{"PUT", "/categories/" + cat.ID, map[string]any{"label": "pwned"}},
			{"DELETE", "/categories/" + cat.ID, nil},
		}
		for _, wr := range writes {
			if code, out := h.do(tok, wr.method, wr.path, wr.body); code != 404 {
				t.Fatalf("%s %s as Ben: %d %s", wr.method, wr.path, code, out)
			}
		}
		// Feed, dashboard, categories, counts.
		var act []store.Activity
		h.json(tok, "GET", "/activity", nil, 200, &act)
		if len(act) != 0 {
			t.Fatalf("Ben sees Ana's activity: %+v", act)
		}
		var d Dashboard
		h.json(tok, "GET", "/dashboard", nil, 200, &d)
		if d.Groups != 0 || len(d.GroupLines) != 0 {
			t.Fatalf("Ben's dashboard shows Ana's groups: %+v", d)
		}
		var cats []store.Category
		h.json(tok, "GET", "/categories", nil, 200, &cats)
		for _, c := range cats {
			if c.ID == cat.ID {
				t.Fatalf("Ben sees Ana's category")
			}
			if c.Count != 0 {
				t.Fatalf("Ben's category counts include Ana's expenses: %+v", c)
			}
		}
		var sys struct {
			Counts map[string]int `json:"counts"`
		}
		h.json(tok, "GET", "/settings/system", nil, 200, &sys)
		if sys.Counts["groups"] != 0 || sys.Counts["expenses"] != 0 {
			t.Fatalf("Ben's About counts include Ana's rows: %+v", sys.Counts)
		}
	}
	// Using Ana's category id in Ben's own group is refused.
	var bg store.Group
	h.json(h.benCore, "POST", "/groups", map[string]any{"name": "Ben's", "currency": "EUR"}, 201, &bg)
	if code, _ := h.do(h.benTok, "POST", fmt.Sprintf("/groups/%d/expenses", bg.ID), map[string]any{"title": "x", "amount": 100, "paid_by": bg.Members[0].ID, "category": cat.ID, "shares": []map[string]any{{"member_id": bg.Members[0].ID}}}); code != 400 {
		t.Fatalf("Ben used Ana's category: %d", code)
	}
	// Nothing of Ana's changed.
	var got store.Group
	h.json(h.anaTok, "GET", gp, nil, 200, &got)
	if got.Name != "Ana private flat" || got.ExpenseCount != 1 || len(got.Members) != 2 {
		t.Fatalf("Ana's group changed: %+v", got)
	}
	// No whole-database download for anyone.
	if code, _ := h.do(h.anaTok, "GET", "/backup", nil); code != 404 {
		t.Fatalf("database download still served: %d", code)
	}
}

func TestPerUserAdminHasNoExtraReach(t *testing.T) {
	h := newPerUserHarness(t)
	var bg store.Group
	h.json(h.benTok, "POST", "/groups", map[string]any{"name": "Ben's trip", "currency": "EUR"}, 201, &bg)
	var list []store.Group
	h.json(h.anaTok, "GET", "/groups?all=1", nil, 200, &list)
	if len(list) != 0 {
		t.Fatalf("admin lists a member's groups: %+v", list)
	}
	for _, m := range []string{"GET", "DELETE"} {
		if code, _ := h.do(h.anaTok, m, fmt.Sprintf("/groups/%d", bg.ID), nil); code != 404 {
			t.Fatalf("admin %s on a member's group: %d", m, code)
		}
	}
	// Ben's own category is his; the admin cannot touch it either.
	var cat store.Category
	h.json(h.benTok, "POST", "/categories", map[string]any{"label": "Climbing"}, 201, &cat)
	if !cat.Mine {
		t.Fatalf("new category not marked as the creator's: %+v", cat)
	}
	if code, _ := h.do(h.anaTok, "PUT", "/categories/"+cat.ID, map[string]any{"label": "x"}); code != 404 {
		t.Fatalf("admin edited a member's category: %d", code)
	}
	// The same name is per person: Ana gets her own "Climbing".
	var anaCat store.Category
	h.json(h.anaTok, "POST", "/categories", map[string]any{"label": "Climbing"}, 201, &anaCat)
	if anaCat.ID == cat.ID || !anaCat.Mine {
		t.Fatalf("Ana got Ben's category: %+v vs %+v", anaCat, cat)
	}
	// Built-in categories stay box-wide: only an admin changes them.
	if code, _ := h.do(h.benTok, "PUT", "/categories/groceries", map[string]any{"label": "x"}); code != 403 {
		t.Fatalf("member edited a built-in category: %d", code)
	}
}

func TestPerUserSharingThroughMembers(t *testing.T) {
	h := newPerUserHarness(t)
	g, e, cat := h.anaGroup()
	gp := fmt.Sprintf("/groups/%d", g.ID)
	// Ana adds Ben as a household account.
	var ben store.Member
	h.json(h.anaTok, "POST", gp+"/members", map[string]any{"user_id": h.ben.ID}, 201, &ben)
	var got store.Group
	h.json(h.benTok, "GET", gp, nil, 200, &got)
	if got.MyMemberID != ben.ID {
		t.Fatalf("Ben not resolved as himself: %+v", got)
	}
	h.json(h.benTok, "GET", fmt.Sprintf("%s/expenses/%d", gp, e.ID), nil, 200, nil)
	// The shared group's category reads the same for Ben, but stays Ana's.
	var cats []store.Category
	h.json(h.benTok, "GET", "/categories", nil, 200, &cats)
	found := false
	for _, c := range cats {
		if c.ID == cat.ID {
			found = true
			if c.Mine {
				t.Fatalf("Ana's category marked as Ben's")
			}
		}
	}
	if !found {
		t.Fatalf("Ben does not see the category used in the shared group")
	}
	if code, _ := h.do(h.benTok, "PUT", "/categories/"+cat.ID, map[string]any{"label": "x"}); code != 404 {
		t.Fatalf("Ben edited Ana's category: %d", code)
	}
	// Ben can add an expense in the shared group…
	h.json(h.benTok, "POST", gp+"/expenses", map[string]any{"title": "Milk", "amount": 300, "paid_by": ben.ID, "category": cat.ID, "shares": []map[string]any{{"member_id": ben.ID}, {"member_id": g.Members[0].ID}}}, 201, nil)
	// …but not unlink or remove Ana, nor delete her group.
	if code, _ := h.do(h.benTok, "PUT", fmt.Sprintf("%s/members/%d", gp, g.Members[0].ID), map[string]any{"user_id": 0}); code != 403 {
		t.Fatalf("Ben unlinked Ana: %d", code)
	}
	if code, _ := h.do(h.benTok, "DELETE", fmt.Sprintf("%s/members/%d", gp, g.Members[0].ID), nil); code != 403 {
		t.Fatalf("Ben removed Ana: %d", code)
	}
	if code, _ := h.do(h.benTok, "DELETE", gp, nil); code != 403 {
		t.Fatalf("Ben deleted Ana's group: %d", code)
	}
	// Ana takes Ben out again: he is back to 404.
	h.json(h.anaTok, "PUT", fmt.Sprintf("%s/members/%d", gp, ben.ID), map[string]any{"user_id": 0}, 200, nil)
	if code, _ := h.do(h.benTok, "GET", gp, nil); code != 404 {
		t.Fatalf("Ben still sees the group after being unlinked: %d", code)
	}
}

func TestPerUserSettings(t *testing.T) {
	h := newPerUserHarness(t)
	var s map[string]any
	h.json(h.anaTok, "PUT", "/settings", map[string]any{"title": "Our box", "default_currency": "BAM", "fx_auto": true, "date_format": "dmy-dot", "time_format": "24h"}, 200, &s)
	// Ben saves his own preferences and tries to rename the box.
	h.json(h.benTok, "PUT", "/settings", map[string]any{"title": "Ben's box", "default_currency": "USD", "fx_auto": false, "date_format": "mdy-slash", "time_format": "12h"}, 200, &s)
	h.json(h.anaTok, "GET", "/settings", nil, 200, &s)
	if s["title"] != "Our box" || s["default_currency"] != "BAM" || s["date_format"] != "dmy-dot" || s["time_format"] != "24h" || s["fx_auto"] != true {
		t.Fatalf("Ana's settings changed by Ben: %+v", s)
	}
	h.json(h.benTok, "GET", "/settings", nil, 200, &s)
	if s["title"] != "Our box" || s["default_currency"] != "USD" || s["date_format"] != "mdy-slash" {
		t.Fatalf("Ben's settings: %+v", s)
	}
	// New groups follow each person's own default currency.
	var g store.Group
	h.json(h.benTok, "POST", "/groups", map[string]any{"name": "x"}, 201, &g)
	if g.Currency != "USD" {
		t.Fatalf("Ben's group currency %s", g.Currency)
	}
}
