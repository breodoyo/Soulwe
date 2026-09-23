-- Phase 6.4: curated breathing-exercise catalog plus a session link to it.
--
-- breathing_exercises is a read-only catalog of guided breathing exercises.
-- The seed rows reuse the technique vocabulary the product already uses
-- ('478' and 'box'), keeping the catalog compatible with free-form
-- breathing_sessions.technique values. Discovery endpoints expose only the
-- public columns of this table never internal identifiers.
--
-- breathing_sessions (009) already stores completion sessions keyed to a
-- registered user (user_id) or a device (device_uuid), so no session table is
-- created here. This migration only adds an optional exercise link so the
-- "verify the exercise exists" guarantee is also enforced by a foreign key:
-- any row written through the Phase 6.4 API must reference a catalog
-- exercise. Existing device-based rows keep a NULL exercise_id (and their
-- free-form technique), so the released 009 schema stays backward compatible.

CREATE TABLE breathing_exercises (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL,
    technique   TEXT NOT NULL,
    inhale_s    INTEGER NOT NULL CHECK (inhale_s > 0),
    hold_s      INTEGER NOT NULL CHECK (hold_s >= 0),
    exhale_s    INTEGER NOT NULL CHECK (exhale_s > 0),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- The catalog is curated, so discovery lists exercises in their defined
-- (insertion) order rather than newest-first.
CREATE INDEX idx_breathing_exercises_order
    ON breathing_exercises (created_at, id);

ALTER TABLE breathing_sessions
    ADD COLUMN exercise_id UUID REFERENCES breathing_exercises(id) ON DELETE SET NULL;

CREATE INDEX idx_breathing_sessions_exercise
    ON breathing_sessions (exercise_id);

INSERT INTO breathing_exercises (slug, name, description, technique, inhale_s, hold_s, exhale_s) VALUES
    ('478', '4-7-8 Breathing',
     'Inhale for 4s, hold for 7s, exhale for 8s - calms anxiety quickly',
     '478', 4, 7, 8),
    ('box', 'Box Breathing',
     'Inhale 4s, hold 4s, exhale 4s, hold 4s - resets stress in a steady rhythm',
     'box', 4, 4, 4);