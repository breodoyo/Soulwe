CREATE TABLE journal_entries (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content_enc     BYTEA NOT NULL,
    content_iv      BYTEA NOT NULL,
    mood_tags       TEXT[],
    prompt_used     TEXT,
    ai_reflection   TEXT,
    word_count      INTEGER,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_journal_entries_user_created
    ON journal_entries(user_id, created_at DESC);
