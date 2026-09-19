-- Phase 3.4: token-bound anonymous sessions.
--
-- anon_identities already covers registered-user (user_id) and device-bound
-- identities, but anonymous sessions authenticate with a bearer token whose
-- SHA-256 hash must be stored for lookup (never the raw token). The original
-- one_identity CHECK required exactly one of user_id/device_uuid, which
-- forbids token-only anonymous sessions that carry no device UUID.

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

-- Replace one_identity: every identity belongs to a registered user, a
-- session token, or (legacy Phase 2 rows) a device UUID. device_uuid must
-- remain an accepted binding so pre-existing device-bound identities keep
-- satisfying the constraint and the ALTER TABLE cannot fail on old data;
-- token-only anonymous sessions (new in this phase) are covered by token_hash.
-- A row with none of the three is still rejected.
ALTER TABLE anon_identities
    DROP CONSTRAINT one_identity;

ALTER TABLE anon_identities
    ADD CONSTRAINT anon_identity_binding CHECK (
        user_id IS NOT NULL OR token_hash IS NOT NULL OR device_uuid IS NOT NULL
    );