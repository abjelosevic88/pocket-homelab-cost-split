-- Strictly per person. A group belongs to the user who created it
-- (groups.created_by is the owner column) and is shared only with the users
-- linked to one of its members. Members, expenses, shares and activity hang
-- off the group and are only ever read through it. Admins no longer see every
-- group. Custom categories and personal preferences get an owner too.
--
-- Everything that exists so far was made by the owner: the first admin.
-- Every statement below is safe to run again.

-- Groups without a creator go to the owner.
UPDATE groups SET created_by = (SELECT id FROM users WHERE role = 'admin' ORDER BY id LIMIT 1)
WHERE created_by IS NULL;

-- The creator is a linked member of their group: link an unlinked member of
-- the same name if there is one, otherwise add them (with no expenses, so no
-- balance changes).
UPDATE members SET user_id = (SELECT g.created_by FROM groups g WHERE g.id = members.group_id)
WHERE user_id IS NULL
  AND id = (
    SELECT m2.id FROM members m2 JOIN groups g ON g.id = m2.group_id JOIN users u ON u.id = g.created_by
    WHERE m2.group_id = members.group_id AND m2.user_id IS NULL
      AND lower(m2.name) IN (lower(u.name), lower(substr(u.email, 1, instr(u.email, '@') - 1)))
    ORDER BY m2.id LIMIT 1)
  AND NOT EXISTS (
    SELECT 1 FROM members m3 JOIN groups g ON g.id = m3.group_id
    WHERE m3.group_id = members.group_id AND m3.user_id = g.created_by);

INSERT INTO members (group_id, user_id, name)
SELECT g.id, g.created_by, COALESCE(NULLIF(u.name, ''), substr(u.email, 1, instr(u.email, '@') - 1))
FROM groups g JOIN users u ON u.id = g.created_by
WHERE NOT EXISTS (SELECT 1 FROM members m WHERE m.group_id = g.id AND m.user_id = g.created_by);

-- Who did it, where it was never recorded: the group's owner.
UPDATE expenses SET created_by = (SELECT g.created_by FROM groups g WHERE g.id = expenses.group_id)
WHERE created_by IS NULL;
UPDATE activity SET user_id = (SELECT g.created_by FROM groups g WHERE g.id = activity.group_id)
WHERE user_id IS NULL;

CREATE INDEX IF NOT EXISTS groups_created_by ON groups(created_by);
CREATE INDEX IF NOT EXISTS members_user ON members(user_id);

-- Custom categories belong to whoever added them; the built-in list stays
-- shared (user_id NULL) and only an admin edits it.
ALTER TABLE categories ADD COLUMN user_id INTEGER REFERENCES users(id) ON DELETE SET NULL;
UPDATE categories SET user_id = (SELECT id FROM users WHERE role = 'admin' ORDER BY id LIMIT 1)
WHERE builtin = 0 AND user_id IS NULL;
CREATE INDEX IF NOT EXISTS categories_user ON categories(user_id);

-- Personal preferences (default currency for new groups, date and time
-- format). The settings table keeps the box-wide ones (title, rate lookups)
-- and the box default currency for people who have not picked their own.
CREATE TABLE IF NOT EXISTS user_settings (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key     TEXT NOT NULL,
  value   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, key)
);
INSERT OR IGNORE INTO user_settings (user_id, key, value)
SELECT (SELECT id FROM users WHERE role = 'admin' ORDER BY id LIMIT 1), key, value FROM settings
WHERE key IN ('default_currency', 'date_format', 'time_format')
  AND (SELECT id FROM users WHERE role = 'admin' ORDER BY id LIMIT 1) IS NOT NULL;
