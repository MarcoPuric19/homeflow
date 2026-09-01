CREATE TABLE household_reminder_rules (
    household_id uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    lead_days integer NOT NULL CHECK (lead_days BETWEEN 0 AND 366),
    PRIMARY KEY (household_id, lead_days)
);

CREATE FUNCTION create_default_household_reminder_rules() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO household_reminder_rules(household_id, lead_days) VALUES (NEW.id, 30), (NEW.id, 7), (NEW.id, 1);
    RETURN NEW;
END;
$$;

CREATE TRIGGER households_default_reminder_rules
AFTER INSERT ON households FOR EACH ROW EXECUTE FUNCTION create_default_household_reminder_rules();

INSERT INTO household_reminder_rules(household_id, lead_days)
SELECT h.id, v.lead_days FROM households h CROSS JOIN (VALUES (30), (7), (1)) AS v(lead_days);
