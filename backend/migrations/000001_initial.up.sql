CREATE TABLE users (
    id uuid PRIMARY KEY,
    username text NOT NULL UNIQUE CHECK (length(username) BETWEEN 3 AND 100),
    display_name text NOT NULL CHECK (length(display_name) BETWEEN 1 AND 200),
    password_hash text NOT NULL,
    role text NOT NULL CHECK (role IN ('admin', 'member')),
    disabled boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE auth_sessions (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    access_token_hash bytea NOT NULL UNIQUE,
    refresh_token_hash bytea NOT NULL UNIQUE,
    access_expires_at timestamptz NOT NULL,
    refresh_expires_at timestamptz NOT NULL,
    rotated_from_id uuid REFERENCES auth_sessions(id),
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz
);
CREATE INDEX auth_sessions_user_active_idx ON auth_sessions(user_id, refresh_expires_at) WHERE revoked_at IS NULL;

CREATE TABLE shopping_lists (
    id uuid PRIMARY KEY,
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    visibility text NOT NULL CHECK (visibility IN ('shared', 'private')),
    owner_id uuid REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (visibility = 'shared' OR owner_id IS NOT NULL)
);

CREATE TABLE shopping_items (
    id uuid PRIMARY KEY,
    list_id uuid NOT NULL REFERENCES shopping_lists(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    quantity text NOT NULL CHECK (length(quantity) BETWEEN 1 AND 40),
    unit text NOT NULL DEFAULT '',
    category text NOT NULL DEFAULT '',
    note text NOT NULL DEFAULT '',
    priority text NOT NULL DEFAULT 'normal' CHECK (priority IN ('low', 'normal', 'high')),
    store text NOT NULL DEFAULT '',
    added_by uuid NOT NULL REFERENCES users(id),
    completed boolean NOT NULL DEFAULT false,
    purchased_by uuid REFERENCES users(id),
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((completed AND purchased_by IS NOT NULL AND completed_at IS NOT NULL) OR
           (NOT completed AND purchased_by IS NULL AND completed_at IS NULL))
);
CREATE INDEX shopping_items_open_idx ON shopping_items(list_id, created_at) WHERE completed = false;

CREATE TABLE calendar_events (
    id uuid PRIMARY KEY,
    title text NOT NULL CHECK (length(title) BETWEEN 1 AND 300),
    description text NOT NULL DEFAULT '',
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    all_day boolean NOT NULL DEFAULT false,
    category text NOT NULL DEFAULT '',
    owner_id uuid NOT NULL REFERENCES users(id),
    visibility text NOT NULL CHECK (visibility IN ('shared', 'private')),
    recurrence_rule text,
    reminder_minutes integer CHECK (reminder_minutes >= 0),
    status text NOT NULL DEFAULT 'planned' CHECK (status IN ('planned', 'completed', 'cancelled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (ends_at >= starts_at)
);
CREATE INDEX calendar_events_time_idx ON calendar_events(starts_at, ends_at);

CREATE TABLE expenses (
    id uuid PRIMARY KEY,
    description text NOT NULL CHECK (length(description) BETWEEN 1 AND 300),
    merchant text NOT NULL DEFAULT '',
    planned_amount_cents bigint NOT NULL CHECK (planned_amount_cents >= 0),
    actual_amount_cents bigint CHECK (actual_amount_cents >= 0),
    currency char(3) NOT NULL DEFAULT 'EUR',
    due_date date NOT NULL,
    paid_at timestamptz,
    category text NOT NULL DEFAULT '',
    recurrence_rule text,
    status text NOT NULL CHECK (status IN ('planned', 'open', 'paid', 'cancelled')),
    user_id uuid NOT NULL REFERENCES users(id),
    payment_method text NOT NULL DEFAULT '',
    notes text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((status = 'paid' AND paid_at IS NOT NULL) OR status <> 'paid')
);
CREATE INDEX expenses_due_idx ON expenses(status, due_date);

CREATE TABLE receipts (
    id uuid PRIMARY KEY,
    expense_id uuid NOT NULL REFERENCES expenses(id) ON DELETE CASCADE,
    merchant text NOT NULL DEFAULT '',
    receipt_date date,
    total_amount_cents bigint CHECK (total_amount_cents >= 0),
    currency char(3) NOT NULL DEFAULT 'EUR',
    category text NOT NULL DEFAULT '',
    original_filename text NOT NULL,
    mime_type text NOT NULL CHECK (mime_type IN ('image/jpeg', 'image/png', 'application/pdf')),
    sha256 char(64) NOT NULL,
    storage_key text NOT NULL UNIQUE,
    uploaded_at timestamptz NOT NULL DEFAULT now(),
    uploaded_by uuid NOT NULL REFERENCES users(id)
);
CREATE INDEX receipts_expense_idx ON receipts(expense_id);

CREATE TABLE audit_log (
    id uuid PRIMARY KEY,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    actor_user_id uuid REFERENCES users(id),
    event_type text NOT NULL,
    target_type text,
    target_id uuid,
    source_ip inet,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX audit_log_time_idx ON audit_log(occurred_at DESC);

CREATE TABLE event_outbox (
    id uuid PRIMARY KEY,
    event_name text NOT NULL,
    event_version integer NOT NULL DEFAULT 1,
    aggregate_type text NOT NULL,
    aggregate_id uuid NOT NULL,
    payload jsonb NOT NULL,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz,
    attempts integer NOT NULL DEFAULT 0
);
CREATE INDEX event_outbox_pending_idx ON event_outbox(occurred_at) WHERE published_at IS NULL;

