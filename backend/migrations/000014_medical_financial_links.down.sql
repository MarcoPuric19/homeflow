DROP TABLE IF EXISTS expense_appointments;
DROP TABLE IF EXISTS expense_medical_records;
DROP TABLE IF EXISTS document_appointments;
DROP TABLE IF EXISTS document_medical_records;
ALTER TABLE expenses DROP CONSTRAINT IF EXISTS expenses_household_pet_id_unique;
ALTER TABLE calendar_events DROP CONSTRAINT IF EXISTS calendar_events_household_pet_id_unique;
ALTER TABLE medical_records DROP CONSTRAINT IF EXISTS medical_records_household_pet_id_unique;
