CREATE TABLE households (
    id uuid PRIMARY KEY,
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE household_members (
    household_id uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role text NOT NULL CHECK (role IN ('owner', 'member')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (household_id, user_id),
    UNIQUE (user_id)
);

INSERT INTO households(id, name)
VALUES ('00000000-0000-4000-8000-000000000010', 'Zuhause');

INSERT INTO household_members(household_id, user_id, role)
SELECT '00000000-0000-4000-8000-000000000010', id,
       CASE WHEN role = 'admin' THEN 'owner' ELSE 'member' END
FROM users;

CREATE TABLE pets (
    id uuid PRIMARY KEY,
    household_id uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    species text NOT NULL CHECK (length(species) BETWEEN 1 AND 80),
    breed text NOT NULL DEFAULT '' CHECK (length(breed) <= 120),
    birth_date date,
    sex text NOT NULL DEFAULT 'unknown' CHECK (sex IN ('female', 'male', 'unknown')),
    microchip_number text CHECK (microchip_number IS NULL OR length(microchip_number) BETWEEN 5 AND 50),
    notes text NOT NULL DEFAULT '' CHECK (length(notes) <= 2000),
    active boolean NOT NULL DEFAULT true,
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX pets_household_active_id_idx ON pets(household_id, active, id);
CREATE UNIQUE INDEX pets_household_microchip_idx
    ON pets(household_id, microchip_number) WHERE microchip_number IS NOT NULL;
