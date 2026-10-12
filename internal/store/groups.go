package store

import (
	"context"
	"database/sql"
	"strings"
)

// GroupFilter narrows ListGroups.
type GroupFilter struct {
	// UserID limits to groups where this user is a linked member; 0 = all.
	UserID   int64
	Archived *bool // nil = both
	Q        string
}

const groupCols = `g.id, g.name, g.description, g.currency, COALESCE(g.created_by, 0), g.archived, g.created_at, g.updated_at`

func scanGroup(row interface{ Scan(...any) error }) (*Group, error) {
	var g Group
	var archived int
	if err := row.Scan(&g.ID, &g.Name, &g.Description, &g.Currency, &g.CreatedBy, &archived, &g.CreatedAt, &g.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	g.Archived = archived == 1
	return &g, nil
}

// CreateGroup inserts a group and its first members.
func (s *Store) CreateGroup(ctx context.Context, g *Group, members []Member) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var createdBy any
	if g.CreatedBy != 0 {
		createdBy = g.CreatedBy
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO groups (name, description, currency, created_by) VALUES (?, ?, ?, ?)`, g.Name, g.Description, g.Currency, createdBy)
	if err != nil {
		return err
	}
	g.ID, _ = res.LastInsertId()
	for i := range members {
		members[i].GroupID = g.ID
		if err := insertMember(ctx, tx, &members[i]); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func insertMember(ctx context.Context, tx *sql.Tx, m *Member) error {
	var uid any
	if m.UserID != nil && *m.UserID > 0 {
		uid = *m.UserID
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO members (group_id, user_id, name) VALUES (?, ?, ?)`, m.GroupID, uid, m.Name)
	if err != nil {
		return conflict(err)
	}
	m.ID, _ = res.LastInsertId()
	return nil
}

// UpdateGroup saves name, description, currency and archived.
func (s *Store) UpdateGroup(ctx context.Context, g *Group) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE groups SET name = ?, description = ?, currency = ?, archived = ?, updated_at = ? WHERE id = ?`,
		g.Name, g.Description, g.Currency, boolInt(g.Archived), Now(), g.ID)
	return err
}

// Touch bumps a group's updated_at.
func (s *Store) Touch(ctx context.Context, groupID int64) {
	_, _ = s.DB.ExecContext(ctx, `UPDATE groups SET updated_at = ? WHERE id = ?`, Now(), groupID)
}

// DeleteGroup removes a group with everything in it.
func (s *Store) DeleteGroup(ctx context.Context, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM groups WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetGroup loads a group with members and the viewer's position (viewer 0 = none).
func (s *Store) GetGroup(ctx context.Context, id, viewer int64) (*Group, error) {
	g, err := scanGroup(s.DB.QueryRowContext(ctx, `SELECT `+groupCols+` FROM groups g WHERE g.id = ?`, id))
	if err != nil {
		return nil, err
	}
	if err := s.decorateGroups(ctx, []*Group{g}, viewer); err != nil {
		return nil, err
	}
	return g, nil
}

// ListGroups returns groups, newest activity first.
func (s *Store) ListGroups(ctx context.Context, f GroupFilter, viewer int64) ([]*Group, error) {
	var where []string
	var args []any
	if f.UserID != 0 {
		where = append(where, `EXISTS (SELECT 1 FROM members m WHERE m.group_id = g.id AND m.user_id = ?)`)
		args = append(args, f.UserID)
	}
	if f.Archived != nil {
		where = append(where, `g.archived = ?`)
		args = append(args, boolInt(*f.Archived))
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		where = append(where, `(g.name LIKE ? OR g.description LIKE ?)`)
		args = append(args, "%"+q+"%", "%"+q+"%")
	}
	sqlStr := `SELECT ` + groupCols + ` FROM groups g`
	if len(where) > 0 {
		sqlStr += ` WHERE ` + strings.Join(where, " AND ")
	}
	sqlStr += ` ORDER BY g.archived, g.updated_at DESC, g.id DESC`
	rows, err := s.DB.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Group{}
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, s.decorateGroups(ctx, out, viewer)
}

// decorateGroups fills members, counts, totals and the viewer's balance.
func (s *Store) decorateGroups(ctx context.Context, groups []*Group, viewer int64) error {
	for _, g := range groups {
		members, err := s.ListMembers(ctx, g.ID)
		if err != nil {
			return err
		}
		g.Members = members
		for _, m := range members {
			if viewer != 0 && m.UserID != nil && *m.UserID == viewer {
				g.MyMemberID = m.ID
			}
		}
		var last sql.NullString
		if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1), COALESCE(SUM(CASE WHEN reimbursement = 0 THEN amount_group ELSE 0 END), 0), MAX(date) FROM expenses WHERE group_id = ?`, g.ID).Scan(&g.ExpenseCount, &g.Total, &last); err != nil {
			return err
		}
		g.LastActivity = last.String
		if g.MyMemberID != 0 {
			bals, err := s.Balances(ctx, g.ID)
			if err != nil {
				return err
			}
			for _, b := range bals {
				if b.MemberID == g.MyMemberID {
					g.MyBalance = b.Balance
				}
			}
		}
	}
	return nil
}

