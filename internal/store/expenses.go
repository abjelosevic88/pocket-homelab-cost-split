package store

import (
	"context"
	"database/sql"
	"fmt"
	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"sort"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/split"
)

// ExpenseFilter narrows ListExpenses.
type ExpenseFilter struct {
	GroupID  int64
	Q        string
	MemberID int64 // paid by or shared by
	Category string
	// Categories limits to any of these ids (used by reports); empty = all.
	Categories []string
	// Reimbursements: "exclude" | "only" | "" (both).
	Reimbursements string
	From, To       string // dates, inclusive
	Limit          int
	Offset         int
}

const expenseCols = `id, group_id, title, amount, currency, fx_rate, amount_group, paid_by, date, category, notes, split_mode, reimbursement, COALESCE(created_by, 0), source, created_at, updated_at`

func scanExpense(row interface{ Scan(...any) error }) (*Expense, error) {
	var e Expense
	var reimb int
	if err := row.Scan(&e.ID, &e.GroupID, &e.Title, &e.Amount, &e.Currency, &e.FxRate, &e.AmountGroup, &e.PaidBy, &e.Date, &e.Category, &e.Notes, &e.SplitMode, &reimb, &e.CreatedBy, &e.Source, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	e.Reimbursement = reimb == 1
	e.Shares = []Share{}
	return &e, nil
}

// ListExpenses returns a group's expenses, newest date first, with shares.
func (s *Store) ListExpenses(ctx context.Context, f ExpenseFilter) ([]*Expense, int, error) {
	where := []string{`group_id = ?`}
	args := []any{f.GroupID}
	if q := strings.TrimSpace(f.Q); q != "" {
		where = append(where, `(title LIKE ? OR notes LIKE ?)`)
		args = append(args, "%"+q+"%", "%"+q+"%")
	}
	if f.MemberID != 0 {
		where = append(where, `(paid_by = ? OR EXISTS (SELECT 1 FROM expense_shares sh WHERE sh.expense_id = expenses.id AND sh.member_id = ?))`)
		args = append(args, f.MemberID, f.MemberID)
	}
	if f.Category != "" {
		where = append(where, `category = ?`)
		args = append(args, f.Category)
	}
	if len(f.Categories) > 0 {
		ph := make([]string, len(f.Categories))
		for i, c := range f.Categories {
			ph[i] = "?"
			args = append(args, c)
		}
		where = append(where, `category IN (`+strings.Join(ph, ",")+`)`)
	}
	switch f.Reimbursements {
	case "exclude":
		where = append(where, `reimbursement = 0`)
	case "only":
		where = append(where, `reimbursement = 1`)
	}
	if f.From != "" {
		where = append(where, `date >= ?`)
		args = append(args, f.From)
	}
	if f.To != "" {
		where = append(where, `date <= ?`)
		args = append(args, f.To)
	}
	w := ` WHERE ` + strings.Join(where, " AND ")
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM expenses`+w, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	q := `SELECT ` + expenseCols + ` FROM expenses` + w + ` ORDER BY date DESC, id DESC`
	if f.Limit > 0 {
		q += fmt.Sprintf(` LIMIT %d OFFSET %d`, f.Limit, f.Offset)
	}
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []*Expense{}
	byID := map[int64]*Expense{}
	for rows.Next() {
		e, err := scanExpense(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, e)
		byID[e.ID] = e
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if len(out) == 0 {
		return out, total, nil
	}
	// Shares for the page in one query.
	ids := make([]string, 0, len(out))
	for _, e := range out {
		ids = append(ids, fmt.Sprint(e.ID))
	}
	srows, err := s.DB.QueryContext(ctx, `SELECT expense_id, member_id, share, amount, amount_group FROM expense_shares WHERE expense_id IN (`+strings.Join(ids, ",")+`) ORDER BY expense_id, member_id`)
	if err != nil {
		return nil, 0, err
	}
	defer srows.Close()
	for srows.Next() {
		var eid int64
		var sh Share
		if err := srows.Scan(&eid, &sh.MemberID, &sh.Share, &sh.Amount, &sh.AmountGroup); err != nil {
			return nil, 0, err
		}
		if e := byID[eid]; e != nil {
			e.Shares = append(e.Shares, sh)
		}
	}
	return out, total, srows.Err()
}

// GetExpense loads one expense of a group with its shares.
func (s *Store) GetExpense(ctx context.Context, groupID, id int64) (*Expense, error) {
	e, err := scanExpense(s.DB.QueryRowContext(ctx, `SELECT `+expenseCols+` FROM expenses WHERE id = ? AND group_id = ?`, id, groupID))
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT member_id, share, amount, amount_group FROM expense_shares WHERE expense_id = ? ORDER BY member_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var sh Share
		if err := rows.Scan(&sh.MemberID, &sh.Share, &sh.Amount, &sh.AmountGroup); err != nil {
			return nil, err
		}
		e.Shares = append(e.Shares, sh)
	}
	return e, rows.Err()
}

// SaveExpense inserts (ID 0) or replaces an expense with its shares. Shares
// must already carry Amount and AmountGroup (see ComputeShares).
func (s *Store) SaveExpense(ctx context.Context, e *Expense, after ...core.CommitHook) error {
	_, err := s.SaveExpenseOperation(ctx, e, nil, after...)
	return err
}
func (s *Store) SaveExpenseOperation(ctx context.Context, e *Expense, op *core.Operation, after ...core.CommitHook) (bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var replay Expense
	if ok, err := op.Replay(ctx, tx, &replay); err != nil {
		return false, err
	} else if ok {
		var id int64
		if err = tx.QueryRowContext(ctx, `SELECT id FROM expenses WHERE id=? AND group_id=?`, replay.ID, e.GroupID).Scan(&id); err != nil {
			return false, notFound(err)
		}
		*e = replay
		return true, nil
	}
	var createdBy any
	if e.CreatedBy != 0 {
		createdBy = e.CreatedBy
	}
	if e.ID == 0 {
		res, err := tx.ExecContext(ctx, `INSERT INTO expenses (group_id, title, amount, currency, fx_rate, amount_group, paid_by, date, category, notes, split_mode, reimbursement, created_by, source)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			e.GroupID, e.Title, e.Amount, e.Currency, e.FxRate, e.AmountGroup, e.PaidBy, e.Date, e.Category, e.Notes, e.SplitMode, boolInt(e.Reimbursement), createdBy, e.Source)
		if err != nil {
			return false, err
		}
		e.ID, _ = res.LastInsertId()
		e.CreatedAt, e.UpdatedAt = Now(), Now()
	} else {
		res, err := tx.ExecContext(ctx, `UPDATE expenses SET title = ?, amount = ?, currency = ?, fx_rate = ?, amount_group = ?, paid_by = ?, date = ?, category = ?, notes = ?, split_mode = ?, reimbursement = ?, updated_at = ?
			WHERE id = ? AND group_id = ?`,
			e.Title, e.Amount, e.Currency, e.FxRate, e.AmountGroup, e.PaidBy, e.Date, e.Category, e.Notes, e.SplitMode, boolInt(e.Reimbursement), Now(), e.ID, e.GroupID)
		if err != nil {
			return false, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return false, ErrNotFound
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM expense_shares WHERE expense_id = ?`, e.ID); err != nil {
			return false, err
		}
	}
	for _, sh := range e.Shares {
		if _, err := tx.ExecContext(ctx, `INSERT INTO expense_shares (expense_id, member_id, share, amount, amount_group) VALUES (?, ?, ?, ?, ?)`, e.ID, sh.MemberID, sh.Share, sh.Amount, sh.AmountGroup); err != nil {
			return false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE groups SET updated_at = ? WHERE id = ?`, Now(), e.GroupID); err != nil {
		return false, err
	}
	if err = op.Save(ctx, tx, e); err != nil {
		return false, err
	}
	return false, core.Commit(tx, after...)
}

