package server

import (
	"context"
	"database/sql"
	"fmt"
	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/store"
)

func (s *Server) expenseEvent(ctx context.Context, u *store.User, g *store.Group, e *store.Expense, topic string) core.CommitHook {
	if !s.core.Configured() {
		return nil
	}
	// Resolve audience before opening the single-connection write transaction.
	audience := s.audienceEmails(ctx, g)
	return func(tx *sql.Tx) error {
		payload := map[string]any{
			"expense_id": e.ID, "group_id": g.ID, "group": g.Name, "title": e.Title, "summary": s.expenseSummary(g, e),
			"amount": e.Amount, "currency": e.Currency, "date": e.Date, "category": e.Category, "paid_by": e.PaidBy, "reimbursement": e.Reimbursement,
			"actor_email": u.Email, "user_email": u.Email, "audience_emails": audience,
		}
		if topic != "split.expense_deleted" {
			payload["amount_group"] = e.AmountGroup
			payload["group_currency"] = g.Currency
			payload["source"] = e.Source
		}
		return core.QueueEvent(ctx, tx, topic, fmt.Sprintf("expense:%d", e.ID), payload)
	}
}
