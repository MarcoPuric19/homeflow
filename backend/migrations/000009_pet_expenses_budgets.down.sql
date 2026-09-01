DROP TABLE IF EXISTS pet_budgets;
DROP INDEX IF EXISTS expenses_household_pet_due_idx;
DROP INDEX IF EXISTS expenses_household_due_id_idx;
ALTER TABLE expenses DROP CONSTRAINT IF EXISTS expenses_household_pet_fk;
ALTER TABLE expenses DROP COLUMN IF EXISTS budget_category;
ALTER TABLE expenses DROP COLUMN IF EXISTS pet_id;
ALTER TABLE expenses DROP COLUMN IF EXISTS household_id;
