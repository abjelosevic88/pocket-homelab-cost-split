package store

import "context"

// DefaultTitle is the app title until someone changes it.
const DefaultTitle = "Pocket Split"

// GetSettings loads all settings with defaults applied.
func (s *Store) GetSettings(ctx context.Context) (Settings, error) {
	st := Settings{Title: DefaultTitle, DefaultCurrency: "EUR", FxAuto: true}
	rows, err := s.DB.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return st, err
		}
		switch k {
		case "title":
			if v != "" {
				st.Title = v
			}
		case "default_currency":
			if v != "" {
				st.DefaultCurrency = v
			}
		case "fx_auto":
			st.FxAuto = v != "0"
		case "setup_complete":
			st.SetupComplete = v == "1"
		}
	}
	return st, rows.Err()
}

// SaveSettings persists all settings.
func (s *Store) SaveSettings(ctx context.Context, st Settings) error {
	b := map[bool]string{true: "1", false: "0"}
	kv := map[string]string{
		"title":            st.Title,
		"default_currency": st.DefaultCurrency,
		"fx_auto":          b[st.FxAuto],
		"setup_complete":   b[st.SetupComplete],
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for k, v := range kv {
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// GetFxRate returns a cached rate for the pair and day, if any.
func (s *Store) GetFxRate(ctx context.Context, base, quote, date string) (rate float64, source, fetchedAt string, err error) {
	err = notFound(s.DB.QueryRowContext(ctx, `SELECT rate, source, fetched_at FROM fx_cache WHERE base = ? AND quote = ? AND date = ?`, base, quote, date).Scan(&rate, &source, &fetchedAt))
	return
}

// PutFxRate stores a rate for the pair and day.
func (s *Store) PutFxRate(ctx context.Context, base, quote, date string, rate float64, source string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO fx_cache (base, quote, date, rate, source, fetched_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(base, quote, date) DO UPDATE SET rate = excluded.rate, source = excluded.source, fetched_at = excluded.fetched_at`, base, quote, date, rate, source, Now())
	return err
}
