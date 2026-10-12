package store

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// ListCategories returns every category in display order with usage counts.
func (s *Store) ListCategories(ctx context.Context) ([]Category, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT c.id, c.label, c.icon, c.builtin, c.sort_order, (SELECT COUNT(1) FROM expenses e WHERE e.category = c.id)
		FROM categories c ORDER BY c.sort_order, c.label`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Category{}
	for rows.Next() {
		var c Category
		var b int
		if err := rows.Scan(&c.ID, &c.Label, &c.Icon, &b, &c.SortOrder, &c.Count); err != nil {
			return nil, err
		}
		c.Builtin = b == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetCategory loads one category.
func (s *Store) GetCategory(ctx context.Context, id string) (*Category, error) {
	var c Category
	var b int
	err := s.DB.QueryRowContext(ctx, `SELECT id, label, icon, builtin, sort_order, (SELECT COUNT(1) FROM expenses e WHERE e.category = categories.id) FROM categories WHERE id = ?`, id).Scan(&c.ID, &c.Label, &c.Icon, &b, &c.SortOrder, &c.Count)
	if err != nil {
		return nil, notFound(err)
	}
	c.Builtin = b == 1
	return &c, nil
}

// CategoryExists reports whether id is a category.
func (s *Store) CategoryExists(ctx context.Context, id string) bool {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM categories WHERE id = ?`, id).Scan(&n)
	return n > 0
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// Slug turns "Ski passes!" into "ski-passes".
func Slug(label string) string {
	s := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(label), "-"), "-")
	if s == "" {
		s = "category"
	}
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}

// CreateCategory adds a category; the id is derived from the label and made
// unique with a numeric suffix if needed. A label that already exists (same
// name ignoring case, or the same slug: "Ski passes" vs "ski-passes!") returns
// that category instead of a duplicate.
func (s *Store) CreateCategory(ctx context.Context, label, icon string) (*Category, error) {
	label = strings.TrimSpace(label)
	base := Slug(label)
	var existing string
	if err := s.DB.QueryRowContext(ctx, `SELECT id FROM categories WHERE lower(label) = lower(?) OR id = ?`, label, base).Scan(&existing); err == nil {
		return s.GetCategory(ctx, existing)
	}
	id := base
	for i := 2; s.CategoryExists(ctx, id); i++ {
		id = fmt.Sprintf("%s-%d", base, i)
	}
	var max int
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), 0) FROM categories`).Scan(&max)
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO categories (id, label, icon, builtin, sort_order) VALUES (?, ?, ?, 0, ?)`, id, label, strings.TrimSpace(icon), max+1); err != nil {
		return nil, conflict(err)
	}
	return s.GetCategory(ctx, id)
}

// UpdateCategory changes label, icon and position.
func (s *Store) UpdateCategory(ctx context.Context, c *Category) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE categories SET label = ?, icon = ?, sort_order = ? WHERE id = ?`, c.Label, c.Icon, c.SortOrder, c.ID)
	return err
}

// DeleteCategory removes a category; its expenses move to "general".
func (s *Store) DeleteCategory(ctx context.Context, id string) error {
	if id == "general" {
		return fmt.Errorf("the General category cannot be removed")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE expenses SET category = 'general' WHERE category = ?`, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM categories WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}
