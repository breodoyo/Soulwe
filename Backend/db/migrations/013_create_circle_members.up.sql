-- Circle memberships, keyed to anon_identities. Joining twice is a unique
-- violation so the service can map it to a 409; a repeated leave is a no-op.

CREATE TABLE circle_members (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    circle_id        UUID NOT NULL REFERENCES circles(id) ON DELETE CASCADE,
    anon_identity_id UUID NOT NULL REFERENCES anon_identities(id) ON DELETE CASCADE,
    joined_at        TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(circle_id, anon_identity_id)
);

-- Keeps "which circles has this person joined" and cascade delete checks cheap.
CREATE INDEX idx_circle_members_anon_identity
    ON circle_members(anon_identity_id);