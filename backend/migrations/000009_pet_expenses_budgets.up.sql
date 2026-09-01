ALTER TABLE expenses ADD COLUMN household_id uuid REFERENCES households(id);
UPDATE expenses e SET household_id=hm.household_id
FROM household_members hm WHERE hm.user_id=e.user_id;
ALTER TABLE expenses ALTER COLUMN household_id SET NOT NULL;
ALTER TABLE expenses ADD COLUMN pet_id uuid;
ALTER TABLE expenses ADD COLUMN budget_category text NOT NULL DEFAULT ''
    CHECK (length(budget_category) <= 100);
ALTER TABLE expenses ADD CONSTRAINT expenses_household_pet_fk
    FOREIGN KEY (household_id, pet_id) REFERENCES pets(household_id, id);
CREATE INDEX expenses_household_due_id_idx ON expenses(household_id, due_date, id);
CREATE INDEX expenses_household_pet_due_idx ON expenses(household_id, pet_id, due_date)
    WHERE pet_id IS NOT NULL;

CREATE TABLE pet_budgets (
    id uuid PRIMARY KEY,
    household_id uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    pet_id uuid NOT NULL,
    month date NOT NULL CHECK (month = date_trunc('month', month)::date),
    category text NOT NULL CHECK (length(category) BETWEEN 1 AND 100),
    amount_cents bigint NOT NULL CHECK (amount_cents >= 0),
    currency char(3) NOT NULL DEFAULT 'EUR',
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (household_id, pet_id) REFERENCES pets(household_id, id) ON DELETE CASCADE,
    UNIQUE (household_id, pet_id, month, category)
);
CREATE INDEX pet_budgets_household_pet_month_idx
    ON pet_budgets(household_id, pet_id, month);
