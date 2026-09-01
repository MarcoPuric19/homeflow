ALTER TABLE pets
    DROP CONSTRAINT IF EXISTS pets_approximate_birth_date_check,
    DROP COLUMN IF EXISTS veterinarian,
    DROP COLUMN IF EXISTS insurance_number,
    DROP COLUMN IF EXISTS insurance,
    DROP COLUMN IF EXISTS color,
    DROP COLUMN IF EXISTS approximate_birth_date;
