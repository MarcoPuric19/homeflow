ALTER TABLE calendar_events
    ADD COLUMN household_id uuid REFERENCES households(id),
    ADD COLUMN pet_id uuid,
    ADD COLUMN appointment_type text;

UPDATE calendar_events
SET household_id = '00000000-0000-4000-8000-000000000010'
WHERE household_id IS NULL;

ALTER TABLE calendar_events
    ALTER COLUMN household_id SET NOT NULL,
    ADD CONSTRAINT calendar_events_pet_same_household_fk
        FOREIGN KEY (household_id, pet_id) REFERENCES pets(household_id, id) ON DELETE SET NULL (pet_id),
    ADD CONSTRAINT calendar_events_appointment_type_check
        CHECK (appointment_type IS NULL OR appointment_type IN
            ('veterinarian','vaccination','medication','grooming','other')),
    ADD CONSTRAINT calendar_events_pet_appointment_check
        CHECK ((pet_id IS NULL AND appointment_type IS NULL) OR pet_id IS NOT NULL);

CREATE INDEX calendar_events_household_time_idx
    ON calendar_events(household_id, starts_at, ends_at);
CREATE INDEX calendar_events_household_pet_time_idx
    ON calendar_events(household_id, pet_id, starts_at) WHERE pet_id IS NOT NULL;
