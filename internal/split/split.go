// Package split is the arithmetic of sharing a bill: how an amount is divided
// between participants, what everyone's balance is, and who pays whom so that
// everybody ends up even. Everything is in integer minor units (cents).
package split

import (
	"errors"
	"sort"
)

// Split modes.
const (
	ModeEven    = "even"    // everyone the same
	ModeShares  = "shares"  // by integer weights (2 shares for the couple, 1 for the single)
	ModePercent = "percent" // by hundredths of a percent (10000 = 100 %)
	ModeAmount  = "amount"  // exact amounts
)

// ValidMode reports whether m is a known split mode.
func ValidMode(m string) bool {
	return m == ModeEven || m == ModeShares || m == ModePercent || m == ModeAmount
}

// Apportion divides total into len(weights) integer parts proportional to the
// weights, so that the parts always add up to total. Leftover units go to the
// parts with the largest remainders (ties: the earlier one), which is the
// usual largest-remainder method. Negative totals are handled symmetrically.
func Apportion(total int64, weights []int64) []int64 {
	out := make([]int64, len(weights))
	var sum int64
	for _, w := range weights {
		sum += w
	}
	if sum <= 0 || len(weights) == 0 {
		return out
	}
	neg := total < 0
	if neg {
		total = -total
	}
	type rem struct {
		i int
		r int64
	}
	rems := make([]rem, 0, len(weights))
	var given int64
	for i, w := range weights {
		exact := total * w // fits int64 for any realistic amount * weight
		out[i] = exact / sum
		given += out[i]
		rems = append(rems, rem{i, exact % sum})
	}
	sort.SliceStable(rems, func(a, b int) bool { return rems[a].r > rems[b].r })
	for k := int64(0); k < total-given; k++ {
		out[rems[k%int64(len(rems))].i]++
	}
	if neg {
		for i := range out {
			out[i] = -out[i]
		}
	}
	return out
}

// Errors from Amounts.
var (
	ErrNoParticipants = errors.New("pick at least one participant")
	ErrShares         = errors.New("shares must be positive numbers")
	ErrPercent        = errors.New("percentages must add up to 100")
	ErrAmounts        = errors.New("the amounts must add up to the total")
)

// Amounts turns a split definition into what each participant owes, in the
// expense's currency. shares means: weights for ModeShares, hundredths of a
// percent for ModePercent, exact minor units for ModeAmount; ignored for
// ModeEven. The result always sums to total.
func Amounts(mode string, total int64, shares []int64) ([]int64, error) {
	n := len(shares)
	if n == 0 {
		return nil, ErrNoParticipants
	}
	switch mode {
	case ModeEven:
		w := make([]int64, n)
		for i := range w {
			w[i] = 1
		}
		return Apportion(total, w), nil
	case ModeShares:
		var sum int64
		for _, s := range shares {
			if s < 0 {
				return nil, ErrShares
			}
			sum += s
		}
		if sum == 0 {
			return nil, ErrShares
		}
		return Apportion(total, shares), nil
	case ModePercent:
		var sum int64
		for _, s := range shares {
			if s < 0 {
				return nil, ErrPercent
			}
			sum += s
		}
		if sum != 10000 {
			return nil, ErrPercent
		}
		return Apportion(total, shares), nil
	case ModeAmount:
		var sum int64
		for _, s := range shares {
			if (total >= 0 && s < 0) || (total < 0 && s > 0) {
				return nil, ErrAmounts
			}
			sum += s
		}
		if sum != total {
			return nil, ErrAmounts
		}
		out := make([]int64, n)
		copy(out, shares)
		return out, nil
	}
	return nil, errors.New("unknown split mode")
}

// Balance is one participant's position in the group currency: what they
// paid for everyone, what their part of all expenses is, and the difference
// (positive = the group owes them, negative = they owe the group).
type Balance struct {
	MemberID int64 `json:"member_id"`
	Paid     int64 `json:"paid"`
	Owed     int64 `json:"owed"`
	Balance  int64 `json:"balance"`
}

// Transfer is one suggested payment that settles part of the debt.
type Transfer struct {
	From   int64 `json:"from"`
	To     int64 `json:"to"`
	Amount int64 `json:"amount"`
}

// Settle proposes the payments that bring every balance to zero with as few
// transfers as the greedy pairing gives (largest debtor pays largest creditor
// first). Balances must sum to zero; a leftover unit from rounding is
// absorbed by the last pairing. Deterministic: ties break on member id.
func Settle(balances []Balance) []Transfer {
	type side struct {
		id  int64
		amt int64
	}
	var debtors, creditors []side
	for _, b := range balances {
		switch {
		case b.Balance < 0:
			debtors = append(debtors, side{b.MemberID, -b.Balance})
		case b.Balance > 0:
			creditors = append(creditors, side{b.MemberID, b.Balance})
		}
	}
	less := func(s []side) func(i, j int) bool {
		return func(i, j int) bool {
			if s[i].amt != s[j].amt {
				return s[i].amt > s[j].amt
			}
			return s[i].id < s[j].id
		}
	}
	sort.Slice(debtors, less(debtors))
	sort.Slice(creditors, less(creditors))
	var out []Transfer
	i, j := 0, 0
	for i < len(debtors) && j < len(creditors) {
		d, c := &debtors[i], &creditors[j]
		amt := d.amt
		if c.amt < amt {
			amt = c.amt
		}
		if amt > 0 {
			out = append(out, Transfer{From: d.id, To: c.id, Amount: amt})
		}
		d.amt -= amt
		c.amt -= amt
		if d.amt == 0 {
			i++
		}
		if c.amt == 0 {
			j++
		}
	}
	return out
}
