-- HF-0116 rückgängig. Stellt die Nullbarkeit wieder her; die vorher entfernte
-- Standardliste kommt nicht zurück, weil sie keine Daten trug.
ALTER TABLE shopping_items DROP COLUMN IF EXISTS for_all_pets;

ALTER TABLE calendar_events ALTER COLUMN pet_id DROP NOT NULL;
ALTER TABLE expenses        ALTER COLUMN pet_id DROP NOT NULL;

ALTER TABLE calendar_events
    ADD CONSTRAINT calendar_events_pet_appointment_check
    CHECK ((pet_id IS NULL AND appointment_type IS NULL) OR pet_id IS NOT NULL);
