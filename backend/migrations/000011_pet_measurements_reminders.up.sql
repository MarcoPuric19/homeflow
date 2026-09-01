CREATE TABLE weight_entries (
    id uuid PRIMARY KEY,
    household_id uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    pet_id uuid NOT NULL,
    measured_on date NOT NULL,
    weight_kg numeric(10,3) NOT NULL CHECK (weight_kg > 0),
    notes text NOT NULL DEFAULT '' CHECK (length(notes) <= 1000),
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (household_id, pet_id) REFERENCES pets(household_id, id) ON DELETE CASCADE
);
CREATE INDEX weight_entries_household_pet_date_id_idx ON weight_entries(household_id,pet_id,measured_on DESC,id DESC);

CREATE TABLE lab_results (
    id uuid PRIMARY KEY,
    household_id uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    pet_id uuid NOT NULL,
    measured_on date NOT NULL,
    test_name text NOT NULL CHECK (length(test_name) BETWEEN 1 AND 200),
    value numeric(14,4) NOT NULL,
    unit text NOT NULL DEFAULT '' CHECK (length(unit) <= 50),
    reference_low numeric(14,4),
    reference_high numeric(14,4),
    notes text NOT NULL DEFAULT '' CHECK (length(notes) <= 1000),
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (household_id, pet_id) REFERENCES pets(household_id, id) ON DELETE CASCADE,
    CHECK (reference_low IS NULL OR reference_high IS NULL OR reference_low <= reference_high)
);
CREATE INDEX lab_results_household_pet_date_id_idx ON lab_results(household_id,pet_id,measured_on DESC,id DESC);
CREATE INDEX vaccinations_household_next_due_idx ON vaccinations(household_id,next_due_date,pet_id) WHERE next_due_date IS NOT NULL;
CREATE INDEX calendar_events_household_pet_start_idx ON calendar_events(household_id,pet_id,starts_at) WHERE pet_id IS NOT NULL AND status='planned';
