package store

import (
	"context"
	"sort"

	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/split"
)

// Claim is what a cost center owes one person (negative: the person owes it).
type Claim struct {
	MemberID int64 `json:"member_id"`
	Amount   int64 `json:"amount"`
}

// CostCenterPosition is where one cost center stands, in the group currency.
type CostCenterPosition struct {
	MemberID int64 `json:"member_id"`
	// Charged is everything put on it: costs people fronted for it and money
	// paid into it. Repaid is what it paid back. Owes = Charged - Repaid,
	// which is also the sum of Claims.
	Charged int64   `json:"charged"`
	Repaid  int64   `json:"repaid"`
	Owes    int64   `json:"owes"`
	Claims  []Claim `json:"claims"` // by member id, zero claims left out
}

// Ledger splits a group's money into two books that never mix:
//
//   - Balances: what people owe each other personally (persons only). This is
//     what "who owes whom" settles.
//   - CostCenters: for each cost center, what it owes back to whoever fronted
//     money for it.
//
// Every share of every expense is one flow from the sharer to the payer, and
// lands in exactly one book:
//
//   - a payment with a cost center (B pays A, buying into C): C's debt moves
//     from A to B — claim[C][B] += x, claim[C][A] -= x;
//   - paid by a cost center (C pays A back): claim[C][A] -= x;
//   - shared by a cost center (A pays for C, or pays money into it):
//     claim[C][A] += x;
//   - everything else is personal: A paid for B.
//
// So paid - owed of the raw expenses still equals a person's personal
// balance plus their claims, and nothing is ever lost between the books.
type Ledger struct {
	Balances    []split.Balance      `json:"balances"`
	CostCenters []CostCenterPosition `json:"cost_centers"`
}

// ClaimsOf is what all cost centers together owe member id.
func (l *Ledger) ClaimsOf(id int64) int64 {
	var n int64
	for _, cc := range l.CostCenters {
		for _, c := range cc.Claims {
			if c.MemberID == id {
				n += c.Amount
			}
		}
	}
	return n
}

// Claim is what cost center cc owes member id.
func (l *Ledger) Claim(cc, id int64) int64 {
	for _, p := range l.CostCenters {
		if p.MemberID != cc {
			continue
		}
		for _, c := range p.Claims {
			if c.MemberID == id {
				return c.Amount
			}
		}
	}
	return 0
}

// Ledger computes both books of a group. except, when given, leaves that
// expense out (to check a payment being edited against everything else).
func (s *Store) Ledger(ctx context.Context, groupID int64, except ...int64) (*Ledger, error) {
	members, err := s.ListMembers(ctx, groupID)
	if err != nil {
		return nil, err
	}
	var skip int64
	if len(except) > 0 {
		skip = except[0]
	}
	cc := map[int64]bool{}
	bal := map[int64]*split.Balance{}
	pos := map[int64]*CostCenterPosition{}
	claims := map[int64]map[int64]int64{}
	l := &Ledger{Balances: []split.Balance{}, CostCenters: []CostCenterPosition{}}
	for _, m := range members {
		if m.IsCostCenter() {
			cc[m.ID] = true
			pos[m.ID] = &CostCenterPosition{MemberID: m.ID, Claims: []Claim{}}
			claims[m.ID] = map[int64]int64{}
		} else {
			bal[m.ID] = &split.Balance{MemberID: m.ID}
		}
	}
	claim := func(c, who, amt int64) {
		if claims[c] != nil {
			claims[c][who] += amt
		}
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT e.paid_by, COALESCE(e.cost_center_id, 0), sh.member_id, sh.amount_group
		FROM expense_shares sh JOIN expenses e ON e.id = sh.expense_id
		WHERE e.group_id = ? AND e.id != ?`, groupID, skip)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var payer, tag, sharer, amt int64
		if err := rows.Scan(&payer, &tag, &sharer, &amt); err != nil {
			return nil, err
		}
		switch {
		case tag != 0 && cc[tag]:
			claim(tag, payer, amt)
			claim(tag, sharer, -amt)
		case cc[payer] && cc[sharer]:
			// Between two cost centers (or one with itself): no person involved.
		case cc[payer]:
			claim(payer, sharer, -amt)
			pos[payer].Repaid += amt
		case cc[sharer]:
			claim(sharer, payer, amt)
			pos[sharer].Charged += amt
		default:
			if b := bal[payer]; b != nil {
				b.Paid += amt
			}
			if b := bal[sharer]; b != nil {
				b.Owed += amt
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, m := range members {
		if b := bal[m.ID]; b != nil {
			b.Balance = b.Paid - b.Owed
			l.Balances = append(l.Balances, *b)
			continue
		}
		p := pos[m.ID]
		for who, amt := range claims[m.ID] {
			if amt != 0 {
				p.Claims = append(p.Claims, Claim{MemberID: who, Amount: amt})
				p.Owes += amt
			}
		}
		sort.Slice(p.Claims, func(i, j int) bool { return p.Claims[i].MemberID < p.Claims[j].MemberID })
		l.CostCenters = append(l.CostCenters, *p)
	}
	return l, nil
}
