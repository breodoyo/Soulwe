-- Reverts 017_add_anon_ownership: personal features go back to requiring a
-- registered user.
--
-- Anonymous rows cannot remain once anon_identity_id is dropped, so they are
-- removed first. These are rows this migration itself introduced (an entry,
-- check-in or session created while signed out); registered rows are
-- untouched.

DELETE FROM journal_entries WHERE user_id IS NULL;
DELETE FROM mood_logs        WHERE user_id IS NULL;
-- Only the anonymous rows go: breathing_sessions also accepts a pre-existing
-- device_uuid owner, and those rows are not ours to delete.
DELETE FROM breathing_sessions WHERE anon_identity_id IS NOT NULL;

-- ── breathing_sessions: restore the original two-column constraint ─────────
DROP INDEX IF EXISTS idx_breathing_sessions_anon_created;

ALTER TABLE breathing_sessions DROP CONSTRAINT breathing_sessions_one_owner;
ALTER TABLE breathing_sessions DROP COLUMN anon_identity_id;

ALTER TABLE breathing_sessions
    ADD CONSTRAINT one_ownership CHECK (
        (user_id IS NULL) != (device_uuid IS NULL)
    );

-- ── mood_logs ─────────────────────────────────────────────────────────────
DROP INDEX IF EXISTS idx_mood_logs_anon_logged;

ALTER TABLE mood_logs DROP CONSTRAINT mood_logs_one_owner;
ALTER TABLE mood_logs DROP COLUMN anon_identity_id;
ALTER TABLE mood_logs ALTER COLUMN user_id SET NOT NULL;

-- ── journal_entries ────────────────────────────────────────────────────────
DROP INDEX IF EXISTS idx_journal_entries_anon_created;

ALTER TABLE journal_entries DROP CONSTRAINT journal_entries_one_owner;
ALTER TABLE journal_entries DROP COLUMN anon_identity_id;
ALTER TABLE journal_entries ALTER COLUMN user_id SET NOT NULL;
