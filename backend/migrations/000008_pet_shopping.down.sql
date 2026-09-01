DROP TABLE IF EXISTS shopping_item_pets;
ALTER TABLE shopping_items DROP CONSTRAINT IF EXISTS shopping_items_list_same_household_fk;
ALTER TABLE shopping_items DROP CONSTRAINT IF EXISTS shopping_items_household_id_id_unique;
ALTER TABLE shopping_items DROP COLUMN IF EXISTS household_id;
DROP INDEX IF EXISTS shopping_lists_household_idx;
ALTER TABLE shopping_lists DROP CONSTRAINT IF EXISTS shopping_lists_household_id_id_unique;
ALTER TABLE shopping_lists DROP COLUMN IF EXISTS household_id;
