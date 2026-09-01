CREATE TABLE medications (
    id uuid PRIMARY KEY,
    household_id uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    active_ingredient text NOT NULL DEFAULT '' CHECK (length(active_ingredient) <= 200),
    dosage_form text NOT NULL DEFAULT '' CHECK (length(dosage_form) <= 100),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (household_id, id)
);

CREATE TABLE pet_medications (
    id uuid PRIMARY KEY,
    household_id uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    pet_id uuid NOT NULL,
    medication_id uuid NOT NULL,
    dose_text text NOT NULL CHECK (length(dose_text) BETWEEN 1 AND 200),
    frequency_text text NOT NULL CHECK (length(frequency_text) BETWEEN 1 AND 200),
    starts_on date NOT NULL,
    ends_on date,
    stock_quantity numeric(12,3) NOT NULL DEFAULT 0 CHECK (stock_quantity >= 0),
    stock_unit text NOT NULL DEFAULT '' CHECK (length(stock_unit) <= 50),
    notes text NOT NULL DEFAULT '' CHECK (length(notes) <= 2000),
    active boolean NOT NULL DEFAULT true,
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (household_id, pet_id) REFERENCES pets(household_id, id) ON DELETE CASCADE,
    FOREIGN KEY (household_id, medication_id) REFERENCES medications(household_id, id) ON DELETE CASCADE,
    CHECK (ends_on IS NULL OR ends_on >= starts_on)
);
CREATE INDEX pet_medications_household_pet_active_idx
    ON pet_medications(household_id, pet_id, active, starts_on DESC, id DESC);
