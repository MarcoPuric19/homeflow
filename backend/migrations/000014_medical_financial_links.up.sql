ALTER TABLE medical_records ADD CONSTRAINT medical_records_household_pet_id_unique UNIQUE (household_id, pet_id, id);
ALTER TABLE calendar_events ADD CONSTRAINT calendar_events_household_pet_id_unique UNIQUE (household_id, pet_id, id);
ALTER TABLE expenses ADD CONSTRAINT expenses_household_pet_id_unique UNIQUE (household_id, pet_id, id);

CREATE TABLE document_medical_records (
    household_id uuid NOT NULL,
    pet_id uuid NOT NULL,
    document_id uuid NOT NULL,
    medical_record_id uuid NOT NULL,
    PRIMARY KEY (document_id, medical_record_id),
    FOREIGN KEY (household_id, pet_id, document_id) REFERENCES documents(household_id, pet_id, id) ON DELETE CASCADE,
    FOREIGN KEY (household_id, pet_id, medical_record_id) REFERENCES medical_records(household_id, pet_id, id) ON DELETE CASCADE
);
CREATE INDEX document_medical_records_record_idx ON document_medical_records(household_id, pet_id, medical_record_id);

CREATE TABLE document_appointments (
    household_id uuid NOT NULL,
    pet_id uuid NOT NULL,
    document_id uuid NOT NULL,
    appointment_id uuid NOT NULL,
    PRIMARY KEY (document_id, appointment_id),
    FOREIGN KEY (household_id, pet_id, document_id) REFERENCES documents(household_id, pet_id, id) ON DELETE CASCADE,
    FOREIGN KEY (household_id, pet_id, appointment_id) REFERENCES calendar_events(household_id, pet_id, id) ON DELETE CASCADE
);
CREATE INDEX document_appointments_appointment_idx ON document_appointments(household_id, pet_id, appointment_id);

CREATE TABLE expense_medical_records (
    household_id uuid NOT NULL,
    pet_id uuid NOT NULL,
    expense_id uuid NOT NULL,
    medical_record_id uuid NOT NULL,
    PRIMARY KEY (expense_id, medical_record_id),
    FOREIGN KEY (household_id, pet_id, expense_id) REFERENCES expenses(household_id, pet_id, id) ON DELETE CASCADE,
    FOREIGN KEY (household_id, pet_id, medical_record_id) REFERENCES medical_records(household_id, pet_id, id) ON DELETE CASCADE
);
CREATE INDEX expense_medical_records_record_idx ON expense_medical_records(household_id, pet_id, medical_record_id);

CREATE TABLE expense_appointments (
    household_id uuid NOT NULL,
    pet_id uuid NOT NULL,
    expense_id uuid NOT NULL,
    appointment_id uuid NOT NULL,
    PRIMARY KEY (expense_id, appointment_id),
    FOREIGN KEY (household_id, pet_id, expense_id) REFERENCES expenses(household_id, pet_id, id) ON DELETE CASCADE,
    FOREIGN KEY (household_id, pet_id, appointment_id) REFERENCES calendar_events(household_id, pet_id, id) ON DELETE CASCADE
);
CREATE INDEX expense_appointments_appointment_idx ON expense_appointments(household_id, pet_id, appointment_id);
