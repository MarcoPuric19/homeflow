-- HF-0116 rückgängig.
--
-- Die Liste wurde in der up-Migration umbenannt und nicht gelöscht, also wird
-- sie hier zurückbenannt. Codex hat im Gegencheck zu Recht angemerkt, dass die
-- ursprüngliche Fassung die Abnahme "stellt den Zustand wieder her" nicht
-- erfüllte.
UPDATE shopping_lists SET name = 'Gemeinsame Einkaufsliste'
 WHERE id = '00000000-0000-4000-8000-000000000100';
ALTER TABLE shopping_items DROP COLUMN IF EXISTS for_all_pets;

ALTER TABLE calendar_events ALTER COLUMN pet_id DROP NOT NULL;
ALTER TABLE expenses        ALTER COLUMN pet_id DROP NOT NULL;

ALTER TABLE calendar_events
    ADD CONSTRAINT calendar_events_pet_appointment_check
    CHECK ((pet_id IS NULL AND appointment_type IS NULL) OR pet_id IS NOT NULL);
