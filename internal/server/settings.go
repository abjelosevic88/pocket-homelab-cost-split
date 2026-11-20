package server

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/currency"
	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/store"
)

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.GetUserSettings(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	writeJSON(w, http.StatusOK, s.publicSettings(st))
}

// publicSettings adds what the UI needs to know about the environment.
func (s *Server) publicSettings(st store.Settings) map[string]any {
	return map[string]any{
		"title": st.Title, "default_currency": st.DefaultCurrency, "fx_auto": st.FxAuto,
		"fx_provider": s.cfg.FXProvider, "fx_enabled": s.fx.Enabled() && st.FxAuto,
		"date_format": st.DateFormat, "time_format": st.TimeFormat,
		"setup_complete": st.SetupComplete,
	}
}

// handlePutSettings saves the caller's own preferences (default currency,
// date and time format); an admin's save also sets the box-wide title and
// rate lookups, a member's leaves those alone.
func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	cur, err := s.store.GetUserSettings(r.Context(), u.ID)
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	var in store.Settings
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(in.Title) == "" {
		in.Title = store.DefaultTitle
	}
	in.DefaultCurrency = strings.ToUpper(strings.TrimSpace(in.DefaultCurrency))
	if !currency.Valid(in.DefaultCurrency) {
		writeErr(w, http.StatusBadRequest, "unknown currency "+in.DefaultCurrency)
		return
	}
	if !store.ValidDateFormat(in.DateFormat) {
		in.DateFormat = "auto"
	}
	if !store.ValidTimeFormat(in.TimeFormat) {
		in.TimeFormat = "auto"
	}
	in.SetupComplete = cur.SetupComplete
	if u.Role != "admin" {
		in.Title, in.FxAuto = cur.Title, cur.FxAuto
	} else if err := s.store.SaveBoxSettings(r.Context(), in.Title, in.FxAuto); err != nil {
		s.fail(w, err, "save settings")
		return
	}
	if err := s.store.SaveUserSettings(r.Context(), u.ID, in); err != nil {
		s.fail(w, err, "save settings")
		return
	}
	writeJSON(w, http.StatusOK, s.publicSettings(in))
}

func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	var size int64
	if fi, err := os.Stat(s.cfg.DBPath); err == nil {
		size = fi.Size()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version": Version, "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH,
		"core":      s.coreStatus(),
		"auth_mode": s.cfg.AuthMode, "data_dir": s.cfg.DataDir, "db_path": s.cfg.DBPath, "db_bytes": size,
		"fx_provider": s.cfg.FXProvider,
		"uptime":      time.Since(s.started).Round(time.Second).String(), "timezone": time.Now().Location().String(),
		"counts": s.store.UserCounts(r.Context(), userFrom(r.Context()).ID),
	})
}

// ---------- export / import ----------
//
// There is no whole-database download over HTTP: the file holds everyone's
// groups. Back the data directory up on the host (`pocket-split backup`).

// Export is the portable JSON shape of one group (also accepted by /import).
type Export struct {
	Version    string           `json:"version"`
	ExportedAt string           `json:"exported_at"`
	Group      store.Group      `json:"group"`
	Expenses   []*store.Expense `json:"expenses"`
	// Categories are the group's own (the built-in ones exist everywhere).
	Categories []store.Category `json:"categories"`
}

// ExportAll is every group one person is in (GET /export.json); /import
// takes it as well as a single group's Export.
type ExportAll struct {
	App        string   `json:"app"` // "pocket-split"
	Version    string   `json:"version"`
	ExportedAt string   `json:"exported_at"`
	Groups     []Export `json:"groups"`
}

func (s *Server) groupExport(ctx context.Context, g *store.Group, viewer int64) (Export, error) {
	list, _, err := s.store.ListExpenses(ctx, store.ExpenseFilter{GroupID: g.ID})
	if err != nil {
		return Export{}, err
	}
	cats, err := s.store.ListCategories(ctx, viewer, g.ID)
	if err != nil {
		return Export{}, err
	}
	own := []store.Category{}
	for _, c := range cats {
		if !c.Builtin {
			own = append(own, c)
		}
	}
	return Export{Version: Version, ExportedAt: store.Now(), Group: *g, Expenses: list, Categories: own}, nil
}

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	g := groupFrom(r.Context())
	ex, err := s.groupExport(r.Context(), g, userFrom(r.Context()).ID)
	if err != nil {
		s.fail(w, err, "export")
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+safeName(g.Name)+`.json"`)
	writeJSON(w, http.StatusOK, ex)
}

