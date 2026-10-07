-- Registered ownership for peer support circles, mirroring 017: a signed-in
-- member posts under their own display name while a guest keeps a
-- server-generated pseudonym. A registered author's user_id is never
-- serialized onto the wire, only the server-resolved display name.

-- ── circle_members ─────────────────────────────────────────────────────────
ALTER TABLE circle_members
    ADD COLUMN user_id UUID REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE circle_members
    ALTER COLUMN anon_identity_id DROP NOT NULL;

-- Exactly one owner, never zero and never two.
ALTER TABLE circle_members
    ADD CONSTRAINT circle_members_one_owner CHECK (
        (user_id IS NOT NULL) <> (anon_identity_id IS NOT NULL)
    );

-- One partial unique index per owner kind, since the old constraint cannot span
-- two owner columns; joining twice still raises a unique violation (409).
ALTER TABLE circle_members
    DROP CONSTRAINT circle_members_circle_id_anon_identity_id_key;

CREATE UNIQUE INDEX idx_circle_members_circle_anon
    ON circle_members(circle_id, anon_identity_id)
    WHERE anon_identity_id IS NOT NULL;

CREATE UNIQUE INDEX idx_circle_members_circle_user
    ON circle_members(circle_id, user_id)
    WHERE user_id IS NOT NULL;

-- Registered twin of the anon_identity_id index: cascade deletes and
-- "which circles has this person joined".
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

-- The existing (circle_id, created_at DESC) index already serves both owner
-- kinds; these keep "which rooms has this person posted in" cheap.
CREATE INDEX idx_circle_messages_anon
    ON circle_messages(anon_identity_id)
    WHERE anon_identity_id IS NOT NULL;

CREATE INDEX idx_circle_messages_user
    ON circle_messages(user_id)
    WHERE user_id IS NOT NULL;

-- ── message_flags ─────────────────────────────────────────────────────────
-- No Go code writes flags yet; widened with the same pattern so a registered
-- member can still report a message once it does.
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
