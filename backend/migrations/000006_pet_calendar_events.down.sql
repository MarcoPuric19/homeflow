DROP INDEX IF EXISTS calendar_events_household_pet_time_idx;
DROP INDEX IF EXISTS calendar_events_household_time_idx;
ALTER TABLE calendar_events
    DROP CONSTRAINT IF EXISTS calendar_events_pet_appointment_check,
    DROP CONSTRAINT IF EXISTS calendar_events_appointment_type_check,
    DROP CONSTRAINT IF EXISTS calendar_events_pet_same_household_fk,
    DROP COLUMN IF EXISTS appointment_type,
    DROP COLUMN IF EXISTS pet_id,
    DROP COLUMN IF EXISTS household_id;
