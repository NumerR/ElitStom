-- DentaCore: accounts, roles and RBAC.
-- The domain layer normalises email and full name before writing; the checks here keep that
-- invariant true of the data itself, so a hand-edited row cannot break the login lookup.

CREATE TABLE users (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text        NOT NULL,
    full_name     text        NOT NULL,
    phone         text        NOT NULL DEFAULT '',
    role          text        NOT NULL,
    status        text        NOT NULL DEFAULT 'active',
    password_hash text        NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),

    -- Login looks up by email exactly as stored, so the column must be its own unique key.
    CONSTRAINT users_email_unique UNIQUE (email),

    CONSTRAINT users_email_is_normalised CHECK (
        email = lower(btrim(email)) AND email <> '' AND char_length(email) <= 254
    ),
    CONSTRAINT users_full_name_is_normalised CHECK (
        char_length(full_name) BETWEEN 2 AND 100 AND full_name = btrim(full_name)
    ),
    CONSTRAINT users_phone_length CHECK (char_length(phone) <= 32),
    CONSTRAINT users_role_known CHECK (role IN ('admin', 'doctor', 'patient')),
    CONSTRAINT users_status_known CHECK (status IN ('active', 'disabled')),

    -- A plaintext password in this column is a breach; require the bcrypt envelope.
    CONSTRAINT users_password_hash_is_bcrypt CHECK (password_hash LIKE '$2$%' OR password_hash LIKE '$2a$%' OR password_hash LIKE '$2b$%' OR password_hash LIKE '$2y$%')
);

-- Admin directory filters by role and status and pages by creation time.
CREATE INDEX users_role_status_created_at_idx ON users (role, status, created_at DESC, id DESC);
