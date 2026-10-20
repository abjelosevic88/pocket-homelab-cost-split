-- Which Pocket app added an expense through a Pocket Core link ("kitchen"),
-- empty for expenses people added themselves. A linked app may change or
-- delete only the expenses whose source is its own slug.
ALTER TABLE expenses ADD COLUMN source TEXT NOT NULL DEFAULT '';
