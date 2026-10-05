-- Reverts 018_circle_registered_ownership: circles go back to requiring an
-- anonymous session.
--
-- A row owned by a registered user cannot survive the loss of its user_id —
-- there is no anonymous identity to re-home it onto, and silently rewriting
-- it would turn a real person's message into a pseudonym or a pseudonym into a
-- real name. So registered-owned rows are removed first, exactly as
-- 017_add_anon_ownership's down migration does for journals, check-ins and
-- breathing sessions. Anonymous rows are untouched.

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
