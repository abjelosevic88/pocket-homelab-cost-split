CREATE TABLE expense_sequence(singleton INTEGER PRIMARY KEY CHECK(singleton=1), value INTEGER NOT NULL);
INSERT INTO expense_sequence VALUES(1,(SELECT COALESCE(MAX(id),0) FROM expenses));
