-- Reverts 018: circles go back to requiring an anonymous session.
--
-- Registered-owned rows are deleted first (as 017's down does): a real name
-- cannot be rewritten as a pseudonym, so there is nowhere to re-home them.
-- Anonymous rows are untouched.

DELETE FROM message_flags  WHERE user_id IS NOT NULL;
DELETE FROM circle_members WHERE user_id IS NOT NULL;
DELETE FROM circle_messages WHERE user_id IS NOT NULL;

-- ── message_flags: back to a single anonymous owner ───────────────────────
DROP INDEX IF EXISTS idx_message_flags_message_anon;
DROP INDEX IF EXISTS idx_message_flags_message_user;

ALTER TABLE message_flags DROP CONSTRAINT message_flags_one_owner;
ALTER TABLE message_flags DROP COLUMN user_id;

ALTER TABLE message_flags ALTER COLUMN flagged_by SET NOT NULL;

ALTER TABLE message_flags
    ADD CONSTRAINT message_flags_message_id_flagged_by_key UNIQUE(message_id, flagged_by);

-- ── circle_messages: back to a single anonymous owner ──────────────────────
DROP INDEX IF EXISTS idx_circle_messages_anon;
DROP INDEX IF EXISTS idx_circle_messages_user;

ALTER TABLE circle_messages DROP CONSTRAINT circle_messages_one_owner;
ALTER TABLE circle_messages DROP COLUMN user_id;

ALTER TABLE circle_messages ALTER COLUMN anon_identity_id SET NOT NULL;

-- ── circle_members: back to a single anonymous owner ───────────────────────
DROP INDEX IF EXISTS idx_circle_members_user;
DROP INDEX IF EXISTS idx_circle_members_circle_user;
DROP INDEX IF EXISTS idx_circle_members_circle_anon;

ALTER TABLE circle_members DROP CONSTRAINT circle_members_one_owner;
ALTER TABLE circle_members DROP COLUMN user_id;

ALTER TABLE circle_members ALTER COLUMN anon_identity_id SET NOT NULL;

ALTER TABLE circle_members
    ADD CONSTRAINT circle_members_circle_id_anon_identity_id_key
    UNIQUE(circle_id, anon_identity_id);
