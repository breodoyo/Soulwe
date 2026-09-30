-- Anonymous ownership for personal features.
--
-- Soulwe is usable without registering. Circles already treat anon_identities
-- as a first-class owner, but journal_entries and mood_logs were hard-bound to
-- users(id) and breathing_sessions only ever wrote a user_id (its device_uuid
-- owner column was designed but never wired).
--
-- This migration lets those three features record against EITHER a registered
-- user OR an anonymous identity, so journalling, checking in and saving a
-- breathing session no longer require signup.
--
-- Privacy guarantees preserved:
--   * Every row still has exactly one owner, enforced by a CHECK constraint.
--   * An anonymous identity is only ever resolvable by the holder of its
--     bearer token (anon_identities.token_hash), so "owner = this anonymous
--     session" is equivalent in privacy to "owner = this user".
--   * No row is ever readable without an owner column, so nothing is public.
--   * Anonymous rows cascade away with their anon_identities row.

-- ── journal_entries ────────────────────────────────────────────────────────
-- user_id becomes nullable so an entry can belong to an anonymous session.
ALTER TABLE journal_entries
    ALTER COLUMN user_id DROP NOT NULL;

ALTER TABLE journal_entries
    ADD COLUMN anon_identity_id UUID REFERENCES anon_identities(id) ON DELETE CASCADE;

-- Exactly one owner, never zero and never two.
ALTER TABLE journal_entries
    ADD CONSTRAINT journal_entries_one_owner CHECK (
        (user_id IS NOT NULL) <> (anon_identity_id IS NOT NULL)
    );

-- Listing is always "my entries, newest first", scoped by whichever owner the
-- caller authenticated as. The old index covered the registered path; this one
-- covers anonymous history reads.
CREATE INDEX idx_journal_entries_anon_created
    ON journal_entries(anon_identity_id, created_at DESC)
    WHERE anon_identity_id IS NOT NULL;

-- ── mood_logs ─────────────────────────────────────────────────────────────
ALTER TABLE mood_logs
    ALTER COLUMN user_id DROP NOT NULL;

ALTER TABLE mood_logs
    ADD COLUMN anon_identity_id UUID REFERENCES anon_identities(id) ON DELETE CASCADE;

ALTER TABLE mood_logs
    ADD CONSTRAINT mood_logs_one_owner CHECK (
        (user_id IS NOT NULL) <> (anon_identity_id IS NOT NULL)
    );

CREATE INDEX idx_mood_logs_anon_logged
    ON mood_logs(anon_identity_id, logged_at DESC)
    WHERE anon_identity_id IS NOT NULL;

-- ── breathing_sessions ────────────────────────────────────────────────────
-- The table already allowed a non-user owner via device_uuid (one_ownership),
-- but no code ever wrote it. anonymous_identity_id is the real identity now:
-- it is backed by a token, so it survives across reloads like a session.
--
-- device_uuid is left in place and still accepted so any legacy device-bound
-- rows keep satisfying exactly-one-owner. The constraint is widened to count
-- all three owner columns rather than just two.
--
-- NOTE: the column added here (and the constraint that counts it) are named
-- anon_identity_id / breathing_sessions_one_owner. If you applied a draft of
-- this migration that called them anonymous_identity_id, drop those first.
ALTER TABLE breathing_sessions
    ADD COLUMN anon_identity_id UUID REFERENCES anon_identities(id) ON DELETE CASCADE;

ALTER TABLE breathing_sessions
    DROP CONSTRAINT one_ownership;

ALTER TABLE breathing_sessions
    ADD CONSTRAINT breathing_sessions_one_owner CHECK (
        num_nonnulls(user_id, anon_identity_id, device_uuid) = 1
    );

CREATE INDEX idx_breathing_sessions_anon_created
    ON breathing_sessions(anon_identity_id, created_at DESC)
    WHERE anon_identity_id IS NOT NULL;
