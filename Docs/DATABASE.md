# Database

Soulwe uses PostgreSQL. This document covers the schema, the reasoning
behind each design decision, and the migration strategy.

---

## Schema overview

```
users
 └── journal_entries
 └── mood_logs
 └── anon_identities   (one per user, for circles)
 └── therapist_matches

circles
 └── circle_members
 └── circle_messages
      └── message_flags

therapists
 └── therapist_availability
 └── therapist_languages

breathing_exercises
 └── breathing_sessions
```

---

## Tables

### users

Stores registered accounts. Anonymous users are not in this table —
they use the `anon_identities` table with a device-generated UUID.

```sql
CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,           -- bcrypt, cost 12
    display_name  TEXT,                    -- optional, shown to therapists only
    language_pref TEXT DEFAULT 'en',       -- 'en', 'sw', 'luo', 'kik'
    is_verified   BOOLEAN DEFAULT FALSE,
    created_at    TIMESTAMPTZ DEFAULT NOW(),
    updated_at    TIMESTAMPTZ DEFAULT NOW(),
    deleted_at    TIMESTAMPTZ             -- soft delete
);
```

**Why UUID instead of integer IDs?**
UUIDs don't expose how many users you have, can't be guessed in sequence,
and work well if you ever shard the database.

**Why soft delete (`deleted_at`)?**
Mental health data has special sensitivity. If a user deletes their account,
we keep a soft-delete record for 30 days before hard deletion, in case they
want to recover. The data is inaccessible to queries during this window.

**Profile updates (`PATCH /users/me`):** only `display_name` and
`language_pref` are editable. `display_name` is trimmed and capped at 100
characters; an empty value is stored as `null`. `language_pref` is lowercased
and must be one of `en`, `sw`, `luo`, `kik`. `password_hash` is never returned
to API clients.

---

### anon_identities

Every person who uses Soulwe — registered or not — gets an anonymous
identity for circles. This table maps between real user IDs (or device UUIDs)
and the anonymous names shown in circles.

```sql
CREATE TABLE anon_identities (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID REFERENCES users(id) ON DELETE CASCADE,
    device_uuid TEXT,                      -- optional, for idempotency per device
    token_hash  TEXT,                      -- SHA-256 of the anonymous session token
    anon_name   TEXT NOT NULL UNIQUE,      -- e.g. "Anon Baobab"
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ DEFAULT NOW(),

    CONSTRAINT anon_identity_binding CHECK (
        user_id IS NOT NULL OR token_hash IS NOT NULL
        OR device_uuid IS NOT NULL  -- every identity belongs to a user, a session, or a phase-2 device
    )
);

CREATE UNIQUE INDEX idx_anon_identities_token_hash
    ON anon_identities (token_hash)
    WHERE token_hash IS NOT NULL;

CREATE UNIQUE INDEX idx_anon_identities_device_uuid
    ON anon_identities (device_uuid)
    WHERE device_uuid IS NOT NULL;
```

The `anon_name` is generated server-side from a curated list of East African
nature words (Baobab, Acacia, Savanna, Kilimanjaro, Serengeti, etc.).

When a user registers, their `anon_identities` row may be linked via `user_id`;
for anonymous sessions the identity is bound to the `token_hash` instead (only
the SHA-256 hash is ever stored — the raw token is shown to the client once and
never persisted). Device UUIDs are optional metadata so a returning device gets
the same `anonymous_id` with a rotated token.

**Anonymous → registered promotion** (`POST /auth/anonymous/promote`):
An anonymous session's identity can be promoted to a registered account by
setting `user_id` on its existing `anon_identities` row. The row itself is
preserved (its ID, `anon_name`, `token_hash`, and timestamps stay intact) so
circle history tied to the anonymous identity survives the promotion. Creating
the user and linking the identity happens in one transaction; the identity row
is locked (`SELECT ... FOR UPDATE`) so two concurrent promotions of the same
identity cannot both succeed. The `user_id` unique index additionally
guarantees an identity can be linked to at most one account, and the email
uniqueness constraint keeps promotion from creating a second account for an
already-registered (or soft-deleted) email.

