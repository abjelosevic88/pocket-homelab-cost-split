package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/store"
	"io"
	"net/http"
	"sync"
	"testing"
)

func TestExpenseReplayAndAtomicEvents(t *testing.T) {
	h := newPerUserHarness(t)
	var g store.Group
	h.json(h.anaTok, "POST", "/groups", map[string]any{"name": "Home", "currency": "EUR", "members": []map[string]any{{"me": true}, {"name": "Partner"}}}, 201, &g)
	path := fmt.Sprintf("/groups/%d/expenses", g.ID)
	body := map[string]any{"title": "Groceries", "amount": 1800, "paid_by": g.Members[0].ID, "shares": []map[string]any{{"member_id": g.Members[0].ID, "share": 1}, {"member_id": g.Members[1].ID, "share": 1}}}
	call := func(tok, key string, in any) (int, store.Expense) {
		b, _ := json.Marshal(in)
		req, _ := http.NewRequest("POST", h.srv.URL+"/api/v1"+path, bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Error(err)
			return 0, store.Expense{}
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		var e store.Expense
		json.Unmarshal(raw, &e)
		return res.StatusCode, e
	}
	ids := make(chan int64, 12)
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, e := call(h.anaTok, "checkout-1", body)
			if code != 201 {
				t.Error("create/replay", code)
				return
			}
			ids <- e.ID
		}()
	}
	wg.Wait()
	close(ids)
	var id int64
	for got := range ids {
		if id == 0 {
			id = got
		}
		if got != id {
			t.Error("different IDs", id, got)
		}
	}
	if id == 0 {
		t.Fatal("no expense")
	}
	for _, table := range []string{"expenses", "pocket_operations", "pocket_outbox"} {
		var n int
		if err := h.st.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil || n != 1 {
			t.Fatal(table, n, err)
		}
	}
	body["amount"] = 1900
	if code, _ := call(h.anaTok, "checkout-1", body); code != 409 {
		t.Fatal("key conflict", code)
	}
	body["amount"] = 1800
	if code, _ := call(h.benTok, "checkout-1", body); code != 404 {
		t.Fatal("unauthorized replay", code)
	}
	if _, err := h.st.DB.Exec(`CREATE TRIGGER reject_event BEFORE INSERT ON pocket_outbox BEGIN SELECT RAISE(ABORT,'queue unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if code, _ := call(h.anaTok, "checkout-2", body); code != 500 {
		t.Fatal("create without event", code)
	}
	if code, _ := h.do(h.anaTok, "DELETE", path+fmt.Sprintf("/%d", id), nil); code != 500 {
		t.Fatal("delete without event", code)
	}
	for _, table := range []string{"expenses", "pocket_operations", "pocket_outbox"} {
		var n int
		h.st.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n)
		if n != 1 {
			t.Fatal("rollback", table, n)
		}
	}
	h.st.DB.Exec(`DROP TRIGGER reject_event`)
	if code, _ := call(h.anaTok, "checkout-2", body); code != 201 {
		t.Fatal("retry", code)
	}
	if code, _ := h.do(h.anaTok, "DELETE", path+fmt.Sprintf("/%d", id), nil); code != 204 {
		t.Fatal("delete", code)
	}
	if code, _ := call(h.anaTok, "checkout-1", body); code != 404 {
		t.Fatal("deleted replay", code)
	}

	code, second := call(h.anaTok, "checkout-2", body)
	if code != 201 {
		t.Fatal(code)
	}
	if code, _ := h.do(h.anaTok, "DELETE", path+fmt.Sprintf("/%d", second.ID), nil); code != 204 {
		t.Fatal(code)
	}
	code, third := call(h.anaTok, "checkout-3", body)
	if code != 201 || third.ID <= second.ID {
		t.Fatal("deleted expense ID reused", code, second.ID, third.ID)
	}
	if code, _ := call(h.anaTok, "checkout-2", body); code != 404 {
		t.Fatal("receipt resolved to replacement expense", code)
	}
}
