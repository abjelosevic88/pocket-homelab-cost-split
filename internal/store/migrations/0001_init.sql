CREATE TABLE users (
  id            INTEGER PRIMARY KEY,
  email         TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL DEFAULT '',
  role          TEXT NOT NULL DEFAULT 'admin',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT
);

CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE api_tokens (
  id           INTEGER PRIMARY KEY,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL DEFAULT '',
  token_hash   TEXT NOT NULL UNIQUE,
  prefix       TEXT NOT NULL DEFAULT '',
  last_used_at TEXT,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);

-- A group is one pot of shared costs: a trip, a flat, a dinner. It has a main
-- currency; every balance is counted in it. Expenses may be paid in any
-- currency and are converted at the rate stored on the expense.
CREATE TABLE groups (
  id          INTEGER PRIMARY KEY,
  name        TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  currency    TEXT NOT NULL,                      -- ISO 4217
  created_by  INTEGER REFERENCES users(id) ON DELETE SET NULL,
  archived    INTEGER NOT NULL DEFAULT 0,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- Who takes part. A member may be linked to a signed-in user (then that user
-- sees the group and "you" is resolved) or be just a name for someone without
-- an account. Linking later keeps all their expenses.
CREATE TABLE members (
  id         INTEGER PRIMARY KEY,
  group_id   INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  user_id    INTEGER REFERENCES users(id) ON DELETE SET NULL,
  name       TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX members_group ON members(group_id);
CREATE UNIQUE INDEX members_group_user ON members(group_id, user_id) WHERE user_id IS NOT NULL;

-- One payment made by one member for some of the members. Amounts are integer
-- minor units (cents) of `currency`; amount_group is the same money in the
-- group currency at fx_rate (1 unit of currency = fx_rate units of the group
-- currency), fixed when the expense is saved so balances never drift.
--   split_mode: even | shares | percent | amount
--   reimbursement = 1: a settle-up payment from paid_by to the single sharer;
--                      shown apart and left out of spending totals.
CREATE TABLE expenses (
  id            INTEGER PRIMARY KEY,
  group_id      INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  title         TEXT NOT NULL,
  amount        INTEGER NOT NULL,
  currency      TEXT NOT NULL,
  fx_rate       REAL NOT NULL DEFAULT 1,
  amount_group  INTEGER NOT NULL,
  paid_by       INTEGER NOT NULL REFERENCES members(id) ON DELETE RESTRICT,
  date          TEXT NOT NULL,                    -- YYYY-MM-DD
  category      TEXT NOT NULL DEFAULT 'general',
  notes         TEXT NOT NULL DEFAULT '',
  split_mode    TEXT NOT NULL DEFAULT 'even',
  reimbursement INTEGER NOT NULL DEFAULT 0,
  created_by    INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX expenses_group_date ON expenses(group_id, date DESC, id DESC);
CREATE INDEX expenses_paid_by ON expenses(paid_by);

-- Who an expense was for. share is the input (weight / hundredths of a
-- percent / minor units, by split_mode); amount and amount_group are the
-- resulting parts, which always add up to the expense exactly.
CREATE TABLE expense_shares (
  expense_id   INTEGER NOT NULL REFERENCES expenses(id) ON DELETE CASCADE,
  member_id    INTEGER NOT NULL REFERENCES members(id) ON DELETE RESTRICT,
  share        INTEGER NOT NULL DEFAULT 1,
  amount       INTEGER NOT NULL,
  amount_group INTEGER NOT NULL,
  PRIMARY KEY (expense_id, member_id)
);
CREATE INDEX shares_member ON expense_shares(member_id);

-- What happened in a group, for the activity feed.
CREATE TABLE activity (
  id         INTEGER PRIMARY KEY,
  group_id   INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  user_id    INTEGER REFERENCES users(id) ON DELETE SET NULL,
  kind       TEXT NOT NULL,                       -- expense.added | expense.updated | expense.deleted | member.added | member.removed | group.updated | …
  expense_id INTEGER,
  summary    TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX activity_group ON activity(group_id, id DESC);

-- Exchange rates already looked up, one row per pair and day.
CREATE TABLE fx_cache (
  base       TEXT NOT NULL,
  quote      TEXT NOT NULL,
  date       TEXT NOT NULL,
  rate       REAL NOT NULL,
  source     TEXT NOT NULL DEFAULT '',
  fetched_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY (base, quote, date)
);
