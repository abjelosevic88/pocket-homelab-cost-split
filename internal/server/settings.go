package server

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/currency"
	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/store"
)

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.GetSettings(r.Context())
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

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	cur, err := s.store.GetSettings(r.Context())
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
	if err := s.store.SaveSettings(r.Context(), in); err != nil {
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
		"counts": s.store.Counts(r.Context()),
	})
}

// ---------- backup / export / import ----------

func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	dir := filepath.Join(s.cfg.DataDir, "backups")
	name := fmt.Sprintf("pocket-split-%s.db", time.Now().Format("20060102-150405"))
	dst := filepath.Join(dir, name)
	if err := s.store.Backup(r.Context(), dst); err != nil {
		s.fail(w, err, "backup")
		return
	}
	defer os.Remove(dst)
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	http.ServeFile(w, r, dst)
}

// Export is the portable JSON shape of one group (also accepted by /import).
type Export struct {
	Version    string           `json:"version"`
	ExportedAt string           `json:"exported_at"`
	Group      store.Group      `json:"group"`
	Expenses   []*store.Expense `json:"expenses"`
}

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	g := groupFrom(r.Context())
	list, _, err := s.store.ListExpenses(r.Context(), store.ExpenseFilter{GroupID: g.ID})
	if err != nil {
		s.fail(w, err, "export")
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+safeName(g.Name)+`.json"`)
	writeJSON(w, http.StatusOK, Export{Version: Version, ExportedAt: store.Now(), Group: *g, Expenses: list})
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

// handleImportJSON creates a new group from an export file. Members are
// re-created by name (linked to the importing user where the export says so
// for them), every expense is re-saved with its stored rate.
func (s *Server) handleImportJSON(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	var ex Export
	if err := decode(r, &ex); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(ex.Group.Name) == "" || !currency.Valid(ex.Group.Currency) {
		writeErr(w, http.StatusBadRequest, "not a Pocket Split export")
		return
	}
	g := &store.Group{Name: ex.Group.Name + " (imported)", Description: ex.Group.Description, Currency: ex.Group.Currency, CreatedBy: u.ID}
	oldToNew := map[int64]int64{}
	var members []store.Member
	hasMe := false
	for _, m := range ex.Group.Members {
		mm := store.Member{Name: m.Name}
		if m.ID == ex.Group.MyMemberID && !hasMe {
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
	if err := s.store.CreateGroup(r.Context(), g, members); err != nil {
		s.fail(w, err, "import")
		return
	}
	for i, m := range ex.Group.Members {
		if i < len(members) {
			oldToNew[m.ID] = members[i].ID
		}
	}
	created, skipped := 0, 0
	for _, old := range ex.Expenses {
		e := &store.Expense{GroupID: g.ID, CreatedBy: u.ID, Title: old.Title, Amount: old.Amount, Currency: old.Currency, FxRate: old.FxRate, AmountGroup: old.AmountGroup,
			PaidBy: oldToNew[old.PaidBy], Date: old.Date, Category: old.Category, Notes: old.Notes, SplitMode: old.SplitMode, Reimbursement: old.Reimbursement}
		if e.PaidBy == 0 || !validDate(e.Date) || !currency.Valid(e.Currency) || e.Amount == 0 {
			skipped++
			continue
		}
		if !s.store.CategoryExists(r.Context(), e.Category) {
			e.Category = "general"
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
		if !ok || store.ComputeShares(e) != nil || s.store.SaveExpense(r.Context(), e) != nil {
			skipped++
			continue
		}
		created++
	}
	s.store.LogActivity(r.Context(), store.Activity{GroupID: g.ID, UserID: u.ID, Kind: "group.imported", Summary: fmt.Sprintf("%d expenses", created)})
	out, _ := s.store.GetGroup(r.Context(), g.ID, u.ID)
	writeJSON(w, http.StatusOK, map[string]any{"group": out, "created": created, "skipped": skipped})
}
