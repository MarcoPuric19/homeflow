ALTER TABLE shopping_items
    ADD COLUMN source_pet_medication_id uuid REFERENCES pet_medications(id) ON DELETE SET NULL;

CREATE UNIQUE INDEX shopping_items_open_medication_source_idx
    ON shopping_items(household_id, source_pet_medication_id)
    WHERE source_pet_medication_id IS NOT NULL AND completed = false;