// handleExportAll is every group the person is in (made or a member of),
// archived ones too, each as its Export: the whole of their Split.
func (s *Server) handleExportAll(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	groups, err := s.store.ListGroups(ctx, store.GroupFilter{}, u.ID)
	if err != nil {
		s.fail(w, err, "export")
		return
	}
	out := ExportAll{App: "pocket-split", Version: Version, ExportedAt: store.Now(), Groups: []Export{}}
	for _, g := range groups {
		ex, err := s.groupExport(ctx, g, u.ID)
		if err != nil {
			s.fail(w, err, "export")
			return
		}
		out.Groups = append(out.Groups, ex)
	}
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-split-`+time.Now().Format("2006-01-02")+`.json"`)
	writeJSON(w, http.StatusOK, out)
}

func safeName(n string) string {
	n = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		if r == ' ' {
			return '-'
		}
		return -1
	}, n)
	if n == "" {
		n = "group"
	}
	return "pocket-split-" + strings.ToLower(n)
}

// handleExportCSV writes one row per expense, with one column per member
// holding their part in the group currency.
func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	g := groupFrom(r.Context())
	list, _, err := s.store.ListExpenses(r.Context(), store.ExpenseFilter{GroupID: g.ID})
	if err != nil {
		s.fail(w, err, "export")
		return
	}
	names := map[int64]string{}
	for _, m := range g.Members {
		names[m.ID] = m.Name
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+safeName(g.Name)+`.csv"`)
	cw := csv.NewWriter(w)
	head := []string{"date", "title", "category", "paid_by", "amount", "currency", "fx_rate", "amount_" + strings.ToLower(g.Currency), "split_mode", "reimbursement", "notes"}
	for _, m := range g.Members {
		head = append(head, m.Name)
	}
	_ = cw.Write(head)
	major := func(minor int64, code string) string {
		d := currency.Decimals(code)
		return strconv.FormatFloat(float64(minor)/pow10(d), 'f', d, 64)
	}
	for _, e := range list {
		row := []string{e.Date, e.Title, e.Category, names[e.PaidBy], major(e.Amount, e.Currency), e.Currency, strconv.FormatFloat(e.FxRate, 'f', -1, 64),
			major(e.AmountGroup, g.Currency), e.SplitMode, map[bool]string{true: "yes", false: ""}[e.Reimbursement], e.Notes}
		for _, m := range g.Members {
			v := ""
			for _, sh := range e.Shares {
				if sh.MemberID == m.ID {
					v = major(sh.AmountGroup, g.Currency)
				}
			}
			row = append(row, v)
		}
		_ = cw.Write(row)
	}
	cw.Flush()
}

func pow10(n int) float64 {
	p := 1.0
	for i := 0; i < n; i++ {
		p *= 10
	}
	return p
}

// handleImportJSON brings groups in from an export file: one group's Export
// (a group's own download) or ExportAll (/export.json, Pocket Move). Each
// group becomes a new group of the person's; members are re-created by name
// (only the person's own member is linked to them: nobody else gets an
// account), every expense is re-saved with its stored rate. A group already
// here (the same group on this box, or one brought in before: same start
// time and name) is skipped, so a second run adds nothing.
func (s *Server) handleImportJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	body, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, 64<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "the file is too big")
		return
	}
	var all ExportAll
	if err := json.Unmarshal(body, &all); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if all.Groups == nil {
		var ex Export
		if err := json.Unmarshal(body, &ex); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		all.Groups = []Export{ex}
	}
	for _, ex := range all.Groups {
		if strings.TrimSpace(ex.Group.Name) == "" || !currency.Valid(ex.Group.Currency) {
			writeErr(w, http.StatusBadRequest, "not a Pocket Split export")
			return
		}
	}
	var first *store.Group
	groups, already, created, skipped := 0, 0, 0, 0
	for _, ex := range all.Groups {
		g, c, sk, have, err := s.importGroup(ctx, u, ex)
		if err != nil {
			s.fail(w, err, "import")
			return
		}
		if first == nil {
			first = g
		}
		if have {
			already++
			continue
		}
		groups++
		created += c
		skipped += sk
	}
	writeJSON(w, http.StatusOK, map[string]any{"group": first, "groups": groups, "already": already, "created": created, "skipped": skipped})
}

