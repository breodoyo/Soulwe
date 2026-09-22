# API Reference

All endpoints return JSON. All timestamps are ISO 8601 UTC.
All IDs are UUIDs.

Base URL: `https://api.soulwe.app` (production)
Local:    `http://localhost:8080`

---

## Authentication

Most endpoints require a Bearer token in the Authorization header:

```
Authorization: Bearer <access_token>
```

Access tokens expire in 15 minutes. Use the refresh endpoint to get a new one.
Anonymous users get a short-lived token that grants access to circles and
basic features without registration.

Protected endpoints are picky about token type:

- **Registered-user endpoints** (`/auth/me`, `/users/me`, `/moods`,
  `/dashboard`) require a registered-user **JWT** and reject anonymous tokens
  with `401`.
- **Anonymous endpoints** (`/auth/anonymous/me`, `/auth/anonymous/promote`)
  require the opaque anonymous token and reject registered-user JWTs with
  `401`.

---

## Endpoints

### Auth

#### `POST /auth/register`
Create a new account.

**Request:**
```json
{
  "email": "bree@example.com",
  "password": "minimum-12-chars"
}
```

**Response `201`:**
```json
{
  "access_token": "eyJ...",
  "refresh_token": "eyJ...",
  "user": {
    "id": "uuid",
    "email": "bree@example.com",
    "anon_name": "Anon Baobab",
    "language_pref": "en",
    "created_at": "2026-08-19T10:00:00Z"
  }
}
```

**Errors:**
- `400` — missing fields, password too short
- `409` — email already registered

---

#### `POST /auth/login`
**Request:**
```json
{
  "email": "bree@example.com",
  "password": "your-password"
}
```

**Response `200`:** same shape as register

---

#### `POST /auth/refresh`
Get a new access token using a refresh token.

**Request:**
```json
{
  "refresh_token": "eyJ..."
}
```

**Response `200`:**
```json
{
  "access_token": "eyJ..."
}
```

---

#### `POST /auth/anonymous`
Get a token for anonymous access (circles, breathe, limited journal).
No body required.

Optional `X-Device-ID` header (a UUID) makes the call idempotent per device:
re-registering with the same device UUID keeps the same `anonymous_id` and
rotates the token.

**Response `201`:**
```json
{
  "anonymous_token": "V2rH...",
  "anonymous_id": "uuid"
}
```
The raw `anonymous_token` is shown exactly once — only its SHA-256 hash is
stored. Send it later as `Authorization: Bearer <anonymous_token>` to access
anonymous-protected endpoints (e.g. `GET /auth/anonymous/me`).

---

#### `POST /auth/anonymous/promote`
Upgrade an anonymous session into a registered account. The authenticated
anonymous identity is preserved and linked to the new account (its ID and
existing identity data remain intact) — it is not deleted.

Requires an anonymous bearer token (`Authorization: Bearer <anonymous_token>`),
not a registered-user JWT.

**Request:**
```json
{
  "email": "bree@example.com",
  "password": "minimum-12-chars",
  "display_name": "Bree"
}
```

`display_name` is optional; an empty or whitespace-only value is stored as
`null`.

**Response `201`:** same shape as login
```json
{
  "access_token": "eyJ...",
  "token_type": "Bearer",
  "expires_in": 900,
  "user": {
    "id": "uuid",
    "email": "bree@example.com",
    "display_name": "Bree",
    "language_pref": "en",
    "is_verified": false,
    "created_at": "2026-08-19T10:00:00Z"
  }
}
```

The user creation and the anonymous-identity link happen in one database
transaction, so promotion is atomic and can only succeed once per anonymous
identity. The returned `access_token` is a normal 15-minute registered JWT.

**Errors:**
- `400` — missing fields, password too short, invalid email
- `401` — missing or invalid anonymous token
- `409` — email already registered, or this anonymous identity was already promoted

---

### Profile

Registered-user profile endpoints. The identity always comes from the bearer
JWT — a user can only ever read or update their **own** profile, and the
response never includes `password_hash` or account flags.