---

### journal_entries

```sql
CREATE TABLE journal_entries (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content_enc     BYTEA NOT NULL,        -- AES-256-GCM encrypted
    content_iv      BYTEA NOT NULL,        -- initialisation vector (per entry)
    mood_tags       TEXT[],                -- ['Anxious', 'Hopeful']
    prompt_used     TEXT,                  -- which prompt chip they tapped
    ai_reflection   TEXT,                  -- Claude's response (plaintext, not encrypted)
    word_count      INTEGER,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);
```

**Why encrypt content but not ai_reflection?**
The AI reflection is Claude's output, not the user's private thoughts. We may
display aggregate (anonymised) insights from reflections in the future to
improve prompts. The user's own words stay encrypted.

**Encryption contract (implemented in Phase 5):**
- A single server-side AES-256-GCM key comes from `JOURNAL_ENCRYPTION_KEY`
  (32 bytes) — it is never stored in the database or committed to code.
- Each entry mints a fresh random 12-byte IV stored in `content_iv`; the
  ciphertext lives in `content_enc`. Encrypting the same text twice never
  produces the same ciphertext.
- The ciphertext is bound to its owner: decryption authenticates against the
  entry's `user_id` (as additional authenticated data), so one user's
  encrypted row cannot be decrypted or swapped under another user's identity.
- Decryption happens only in the service layer, and only to return an entry to
  its owner. Journal plaintext and ciphertext never appear in API responses
  (list payloads exclude `content` entirely), logs, or errors.

**Journal types:**
The schema has no journal-type column; the product defines a single regular
journal entry. No "prayer journal" (or other) type is modelled, and the API
does not accept a type field.

**Index for performance:**
```sql
CREATE INDEX idx_journal_entries_user_created
    ON journal_entries(user_id, created_at DESC);
```

---

### mood_logs

Separate from journal entries — a user might log their mood without writing
anything. This powers the mood trend chart (v2 feature).

```sql
CREATE TABLE mood_logs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mood        TEXT NOT NULL,             -- 'Heavy', 'Okay', 'Better', 'At peace', 'Grateful'
    logged_at   TIMESTAMPTZ DEFAULT NOW()
);
```

Mood check-ins are always queried per user, newest first (list, latest, and
count), so `mood_logs` needs an index that mirrors the journal approach:

```sql
CREATE INDEX idx_mood_logs_user_logged
    ON mood_logs(user_id, logged_at DESC);
```

Added in migration `012_add_mood_logs_user_index`.

---

### circles

The topic-based anonymous chat rooms.

```sql
CREATE TABLE circles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug        TEXT UNIQUE NOT NULL,      -- 'grief', 'work-pressure', 'family'
    name        TEXT NOT NULL,
    description TEXT,
    icon        TEXT,                      -- emoji
    is_active   BOOLEAN DEFAULT TRUE,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);
```

Circles are seeded at startup, not user-created. This keeps the quality high
and prevents abuse. New circles are added by the team.

---

### circle_members

Membership in a circle. A member is **either** a registered user or an
anonymous identity, never both — migration `018` added the `user_id` side
alongside `anon_identity_id` so a signed-in member can take part too.

```sql
CREATE TABLE circle_members (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    circle_id        UUID NOT NULL REFERENCES circles(id) ON DELETE CASCADE,
    anon_identity_id UUID REFERENCES anon_identities(id) ON DELETE CASCADE,
    user_id          UUID REFERENCES users(id) ON DELETE CASCADE,
    joined_at        TIMESTAMPTZ DEFAULT NOW(),

    CONSTRAINT circle_members_one_owner
        CHECK ((user_id IS NOT NULL) <> (anon_identity_id IS NOT NULL))
);

-- One join per identity. The original single UNIQUE(circle_id, anon_identity_id)
-- is replaced by one partial index per owner kind, because a plain
-- UNIQUE(circle_id, anon_identity_id, user_id) would not catch NULLs: in
-- Postgres NULLs are distinct, so a second anonymous join would slip through
-- as (circle, anon, NULL) twice.
CREATE UNIQUE INDEX idx_circle_members_circle_anon
    ON circle_members(circle_id, anon_identity_id)
    WHERE anon_identity_id IS NOT NULL;
CREATE UNIQUE INDEX idx_circle_members_circle_user
    ON circle_members(circle_id, user_id)
    WHERE user_id IS NOT NULL;

-- The anonymous side is already indexed by idx_circle_members_anon_identity
-- (migration 013) for cascade deletes and "which circles has this person
-- joined". This is its registered twin, added for the same reasons.
CREATE INDEX idx_circle_members_user ON circle_members(user_id);
```

