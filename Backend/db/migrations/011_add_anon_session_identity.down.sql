ALTER TABLE anon_identities
    DROP CONSTRAINT anon_identity_binding;

-- The restored one_identity CHECK requires EXACTLY one of user_id/device_uuid,
-- so the complement is deleted first: token-only sessions (both NULL) and rows
-- carrying both. Without this, the ALTER below fails on the first such row, and
-- token-only sessions are the common case because X-Device-ID is optional.
DELETE FROM anon_identities
    WHERE (user_id IS NULL) = (device_uuid IS NULL);

ALTER TABLE anon_identities
    ADD CONSTRAINT one_identity CHECK (
        (user_id IS NULL) != (device_uuid IS NULL)
    );

DROP INDEX IF EXISTS idx_anon_identities_device_uuid;
DROP INDEX IF EXISTS idx_anon_identities_token_hash;

ALTER TABLE anon_identities
    DROP COLUMN IF EXISTS last_seen_at,
    DROP COLUMN IF EXISTS token_hash;