#### `GET /users/me`
Read the authenticated user's profile.

**Response `200`:**
```json
{
  "user": {
    "id": "uuid",
    "email": "bree@example.com",
    "display_name": "Bree",
    "language_pref": "sw",
    "is_verified": false,
    "created_at": "2026-08-19T10:00:00Z",
    "updated_at": "2026-08-19T10:00:00Z",
    "deleted_at": null
  }
}
```

**Errors:**
- `401` — missing or invalid JWT
- `404` — account no longer exists (e.g. deleted)

---

#### `PATCH /users/me`
Update select profile fields. Only `display_name` and `language_pref` are
accepted; `id`, `email`, `password`, `is_verified`, and timestamps sent in the
body are ignored.

**Request:**
```json
{
  "display_name": "Bree",
  "language_pref": "sw"
}
```

Both fields are optional — an omitted field is left untouched. Send
`"display_name": ""` (or whitespace-only) to clear it to `null`.
`language_pref` must be one of `en`, `sw`, `luo`, `kik`.

**Response `200`:** the updated profile, same shape as `GET /users/me`.

**Errors:**
- `400` — malformed JSON, `language_pref` not in the supported set, or
  `display_name` longer than 100 characters (with the offending `field`)
- `401` — missing or invalid JWT
- `404` — account no longer exists

---

### Mood (check-ins)

Mood check-ins are separate from journal entries — a registered user can log
their mood without writing anything. All mood endpoints are scoped to the
authenticated user.

#### `POST /moods`
Log a mood for the authenticated user.

**Request:**
```json
{
  "mood": "At peace"
}
```

Valid mood values (exact, case-sensitive): `Heavy`, `Okay`, `Better`,
`At peace`, `Grateful`.

**Response `201`:**
```json
{
  "id": "uuid",
  "mood": "At peace",
  "logged_at": "2026-08-19T10:00:00Z"
}
```

**Errors:**
- `400` — malformed JSON, or `mood` not in the valid set (field `mood`)
- `401` — missing or invalid JWT

---

#### `GET /moods`
List the authenticated user's mood check-ins, newest first.

**Query params:**
- `limit` — default 20, max 50

**Response `200`:**
```json
{
  "moods": [
    { "id": "uuid", "mood": "At peace", "logged_at": "2026-08-19T10:00:00Z" }
  ]
}
```

Returns `{"moods": []}` when the user has no check-ins.

**Errors:**
- `400` — `limit` is not a positive integer (field `limit`)
- `401` — missing or invalid JWT

---

### Dashboard

#### `GET /dashboard`
The authenticated user's wellness snapshot: profile plus their mood summary.
`latest_mood` is `null`, `recent_moods` `[]`, and `mood_checkins_count` `0`
when the user has no check-ins yet.

**Response `200`:**
```json
{
  "user": {
    "id": "uuid",
    "email": "bree@example.com",
    "display_name": "Bree",
    "language_pref": "sw",
    "is_verified": false,
    "created_at": "2026-08-19T10:00:00Z",
    "updated_at": "2026-08-19T10:00:00Z",
    "deleted_at": null
  },
  "latest_mood": {
    "id": "uuid",
    "mood": "At peace",
    "logged_at": "2026-08-19T10:00:00Z"
  },
  "recent_moods": [
    { "id": "uuid", "mood": "At peace", "logged_at": "2026-08-19T10:00:00Z" }
  ],
  "mood_checkins_count": 3
}
```

**Errors:**
- `401` — missing or invalid JWT
- `404` — account no longer exists

---

### Journal

All journal endpoints require a registered user token (not anonymous). Anonymous
tokens are rejected.

Journal text is always encrypted in the database (AES-256-GCM). The server
decrypts content only to build a response for the entry's own owner; ciphertext
never reaches the client and ownership is never taken from the request body.

#### `GET /journal`
List the user's journal entries, newest first.

**Query params:**
- `limit` — default 20, max 50
- `before` — cursor (ISO timestamp) for pagination, exclusive

