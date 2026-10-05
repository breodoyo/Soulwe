-- Registered ownership for peer support circles.
--
-- Circles were the last part of the app that only accepted an anonymous
-- session: every route sat behind AnonymousAuthRequired, and circle_members,
-- circle_messages and message_flags were hard-bound to anon_identities(id).
-- That made peer support unusable for anyone with an account — the one group
-- of people most likely to seek it.
--
-- This migration lets a circle membership and a circle message belong to
-- EITHER a registered user OR an anonymous identity, so a signed-in member
-- can take part openly (their own display name) while a guest keeps posting
-- under a server-generated pseudonym.
--
-- This mirrors 017_add_anon_ownership, which opened journalling, check-ins
-- and breathing history to the same two kinds of owner.
--
-- Privacy guarantees preserved:
--   * Every row still has exactly one owner, enforced by a CHECK constraint.
--   * An anonymous identity is only ever resolvable by the holder of its
--     bearer token (anon_identities.token_hash), so "owner = this anonymous
--     session" stays equivalent in privacy to "owner = this user".
--   * A registered author's user_id is never serialized onto the wire: the
--     API exposes only the resolved display name, resolved server-side.
--   * Anonymous rows still cascade away with their anon_identities row, and
--     registered rows with their users row.

-- ── circle_members ─────────────────────────────────────────────────────────
-- user_id is added so a signed-in member can join; anon_identity_id becomes
-- nullable so the same row can be owned by an anonymous session instead.
ALTER TABLE circle_members
    ADD COLUMN user_id UUID REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE circle_members
    ALTER COLUMN anon_identity_id DROP NOT NULL;

-- Exactly one owner, never zero and never two.
ALTER TABLE circle_members
    ADD CONSTRAINT circle_members_one_owner CHECK (
        (user_id IS NOT NULL) <> (anon_identity_id IS NOT NULL)
    );

-- The old single unique constraint cannot express "one membership per owner"
-- across two owner columns, so it is replaced by one partial unique index per
-- owner kind. The anonymous one reproduces the previous guarantee exactly;
-- joining twice still raises a unique violation, which the service maps to 409.
ALTER TABLE circle_members
    DROP CONSTRAINT circle_members_circle_id_anon_identity_id_key;

CREATE UNIQUE INDEX idx_circle_members_circle_anon
    ON circle_members(circle_id, anon_identity_id)
    WHERE anon_identity_id IS NOT NULL;

CREATE UNIQUE INDEX idx_circle_members_circle_user
    ON circle_members(circle_id, user_id)
    WHERE user_id IS NOT NULL;

-- The identity-side index on anon_identity_id already exists for cascade
-- deletes and "which circles has this person joined". This is its registered
-- twin, needed for the same reasons.
CREATE INDEX idx_circle_members_user
    ON circle_members(user_id);

-- ── circle_messages ───────────────────────────────────────────────────────
ALTER TABLE circle_messages
    ADD COLUMN user_id UUID REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE circle_messages
    ALTER COLUMN anon_identity_id DROP NOT NULL;

ALTER TABLE circle_messages
    ADD CONSTRAINT circle_messages_one_owner CHECK (
        (user_id IS NOT NULL) <> (anon_identity_id IS NOT NULL)
    );

-- Reading a room is always "this circle, newest first", so the existing
-- (circle_id, created_at DESC) index already covers both owner kinds. The
-- author-side indexes keep "which rooms has this person posted in" cheap.
CREATE INDEX idx_circle_messages_anon
    ON circle_messages(anon_identity_id)
    WHERE anon_identity_id IS NOT NULL;

CREATE INDEX idx_circle_messages_user
    ON circle_messages(user_id)
    WHERE user_id IS NOT NULL;

-- ── message_flags ─────────────────────────────────────────────────────────
-- No Go code writes flags yet, but the table is part of the circles domain
-- and leaving it anonymous-only would mean a registered member could never
-- report a message. Widened with the same pattern so the domain is uniform.
ALTER TABLE message_flags
    ADD COLUMN user_id UUID REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE message_flags
    ALTER COLUMN flagged_by DROP NOT NULL;

ALTER TABLE message_flags
    ADD CONSTRAINT message_flags_one_owner CHECK (
        (user_id IS NOT NULL) <> (flagged_by IS NOT NULL)
    );

ALTER TABLE message_flags
    DROP CONSTRAINT message_flags_message_id_flagged_by_key;

CREATE UNIQUE INDEX idx_message_flags_message_anon
    ON message_flags(message_id, flagged_by)
    WHERE flagged_by IS NOT NULL;

CREATE UNIQUE INDEX idx_message_flags_message_user
    ON message_flags(message_id, user_id)
    WHERE user_id IS NOT NULL;
