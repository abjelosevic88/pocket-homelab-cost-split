-- Cost centers. A member is a person (default) or a cost center: a project the
-- people of the group fund up front and that pays them back later. A cost
-- center never carries a personal share and never shows in person-to-person
-- "who owes whom"; it only tracks what it owes back to whoever fronted money
-- for it (see store.Ledger).
ALTER TABLE members ADD COLUMN kind TEXT NOT NULL DEFAULT 'person';

-- A payment from B to A with cost_center_id = C is B buying into C's debt to
-- A: C then owes B that amount instead of A. Nobody owes anybody personally.
ALTER TABLE expenses ADD COLUMN cost_center_id INTEGER REFERENCES members(id) ON DELETE RESTRICT;
CREATE INDEX IF NOT EXISTS expenses_cost_center ON expenses(cost_center_id) WHERE cost_center_id IS NOT NULL;

-- Categories belong to a group. The built-in ones stay shared defaults
-- (group_id NULL); every custom category belongs to exactly one group.
ALTER TABLE categories ADD COLUMN group_id INTEGER REFERENCES groups(id) ON DELETE CASCADE;

-- A custom category used in several groups stays with the first one; every
-- other group gets its own copy (id "<old>-g<group>") and its expenses move
-- to that copy.
CREATE TEMP TABLE cat_moves AS
  SELECT u.category AS old_id, u.group_id AS gid,
         ROW_NUMBER() OVER (PARTITION BY u.category ORDER BY u.group_id) AS rn
  FROM (SELECT DISTINCT category, group_id FROM expenses) u
  JOIN categories c ON c.id = u.category
  WHERE c.builtin = 0;

INSERT INTO categories (id, label, icon, builtin, sort_order, user_id, group_id)
  SELECT m.old_id || '-g' || m.gid, c.label, c.icon, 0, c.sort_order, c.user_id, m.gid
  FROM cat_moves m JOIN categories c ON c.id = m.old_id
  WHERE m.rn > 1;

UPDATE expenses SET category = category || '-g' || group_id
WHERE EXISTS (SELECT 1 FROM cat_moves m WHERE m.old_id = expenses.category AND m.gid = expenses.group_id AND m.rn > 1);

UPDATE categories SET group_id = (SELECT m.gid FROM cat_moves m WHERE m.old_id = categories.id AND m.rn = 1)
WHERE builtin = 0 AND group_id IS NULL;

-- A custom category no expense ever used has no group to go to.
DELETE FROM categories WHERE builtin = 0 AND group_id IS NULL;

DROP TABLE cat_moves;
CREATE INDEX IF NOT EXISTS categories_group ON categories(group_id);
