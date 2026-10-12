package split

import (
	"reflect"
	"testing"
)

func TestApportion(t *testing.T) {
	cases := []struct {
		total int64
		w     []int64
		want  []int64
	}{
		{100, []int64{1, 1, 1}, []int64{34, 33, 33}},
		{101, []int64{1, 1, 1}, []int64{34, 34, 33}},
		{1000, []int64{2, 1}, []int64{667, 333}},
		{-100, []int64{1, 1, 1}, []int64{-34, -33, -33}},
		{10, []int64{0, 1}, []int64{0, 10}},
		{5, []int64{1, 1, 1, 1, 1, 1}, []int64{1, 1, 1, 1, 1, 0}},
		{0, []int64{1, 2}, []int64{0, 0}},
	}
	for _, c := range cases {
		got := Apportion(c.total, c.w)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("Apportion(%d, %v) = %v, want %v", c.total, c.w, got, c.want)
		}
		var sum int64
		for _, v := range got {
			sum += v
		}
		if sum != c.total {
			t.Errorf("Apportion(%d, %v) sums to %d", c.total, c.w, sum)
		}
	}
}

func TestAmounts(t *testing.T) {
	if got, _ := Amounts(ModeEven, 1000, []int64{0, 0, 0}); !reflect.DeepEqual(got, []int64{334, 333, 333}) {
		t.Errorf("even: %v", got)
	}
	if got, _ := Amounts(ModeShares, 900, []int64{2, 1}); !reflect.DeepEqual(got, []int64{600, 300}) {
		t.Errorf("shares: %v", got)
	}
	if got, _ := Amounts(ModePercent, 1000, []int64{2500, 7500}); !reflect.DeepEqual(got, []int64{250, 750}) {
		t.Errorf("percent: %v", got)
	}
	if _, err := Amounts(ModePercent, 1000, []int64{2500, 7000}); err != ErrPercent {
		t.Errorf("percent sum: %v", err)
	}
	if got, _ := Amounts(ModeAmount, 1000, []int64{400, 600}); !reflect.DeepEqual(got, []int64{400, 600}) {
		t.Errorf("amount: %v", got)
	}
	if _, err := Amounts(ModeAmount, 1000, []int64{400, 500}); err != ErrAmounts {
		t.Errorf("amount sum: %v", err)
	}
	if _, err := Amounts(ModeShares, 1000, []int64{0, 0}); err != ErrShares {
		t.Errorf("zero shares: %v", err)
	}
	if _, err := Amounts(ModeEven, 1000, nil); err != ErrNoParticipants {
		t.Errorf("none: %v", err)
	}
}

func TestSettle(t *testing.T) {
	// Ana paid 90 for the three of them; Bojan paid 30 for the three.
	// Each owes 40: Ana +50, Bojan -10, Ceca -40.
	tr := Settle([]Balance{{MemberID: 1, Balance: 50}, {MemberID: 2, Balance: -10}, {MemberID: 3, Balance: -40}})
	want := []Transfer{{From: 3, To: 1, Amount: 40}, {From: 2, To: 1, Amount: 10}}
	if !reflect.DeepEqual(tr, want) {
		t.Errorf("settle: %+v", tr)
	}
	if got := Settle([]Balance{{MemberID: 1, Balance: 0}}); len(got) != 0 {
		t.Errorf("settled group: %+v", got)
	}
	// Two creditors, two debtors: at most n-1 transfers.
	tr = Settle([]Balance{{MemberID: 1, Balance: 30}, {MemberID: 2, Balance: 20}, {MemberID: 3, Balance: -25}, {MemberID: 4, Balance: -25}})
	if len(tr) != 3 {
		t.Errorf("four-way: %+v", tr)
	}
}
