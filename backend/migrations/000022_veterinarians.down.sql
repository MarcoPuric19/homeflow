ALTER TABLE pets DROP CONSTRAINT IF EXISTS pets_primary_vet_fk;
ALTER TABLE pets DROP COLUMN IF EXISTS primary_vet_id;
DROP TABLE IF EXISTS veterinarians;
