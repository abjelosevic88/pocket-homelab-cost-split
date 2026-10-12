-- Categories became editable: the fixed list moved into a table so people can
-- add their own ("Ski passes") and rename or retire the built-in ones.
-- expenses.category keeps holding the id.
CREATE TABLE categories (
  id         TEXT PRIMARY KEY,
  label      TEXT NOT NULL,
  icon       TEXT NOT NULL DEFAULT '',
  builtin    INTEGER NOT NULL DEFAULT 0,
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
INSERT INTO categories (id, label, icon, builtin, sort_order) VALUES
  ('general', 'General', '🧾', 1, 1),
  ('groceries', 'Groceries', '🛒', 1, 2),
  ('restaurant', 'Restaurants & cafés', '🍽️', 1, 3),
  ('drinks', 'Drinks & nightlife', '🍻', 1, 4),
  ('transport', 'Transport', '🚆', 1, 5),
  ('fuel', 'Fuel & parking', '⛽', 1, 6),
  ('accommodation', 'Accommodation', '🏨', 1, 7),
  ('entertainment', 'Entertainment', '🎟️', 1, 8),
  ('shopping', 'Shopping', '🛍️', 1, 9),
  ('health', 'Health', '💊', 1, 10),
  ('utilities', 'Rent & utilities', '🏠', 1, 11),
  ('gifts', 'Gifts', '🎁', 1, 12),
  ('travel', 'Flights & travel', '✈️', 1, 13),
  ('other', 'Other', '📦', 1, 14);