- `circle_members_one_owner` enforces exactly one owner, so a row can never be
  ownerless (unattributable) or doubly owned (ambiguous). The `<>` between two
  `IS NOT NULL` tests is the XOR that expresses "exactly one".
- Migration 018 drops the original `circle_members_circle_id_anon_identity_id_key`
  and replaces it with the two partial indexes. The anonymous index reproduces
  the previous guarantee exactly, and joining twice still raises a unique
  violation, which the service maps to `409 ALREADY_MEMBER`.
- Anonymous and registered memberships are counted together for the public
  member count, and neither can see the other's rows.
- Live member counts are a `COUNT` over `circle_members` grouped by circle.
- Leaving is an idempotent `DELETE` (no-op when the row is absent).

**Why does promotion not move old memberships?** A user who promotes an
anonymous identity still has their older `anon_identity_id` rows intact —
nothing rewrites them to the new `user_id`. That matches migration `017`'s
behaviour for the other anonymous-owned tables: history keeps the identity it
was written under. The trade-off is that a promoted user does not see the
circles they joined anonymously before signing up. Rewriting ownership in
place would be a silent, irreversible change to data attributed to a
pseudonym, so it is left as a follow-up rather than done automatically.

---

### circle_messages

```sql
CREATE TABLE circle_messages (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    circle_id        UUID NOT NULL REFERENCES circles(id) ON DELETE CASCADE,
    anon_identity_id UUID REFERENCES anon_identities(id) ON DELETE CASCADE,
    content          TEXT NOT NULL,
    reaction_counts  JSONB DEFAULT '{}',    -- {"💙": 12, "🙏": 4}
    is_flagged       BOOLEAN DEFAULT FALSE,
    user_id          UUID REFERENCES users(id) ON DELETE CASCADE,
    created_at       TIMESTAMPTZ DEFAULT NOW(),

    CONSTRAINT circle_messages_one_owner
        CHECK ((user_id IS NOT NULL) <> (anon_identity_id IS NOT NULL))
);

CREATE INDEX idx_circle_messages_circle_created
    ON circle_messages(circle_id, created_at DESC);

-- Author-side lookups ("which rooms has this person posted in"). The
-- (circle_id, created_at DESC) index above already serves every read of a
-- room, newest first, for both owner kinds.
CREATE INDEX idx_circle_messages_anon
    ON circle_messages(anon_identity_id) WHERE anon_identity_id IS NOT NULL;
CREATE INDEX idx_circle_messages_user
    ON circle_messages(user_id) WHERE user_id IS NOT NULL;
```

**Author name is resolved, not stored.** The message row keeps no name at all.
Reads `LEFT JOIN` `anon_identities` and `users` to derive what the API returns:
an anonymous author's pseudonym, or a registered author's `display_name`
(falling back to the literal `Member` so an email can never surface). Renaming
a profile therefore updates the circle history immediately, and deleting a
user removes their messages with them — neither would be true of a denormalized
name column. Both owner columns cascade, so either identity vanishing takes its
messages with it.

**Why JSONB for reactions?**
Reaction types may change over time (adding new emojis). JSONB lets us add
new reaction types without a schema migration. The tradeoff is that we can't
do relational queries on individual reactions, but we don't need to.

---

### message_flags

When a user flags a message as harmful. Ownership is dual here too, kept
consistent with `circle_messages` even though flagging is not implemented yet.