// DeleteExpense removes an expense with its shares.
func (s *Store) DeleteExpense(ctx context.Context, groupID, id int64, after ...core.CommitHook) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `DELETE FROM expenses WHERE id=? AND group_id=?`, id, groupID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if _, err = tx.ExecContext(ctx, `UPDATE groups SET updated_at=? WHERE id=?`, Now(), groupID); err != nil {
		return err
	}
	return core.Commit(tx, after...)
}

// ComputeShares fills Amount and AmountGroup on e.Shares from e.Amount,
// e.AmountGroup and e.SplitMode. Shares are sorted by member id first so the
// rounding leftovers land deterministically.
func ComputeShares(e *Expense) error {
	if !split.ValidMode(e.SplitMode) {
		return fmt.Errorf("unknown split mode %q", e.SplitMode)
	}
	sort.Slice(e.Shares, func(i, j int) bool { return e.Shares[i].MemberID < e.Shares[j].MemberID })
	in := make([]int64, len(e.Shares))
	for i, sh := range e.Shares {
		in[i] = sh.Share
	}
	amounts, err := split.Amounts(e.SplitMode, e.Amount, in)
	if err != nil {
		return err
	}
	// Each part in the group currency: proportional to the part in the paid
	// currency, adding up to amount_group exactly.
	weights := make([]int64, len(amounts))
	for i, a := range amounts {
		if a < 0 {
			weights[i] = -a
		} else {
			weights[i] = a
		}
	}
	allZero := true
	for _, w := range weights {
		if w != 0 {
			allZero = false
		}
	}
	if allZero {
		for i := range weights {
			weights[i] = 1
		}
	}
	groupAmounts := split.Apportion(e.AmountGroup, weights)
	for i := range e.Shares {
		e.Shares[i].Amount = amounts[i]
		e.Shares[i].AmountGroup = groupAmounts[i]
		if e.SplitMode == split.ModeEven {
			e.Shares[i].Share = 1
		}
	}
	return nil
}

