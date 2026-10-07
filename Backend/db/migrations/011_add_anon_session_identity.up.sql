-- Token-bound anonymous sessions: the bearer token's SHA-256 hash is stored for
-- lookup (never the raw token). The original one_identity CHECK forbade
-- token-only sessions carrying no device UUID.

ALTER TABLE anon_identities
    ADD COLUMN token_hash   TEXT,
    ADD COLUMN last_seen_at TIMESTAMPTZ DEFAULT NOW();

-- Only hashes are ever stored; a duplicate hash would corrupt session auth.
CREATE UNIQUE INDEX idx_anon_identities_token_hash
    ON anon_identities (token_hash)
    WHERE token_hash IS NOT NULL;

-- One identity per device keeps repeated anonymous calls idempotent.
CREATE UNIQUE INDEX idx_anon_identities_device_uuid
    ON anon_identities (device_uuid)
    WHERE device_uuid IS NOT NULL;

-- device_uuid stays an accepted binding so legacy device-bound rows keep
-- satisfying the constraint and this ALTER cannot fail on old data.
ALTER TABLE anon_identities
    DROP CONSTRAINT one_identity;

ALTER TABLE anon_identities
    ADD CONSTRAINT anon_identity_binding CHECK (
        user_id IS NOT NULL OR token_hash IS NOT NULL OR device_uuid IS NOT NULL
    );