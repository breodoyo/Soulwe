-- Anonymous ownership for journal_entries, mood_logs and breathing_sessions so
-- those features no longer require signup. An anonymous identity resolves only
-- via anon_identities.token_hash, so owning a row is as private as owning it
-- as a user; anonymous rows cascade with their anon_identities row.

-- ── journal_entries ────────────────────────────────────────────────────────
ALTER TABLE journal_entries
    ALTER COLUMN user_id DROP NOT NULL;

ALTER TABLE journal_entries
    ADD COLUMN anon_identity_id UUID REFERENCES anon_identities(id) ON DELETE CASCADE;

-- Exactly one owner, never zero and never two.
ALTER TABLE journal_entries
    ADD CONSTRAINT journal_entries_one_owner CHECK (
        (user_id IS NOT NULL) <> (anon_identity_id IS NOT NULL)
    );

-- Covers anonymous history reads, which are always scoped by the caller.
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
-- device_uuid is left in place and still counted so legacy device-bound rows
-- keep satisfying exactly-one-owner.
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