// Balances computes every member's position in the group currency.
func (s *Store) Balances(ctx context.Context, groupID int64) ([]split.Balance, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT m.id,
		       COALESCE((SELECT SUM(e.amount_group) FROM expenses e WHERE e.paid_by = m.id), 0),
		       COALESCE((SELECT SUM(sh.amount_group) FROM expense_shares sh WHERE sh.member_id = m.id), 0)
		FROM members m WHERE m.group_id = ? ORDER BY m.id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []split.Balance{}
	for rows.Next() {
		var b split.Balance
		if err := rows.Scan(&b.MemberID, &b.Paid, &b.Owed); err != nil {
			return nil, err
		}
		b.Balance = b.Paid - b.Owed
		out = append(out, b)
	}
	return out, rows.Err()
}

// GroupStats is what the Stats tab shows.
type GroupStats struct {
	Currency      string            `json:"currency"`
	Total         int64             `json:"total"`      // spent, reimbursements excluded
	Reimbursed    int64             `json:"reimbursed"` // settle-up payments
	Count         int               `json:"count"`      // expenses, reimbursements excluded
	Months        []MonthTotal      `json:"months"`     // oldest first
	Categories    []CategoryTotal   `json:"categories"` // largest first
	Members       []MemberTotal     `json:"members"`    // by member id
	Currencies    []CurrencyTotal   `json:"currencies"` // what was paid in which currency
	FirstDate     string            `json:"first_date"`
	LastDate      string            `json:"last_date"`
	ByCategoryMem map[string]string `json:"-"`
}

// MonthTotal is spending in one month.
type MonthTotal struct {
	Month string `json:"month"` // YYYY-MM
	Total int64  `json:"total"`
	Count int    `json:"count"`
}

// CategoryTotal is spending in one category.
type CategoryTotal struct {
	Category string `json:"category"`
	Total    int64  `json:"total"`
	Count    int    `json:"count"`
}

// MemberTotal is what one member paid and what their part was.
type MemberTotal struct {
	MemberID int64 `json:"member_id"`
	Paid     int64 `json:"paid"`
	Share    int64 `json:"share"`
	Count    int   `json:"count"` // expenses they paid
}

// CurrencyTotal is how much was paid in one currency (its own minor units).
type CurrencyTotal struct {
	Currency string `json:"currency"`
	Total    int64  `json:"total"`
	Count    int    `json:"count"`
}