// sameGroup is the person's group that ex came from or was imported as
// before (same start time; same name, or the name with " (imported)").
func (s *Server) sameGroup(ctx context.Context, viewer int64, ex Export) *store.Group {
	if ex.Group.CreatedAt == "" {
		return nil
	}
	mine, _ := s.store.ListGroups(ctx, store.GroupFilter{}, viewer)
	for _, g := range mine {
		if g.CreatedAt == ex.Group.CreatedAt && (g.Name == ex.Group.Name || g.Name == ex.Group.Name+" (imported)") {
			return g
		}
	}
	return nil
}

// importGroup makes one group from an Export. have is true (and nothing is
// made) when the group is here already.
func (s *Server) importGroup(ctx context.Context, u *store.User, ex Export) (out *store.Group, created, skipped int, have bool, err error) {
	if g := s.sameGroup(ctx, u.ID, ex); g != nil {
		return g, 0, 0, true, nil
	}
	g := &store.Group{Name: ex.Group.Name + " (imported)", Description: ex.Group.Description, Currency: ex.Group.Currency, CreatedBy: u.ID, CreatedAt: ex.Group.CreatedAt}
	oldToNew := map[int64]int64{}
	var members []store.Member
	hasMe := false
	for _, m := range ex.Group.Members {
		mm := store.Member{Name: m.Name, Kind: store.KindPerson}
		if m.Kind == store.KindCostCenter {
			mm.Kind = store.KindCostCenter
		} else if m.ID == ex.Group.MyMemberID && !hasMe {
			uid := u.ID
			mm.UserID = &uid
			hasMe = true
		}
		members = append(members, mm)
	}
	if !hasMe {
		uid := u.ID
		members = append(members, store.Member{Name: displayName(u), UserID: &uid})
	}
	if err := s.store.CreateGroup(ctx, g, members); err != nil {
		return nil, 0, 0, false, err
	}
	for i, m := range ex.Group.Members {
		if i < len(members) {
			oldToNew[m.ID] = members[i].ID
		}
	}
	// The group's own categories come along under fresh ids.
	catIDs := map[string]string{}
	for _, c := range ex.Categories {
		if c.Builtin || strings.TrimSpace(c.Label) == "" {
			continue
		}
		if nc, err := s.store.CreateCategory(ctx, u.ID, g.ID, c.Label, c.Icon); err == nil {
			catIDs[c.ID] = nc.ID
		}
	}
	for _, old := range ex.Expenses {
		e := &store.Expense{GroupID: g.ID, CreatedBy: u.ID, Title: old.Title, Amount: old.Amount, Currency: old.Currency, FxRate: old.FxRate, AmountGroup: old.AmountGroup,
			PaidBy: oldToNew[old.PaidBy], Date: old.Date, Category: old.Category, Notes: old.Notes, SplitMode: old.SplitMode, Reimbursement: old.Reimbursement,
			CostCenterID: oldToNew[old.CostCenterID]}
		if id, ok := catIDs[e.Category]; ok {
			e.Category = id
		}
		if old.CostCenterID != 0 && e.CostCenterID == 0 {
			skipped++
			continue
		}
		if e.PaidBy == 0 || !validDate(e.Date) || !currency.Valid(e.Currency) || e.Amount == 0 {
			skipped++
			continue
		}
		if e.Reimbursement {
			e.Category = ""
		} else if !s.store.CategoryExists(ctx, e.Category, g.ID) {
			e.Category = s.store.FallbackCategory(ctx)
		}
		ok := true
		for _, sh := range old.Shares {
			nid := oldToNew[sh.MemberID]
			if nid == 0 {
				ok = false
				break
			}
			e.Shares = append(e.Shares, store.Share{MemberID: nid, Share: sh.Share})
		}
		if !ok || store.ComputeShares(e) != nil || s.store.SaveExpense(ctx, e) != nil {
			skipped++
			continue
		}
		created++
	}
	s.store.LogActivity(ctx, store.Activity{GroupID: g.ID, UserID: u.ID, Kind: "group.imported", Summary: fmt.Sprintf("%d expenses", created)})
	out, err = s.store.GetGroup(ctx, g.ID, u.ID)
	return out, created, skipped, false, err
}
