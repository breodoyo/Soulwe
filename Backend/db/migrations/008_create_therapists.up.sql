CREATE TABLE therapists (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    full_name       TEXT NOT NULL,
    credentials     TEXT NOT NULL,
    years_exp       INTEGER,
    bio             TEXT,
    photo_url       TEXT,
    location        TEXT,
    is_online_only  BOOLEAN DEFAULT FALSE,
    price_kes       INTEGER,
    free_sessions   INTEGER DEFAULT 0,
    specialties     TEXT[],
    is_active       BOOLEAN DEFAULT TRUE,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE therapist_languages (
    therapist_id UUID NOT NULL REFERENCES therapists(id) ON DELETE CASCADE,
    language     TEXT NOT NULL,
    proficiency  TEXT DEFAULT 'fluent',
    PRIMARY KEY  (therapist_id, language)
);
