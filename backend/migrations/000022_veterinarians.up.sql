CREATE TABLE veterinarians (
    id uuid PRIMARY KEY,
    household_id uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    practice text NOT NULL CHECK (length(practice) BETWEEN 1 AND 200),
    vet_name text NOT NULL DEFAULT '' CHECK (length(vet_name) <= 200),
    street text NOT NULL DEFAULT '' CHECK (length(street) <= 200),
    postal_code text NOT NULL DEFAULT '' CHECK (length(postal_code) <= 20),
    city text NOT NULL DEFAULT '' CHECK (length(city) <= 120),
    phone text NOT NULL DEFAULT '' CHECK (length(phone) <= 60),
    email text NOT NULL DEFAULT '' CHECK (length(email) <= 200),
    notes text NOT NULL DEFAULT '' CHECK (length(notes) <= 2000),
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (household_id, id),
    UNIQUE (household_id, practice, postal_code)
);
CREATE INDEX veterinarians_household_id_idx ON veterinarians(household_id, id);

ALTER TABLE pets ADD COLUMN primary_vet_id uuid;
ALTER TABLE pets ADD CONSTRAINT pets_primary_vet_fk
    FOREIGN KEY (household_id, primary_vet_id)
    REFERENCES veterinarians(household_id, id) ON DELETE SET NULL;
