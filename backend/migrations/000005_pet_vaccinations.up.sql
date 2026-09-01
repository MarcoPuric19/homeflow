CREATE TABLE vaccinations (
    id uuid PRIMARY KEY,
    household_id uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    pet_id uuid NOT NULL,
    vaccine_name text NOT NULL CHECK (length(vaccine_name) BETWEEN 1 AND 200),
    administered_on date NOT NULL,
    next_due_date date,
    veterinarian text NOT NULL DEFAULT '' CHECK (length(veterinarian) <= 200),
    batch_number text NOT NULL DEFAULT '' CHECK (length(batch_number) <= 100),
    notes text NOT NULL DEFAULT '' CHECK (length(notes) <= 2000),
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (household_id, pet_id) REFERENCES pets(household_id, id) ON DELETE CASCADE,
    CHECK (next_due_date IS NULL OR next_due_date >= administered_on)
);
CREATE INDEX vaccinations_household_pet_due_idx
    ON vaccinations(household_id, pet_id, next_due_date, id);
CREATE INDEX vaccinations_household_pet_history_idx
    ON vaccinations(household_id, pet_id, administered_on DESC, id DESC);