**Response `200`:**
```json
{
  "entries": [
    {
      "id": "uuid",
      "mood_tags": ["Anxious", "Hopeful"],
      "prompt_used": "My day",
      "word_count": 142,
      "ai_reflection": "What you're carrying sounds heavy...",
      "created_at": "2026-08-19T08:30:00Z"
    }
  ],
  "next_cursor": "2026-08-18T08:30:00Z"
}
```

Note: `content` (the actual journal text) is NOT returned in the list.
It's only returned in the single-entry endpoints, decrypted server-side.
`next_cursor` is the creation timestamp of the last entry in a full page; when
the page is not full (no more entries), it is `null`.

---

#### `POST /journal`
Save a new journal entry.

**Request:**
```json
{
  "content": "Today was hard. Mama called again...",
  "mood_tags": ["Overwhelmed", "Loved"],
  "prompt_used": "Family & pressure"
}
```

**Response `201`:**
```json
{
  "entry": {
    "id": "uuid",
    "mood_tags": ["Overwhelmed", "Loved"],
    "prompt_used": "Family & pressure",
    "word_count": 47,
    "ai_reflection": "Your love for your mother comes through even in the hardest moments...",
    "created_at": "2026-08-19T10:00:00Z"
  }
}
```

The AI reflection is generated server-side. If the Claude API fails (or is not
configured), the entry is still saved and `ai_reflection` is `null`. The
reflection is stored alongside the entry; use `POST /journal/:id/reflect` to
request a fresh one on demand.

---

#### `GET /journal/:id`
Get a single entry including the decrypted content.

**Response `200`:**
```json
{
  "entry": {
    "id": "uuid",
    "content": "Today was hard. Mama called again...",
    "mood_tags": ["Overwhelmed", "Loved"],
    "prompt_used": "Family & pressure",
    "word_count": 47,
    "ai_reflection": "...",
    "created_at": "2026-08-19T10:00:00Z"
  }
}
```

**Errors:**
- `404` — entry not found or belongs to another user

---

#### `PATCH /journal/:id`
Update an existing entry. All fields are optional; at least one must be
provided. Passing `"prompt_used": ""` clears the stored prompt.

**Request:**
```json
{
  "content": "Today was hard. Mama called again... and I called her back.",
  "mood_tags": ["Overwhelmed", "Grateful"],
  "prompt_used": "Family & pressure"
}
```

**Response `200`:** same shape as `GET /journal/:id` (content decrypted
server-side).

Editing `content` re-encrypts it and clears any stored `ai_reflection` (it is
now stale); re-request it with `POST /journal/:id/reflect`.

---

#### `DELETE /journal/:id`
Permanently delete an entry. This is immediate and irreversible.

**Response `204`:** no body

---

#### `POST /journal/:id/reflect`
Generate (and store) a fresh AI reflection for an existing entry. The journal
text is decrypted server-side and only that text is sent to the Claude API;
credentials, tokens, and keys never leave the server.

**Response `200`:**
```json
{
  "entry": {
    "id": "uuid",
    "content": "Today was hard. Mama called again...",
    "mood_tags": ["Overwhelmed", "Loved"],
    "word_count": 47,
    "ai_reflection": "A fresh reflection...",
    "created_at": "2026-08-19T10:00:00Z"
  }
}
```

**Errors:**
- `404` — entry not found or belongs to another user
- `503` — AI reflection is temporarily unavailable (returns a safe message,
  with no key or configuration details)

---

### Circles

#### `GET /circles`
List all active circles with live member counts.

**Response `200`:**
```json
{
  "circles": [
    {
      "id": "uuid",
      "slug": "grief",
      "name": "Grief & loss circle",
      "description": "Navigating death and mourning in African families",
      "icon": "🕊️",
      "online_count": 8
    }
  ]
}
```

---

#### `GET /circles/:slug/messages`
Get recent messages in a circle.

**Query params:**
- `limit` — default 30, max 100
- `before` — cursor (ISO timestamp) for pagination

