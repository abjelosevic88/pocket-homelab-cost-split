-- A payment (reimbursement) moves money between two people to settle up; it
-- has no category. Clearing the old ones also lets General be removed once no
-- real expense uses it.
UPDATE expenses SET category = '' WHERE reimbursement = 1;
