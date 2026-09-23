-- Phase 6.3 hardening: enforce the 60-minute session window inside the
-- database, so overlapping bookings are impossible even for concurrent
-- requests. Migration 014 already prevented exact same-{therapist,user,slot}
-- collisions via partial unique indexes; this migration extends the guarantee
-- to any two bookings whose [scheduled_at, scheduled_at + 60 minutes) windows
-- overlap.
--
-- Strategy
-- --------
-- Every booking owns the half-open window [scheduled_at, scheduled_at + 60m).
-- Two partial GiST EXCLUDE constraints guard the live bookings (pending and
-- confirmed); cancelling or completing a booking drops it from the WHERE
-- predicate, so its interval becomes immediately rebookable:
--
--   * bookings_therapist_window_excl — no two live windows may overlap for the
--     same therapist. The sequential cases work out to:
--          10:00-11:00 + 10:30-11:30  -> rejected
--          10:00-11:00 + 11:00-12:00  -> allowed (adjacent, half-open bounds)
--   * bookings_user_window_excl — one user can never hold two live sessions
--     whose windows overlap, closing the check-then-insert TOCTOU window that
--     a service-layer overlap check alone cannot close.
--
-- Why btree_gist: EXCLUDE needs the equality operator for the uuid columns
-- inside a GiST index. btree_gist is a standard contrib module (the same
-- family as the pgcrypto used in 001) that provides `=` for uuid in GiST.
-- It was confirmed available on this server and is created idempotently.
--
-- Why not keep it service-side: the service still performs a friendly overlap
-- pre-check, but under concurrency two requests can both pass it. GiST
-- exclusion conflicts are resolved natively by PostgreSQL's speculative
-- insertion: the losing INSERT blocks until the winner commits and then fails
-- with SQLSTATE 23P01, which the bookings repository maps to the existing
-- ErrBookingConflict -> 409 BOOKING_CONFLICT contract.

CREATE EXTENSION IF NOT EXISTS btree_gist;

-- booking_window computes the half-open session window for a start time. The
-- PostgreSQL catalog marks `timestamptz + interval` STABLE, but EXCLUDE index
-- expressions must be IMMUTABLE, so the (deterministic) computation lives in a
-- small IMMUTABLE wrapper. Adding a fixed 60-minute interval to a timestamptz
-- is independent of session state, making the stricter marking safe.
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