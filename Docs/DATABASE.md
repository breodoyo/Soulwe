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
 └── circle_messages
      └── message_flags

therapists
 └── therapist_availability
 └── therapist_languages

breathing_sessions
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

---

### anon_identities

Every person who uses Soulwe — registered or not — gets an anonymous
identity for circles. This table maps between real user IDs (or device UUIDs)
and the anonymous names shown in circles.

```sql
CREATE TABLE anon_identities (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID REFERENCES users(id) ON DELETE CASCADE,
    device_uuid TEXT,                      -- for unregistered users
    anon_name   TEXT NOT NULL UNIQUE,      -- e.g. "Anon Baobab"
    created_at  TIMESTAMPTZ DEFAULT NOW(),

    CONSTRAINT one_identity CHECK (
        (user_id IS NULL) != (device_uuid IS NULL)  -- exactly one must be set
    )
);
```

The `anon_name` is generated server-side from a curated list of East African
nature words (Baobab, Acacia, Savanna, Kilimanjaro, Serengeti, etc.).

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

### circle_messages

```sql
CREATE TABLE circle_messages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    circle_id       UUID NOT NULL REFERENCES circles(id) ON DELETE CASCADE,
    anon_identity_id UUID NOT NULL REFERENCES anon_identities(id),
    content         TEXT NOT NULL,
    reaction_counts JSONB DEFAULT '{}',    -- {"💙": 12, "🙏": 4}
    is_flagged      BOOLEAN DEFAULT FALSE,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_circle_messages_circle_created
    ON circle_messages(circle_id, created_at DESC);
```

**Why JSONB for reactions?**
Reaction types may change over time (adding new emojis). JSONB lets us add
new reaction types without a schema migration. The tradeoff is that we can't
do relational queries on individual reactions, but we don't need to.

---

### message_flags

When a user flags a message as harmful.

```sql
CREATE TABLE message_flags (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id  UUID NOT NULL REFERENCES circle_messages(id) ON DELETE CASCADE,
    flagged_by  UUID NOT NULL REFERENCES anon_identities(id),
    reason      TEXT,
    created_at  TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(message_id, flagged_by)         -- one flag per person per message
);
```

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

### breathing_sessions

```sql
CREATE TABLE breathing_sessions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID REFERENCES users(id) ON DELETE CASCADE,
    device_uuid TEXT,                      -- for anonymous users
    technique   TEXT NOT NULL,             -- '478' or 'box'
    breaths     INTEGER NOT NULL,
    duration_s  INTEGER NOT NULL,          -- total seconds
    completed   BOOLEAN DEFAULT FALSE,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);
```

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
```

We run them with `golang-migrate`. Each file contains both an `up` migration
(what to apply) and a corresponding `down` migration in a separate file
(how to undo it) — e.g. `001_create_users.up.sql` and
`001_create_users.down.sql`.

**Rule:** Never edit a migration file after it has been run in production.
If you need to change something, write a new migration.

---

## Design rules

1. Every table has a UUID primary key.
2. Every table has `created_at`. Tables with mutable rows also have `updated_at`.
3. Sensitive user-generated content is encrypted at rest.
4. Foreign keys always have `ON DELETE CASCADE` or `ON DELETE SET NULL` — never leave orphaned rows.
5. Use arrays (`TEXT[]`) only for simple flat lists. Use JSONB for structured variable data. Use separate tables for anything you need to query relationally.