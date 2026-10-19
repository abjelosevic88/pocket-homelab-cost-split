package store

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// TestPerUserMigrationBackfill opens a database as it was before 0003 (a group
// without a creator whose owner is only a name-only member, a custom category,
// box-wide preferences) and checks the migration hands it all to the owner.
func TestPerUserMigrationBackfill(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE schema_migrations (name TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"0001_init.sql", "0002_categories.sql"} {
		body, _ := migrationFS.ReadFile("migrations/" + name)
		if _, err := raw.Exec(string(body)); err != nil {
			t.Fatal(err)
		}
		_, _ = raw.Exec(`INSERT INTO schema_migrations VALUES (?, ?)`, name, Now())
	}
	for _, q := range []string{
		`INSERT INTO users (id, email, name, role) VALUES (1, 'aleksandar@example.com', 'Aleksandar', 'admin'), (2, 'sani@example.com', 'Sani', 'member')`,
		`INSERT INTO groups (id, name, currency) VALUES (7, 'Old flat', 'EUR')`,
		`INSERT INTO groups (id, name, currency, created_by) VALUES (8, 'Linked already', 'EUR', 1)`,
		`INSERT INTO members (id, group_id, name) VALUES (1, 7, 'Aleksandar'), (2, 7, 'Sani')`,
		`INSERT INTO members (id, group_id, user_id, name) VALUES (3, 8, 1, 'Me'), (4, 8, NULL, 'Aleksandar')`,
		`INSERT INTO expenses (id, group_id, title, amount, currency, amount_group, paid_by, date) VALUES (1, 7, 'Rent', 100, 'EUR', 100, 1, '2026-10-01')`,
		`INSERT INTO expense_shares (expense_id, member_id, amount, amount_group) VALUES (1, 1, 50, 50), (1, 2, 50, 50)`,
		`INSERT INTO activity (group_id, kind) VALUES (7, 'expense.added')`,
		`INSERT INTO categories (id, label) VALUES ('testi', 'testi')`,
		`INSERT INTO settings (key, value) VALUES ('default_currency', 'BAM'), ('title', 'Pocket Split')`,
	} {
		if _, err := raw.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	raw.Close()

	open := func() *Store {
		s, err := Open(path, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	s := open()
	s.Close()
	s = open() // a second start changes nothing
	defer s.Close()
	ctx := context.Background()

	g, err := s.GetGroup(ctx, 7, 1)
	if err != nil {
		t.Fatalf("owner cannot see the backfilled group: %v", err)
	}
	if g.CreatedBy != 1 || g.MyMemberID != 1 || len(g.Members) != 2 {
		t.Fatalf("group 7: the name-only member should be linked, not duplicated: %+v", g)
	}
	g8, _ := s.GetGroup(ctx, 8, 1)
	if len(g8.Members) != 2 || g8.MyMemberID != 3 {
		t.Fatalf("group 8 should be left alone: %+v", g8)
	}
	if _, err := s.GetGroup(ctx, 7, 2); err != ErrNotFound {
		t.Fatalf("a member with no link sees the owner's group: %v", err)
	}
	var orphans int
	_ = s.DB.QueryRow(`SELECT (SELECT COUNT(1) FROM groups WHERE created_by IS NULL) + (SELECT COUNT(1) FROM expenses WHERE created_by IS NULL)
		+ (SELECT COUNT(1) FROM activity WHERE user_id IS NULL) + (SELECT COUNT(1) FROM categories WHERE builtin = 0 AND user_id IS NULL)`).Scan(&orphans)
	if orphans != 0 {
		t.Fatalf("%d rows without an owner", orphans)
	}
	cats, _ := s.ListCategories(ctx, 1)
	mine := false
	for _, c := range cats {
		if c.ID == "testi" && c.Mine {
			mine = true
		}
	}
	if !mine {
		t.Fatalf("custom category not given to the owner")
	}
	if cats, _ := s.ListCategories(ctx, 2); len(cats) != 14 {
		t.Fatalf("member sees %d categories, want the 14 built-in ones", len(cats))
	}
	if us, _ := s.GetUserSettings(ctx, 1); us.DefaultCurrency != "BAM" {
		t.Fatalf("owner's preferences not copied: %+v", us)
	}
	bals, _ := s.Balances(ctx, 7)
	if len(bals) != 2 || bals[0].Balance != 50 || bals[1].Balance != -50 {
		t.Fatalf("balances changed: %+v", bals)
	}
}