```sql
CREATE TABLE message_flags (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id  UUID NOT NULL REFERENCES circle_messages(id) ON DELETE CASCADE,
    flagged_by  UUID REFERENCES anon_identities(id),
    reason      TEXT,
    user_id     UUID REFERENCES users(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ DEFAULT NOW(),

    CONSTRAINT message_flags_one_owner
        CHECK ((user_id IS NOT NULL) <> (flagged_by IS NOT NULL))
);

CREATE UNIQUE INDEX idx_message_flags_message_anon
    ON message_flags(message_id, flagged_by) WHERE flagged_by IS NOT NULL;
CREATE UNIQUE INDEX idx_message_flags_message_user
    ON message_flags(message_id, user_id) WHERE user_id IS NOT NULL;
```

The original `UNIQUE(message_id, flagged_by)` (`message_flags_message_id_flagged_by_key`)
is dropped and replaced, because a single multi-column unique cannot express
"one flag per owner" across two owner columns — and once either side is
nullable, NULLs being distinct would let the same anonymous person flag the
same message repeatedly. The partial indexes reproduce the original guarantee
and add the registered one.

---

### therapists

```sql
CREATE TABLE therapists (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    full_name       TEXT NOT NULL,
    credentials     TEXT NOT NULL,         -- "PhD Clinical Psychology · Kenyatta University"
    years_exp       INTEGER,
    bio             TEXT,
    photo_url       TEXT,
    location        TEXT,                  -- "Nairobi / Online"
    is_online_only  BOOLEAN DEFAULT FALSE,
    price_kes       INTEGER,               -- price per session in KES
    free_sessions   INTEGER DEFAULT 0,     -- number of free sessions offered
    specialties     TEXT[],               -- ['Grief', 'Trauma', 'Family']
    is_active       BOOLEAN DEFAULT TRUE,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);
```

---

### therapist_languages

```sql
CREATE TABLE therapist_languages (
    therapist_id UUID NOT NULL REFERENCES therapists(id) ON DELETE CASCADE,
    language     TEXT NOT NULL,            -- 'Swahili', 'Dholuo', 'Kikuyu', 'English'
    proficiency  TEXT DEFAULT 'fluent',
    PRIMARY KEY  (therapist_id, language)
);
```

Stored separately so we can filter therapists by language with a simple JOIN
rather than searching inside an array.

---

### breathing_exercises

```sql
CREATE TABLE breathing_exercises (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL,
    technique   TEXT NOT NULL CHECK (technique IN ('478', 'box')),
    inhale_s    INTEGER NOT NULL CHECK (inhale_s > 0),
    hold_s      INTEGER NOT NULL DEFAULT 0 CHECK (hold_s >= 0),
    exhale_s    INTEGER NOT NULL CHECK (exhale_s > 0),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

The catalog is seeded by migration `016_create_breathing_exercises` with the
same two techniques the frontend already recognizes:

| slug | name          | technique | inhale_s | hold_s | exhale_s |
| ---- | ------------- | --------- | -------- | ------ | -------- |
| 478  | 4-7-8 Breathing | 478      | 4        | 7      | 8        |
| box  | Box Breathing | box       | 4        | 4      | 4        |

### breathing_sessions

```sql
CREATE TABLE breathing_sessions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID REFERENCES users(id) ON DELETE CASCADE,
    device_uuid TEXT,                       -- for anonymous users
    exercise_id UUID REFERENCES breathing_exercises(id) ON DELETE SET NULL,
    technique   TEXT NOT NULL,              -- '478' or 'box'
    breaths     INTEGER NOT NULL,
    duration_s  INTEGER NOT NULL,           -- total seconds
    completed   BOOLEAN DEFAULT FALSE,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);
