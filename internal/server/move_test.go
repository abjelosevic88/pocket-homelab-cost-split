package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/store"
)

// moveTestTokens are Pocket Move links the fake Core (peruser_test.go)
// answers for Ana or Ben: who, scope.
var moveTestTokens = map[string][2]string{
	"pcx_ana_export": {"ana", core.ScopeMoveExport},
	"pcx_ana_import": {"ana", core.ScopeMoveImport},
	"pcx_ben_export": {"ben", core.ScopeMoveExport},
	"pcx_ben_import": {"ben", core.ScopeMoveImport},
}

// /api/v1 paths as the harness takes them
var portablePath = strings.TrimPrefix(core.PortablePath, "/api/v1")

func TestMoveLinkScopes(t *testing.T) {
	h := newPerUserHarness(t)
	var g store.Group
	h.json(h.benTok, "POST", "/groups", map[string]any{"name": "Ben's flat", "currency": "EUR", "members": []map[string]any{{"me": true}}}, 201, &g)
	gp := fmt.Sprintf("/groups/%d", g.ID)
	exp, imp, read := "pcx_ben_export", "pcx_ben_import", "pcx_ben_read"
	for _, c := range []struct {
		tok, method, path string
		want              int
	}{
		{exp, "GET", portablePath, 200},
		{exp, "GET", "/export.json", 200},
		{exp, "POST", "/import", 403},
		{exp, "GET", "/groups", 403},
		{exp, "GET", gp + "/export.json", 403},
		{exp, "GET", "/settings", 403},
		{imp, "GET", portablePath, 200},
		{imp, "GET", "/export.json", 403},
		{imp, "POST", "/groups", 403},
		{imp, "POST", gp + "/expenses", 403},
		{imp, "PUT", "/settings", 403},
		{read, "GET", portablePath, 403},
		{read, "GET", "/export.json", 403},
	} {
		if code, body := h.do(c.tok, c.method, c.path, map[string]any{}); code != c.want {
			t.Errorf("%s %s %s → %d, want %d (%s)", c.tok, c.method, c.path, code, c.want, body)
		}
	}
	var p core.Portable
	h.json(exp, "GET", portablePath, nil, 200, &p)
	if len(p.Exports) != 1 || len(p.Imports) != 1 || p.Exports[0].Kind != core.KindPocket || p.Imports[0].Kind != core.KindPocket {
		t.Fatalf("portable: %+v", p)
	}
}

// Move brings Ana's groups to Ben: the other members stay names (no
// accounts made), a second run adds nothing, Ana's own file back adds
// nothing, and neither sees the other's groups.
func TestMoveImportsAddOnce(t *testing.T) {
	h := newPerUserHarness(t)
	h.anaGroup()

	code, export := h.do("pcx_ana_export", "GET", "/export.json", nil)
	if code != 200 || !strings.Contains(string(export), "Ana private flat") || !strings.Contains(string(export), "Rent") {
		t.Fatalf("ana's export: %d %s", code, export)
	}
	if _, ben := h.do("pcx_ben_export", "GET", "/export.json", nil); strings.Contains(string(ben), "Ana private flat") {
		t.Fatalf("ben's export holds ana's group: %s", ben)
	}
	var doc any
	_ = json.Unmarshal(export, &doc)

	var own, first, second map[string]any
	h.json("pcx_ana_import", "POST", "/import", doc, 200, &own)
	if own["groups"].(float64) != 0 || own["already"].(float64) != 1 {
		t.Fatalf("ana's own file back: %v", own)
	}
	h.json("pcx_ben_import", "POST", "/import", doc, 200, &first)
	h.json("pcx_ben_import", "POST", "/import", doc, 200, &second)
	if first["groups"].(float64) != 1 || first["created"].(float64) != 1 || second["groups"].(float64) != 0 || second["already"].(float64) != 1 {
		t.Fatalf("imports: %v then %v", first, second)
	}

	var bens, anas []store.Group
	h.json(h.benTok, "GET", "/groups", nil, 200, &bens)
	h.json(h.anaTok, "GET", "/groups", nil, 200, &anas)
	if len(bens) != 1 || len(anas) != 1 || bens[0].ID == anas[0].ID {
		t.Fatalf("groups: ben %+v, ana %+v", bens, anas)
	}
	if len(bens[0].Members) != 2 || bens[0].MyMemberID == 0 {
		t.Fatalf("ben's imported members: %+v", bens[0].Members)
	}
	for _, m := range bens[0].Members {
		if m.UserID != nil && *m.UserID != h.ben.ID {
			t.Fatalf("a member linked to someone else: %+v", m)
		}
	}
	var users int
	_ = h.st.DB.QueryRow(`SELECT COUNT(1) FROM users`).Scan(&users)
	if users != 2 {
		t.Fatalf("the import made accounts: %d users", users)
	}
}
