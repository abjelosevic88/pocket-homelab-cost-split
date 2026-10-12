// Package report builds a filtered summary of a group's spending (what the
// Reports page shows) and renders the same thing as a PDF.
package report

import (
	"bytes"
	"embed"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"

	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/currency"
	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/split"
	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/store"
)

//go:embed fonts/DejaVuSans.ttf fonts/DejaVuSans-Bold.ttf
var fontFS embed.FS

// Filter describes what went into the report (shown on the page and the PDF).
type Filter struct {
	From           string   `json:"from"`
	To             string   `json:"to"`
	Categories     []string `json:"categories"`
	MemberID       int64    `json:"member_id"`
	Reimbursements string   `json:"reimbursements"` // exclude | include
	Debts          bool     `json:"debts"`          // show who owes whom
}

// Report is the result.
type Report struct {
	Group       GroupInfo     `json:"group"`
	Filter      Filter        `json:"filter"`
	FilterText  string        `json:"filter_text"`
	GeneratedAt string        `json:"generated_at"`
	GeneratedBy string        `json:"generated_by"`
	Total       int64         `json:"total"`      // spent, reimbursements excluded
	Reimbursed  int64         `json:"reimbursed"` // settle-up payments in the selection
	Count       int           `json:"count"`      // expenses (not reimbursements)
	PerHead     int64         `json:"per_head"`   // total / people in the selection
	FirstDate   string        `json:"first_date"`
	LastDate    string        `json:"last_date"`
	Members     []MemberRow   `json:"members"`
	Categories  []CategoryRow `json:"categories"`
	Months      []MonthRow    `json:"months"`
	Expenses    []ExpenseRow  `json:"expenses"`
	Currencies  []CurrencyRow `json:"currencies"`
	// Payments are the settle-up payments in the selection (only when the
	// filter includes them); Expenses holds the shared costs only.
	Payments []ExpenseRow `json:"payments"`
	// Settlements are the group's real open debts as of now (every expense and
	// payment, regardless of the filter).
	Settlements []Debt           `json:"settlements"`
	Filtered    bool             `json:"filtered"`
	MemberNames map[int64]string `json:"-"`
}

// Debt is "From owes To Amount".
type Debt struct {
	From     int64  `json:"from"`
	To       int64  `json:"to"`
	FromName string `json:"from_name"`
	ToName   string `json:"to_name"`
	Amount   int64  `json:"amount"`
}

// GroupInfo is the bit of the group the report needs.
type GroupInfo struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Currency string `json:"currency"`
}

// MemberRow is one person's totals within the selection.
type MemberRow struct {
	MemberID int64  `json:"member_id"`
	Name     string `json:"name"`
	Paid     int64  `json:"paid"`
	Share    int64  `json:"share"`
	Net      int64  `json:"net"`
	Count    int    `json:"count"`
}

// CategoryRow is one category's totals.
type CategoryRow struct {
	ID      string  `json:"id"`
	Label   string  `json:"label"`
	Icon    string  `json:"icon"`
	Total   int64   `json:"total"`
	Count   int     `json:"count"`
	Percent float64 `json:"percent"`
}

// MonthRow is one month's total.
type MonthRow struct {
	Month string `json:"month"`
	Total int64  `json:"total"`
	Count int    `json:"count"`
}

// CurrencyRow is what was paid in one currency.
type CurrencyRow struct {
	Currency string `json:"currency"`
	Total    int64  `json:"total"`
	Count    int    `json:"count"`
}

// ExpenseRow is one line of the expense table.
type ExpenseRow struct {
	ID            int64  `json:"id"`
	Date          string `json:"date"`
	Title         string `json:"title"`
	Category      string `json:"category"`
	CategoryLabel string `json:"category_label"`
	PaidBy        string `json:"paid_by"`
	Amount        int64  `json:"amount"`
	Currency      string `json:"currency"`
	AmountGroup   int64  `json:"amount_group"`
	Reimbursement bool   `json:"reimbursement"`
	For           string `json:"for"` // "everyone" / "Ana, Marko"
	Notes         string `json:"notes"`
}