```

One ownership marker must be set (`user_id` XOR `device_uuid`). Phase 6.4
records sessions for registered users only, so `user_id` is set and
`device_uuid` stays NULL. Migration `016_create_breathing_exercises` adds the
`exercise_id` column (and a matching index) to this table.

---

### bookings

```sql
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
```

A booking starts as `pending`; cancelling transitions `pending → cancelled`.
`confirmed` and `completed` are reserved for later therapist-side flows.

Every live booking occupies the half-open session window
`[scheduled_at, scheduled_at + 60 minutes)`, and a session can **never** be
double-sold. Two layers enforce it, both database-side so racing requests
cannot slip past:

- **Exact-minute guard (014).** Two partial unique indexes cover live bookings
  (pending or confirmed); cancelling a booking lets its slot be rebooked:
  - `bookings_active_slot_unique` on `(therapist_id, scheduled_at)` — the exact
    same therapist at the exact same minute cannot be booked twice.
  - `bookings_active_user_slot_unique` on `(user_id, scheduled_at)` — one user
    cannot book the exact same minute twice.
- **Window guard (015).** Two partial GiST `EXCLUDE` constraints reject any two
  live bookings whose 60-minute windows *overlap*, around a small IMMUTABLE
  `booking_window(timestamptz)` helper that maps a start time to its half-open
  window:
  - `bookings_therapist_window_excl` — no overlapping windows for the same
    therapist: 10:00–11:00 + 10:30–11:30 is rejected, while 10:00–11:00 +
    11:00–12:00 is allowed (adjacent windows only touch).
  - `bookings_user_window_excl` — one user cannot hold two live windows that
    overlap in time, closing the check-then-insert TOCTOU race a service check
    could not.
  These constraints need `btree_gist` (a standard contrib module, same family
  as `pgcrypto` already used in 001), which migration 015 creates idempotently.
  A concurrent loser fails with SQLSTATE `23P01`, which the repository maps to
  the same `ErrBookingConflict` as the `23505` unique violations — the API just
  sees `409 BOOKING_CONFLICT`.
  Migration 015 adds no columns or tables; `migrate_test.go`'s expected-table
  list is unchanged.

The service layer still runs a friendly overlap pre-check for the common serial
cases, but it is no longer the enforcement point.

Two b-tree indexes keep the common reads cheap:
- `idx_bookings_user_id` on `(user_id, created_at DESC)` — "my bookings" list.
- `idx_bookings_therapist_id` on `(therapist_id, scheduled_at)` — a future
  therapist diary view.

---

## Migration strategy

Migrations live in `backend/db/migrations/` and are numbered sequentially:

```
001_create_users.sql
002_create_anon_identities.sql
003_create_journal_entries.sql
004_create_mood_logs.sql
005_create_circles.sql
006_create_circle_messages.sql
007_create_message_flags.sql
008_create_therapists.sql
009_create_breathing_sessions.sql
010_seed_circles.sql
011_add_anon_session_identity.sql
012_add_mood_logs_user_index.sql
013_create_circle_members.sql
014_create_bookings.sql
015_create_booking_overlap_guard.sql
016_create_breathing_exercises.sql
017_add_anon_ownership.sql
018_circle_registered_ownership.sql
```

We run them with `golang-migrate`. Each file contains both an `up` migration
(what to apply) and a corresponding `down` migration in a separate file
(how to undo it) — e.g. `001_create_users.up.sql` and
`001_create_users.down.sql`.

**Rule:** Never edit a migration file after it has been run in production.
If you need to change something, write a new migration.

**Widening an owner column (`017`, `018`).** Both migrations take a
`NOT NULL` anonymous owner column and add a nullable `user_id` alternative,
guarded by a `<>`-between-`IS NOT NULL` CHECK plus one partial unique index per
owner kind. These migrations are not reversible in the strict sense: `down`
cannot recover which anonymous identity a registered-owned row "really" belonged
to, so it **deletes** registered-owned rows before dropping the column.
Anonymous data is always preserved. That direction is the safe one to lose.

---

## Design rules

1. Every table has a UUID primary key.
2. Every table has `created_at`. Tables with mutable rows also have `updated_at`.
3. Sensitive user-generated content is encrypted at rest.
4. Foreign keys always have `ON DELETE CASCADE` or `ON DELETE SET NULL` — never leave orphaned rows.
5. Use arrays (`TEXT[]`) only for simple flat lists. Use JSONB for structured variable data. Use separate tables for anything you need to query relationally.