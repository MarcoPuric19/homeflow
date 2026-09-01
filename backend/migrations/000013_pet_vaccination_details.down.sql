ALTER TABLE vaccinations
    DROP CONSTRAINT IF EXISTS vaccinations_document_same_pet_fk,
    DROP CONSTRAINT IF EXISTS vaccinations_valid_until_check,
    DROP CONSTRAINT IF EXISTS vaccinations_diseases_count_check,
    DROP COLUMN IF EXISTS document_id,
    DROP COLUMN IF EXISTS valid_until,
    DROP COLUMN IF EXISTS diseases,
    DROP COLUMN IF EXISTS manufacturer;
ALTER TABLE documents DROP CONSTRAINT IF EXISTS documents_household_pet_id_unique;