// Stats aggregates a group's spending.
func (s *Store) Stats(ctx context.Context, groupID int64, currency string) (*GroupStats, error) {
	st := &GroupStats{Currency: currency, Months: []MonthTotal{}, Categories: []CategoryTotal{}, Members: []MemberTotal{}, Currencies: []CurrencyTotal{}}
	var first, last sql.NullString
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(CASE WHEN reimbursement = 0 THEN amount_group END), 0), COALESCE(SUM(CASE WHEN reimbursement = 1 THEN amount_group END), 0),
		COUNT(CASE WHEN reimbursement = 0 THEN 1 END), MIN(date), MAX(date) FROM expenses WHERE group_id = ?`, groupID).Scan(&st.Total, &st.Reimbursed, &st.Count, &first, &last); err != nil {
		return nil, err
	}
	st.FirstDate, st.LastDate = first.String, last.String
	rows, err := s.DB.QueryContext(ctx, `SELECT substr(date, 1, 7), SUM(amount_group), COUNT(1) FROM expenses WHERE group_id = ? AND reimbursement = 0 GROUP BY 1 ORDER BY 1`, groupID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var m MonthTotal
		if err := rows.Scan(&m.Month, &m.Total, &m.Count); err != nil {
			rows.Close()
			return nil, err
		}
		st.Months = append(st.Months, m)
	}
	rows.Close()
	rows, err = s.DB.QueryContext(ctx, `SELECT category, SUM(amount_group), COUNT(1) FROM expenses WHERE group_id = ? AND reimbursement = 0 GROUP BY 1 ORDER BY 2 DESC`, groupID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var c CategoryTotal
		if err := rows.Scan(&c.Category, &c.Total, &c.Count); err != nil {
			rows.Close()
			return nil, err
		}
		st.Categories = append(st.Categories, c)
	}
	rows.Close()
	rows, err = s.DB.QueryContext(ctx, `SELECT m.id,
		COALESCE((SELECT SUM(e.amount_group) FROM expenses e WHERE e.paid_by = m.id AND e.reimbursement = 0), 0),
		COALESCE((SELECT SUM(sh.amount_group) FROM expense_shares sh JOIN expenses e ON e.id = sh.expense_id WHERE sh.member_id = m.id AND e.reimbursement = 0), 0),
		(SELECT COUNT(1) FROM expenses e WHERE e.paid_by = m.id AND e.reimbursement = 0)
		FROM members m WHERE m.group_id = ? ORDER BY m.id`, groupID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var m MemberTotal
		if err := rows.Scan(&m.MemberID, &m.Paid, &m.Share, &m.Count); err != nil {
			rows.Close()
			return nil, err
		}
		st.Members = append(st.Members, m)
	}
	rows.Close()
	rows, err = s.DB.QueryContext(ctx, `SELECT currency, SUM(amount), COUNT(1) FROM expenses WHERE group_id = ? AND reimbursement = 0 GROUP BY 1 ORDER BY 3 DESC`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c CurrencyTotal
		if err := rows.Scan(&c.Currency, &c.Total, &c.Count); err != nil {
			return nil, err
		}
		st.Currencies = append(st.Currencies, c)
	}
	return st, rows.Err()
}

// MonthSpend is the viewer's own share of spending (reimbursements excluded)
// in groups where they are a member, this month, per group currency.
func (s *Store) MonthSpend(ctx context.Context, userID int64, month string) ([]CurrencyTotal, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT g.currency, COALESCE(SUM(sh.amount_group), 0), COUNT(1)
		FROM expense_shares sh JOIN expenses e ON e.id = sh.expense_id JOIN members m ON m.id = sh.member_id JOIN groups g ON g.id = e.group_id
		WHERE m.user_id = ? AND e.reimbursement = 0 AND substr(e.date, 1, 7) = ? GROUP BY g.currency ORDER BY 2 DESC`, userID, month)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CurrencyTotal{}
	for rows.Next() {
		var c CurrencyTotal
		if err := rows.Scan(&c.Currency, &c.Total, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
