CREATE TABLE circle_messages (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    circle_id        UUID NOT NULL REFERENCES circles(id) ON DELETE CASCADE,
    anon_identity_id UUID NOT NULL REFERENCES anon_identities(id) ON DELETE CASCADE,
    content          TEXT NOT NULL,
    reaction_counts  JSONB DEFAULT '{}',
    is_flagged       BOOLEAN DEFAULT FALSE,
    created_at       TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_circle_messages_circle_created
    ON circle_messages(circle_id, created_at DESC);
