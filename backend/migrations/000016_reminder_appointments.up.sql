CREATE TABLE reminder_appointments (
    household_id uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    source_kind text NOT NULL CHECK (source_kind = 'vaccination'),
    source_id uuid NOT NULL REFERENCES vaccinations(id) ON DELETE CASCADE,
    appointment_id uuid NOT NULL UNIQUE REFERENCES calendar_events(id) ON DELETE CASCADE,
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (household_id, source_kind, source_id)
);
