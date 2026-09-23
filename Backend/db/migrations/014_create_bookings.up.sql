-- Phase 6.3: therapist bookings.
--
-- A booking ties a registered user to a therapist at a scheduled time and
-- starts life as pending. Two partial unique indexes are the database-level
-- guarantee that a slot can never be double-sold:
--
--   * bookings_active_slot_unique forbids two ACTIVE bookings for the same
--     therapist at the exact same scheduled_at, no matter how many concurrent
--     INSERTs race. Cancelling (or completing) a booking removes it from the
--     guard set, freeing the slot for rebooking.
--   * bookings_active_user_slot_unique gives the same exact-minute guarantee
--     per user, so one person can't book two therapists at the same time via
--     two simultaneous requests.
--
-- Both indexes are partial (WHERE status IN ('pending','confirmed')) so the
-- constraint only applies while a booking is live.

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

-- "My bookings" (list newest-first by created_at) and "this therapist's
-- diary" reads stay cheap.
CREATE INDEX idx_bookings_user_id ON bookings (user_id, created_at DESC);
CREATE INDEX idx_bookings_therapist_id ON bookings (therapist_id, scheduled_at);