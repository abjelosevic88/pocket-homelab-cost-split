package store

import (
	"context"
	"fmt"
	"strings"
)

// Queries behind the Pocket capability routes (server/capabilities.go):
// the same fence as everything else, the groups the viewer may see
// (canSeeGroup), asked across all of them at once.

// ExpenseHit is an expense with the name and currency of its group.
type ExpenseHit struct {
	Expense
	GroupName     string
	GroupCurrency string
}

const expenseColsE = `e.id, e.group_id, e.title, e.amount, e.currency, e.fx_rate, e.amount_group, e.paid_by, e.date, e.category, e.notes, e.split_mode, e.reimbursement, COALESCE(e.cost_center_id, 0), COALESCE(e.created_by, 0), e.source, e.created_at, e.updated_at`

// withExtra scans an expense row with more columns after it.
type withExtra struct {
	row   interface{ Scan(...any) error }
	extra []any
}

func (w withExtra) Scan(dest ...any) error { return w.row.Scan(append(dest, w.extra...)...) }

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// likeArg is a LIKE pattern that finds w anywhere, % and _ taken literally.
func likeArg(w string) string { return "%" + likeEscaper.Replace(w) + "%" }

// SearchGroups returns the groups the viewer may see whose name or
// description has every one of the words, in the groups list's order, with
// members and the viewer's position.
func (s *Store) SearchGroups(ctx context.Context, words []string, limit int, viewer int64) ([]*Group, error) {
	if len(words) == 0 || limit <= 0 {
		return []*Group{}, nil
	}
	where := []string{canSeeGroup}
	args := []any{viewer, viewer}
	for _, w := range words {
		where = append(where, `(g.name LIKE ? ESCAPE '\' OR g.description LIKE ? ESCAPE '\')`)
		args = append(args, likeArg(w), likeArg(w))
	}
	args = append(args, limit)
	rows, err := s.DB.QueryContext(ctx, `SELECT `+groupCols+` FROM groups g WHERE `+strings.Join(where, " AND ")+
		` ORDER BY g.archived, g.updated_at DESC, g.id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	out := []*Group{}
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, g)
	}
	err = rows.Err()
	rows.Close() // before decorating: the store has a single connection
	if err != nil {
		return nil, err
	}
	return out, s.decorateGroups(ctx, out, viewer)
}

// SearchExpenses returns the expenses and payments in the groups the viewer
// may see that have every one of the words in their title, notes or
// category name, newest first, with shares.
func (s *Store) SearchExpenses(ctx context.Context, words []string, limit int, viewer int64) ([]*ExpenseHit, error) {
	if len(words) == 0 || limit <= 0 {
		return []*ExpenseHit{}, nil
	}
	where := []string{canSeeGroup}
	args := []any{viewer, viewer}
	for _, w := range words {
		where = append(where, `(e.title LIKE ? ESCAPE '\' OR e.notes LIKE ? ESCAPE '\' OR e.category IN (SELECT c.id FROM categories c WHERE c.label LIKE ? ESCAPE '\'))`)
		args = append(args, likeArg(w), likeArg(w), likeArg(w))
	}
	args = append(args, limit)
	return s.expenseHits(ctx, `SELECT `+expenseColsE+`, g.name, g.currency FROM expenses e JOIN groups g ON g.id = e.group_id WHERE `+
		strings.Join(where, " AND ")+` ORDER BY e.date DESC, e.id DESC LIMIT ?`, args)
}

// ExpensesOn returns the expenses and payments dated date (YYYY-MM-DD) in
// the groups the viewer may see, in the order they were added, with shares.
func (s *Store) ExpensesOn(ctx context.Context, viewer int64, date string) ([]*ExpenseHit, error) {
	return s.expenseHits(ctx, `SELECT `+expenseColsE+`, g.name, g.currency FROM expenses e JOIN groups g ON g.id = e.group_id WHERE `+
		canSeeGroup+` AND e.date = ? ORDER BY e.id`, []any{viewer, viewer, date})
}

func (s *Store) expenseHits(ctx context.Context, q string, args []any) ([]*ExpenseHit, error) {
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	out := []*ExpenseHit{}
	for rows.Next() {
		h := &ExpenseHit{}
		e, err := scanExpense(withExtra{rows, []any{&h.GroupName, &h.GroupCurrency}})
		if err != nil {
			rows.Close()
			return nil, err
		}
		h.Expense = *e
		out = append(out, h)
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(out) == 0 {
		return out, err
	}
	ids := make([]string, 0, len(out))
	byID := map[int64]*ExpenseHit{}
	for _, h := range out {
		ids = append(ids, fmt.Sprint(h.ID))
		byID[h.ID] = h
	}
	srows, err := s.DB.QueryContext(ctx, `SELECT expense_id, member_id, share, amount, amount_group FROM expense_shares WHERE expense_id IN (`+strings.Join(ids, ",")+`) ORDER BY expense_id, member_id`)
	if err != nil {
		return nil, err
	}
	defer srows.Close()
	for srows.Next() {
		var eid int64
		var sh Share
		if err := srows.Scan(&eid, &sh.MemberID, &sh.Share, &sh.Amount, &sh.AmountGroup); err != nil {
			return nil, err
		}
		if h := byID[eid]; h != nil {
			h.Shares = append(h.Shares, sh)
		}
	}
	return out, srows.Err()
}
