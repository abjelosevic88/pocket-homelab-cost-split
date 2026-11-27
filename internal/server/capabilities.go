package server

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/currency"
	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/split"
	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/store"
)

// What Pocket Search, Pocket Day and Pocket Hub ask Split (the capabilities
// value in links.go): one route each, in the shape every Pocket app answers
// in. They read the groups the person sees in the app, with the same rule as
// every other route (canSeeGroup: the groups they created or are a member
// of), and every expense in those groups, as the group page shows them.
// Nothing here adds an expense or settles up: an expense needs a group, who
// paid and how it is split, which a quick line cannot say.

func groupURL(id int64) string { return "/groups/" + strconv.FormatInt(id, 10) }

func expenseURL(gid, id int64) string {
	return fmt.Sprintf("/groups/%d/expenses/%d", gid, id)
}

// dots joins the parts of a line that are there: "12.50 EUR · paid by Ana · Trip".
func dots(parts ...string) string {
	out := parts[:0:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}

// nameList writes names out: "Ana", "Ana and Marko", "Ana, Marko and Ben",
// "Ana, Marko, Ben and 2 others".
func nameList(names []string) string {
	if len(names) > 4 {
		rest := len(names) - 3
		return strings.Join(names[:3], ", ") + " and " + strconv.Itoa(rest) + " " + plural(rest, "other", "others")
	}
	if n := len(names); n > 1 {
		return strings.Join(names[:n-1], ", ") + " and " + names[n-1]
	}
	return strings.Join(names, "")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// capFirst writes the first letter as a capital: "you owe" → "You owe".
func capFirst(s string) string {
	for i, r := range s {
		return string(unicode.ToUpper(r)) + s[i+len(string(r)):]
	}
	return s
}

// plainDate writes YYYY-MM-DD as "3 Nov 2026".
func plainDate(d string) string {
	t, err := store.ParseDate(d)
	if err != nil {
		return d
	}
	return t.Format("2 Jan 2006")
}

// sums adds up money per currency, in the order the currencies came up.
type sums struct {
	order []string
	by    map[string]int64
}

func (m *sums) add(cur string, v int64) {
	if v == 0 {
		return
	}
	if m.by == nil {
		m.by = map[string]int64{}
	}
	if _, ok := m.by[cur]; !ok {
		m.order = append(m.order, cur)
	}
	m.by[cur] += v
}

// text writes the sums out: "18.00 EUR", "18.00 EUR and 30.00 BAM". Empty
// when nothing was added.
func (m *sums) text(sep, last string) string {
	parts := []string{}
	for _, cur := range m.order {
		if v := m.by[cur]; v != 0 {
			parts = append(parts, FormatMoney(v, cur))
		}
	}
	if n := len(parts); n > 1 {
		return strings.Join(parts[:n-1], sep) + last + parts[n-1]
	}
	return strings.Join(parts, "")
}

// people is who is in one group, by member id, and which member is the
// viewer (0 when they only created the group).
type people struct {
	names map[int64]string
	me    int64
}

func newPeople(members []store.Member, viewer int64) people {
	p := people{names: map[int64]string{}}
	for _, m := range members {
		p.names[m.ID] = m.Name
		if m.UserID != nil && *m.UserID == viewer {
			p.me = m.ID
		}
	}
	return p
}

// name is "you" for the viewer, the member's name for anybody else.
func (p people) name(id int64) string {
	if id != 0 && id == p.me {
		return "you"
	}
	return p.names[id]
}

// groupPeople loads who is in each group once per request.
type groupPeople struct {
	s      *Server
	viewer int64
	by     map[int64]people
}

func (g *groupPeople) of(ctx context.Context, gid int64) (people, error) {
	if p, ok := g.by[gid]; ok {
		return p, nil
	}
	members, err := g.s.store.ListMembers(ctx, gid)
	if err != nil {
		return people{}, err
	}
	p := newPeople(members, g.viewer)
	g.by[gid] = p
	return p, nil
}

// standing says in words where the viewer stands in a group: "you owe Ana
// 12.50 EUR", "Marko owes you 30.00 EUR", "Car fund owes you 20.00 EUR", or
// "all settled". Empty when the viewer is not one of its people. These are
// the payments the Balances page suggests.
func standing(g *store.Group, l *store.Ledger, p people) []string {
	if p.me == 0 {
		return nil
	}
	var out []string
	for _, t := range split.Settle(l.Balances) {
		switch {
		case t.From == p.me:
			out = append(out, "you owe "+p.names[t.To]+" "+FormatMoney(t.Amount, g.Currency))
		case t.To == p.me:
			out = append(out, p.names[t.From]+" owes you "+FormatMoney(t.Amount, g.Currency))
		}
	}
	for _, cc := range l.CostCenters {
		for _, c := range cc.Claims {
			switch {
			case c.MemberID != p.me:
			case c.Amount > 0:
				out = append(out, p.names[cc.MemberID]+" owes you "+FormatMoney(c.Amount, g.Currency))
			default:
				out = append(out, "you owe "+p.names[cc.MemberID]+" "+FormatMoney(-c.Amount, g.Currency))
			}
		}
	}
	if len(out) == 0 {
		out = append(out, "all settled")
	}
	return out
}

// words are the parts of the query with a letter or a digit in them.
func words(q string) []string {
	var out []string
	for _, w := range strings.Fields(q) {
		if strings.IndexFunc(w, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0 {
			out = append(out, w)
		}
	}
	return out
}

// ---------- search ----------

// pocketSearch answers Pocket Search: the groups whose name or description
// has the words (as the groups list's own search finds them), then the
// expenses and payments in the person's groups whose title, notes or
// category has them, newest first. Every word must be found.
func (s *Server) pocketSearch(r *http.Request, q string, limit int) ([]core.SearchItem, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	ws := words(q)
	if len(ws) == 0 {
		return nil, nil
	}
	out := []core.SearchItem{}
	groups, err := s.store.SearchGroups(ctx, ws, limit, uid)
	if err != nil {
		return nil, err
	}
	for _, g := range groups {
		p := newPeople(g.Members, uid)
		var persons []string
		for _, m := range g.Members {
			if !m.IsCostCenter() && m.ID != p.me {
				persons = append(persons, m.Name)
			}
		}
		if p.me != 0 {
			persons = append(persons, "you")
		}
		var where []string
		if p.me != 0 {
			l, err := s.store.Ledger(ctx, g.ID)
			if err != nil {
				return nil, err
			}
			where = standing(g, l, p)
		}
		archived := ""
		if g.Archived {
			archived = "archived"
		}
		out = append(out, core.SearchItem{ID: "group:" + strconv.FormatInt(g.ID, 10), Title: g.Name, Kind: "group", Date: g.UpdatedAt, URL: groupURL(g.ID),
			Snippet: dots(nameList(persons), strings.Join(where, ", "), archived)})
	}
	hits, err := s.store.SearchExpenses(ctx, ws, limit-len(out), uid)
	if err != nil {
		return nil, err
	}
	gp := &groupPeople{s: s, viewer: uid, by: map[int64]people{}}
	for _, e := range hits {
		p, err := gp.of(ctx, e.GroupID)
		if err != nil {
			return nil, err
		}
		kind, who := "expense", "paid by "+p.name(e.PaidBy)
		if e.Reimbursement {
			kind = "settle-up"
			to := int64(0)
			if len(e.Shares) > 0 {
				to = e.Shares[0].MemberID
			}
			who = p.name(e.PaidBy) + " paid " + p.name(to)
			who = capFirst(who)
		}
		out = append(out, core.SearchItem{ID: "expense:" + strconv.FormatInt(e.ID, 10), Title: e.Title, Kind: kind, Date: e.Date, URL: expenseURL(e.GroupID, e.ID),
			Snippet: dots(FormatMoney(e.Amount, e.Currency), who, e.GroupName, plainDate(e.Date))})
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ---------- day ----------

// pocketDay answers Pocket Day: the expenses and payments dated that day in
// the person's groups. Each expense carries the person's own share, in the
// currency it was paid in (nothing is converted); a payment that settles a
// debt is listed without an amount, as it is nobody's spending. Expenses
// carry a plain date, so the time zone is not used.
func (s *Server) pocketDay(r *http.Request, date string, _ *time.Location) (*core.Day, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	hits, err := s.store.ExpensesOn(ctx, uid, date)
	if err != nil {
		return nil, err
	}
	if len(hits) == 0 {
		return nil, nil
	}
	gp := &groupPeople{s: s, viewer: uid, by: map[int64]people{}}
	day := &core.Day{Date: date}
	var share, paid sums
	expenses, payments := 0, 0
	var first string // the summary of a day with one expense
	for _, e := range hits {
		p, err := gp.of(ctx, e.GroupID)
		if err != nil {
			return nil, err
		}
		it := core.DayItem{ID: strconv.FormatInt(e.ID, 10), Title: e.Title, URL: expenseURL(e.GroupID, e.ID), Note: strings.TrimSpace(e.Notes)}
		if e.Reimbursement {
			payments++
			to := int64(0)
			if len(e.Shares) > 0 {
				to = e.Shares[0].MemberID
			}
			it.Kind = "settle-up"
			it.Sub = dots(e.GroupName, capFirst(p.name(e.PaidBy)+" paid "+p.name(to)+" "+FormatMoney(e.Amount, e.Currency)))
			day.Items = append(day.Items, it)
			continue
		}
		expenses++
		it.Kind = "expense"
		var mine int64
		mineIn := false
		var others []string
		for _, sh := range e.Shares {
			if p.me != 0 && sh.MemberID == p.me {
				mine, mineIn = sh.Amount, true
				continue
			}
			others = append(others, p.names[sh.MemberID])
		}
		if mine != 0 {
			it.Amount, it.Currency = mine, e.Currency
			share.add(e.Currency, mine)
		}
		amount := FormatMoney(e.Amount, e.Currency)
		who := "paid by " + p.name(e.PaidBy)
		if p.me != 0 && e.PaidBy == p.me {
			paid.add(e.Currency, e.Amount)
			switch {
			case len(others) > 0 && mineIn:
				who = "you paid " + amount + ", split with " + nameList(others)
			case len(others) > 0:
				who = "you paid " + amount + " for " + nameList(others)
			}
		}
		it.Sub = dots(e.GroupName, who)
		// "Dinner split with Ana and Marko, your share 18.00 EUR"
		switch {
		case mineIn && len(others) > 0:
			first = e.Title + " split with " + nameList(others)
		case p.me != 0 && e.PaidBy == p.me && len(others) > 0:
			first = e.Title + ", you paid " + amount + " for " + nameList(others)
		case !mineIn:
			first = e.Title + ", paid by " + p.name(e.PaidBy)
		default:
			first = e.Title
		}
		if mine != 0 {
			first += ", your share " + FormatMoney(mine, e.Currency)
		}
		day.Items = append(day.Items, it)
	}
	var parts []string
	switch {
	case expenses == 1:
		parts = append(parts, first)
	case expenses > 1:
		line := strconv.Itoa(expenses) + " expenses"
		if t := share.text(", ", " and "); t != "" {
			line += ", your share " + t
		}
		parts = append(parts, line)
	}
	if payments > 0 {
		parts = append(parts, strconv.Itoa(payments)+" "+plural(payments, "settle-up", "settle-ups"))
	}
	day.Summary = capFirst(strings.Join(parts, "; "))
	if t := share.text(" + ", " + "); t != "" {
		day.Metrics = append(day.Metrics, core.Metric{Label: "Your share", Value: t})
	}
	if t := paid.text(" + ", " + "); t != "" {
		day.Metrics = append(day.Metrics, core.Metric{Label: "You paid", Value: t})
	}
	return day, nil
}

// ---------- widget ----------

// pocketWidget answers Pocket Hub with the person's balances, as the home
// page counts them: across the groups that are not archived, who they owe
// and who owes them (the payments each group's Balances page suggests, and
// what its projects owe them back), largest first. Everything is counted
// from the app's own tables; nothing is converted between currencies.
func (s *Server) pocketWidget(r *http.Request) (*core.WidgetData, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	open := false
	groups, err := s.store.ListGroups(ctx, store.GroupFilter{Archived: &open}, uid)
	if err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return nil, nil // no group yet
	}
	type row struct {
		item  core.WidgetItem
		major float64 // to put the largest first, whatever the currency
	}
	var rows []row
	var owe, owed, claims sums
	add := func(g *store.Group, other int64, title string, amount int64) {
		rows = append(rows, row{core.WidgetItem{ID: strconv.FormatInt(other, 10), Title: title, Detail: g.Name, URL: groupURL(g.ID) + "/balances"},
			float64(amount) / pow10(currency.Decimals(g.Currency))})
	}
	for _, g := range groups {
		if g.MyMemberID == 0 {
			continue // created by the person, who is not one of its people
		}
		if g.MyBalance < 0 {
			owe.add(g.Currency, -g.MyBalance)
		} else {
			owed.add(g.Currency, g.MyBalance)
		}
		claims.add(g.Currency, g.MyClaims)
		l, err := s.store.Ledger(ctx, g.ID)
		if err != nil {
			return nil, err
		}
		p := newPeople(g.Members, uid)
		for _, t := range split.Settle(l.Balances) {
			switch {
			case t.From == p.me:
				add(g, t.To, "You owe "+p.names[t.To]+" "+FormatMoney(t.Amount, g.Currency), t.Amount)
			case t.To == p.me:
				add(g, t.From, p.names[t.From]+" owes you "+FormatMoney(t.Amount, g.Currency), t.Amount)
			}
		}
		for _, cc := range l.CostCenters {
			for _, c := range cc.Claims {
				switch {
				case c.MemberID != p.me:
				case c.Amount > 0:
					add(g, cc.MemberID, p.names[cc.MemberID]+" owes you "+FormatMoney(c.Amount, g.Currency), c.Amount)
				default:
					add(g, cc.MemberID, "You owe "+p.names[cc.MemberID]+" "+FormatMoney(-c.Amount, g.Currency), -c.Amount)
				}
			}
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].major > rows[j].major })
	w := &core.WidgetData{URL: "/home", Items: []core.WidgetItem{}}
	for _, r := range rows {
		w.Items = append(w.Items, r.item)
	}
	var parts []string
	if t := owe.text(", ", " and "); t != "" {
		parts = append(parts, "you owe "+t)
		w.Metrics = append(w.Metrics, core.Metric{Label: "You owe", Value: owe.text(" + ", " + ")})
	}
	if t := owed.text(", ", " and "); t != "" {
		parts = append(parts, "you are owed "+t)
		w.Metrics = append(w.Metrics, core.Metric{Label: "You are owed", Value: owed.text(" + ", " + ")})
	}
	// what projects owe the person back is kept apart, as on the home page
	var back, out sums
	for _, cur := range claims.order {
		if v := claims.by[cur]; v > 0 {
			back.add(cur, v)
		} else {
			out.add(cur, -v)
		}
	}
	if t := back.text(", ", " and "); t != "" {
		parts = append(parts, "projects owe you "+t)
	}
	if t := out.text(", ", " and "); t != "" {
		parts = append(parts, "you owe projects "+t)
	}
	w.Line = capFirst(strings.Join(parts, ", "))
	if w.Line == "" {
		w.Line = "All settled"
	}
	return w, nil
}
