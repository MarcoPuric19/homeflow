ALTER TABLE pet_budgets
    ADD COLUMN warning_threshold_cents bigint
        CHECK (warning_threshold_cents IS NULL OR warning_threshold_cents >= 0),
    ADD COLUMN notes text NOT NULL DEFAULT ''
        CHECK (length(notes) <= 2000);