// ---- members ----

// ListMembers returns a group's members in join order.
func (s *Store) ListMembers(ctx context.Context, groupID int64) ([]Member, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, group_id, user_id, name, created_at FROM members WHERE group_id = ? ORDER BY id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Member{}
	for rows.Next() {
		var m Member
		var uid sql.NullInt64
		if err := rows.Scan(&m.ID, &m.GroupID, &uid, &m.Name, &m.CreatedAt); err != nil {
			return nil, err
		}
		if uid.Valid {
			v := uid.Int64
			m.UserID = &v
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetMember loads one member of a group.
func (s *Store) GetMember(ctx context.Context, groupID, id int64) (*Member, error) {
	var m Member
	var uid sql.NullInt64
	err := s.DB.QueryRowContext(ctx, `SELECT id, group_id, user_id, name, created_at FROM members WHERE id = ? AND group_id = ?`, id, groupID).Scan(&m.ID, &m.GroupID, &uid, &m.Name, &m.CreatedAt)
	if err != nil {
		return nil, notFound(err)
	}
	if uid.Valid {
		v := uid.Int64
		m.UserID = &v
	}
	return &m, nil
}

// AddMember adds a member to a group.
func (s *Store) AddMember(ctx context.Context, m *Member) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := insertMember(ctx, tx, m); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateMember renames or (un)links a member.
func (s *Store) UpdateMember(ctx context.Context, m *Member) error {
	var uid any
	if m.UserID != nil && *m.UserID > 0 {
		uid = *m.UserID
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE members SET name = ?, user_id = ? WHERE id = ? AND group_id = ?`, m.Name, uid, m.ID, m.GroupID)
	return conflict(err)
}

// MemberInUse reports whether a member paid or shares any expense.
func (s *Store) MemberInUse(ctx context.Context, id int64) (bool, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT (SELECT COUNT(1) FROM expenses WHERE paid_by = ?) + (SELECT COUNT(1) FROM expense_shares WHERE member_id = ?)`, id, id).Scan(&n)
	return n > 0, err
}

// DeleteMember removes a member that is not part of any expense.
func (s *Store) DeleteMember(ctx context.Context, groupID, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM members WHERE id = ? AND group_id = ?`, id, groupID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- activity ----

// LogActivity appends a line to a group's feed.
func (s *Store) LogActivity(ctx context.Context, a Activity) {
	var uid, eid any
	if a.UserID != 0 {
		uid = a.UserID
	}
	if a.ExpenseID != 0 {
		eid = a.ExpenseID
	}
	_, _ = s.DB.ExecContext(ctx, `INSERT INTO activity (group_id, user_id, kind, expense_id, summary) VALUES (?, ?, ?, ?, ?)`, a.GroupID, uid, a.Kind, eid, a.Summary)
}

// ListActivity returns the newest lines for one group (groupID > 0) or for all
// groups the user is in (groupID == 0, userID > 0; userID 0 = everything).
func (s *Store) ListActivity(ctx context.Context, groupID, userID int64, limit int) ([]Activity, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	q := `SELECT a.id, a.group_id, g.name, COALESCE(a.user_id, 0), COALESCE(u.name, u.email, ''), a.kind, COALESCE(a.expense_id, 0), a.summary, a.created_at
		FROM activity a JOIN groups g ON g.id = a.group_id LEFT JOIN users u ON u.id = a.user_id`
	var args []any
	switch {
	case groupID > 0:
		q += ` WHERE a.group_id = ?`
		args = append(args, groupID)
	case userID > 0:
		q += ` WHERE EXISTS (SELECT 1 FROM members m WHERE m.group_id = a.group_id AND m.user_id = ?)`
		args = append(args, userID)
	}
	q += ` ORDER BY a.id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Activity{}
	for rows.Next() {
		var a Activity
		if err := rows.Scan(&a.ID, &a.GroupID, &a.GroupName, &a.UserID, &a.UserName, &a.Kind, &a.ExpenseID, &a.Summary, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
