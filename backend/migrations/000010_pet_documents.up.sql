CREATE TABLE documents (
    id uuid PRIMARY KEY,
    household_id uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    pet_id uuid NOT NULL,
    title text NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    document_type text NOT NULL CHECK (document_type IN ('report','lab','vaccination','imaging','insurance','other')),
    occurred_on date,
    notes text NOT NULL DEFAULT '' CHECK (length(notes) <= 2000),
    original_filename text NOT NULL CHECK (length(original_filename) BETWEEN 1 AND 255),
    mime_type text NOT NULL CHECK (mime_type IN ('image/jpeg','image/png','application/pdf')),
    sha256 char(64) NOT NULL,
    storage_key text NOT NULL UNIQUE,
    uploaded_by uuid NOT NULL REFERENCES users(id),
    uploaded_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (household_id, pet_id) REFERENCES pets(household_id, id) ON DELETE CASCADE
);
CREATE INDEX documents_household_pet_date_id_idx
    ON documents(household_id, pet_id, occurred_on DESC NULLS LAST, id DESC);
