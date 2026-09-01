ALTER TABLE pets ADD CONSTRAINT pets_household_id_id_unique UNIQUE (household_id, id);

CREATE TABLE medical_records (
    id uuid PRIMARY KEY,
    household_id uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    pet_id uuid NOT NULL,
    record_type text NOT NULL CHECK (record_type IN ('diagnosis','treatment','procedure','examination','note','imaging','report')),
    occurred_on date NOT NULL,
    title text NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    summary text NOT NULL DEFAULT '' CHECK (length(summary) <= 4000),
    veterinarian text NOT NULL DEFAULT '' CHECK (length(veterinarian) <= 200),
    details jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(details) = 'object'),
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (household_id, pet_id) REFERENCES pets(household_id, id) ON DELETE CASCADE
);
CREATE INDEX medical_records_household_pet_timeline_idx
    ON medical_records(household_id, pet_id, occurred_on DESC, id DESC);
