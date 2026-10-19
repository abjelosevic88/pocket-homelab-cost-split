package store

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// Who sees which category: the built-in list is shared; a custom category is
// its creator's, and also shows to the people of a group where an expense
// already uses it (so a shared group reads the same for everyone in it).
// Takes the user id three times.
const visibleCategory = `(c.builtin = 1 OR c.user_id = ? OR c.id IN (SELECT e.category FROM expenses e JOIN groups g ON g.id = e.group_id WHERE ` + canSeeGroup + `))`

// categoryUse counts the expenses using c.id in the groups the user may see.
// Takes the user id twice.
const categoryUse = `(SELECT COUNT(1) FROM expenses e JOIN groups g ON g.id = e.group_id WHERE e.category = c.id AND ` + canSeeGroup + `)`

const categoryCols = `c.id, c.label, c.icon, c.builtin, c.sort_order, COALESCE(c.user_id, 0), ` + categoryUse

func scanCategory(row interface{ Scan(...any) error }, userID int64) (*Category, error) {
	var c Category
	var b int
	var owner int64
	if err := row.Scan(&c.ID, &c.Label, &c.Icon, &b, &c.SortOrder, &owner, &c.Count); err != nil {
		return nil, notFound(err)
	}
	c.Builtin = b == 1
	c.Mine = !c.Builtin && owner == userID && userID != 0
	return &c, nil
}

// ListCategories returns the categories userID sees, in display order, with
// how many of their visible expenses use each.
func (s *Store) ListCategories(ctx context.Context, userID int64) ([]Category, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+categoryCols+` FROM categories c WHERE `+visibleCategory+` ORDER BY c.sort_order, c.label`,
		userID, userID, userID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Category{}
	for rows.Next() {
		c, err := scanCategory(rows, userID)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// GetCategory loads a category userID may change: a built-in one or one of
// their own. Anything else is ErrNotFound.
func (s *Store) GetCategory(ctx context.Context, id string, userID int64) (*Category, error) {
	return scanCategory(s.DB.QueryRowContext(ctx, `SELECT `+categoryCols+` FROM categories c WHERE c.id = ? AND (c.builtin = 1 OR c.user_id = ?)`,
		userID, userID, id, userID), userID)
}

// CategoryExists reports whether id is a category userID may use.
func (s *Store) CategoryExists(ctx context.Context, id string, userID int64) bool {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM categories c WHERE c.id = ? AND `+visibleCategory, id, userID, userID, userID).Scan(&n)
	return n > 0
}

func (s *Store) categoryIDTaken(ctx context.Context, id string) bool {
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

// CreateCategory adds a category owned by userID; the id is derived from the
// label and made unique with a numeric suffix if needed (ids are shared by the
// whole box, names are per person). A label the user already has, built-in or
// their own (same name ignoring case, or the same slug: "Ski passes" vs
// "ski-passes!"), returns that category instead of a duplicate.
func (s *Store) CreateCategory(ctx context.Context, userID int64, label, icon string) (*Category, error) {
	label = strings.TrimSpace(label)
	base := Slug(label)
	var existing string
	if err := s.DB.QueryRowContext(ctx, `SELECT id FROM categories WHERE (builtin = 1 OR user_id = ?) AND (lower(label) = lower(?) OR id = ?) ORDER BY builtin DESC LIMIT 1`, userID, label, base).Scan(&existing); err == nil {
		return s.GetCategory(ctx, existing, userID)
	}
	id := base
	for i := 2; s.categoryIDTaken(ctx, id); i++ {
		id = fmt.Sprintf("%s-%d", base, i)
	}
	var max int
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), 0) FROM categories`).Scan(&max)
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO categories (id, label, icon, builtin, sort_order, user_id) VALUES (?, ?, ?, 0, ?, ?)`, id, label, strings.TrimSpace(icon), max+1, userID); err != nil {
		return nil, conflict(err)
	}
	return s.GetCategory(ctx, id, userID)
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
