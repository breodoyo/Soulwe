CREATE TABLE message_flags (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id  UUID NOT NULL REFERENCES circle_messages(id) ON DELETE CASCADE,
    flagged_by  UUID NOT NULL REFERENCES anon_identities(id),
    reason      TEXT,
    created_at  TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(message_id, flagged_by)
);
