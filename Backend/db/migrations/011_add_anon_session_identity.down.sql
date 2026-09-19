ALTER TABLE anon_identities
    DROP CONSTRAINT anon_identity_binding;

-- Phase 3.4 relaxed the binding rule; rows that the restored Phase 2
-- one_identity CHECK cannot accept are removed with the feature they belong
-- to. one_identity requires EXACTLY one of user_id/device_uuid (XOR), so we
-- delete the complement: token-only sessions (both NULL) and any row carrying
-- both columns. User-bound and device-bound-only identities survive intact.
-- Without this, ALTER TABLE below would fail on the first incompatible row
-- (token-only anonymous sessions are the common case, since X-Device-ID is
-- optional).
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