CREATE TABLE breathing_sessions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID REFERENCES users(id) ON DELETE CASCADE,
    device_uuid TEXT,
    technique   TEXT NOT NULL,
    breaths     INTEGER NOT NULL,
    duration_s  INTEGER NOT NULL,
    completed   BOOLEAN DEFAULT FALSE,
    created_at  TIMESTAMPTZ DEFAULT NOW(),

    CONSTRAINT one_ownership CHECK (
        (user_id IS NULL) != (device_uuid IS NULL)
    )
);
