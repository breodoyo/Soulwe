-- Drops the 60-minute overlap guards. btree_gist is left installed, matching
-- pgcrypto in 001; the UP's CREATE EXTENSION is idempotent.

ALTER TABLE bookings DROP CONSTRAINT IF EXISTS bookings_therapist_window_excl;
ALTER TABLE bookings DROP CONSTRAINT IF EXISTS bookings_user_window_excl;

DROP FUNCTION IF EXISTS booking_window(timestamptz);