CREATE TABLE mood_logs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mood        TEXT NOT NULL,
    logged_at   TIMESTAMPTZ DEFAULT NOW()
);