// Build aggregates the already-filtered expenses. balances are the group's
// current balances over everything (for the "who owes whom" section).
func Build(g *store.Group, cats []store.Category, exps []*store.Expense, balances []split.Balance, f Filter, by string) *Report {
	r := &Report{Group: GroupInfo{ID: g.ID, Name: g.Name, Currency: g.Currency}, Filter: f, GeneratedAt: time.Now().Format("2006-01-02 15:04"), GeneratedBy: by,
		Members: []MemberRow{}, Categories: []CategoryRow{}, Months: []MonthRow{}, Expenses: []ExpenseRow{}, Currencies: []CurrencyRow{}, Payments: []ExpenseRow{}, Settlements: []Debt{}, MemberNames: map[int64]string{}}
	r.Filtered = f.From != "" || f.To != "" || len(f.Categories) > 0 || f.MemberID != 0
	catLabel, catIcon := map[string]string{}, map[string]string{}
	for _, c := range cats {
		catLabel[c.ID], catIcon[c.ID] = c.Label, c.Icon
	}
	memRows := map[int64]*MemberRow{}
	for _, m := range g.Members {
		r.MemberNames[m.ID] = m.Name
		memRows[m.ID] = &MemberRow{MemberID: m.ID, Name: m.Name}
	}
	catRows, monthRows, curRows := map[string]*CategoryRow{}, map[string]*MonthRow{}, map[string]*CurrencyRow{}
	for _, e := range exps {
		if r.FirstDate == "" || e.Date < r.FirstDate {
			r.FirstDate = e.Date
		}
		if e.Date > r.LastDate {
			r.LastDate = e.Date
		}
		names := make([]string, 0, len(e.Shares))
		for _, sh := range e.Shares {
			names = append(names, r.MemberNames[sh.MemberID])
		}
		forText := strings.Join(names, ", ")
		if len(e.Shares) == len(g.Members) && len(g.Members) > 1 {
			forText = "everyone"
		}
		row := ExpenseRow{ID: e.ID, Date: e.Date, Title: e.Title, Category: e.Category, CategoryLabel: catLabel[e.Category], PaidBy: r.MemberNames[e.PaidBy], Amount: e.Amount, Currency: e.Currency, AmountGroup: e.AmountGroup, Reimbursement: e.Reimbursement, For: forText, Notes: e.Notes}
		if e.Reimbursement {
			if e.Title == "Reimbursement" {
				row.Title = ""
			}
			r.Payments = append(r.Payments, row)
		} else {
			r.Expenses = append(r.Expenses, row)
		}
		if e.Reimbursement {
			r.Reimbursed += e.AmountGroup
			if m := memRows[e.PaidBy]; m != nil {
				m.Paid += e.AmountGroup
				m.Net += e.AmountGroup
			}
			for _, sh := range e.Shares {
				if m := memRows[sh.MemberID]; m != nil {
					m.Share += sh.AmountGroup
					m.Net -= sh.AmountGroup
				}
			}
			continue
		}
		r.Total += e.AmountGroup
		r.Count++
		if m := memRows[e.PaidBy]; m != nil {
			m.Paid += e.AmountGroup
			m.Net += e.AmountGroup
			m.Count++
		}
		for _, sh := range e.Shares {
			if m := memRows[sh.MemberID]; m != nil {
				m.Share += sh.AmountGroup
				m.Net -= sh.AmountGroup
			}
		}
		c := catRows[e.Category]
		if c == nil {
			c = &CategoryRow{ID: e.Category, Label: catLabel[e.Category], Icon: catIcon[e.Category]}
			if c.Label == "" {
				c.Label = e.Category
			}
			catRows[e.Category] = c
		}
		c.Total += e.AmountGroup
		c.Count++
		mo := e.Date[:7]
		mr := monthRows[mo]
		if mr == nil {
			mr = &MonthRow{Month: mo}
			monthRows[mo] = mr
		}
		mr.Total += e.AmountGroup
		mr.Count++
		cr := curRows[e.Currency]
		if cr == nil {
			cr = &CurrencyRow{Currency: e.Currency}
			curRows[e.Currency] = cr
		}
		cr.Total += e.Amount
		cr.Count++
	}
	for _, m := range g.Members {
		r.Members = append(r.Members, *memRows[m.ID])
	}
	for _, c := range catRows {
		if r.Total != 0 {
			c.Percent = float64(c.Total) * 100 / float64(r.Total)
		}
		r.Categories = append(r.Categories, *c)
	}
	sort.Slice(r.Categories, func(i, j int) bool { return r.Categories[i].Total > r.Categories[j].Total })
	for _, m := range monthRows {
		r.Months = append(r.Months, *m)
	}
	sort.Slice(r.Months, func(i, j int) bool { return r.Months[i].Month < r.Months[j].Month })
	for _, c := range curRows {
		r.Currencies = append(r.Currencies, *c)
	}
	sort.Slice(r.Currencies, func(i, j int) bool { return r.Currencies[i].Count > r.Currencies[j].Count })
	if n := len(g.Members); n > 0 {
		r.PerHead = r.Total / int64(n)
	}
	r.FilterText = describe(f, r, catLabel)
	r.Settlements = r.debts(split.Settle(balances))
	return r
}

