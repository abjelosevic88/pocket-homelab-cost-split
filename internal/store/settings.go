package store

import "context"

// DefaultTitle is the app title until someone changes it.
const DefaultTitle = "Pocket Split"

// GetSettings loads all settings with defaults applied.
func (s *Store) GetSettings(ctx context.Context) (Settings, error) {
	st := Settings{Title: DefaultTitle, DefaultCurrency: "EUR", FxAuto: true, DateFormat: "auto", TimeFormat: "auto"}
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
		case "date_format":
			if ValidDateFormat(v) {
				st.DateFormat = v
			}
		case "time_format":
			if ValidTimeFormat(v) {
				st.TimeFormat = v
			}
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
		"date_format":      st.DateFormat,
		"time_format":      st.TimeFormat,
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

// personalKeys are the settings each person keeps for themselves.
var personalKeys = []string{"default_currency", "date_format", "time_format"}

// GetUserSettings is GetSettings with userID's own preferences laid over the
// box-wide values.
func (s *Store) GetUserSettings(ctx context.Context, userID int64) (Settings, error) {
	st, err := s.GetSettings(ctx)
	if err != nil {
		return st, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT key, value FROM user_settings WHERE user_id = ?`, userID)
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
		case "default_currency":
			if v != "" {
				st.DefaultCurrency = v
			}
		case "date_format":
			if ValidDateFormat(v) {
				st.DateFormat = v
			}
		case "time_format":
			if ValidTimeFormat(v) {
				st.TimeFormat = v
			}
		}
	}
	return st, rows.Err()
}

// SaveUserSettings stores userID's own preferences (the personal keys only).
func (s *Store) SaveUserSettings(ctx context.Context, userID int64, st Settings) error {
	kv := map[string]string{"default_currency": st.DefaultCurrency, "date_format": st.DateFormat, "time_format": st.TimeFormat}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, k := range personalKeys {
		if _, err := tx.ExecContext(ctx, `INSERT INTO user_settings (user_id, key, value) VALUES (?, ?, ?) ON CONFLICT(user_id, key) DO UPDATE SET value = excluded.value`, userID, k, kv[k]); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SaveBoxSettings stores only the box-wide keys (title, rate lookups).
func (s *Store) SaveBoxSettings(ctx context.Context, title string, fxAuto bool) error {
	cur, err := s.GetSettings(ctx)
	if err != nil {
		return err
	}
	cur.Title, cur.FxAuto = title, fxAuto
	return s.SaveSettings(ctx, cur)
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
