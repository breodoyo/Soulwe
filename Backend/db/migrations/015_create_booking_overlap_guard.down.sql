-- Phase 6.3 hardening: drop the 60-minute-window overlap guards added in
-- 015. The btree_gist extension is intentionally left installed (matching the
-- precedent of pgcrypto in 001, whose DOWN leaves the extension in place);
-- CREATE EXTENSION in the UP file is idempotent.

ALTER TABLE bookings DROP CONSTRAINT IF EXISTS bookings_therapist_window_excl;
ALTER TABLE bookings DROP CONSTRAINT IF EXISTS bookings_user_window_excl;

DROP FUNCTION IF EXISTS booking_window(timestamptz);