func (r *Report) debts(ts []split.Transfer) []Debt {
	out := make([]Debt, 0, len(ts))
	for _, t := range ts {
		out = append(out, Debt{From: t.From, To: t.To, FromName: r.MemberNames[t.From], ToName: r.MemberNames[t.To], Amount: t.Amount})
	}
	return out
}

func describe(f Filter, r *Report, catLabel map[string]string) string {
	var parts []string
	switch {
	case f.From != "" && f.To != "":
		parts = append(parts, f.From+" – "+f.To)
	case f.From != "":
		parts = append(parts, "from "+f.From)
	case f.To != "":
		parts = append(parts, "until "+f.To)
	default:
		parts = append(parts, "all time")
	}
	if len(f.Categories) > 0 {
		var ls []string
		for _, c := range f.Categories {
			if l := catLabel[c]; l != "" {
				ls = append(ls, l)
			} else {
				ls = append(ls, c)
			}
		}
		parts = append(parts, strings.Join(ls, ", "))
	}
	if f.MemberID != 0 {
		parts = append(parts, "involving "+r.MemberNames[f.MemberID])
	}
	if f.Reimbursements == "include" {
		parts = append(parts, "payments included")
	}
	return strings.Join(parts, " · ")
}

// Money formats minor units as "1,234.50 EUR".
func Money(minor int64, code string) string {
	d := currency.Decimals(code)
	neg := minor < 0
	if neg {
		minor = -minor
	}
	p := int64(1)
	for i := 0; i < d; i++ {
		p *= 10
	}
	whole := fmt.Sprint(minor / p)
	// thousands separators
	var b strings.Builder
	for i, c := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	s := b.String()
	if d > 0 {
		s += fmt.Sprintf(".%0*d", d, minor%p)
	}
	if neg {
		s = "-" + s
	}
	return s + " " + code
}

