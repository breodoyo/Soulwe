CREATE TABLE anon_identities (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID REFERENCES users(id) ON DELETE CASCADE,
    device_uuid TEXT,
    anon_name   TEXT NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ DEFAULT NOW(),

    CONSTRAINT one_identity CHECK (
        (user_id IS NULL) != (device_uuid IS NULL)
    )
);

CREATE UNIQUE INDEX idx_anon_identities_user_id ON anon_identities (user_id) WHERE user_id IS NOT NULL;
