ALTER TABLE documents ADD CONSTRAINT documents_household_pet_id_unique UNIQUE (household_id, pet_id, id);

ALTER TABLE vaccinations
    ADD COLUMN manufacturer text NOT NULL DEFAULT '' CHECK (length(manufacturer) <= 200),
    ADD COLUMN diseases text[] NOT NULL DEFAULT '{}',
    ADD COLUMN valid_until date,
    ADD COLUMN document_id uuid,
    ADD CONSTRAINT vaccinations_diseases_count_check CHECK (cardinality(diseases) <= 20),
    ADD CONSTRAINT vaccinations_valid_until_check CHECK (valid_until IS NULL OR valid_until >= administered_on),
    ADD CONSTRAINT vaccinations_document_same_pet_fk
        FOREIGN KEY (household_id, pet_id, document_id)
        REFERENCES documents(household_id, pet_id, id) ON DELETE SET NULL (document_id);
