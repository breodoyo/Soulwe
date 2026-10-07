-- Therapist bookings. Both unique indexes are partial (status IN
-- ('pending','confirmed')) so a cancelled or completed booking leaves the
-- guard set and frees the slot: no double-sold therapist slot, and no one
-- user booking two therapists at the same minute.

CREATE TABLE bookings (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    therapist_id UUID NOT NULL REFERENCES therapists(id) ON DELETE CASCADE,
    scheduled_at TIMESTAMPTZ NOT NULL,
    status       TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'confirmed', 'cancelled', 'completed')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX bookings_active_slot_unique
    ON bookings (therapist_id, scheduled_at)
    WHERE status IN ('pending', 'confirmed');

CREATE UNIQUE INDEX bookings_active_user_slot_unique
    ON bookings (user_id, scheduled_at)
    WHERE status IN ('pending', 'confirmed');

-- Serves "my bookings" (newest first) and "this therapist's diary".
CREATE INDEX idx_bookings_user_id ON bookings (user_id, created_at DESC);
CREATE INDEX idx_bookings_therapist_id ON bookings (therapist_id, scheduled_at);