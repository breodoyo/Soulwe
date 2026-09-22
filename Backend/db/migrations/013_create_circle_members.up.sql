-- Phase 6.1: peer support circle memberships.
--
-- Circles are anonymous chat rooms: memberships are keyed to anon_identities
-- (never registered users), exactly like circle_messages and message_flags.
-- Joining the same circle twice is a unique violation, so the service can map
-- it to a 409 as soon as one identity re-joins, and a repeated leave is a
-- no-op (idempotent DELETE).

CREATE TABLE circle_members (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    circle_id        UUID NOT NULL REFERENCES circles(id) ON DELETE CASCADE,
    anon_identity_id UUID NOT NULL REFERENCES anon_identities(id) ON DELETE CASCADE,
    joined_at        TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(circle_id, anon_identity_id)
);

-- Member counts come from COUNT over circle_members grouped by circle, but an
-- index on the identity side keeps "which circles has this person joined"
-- (and cascade delete checks) cheap.
CREATE INDEX idx_circle_members_anon_identity
    ON circle_members(anon_identity_id);