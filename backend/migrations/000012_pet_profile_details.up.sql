ALTER TABLE pets
    ADD COLUMN approximate_birth_date boolean NOT NULL DEFAULT false,
    ADD COLUMN color text NOT NULL DEFAULT '' CHECK (length(color) <= 100),
    ADD COLUMN insurance text NOT NULL DEFAULT '' CHECK (length(insurance) <= 200),
    ADD COLUMN insurance_number text NOT NULL DEFAULT '' CHECK (length(insurance_number) <= 100),
    ADD COLUMN veterinarian text NOT NULL DEFAULT '' CHECK (length(veterinarian) <= 200),
    ADD CONSTRAINT pets_approximate_birth_date_check
        CHECK (NOT approximate_birth_date OR birth_date IS NOT NULL);
