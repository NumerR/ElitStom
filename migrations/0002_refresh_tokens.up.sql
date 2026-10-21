-- DentaCore: rotating refresh tokens.
-- token_hash stores the SHA-256 digest of the opaque secret handed to the client, never the
-- secret itself: a read of this table must not be replayable as a session.

CREATE TABLE refresh_tokens (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    family_id  text        NOT NULL,
    token_hash char(64)    NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    used_at    timestamptz,
    revoked_at timestamptz,

    CONSTRAINT refresh_tokens_token_hash_unique UNIQUE (token_hash),

    -- 64 hex characters is a SHA-256 digest. Anything else means the application wrote
    -- plaintext or a truncated value.
    CONSTRAINT refresh_tokens_token_hash_is_sha256 CHECK (token_hash ~ '^[0-9a-f]{64}$'),
    CONSTRAINT refresh_tokens_family_id_is_hex CHECK (family_id ~ '^[0-9a-f]{32}$'),
    CONSTRAINT refresh_tokens_expiry_after_creation CHECK (expires_at > created_at)
);

-- Rotation looks a token up by digest (the unique index above) and revokes a whole family
-- when reuse is detected.
CREATE INDEX refresh_tokens_family_id_idx ON refresh_tokens (family_id);

-- Revoke-all-for-user runs on logout, password/role change and account disable.
CREATE INDEX refresh_tokens_user_id_idx ON refresh_tokens (user_id);

-- Finds rows a cleanup job can delete; partial because live tokens are the rare case.
CREATE INDEX refresh_tokens_expired_idx ON refresh_tokens (expires_at) WHERE used_at IS NOT NULL OR revoked_at IS NOT NULL;
