-- Extend 014's exact-slot partial unique indexes to any two bookings whose
-- half-open [scheduled_at, scheduled_at + 60m) windows overlap. Adjacent
-- bookings (10:00-11:00 + 11:00-12:00) stay allowed. Cancelled or completed
-- bookings drop out of the WHERE predicate, so their interval is rebookable.
--
-- The service keeps a friendly overlap pre-check, but under concurrency two
-- requests can both pass it; PostgreSQL's speculative insertion blocks the
-- losing INSERT until the winner commits, then fails it with SQLSTATE 23P01,
-- which the repository maps to ErrBookingConflict.
--
-- btree_gist supplies `=` for uuid so the uuid columns can sit inside GiST.

CREATE EXTENSION IF NOT EXISTS btree_gist;

-- EXCLUDE index expressions must be IMMUTABLE, but `timestamptz + interval` is
-- only STABLE; adding a fixed interval is session-state independent, so this
-- wrapper can mark it IMMUTABLE safely.
CREATE OR REPLACE FUNCTION booking_window(start_at timestamptz)
RETURNS tstzrange
LANGUAGE sql
IMMUTABLE
STRICT
AS $$ SELECT tstzrange(start_at, start_at + interval '60 minutes') $$;

ALTER TABLE bookings
    ADD CONSTRAINT bookings_therapist_window_excl
    EXCLUDE USING gist (
        therapist_id WITH =,
        booking_window(scheduled_at) WITH &&
    ) WHERE (status IN ('pending', 'confirmed'));

ALTER TABLE bookings
    ADD CONSTRAINT bookings_user_window_excl
    EXCLUDE USING gist (
        user_id WITH =,
        booking_window(scheduled_at) WITH &&
    ) WHERE (status IN ('pending', 'confirmed'));