**Response `200`:**
```json
{
  "messages": [
    {
      "id": "uuid",
      "anon_name": "Anon Baobab",
      "content": "Lost my father last month...",
      "reaction_counts": { "💙": 12, "🙏": 4 },
      "created_at": "2026-08-19T09:45:00Z"
    }
  ]
}
```

---

#### `POST /circles/:slug/messages`
Send an anonymous message to a circle.

**Request:**
```json
{
  "content": "I understand this so deeply..."
}
```

**Response `201`:**
```json
{
  "message": {
    "id": "uuid",
    "anon_name": "Anon Willow",
    "content": "I understand this so deeply...",
    "reaction_counts": {},
    "created_at": "2026-08-19T10:01:00Z"
  }
}
```

---

#### `POST /circles/messages/:id/react`
Add or toggle a reaction on a message.

**Request:**
```json
{
  "emoji": "💙"
}
```

**Response `200`:**
```json
{
  "reaction_counts": { "💙": 13 }
}
```

---

#### `POST /circles/messages/:id/flag`
Flag a message as harmful.

**Request:**
```json
{
  "reason": "This content could be harmful to someone in crisis"
}
```

**Response `201`:** `{ "flagged": true }`

---

### Therapists

#### `GET /therapists`
Search and filter therapist profiles.

**Query params:**
- `language` — e.g. `Swahili`, `Dholuo`
- `specialty` — e.g. `Grief`, `Trauma`
- `max_price` — in KES
- `free_only` — `true` to show only therapists with free sessions
- `online_only` — `true`

**Response `200`:**
```json
{
  "therapists": [
    {
      "id": "uuid",
      "full_name": "Dr. Amina Korir",
      "credentials": "PhD Clinical Psychology · Kenyatta University",
      "years_exp": 8,
      "location": "Nairobi / Online",
      "price_kes": 800,
      "free_sessions": 2,
      "specialties": ["Grief", "Trauma", "Family"],
      "languages": ["Swahili", "English"],
      "next_available": "2026-08-19T16:00:00Z"
    }
  ]
}
```

---

#### `POST /therapists/:id/book`
Request a booking. Sends a notification to the therapist.

**Request:**
```json
{
  "preferred_time": "2026-08-20T10:00:00Z",
  "notes": "I have been dealing with grief after losing a parent"
}
```

**Response `201`:**
```json
{
  "booking_id": "uuid",
  "status": "pending",
  "therapist_name": "Dr. Amina Korir",
  "message": "Dr. Korir will confirm within 24 hours."
}
```

---

### Breathing

#### `POST /breathing/sessions`
Log a completed breathing session.

**Request:**
```json
{
  "technique": "478",
  "breaths": 5,
  "duration_s": 95,
  "completed": true
}
```

**Response `201`:**
```json
{
  "id": "uuid",
  "technique": "478",
  "breaths": 5,
  "duration_s": 95,
  "created_at": "2026-08-19T10:00:00Z"
}
```

---

## Error format

All errors follow the same shape:

```json
{
  "error": {
    "code": "INVALID_INPUT",
    "message": "password must be at least 12 characters",
    "field": "password"
  }
}
```

`field` is only present for validation errors tied to a specific field.

---

## Error codes

| Code                | HTTP | Meaning                                      |
|---------------------|------|----------------------------------------------|
| `INVALID_INPUT`     | 400  | Malformed request or failed validation       |
| `UNAUTHORIZED`      | 401  | Missing or invalid token                     |
| `FORBIDDEN`         | 403  | Token valid but user lacks permission        |
| `NOT_FOUND`         | 404  | Resource does not exist                      |
| `CONFLICT`          | 409  | Duplicate resource (e.g. email taken)        |
| `RATE_LIMITED`      | 429  | Too many requests                            |
| `INTERNAL`          | 500  | Something went wrong on our side             |

---

## Rate limits

| Endpoint group     | Limit                        |
|--------------------|------------------------------|
| Auth (login/register) | 10 requests / minute / IP |
| Journal POST       | 20 requests / hour / user    |
| Circle messages    | 30 messages / hour / user    |
| AI reflection      | Included in Journal POST     |
| All others         | 100 requests / minute / user |