// PDF renders the report (A4, portrait).
func PDF(r *Report, appTitle string) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	reg, _ := fontFS.ReadFile("fonts/DejaVuSans.ttf")
	bold, _ := fontFS.ReadFile("fonts/DejaVuSans-Bold.ttf")
	pdf.AddUTF8FontFromBytes("dv", "", reg)
	pdf.AddUTF8FontFromBytes("dv", "B", bold)
	pdf.SetMargins(14, 16, 14)
	pdf.SetAutoPageBreak(true, 18)
	pdf.SetFooterFunc(func() {
		pdf.SetY(-12)
		pdf.SetFont("dv", "", 7.5)
		pdf.SetTextColor(130, 130, 130)
		pdf.CellFormat(0, 5, fmt.Sprintf("%s · %s · generated %s", appTitle, r.Group.Name, r.GeneratedAt), "", 0, "L", false, 0, "")
		pdf.CellFormat(0, 5, fmt.Sprintf("Page %d of {nb}", pdf.PageNo()), "", 0, "R", false, 0, "")
	})
	pdf.AliasNbPages("")
	pdf.AddPage()
	w, _ := pdf.GetPageSize()
	usable := w - 28
	cur := r.Group.Currency
	accent := [3]int{13, 148, 136}

	// Title
	pdf.SetTextColor(17, 24, 39)
	pdf.SetFont("dv", "B", 18)
	pdf.CellFormat(0, 9, r.Group.Name, "", 1, "L", false, 0, "")
	pdf.SetFont("dv", "", 10)
	pdf.SetTextColor(107, 114, 128)
	pdf.CellFormat(0, 6, "Expense report · "+r.FilterText, "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 5, fmt.Sprintf("All amounts in %s at the rate saved on each expense. Generated %s by %s.", cur, r.GeneratedAt, r.GeneratedBy), "", 1, "L", false, 0, "")
	pdf.Ln(4)

	// Summary tiles
	tiles := []struct{ l, v string }{
		{"Total spent", Money(r.Total, cur)},
		{"Expenses", fmt.Sprint(r.Count)},
		{"Per person", Money(r.PerHead, cur)},
		{"Payments", Money(r.Reimbursed, cur)},
	}
	tw := (usable - 3*4) / 4
	x0 := pdf.GetX()
	y0 := pdf.GetY()
	for i, t := range tiles {
		x := x0 + float64(i)*(tw+4)
		pdf.SetFillColor(241, 243, 246)
		pdf.Rect(x, y0, tw, 16, "F")
		pdf.SetXY(x+3, y0+2.5)
		pdf.SetFont("dv", "", 7.5)
		pdf.SetTextColor(107, 114, 128)
		pdf.CellFormat(tw-6, 4, t.l, "", 2, "L", false, 0, "")
		pdf.SetFont("dv", "B", 11.5)
		pdf.SetTextColor(17, 24, 39)
		pdf.CellFormat(tw-6, 7, t.v, "", 0, "L", false, 0, "")
	}
	pdf.SetXY(x0, y0+22)

	header := func(title string) {
		pdf.SetFont("dv", "B", 11)
		pdf.SetTextColor(accent[0], accent[1], accent[2])
		pdf.CellFormat(0, 7, title, "", 1, "L", false, 0, "")
		pdf.SetTextColor(17, 24, 39)
	}
	table := func(cols []string, widths []float64, align []string, rows [][]string, boldLast bool) {
		th := func() {
			pdf.SetFont("dv", "B", 8)
			pdf.SetFillColor(241, 243, 246)
			pdf.SetTextColor(107, 114, 128)
			for i, c := range cols {
				pdf.CellFormat(widths[i], 6, c, "", 0, align[i], true, 0, "")
			}
			pdf.Ln(-1)
			pdf.SetTextColor(17, 24, 39)
		}
		th()
		pdf.SetFont("dv", "", 8.5)
		for ri, row := range rows {
			if pdf.GetY() > 270 {
				pdf.AddPage()
				th()
				pdf.SetFont("dv", "", 8.5)
			}
			if boldLast && ri == len(rows)-1 {
				pdf.SetFont("dv", "B", 8.5)
			}
			pdf.SetDrawColor(227, 230, 235)
			for i, c := range row {
				pdf.CellFormat(widths[i], 6, trunc(pdf, c, widths[i]-2), "B", 0, align[i], false, 0, "")
			}
			pdf.Ln(-1)
		}
		pdf.SetFont("dv", "", 8.5)
		pdf.Ln(4)
	}

	// Who owes whom: the point of the whole thing, so it goes first and big.
	owes := func(title, note string, debts []Debt) {
		header(title)
		pdf.SetFont("dv", "", 8)
		pdf.SetTextColor(107, 114, 128)
		pdf.MultiCell(0, 4.2, note, "", "L", false)
		pdf.Ln(1.5)
		if len(debts) == 0 {
			pdf.SetFont("dv", "B", 10)
			pdf.SetTextColor(21, 128, 61)
			pdf.CellFormat(0, 8, "All settled — nobody owes anybody.", "", 1, "L", false, 0, "")
			pdf.SetTextColor(17, 24, 39)
			pdf.Ln(3)
			return
		}
		for _, d := range debts {
			y := pdf.GetY()
			pdf.SetFillColor(230, 247, 244)
			pdf.Rect(14, y, usable, 10, "F")
			pdf.SetFillColor(accent[0], accent[1], accent[2])
			pdf.Rect(14, y, 1.5, 10, "F")
			pdf.SetXY(18, y+1)
			pdf.SetFont("dv", "B", 11)
			pdf.SetTextColor(17, 24, 39)
			pdf.CellFormat(usable-70, 8, d.FromName, "", 0, "L", false, 0, "")
			pdf.SetXY(18, y+1)
			pdf.CellFormat(usable-70, 8, "", "", 0, "L", false, 0, "")
			pdf.SetXY(18+pdf.GetStringWidth(d.FromName)+2, y+1)
			pdf.SetFont("dv", "", 10)
			pdf.SetTextColor(107, 114, 128)
			pdf.CellFormat(12, 8, "owes", "", 0, "L", false, 0, "")
			pdf.SetFont("dv", "B", 11)
			pdf.SetTextColor(17, 24, 39)
			pdf.CellFormat(usable-100, 8, d.ToName, "", 0, "L", false, 0, "")
			pdf.SetXY(14+usable-66, y+1)
			pdf.SetFont("dv", "B", 12)
			pdf.SetTextColor(accent[0], accent[1], accent[2])
			pdf.CellFormat(62, 8, Money(d.Amount, cur), "", 0, "R", false, 0, "")
			pdf.SetTextColor(17, 24, 39)
			pdf.SetXY(14, y+12)
		}
		pdf.Ln(3)
	}
	if r.Filter.Debts {
		owes("Who owes whom", fmt.Sprintf("Open debts in the group as of %s, counting every expense and every payment made so far. Paying these amounts settles everything with the fewest transfers.", r.GeneratedAt), r.Settlements)
	}

	// Per person
	header("Per person")
	var rows [][]string
	var tp, ts int64
	for _, m := range r.Members {
		rows = append(rows, []string{m.Name, fmt.Sprint(m.Count), Money(m.Paid, cur), Money(m.Share, cur), Money(m.Net, cur)})
		tp += m.Paid
		ts += m.Share
	}
	rows = append(rows, []string{"Total", fmt.Sprint(r.Count), Money(tp, cur), Money(ts, cur), Money(tp-ts, cur)})
	table([]string{"Person", "Paid for", "Paid", "Share", "Net"}, []float64{usable - 130, 25, 35, 35, 35}, []string{"L", "R", "R", "R", "R"}, rows, true)

	// By category
	if len(r.Categories) > 0 {
		header("By category")
		rows = nil
		for _, c := range r.Categories {
			rows = append(rows, []string{c.Label, fmt.Sprint(c.Count), Money(c.Total, cur), fmt.Sprintf("%.1f %%", c.Percent)})
		}
		table([]string{"Category", "Expenses", "Total", "Share"}, []float64{usable - 95, 25, 40, 30}, []string{"L", "R", "R", "R"}, rows, false)
	}

	// By month (only when the selection spans more than one)
	if len(r.Months) > 1 {
		header("By month")
		rows = nil
		for _, m := range r.Months {
			t, _ := time.Parse("2006-01", m.Month)
			rows = append(rows, []string{t.Format("January 2006"), fmt.Sprint(m.Count), Money(m.Total, cur)})
		}
		table([]string{"Month", "Expenses", "Total"}, []float64{usable - 65, 25, 40}, []string{"L", "R", "R"}, rows, false)
	}

	// Expenses
	if len(r.Expenses) > 0 {
		header(fmt.Sprintf("Expenses (%d)", len(r.Expenses)))
		rows = nil
		amtW := 30.0
		for _, e := range r.Expenses {
			amt := Money(e.AmountGroup, cur)
			if e.Currency != cur {
				amt = Money(e.Amount, e.Currency) + " = " + amt
				amtW = 46
			}
			title := e.Title
			if e.Notes != "" {
				title += " — " + e.Notes
			}
			rows = append(rows, []string{e.Date, title, e.CategoryLabel, e.PaidBy, e.For, amt})
		}
		table([]string{"Date", "Expense", "Category", "Paid by", "For", "Amount"}, []float64{22, usable - 22 - 26 - 19 - 24 - amtW, 26, 19, 24, amtW}, []string{"L", "L", "L", "L", "L", "R"}, rows, false)
	}

	// Payments, apart from the shared costs
	if len(r.Payments) > 0 {
		header(fmt.Sprintf("Payments (%d)", len(r.Payments)))
		pdf.SetFont("dv", "", 8)
		pdf.SetTextColor(107, 114, 128)
		pdf.MultiCell(0, 4.2, "Money handed from one person to another to settle up. Not counted as spending.", "", "L", false)
		pdf.Ln(1.5)
		rows = nil
		amtW := 30.0
		for _, e := range r.Payments {
			amt := Money(e.AmountGroup, cur)
			if e.Currency != cur {
				amt = Money(e.Amount, e.Currency) + " = " + amt
				amtW = 46
			}
			note := strings.TrimSpace(strings.Join([]string{e.Title, e.Notes}, " "))
			rows = append(rows, []string{e.Date, e.PaidBy, e.For, e.CategoryLabel, note, amt})
		}
		table([]string{"Date", "From", "To", "Category", "Note", "Amount"}, []float64{22, 30, 30, 30, usable - 22 - 30 - 30 - 30 - amtW, amtW}, []string{"L", "L", "L", "L", "L", "R"}, rows, false)
	}

	// Currencies
	if len(r.Currencies) > 1 {
		header("Currencies used")
		rows = nil
		for _, c := range r.Currencies {
			rows = append(rows, []string{c.Currency, fmt.Sprint(c.Count), Money(c.Total, c.Currency)})
		}
		table([]string{"Currency", "Expenses", "Paid"}, []float64{usable - 65, 25, 40}, []string{"L", "R", "R"}, rows, false)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// trunc shortens s with an ellipsis so it fits width mm in the current font.
func trunc(pdf *fpdf.Fpdf, s string, width float64) string {
	if pdf.GetStringWidth(s) <= width {
		return s
	}
	rs := []rune(s)
	for len(rs) > 1 {
		rs = rs[:len(rs)-1]
		if pdf.GetStringWidth(string(rs)+"…") <= width {
			return string(rs) + "…"
		}
	}
	return "…"
}
