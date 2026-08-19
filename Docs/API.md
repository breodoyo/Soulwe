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

**Response `200`:**
```json
{
  "access_token": "eyJ...",
  "anon_name": "Anon Willow"
}
```

---

### Journal

All journal endpoints require a registered user token (not anonymous).

#### `GET /journal`
List the user's journal entries, newest first.

**Query params:**
- `limit` — default 20, max 50
- `before` — cursor (ISO timestamp) for pagination

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
It's only returned in the single-entry endpoint, decrypted server-side.

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

The AI reflection is generated server-side. If the Claude API fails, the
entry is still saved and `ai_reflection` is `null`.

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

---

#### `DELETE /journal/:id`
Permanently delete an entry. This is immediate and irreversible.

**Response `204`:** no body

---

### Mood

#### `POST /mood`
Log a mood without writing a journal entry.

**Request:**
```json
{
  "mood": "At peace"
}
```

**Response `201`:**
```json
{
  "id": "uuid",
  "mood": "At peace",
  "logged_at": "2026-08-19T10:00:00Z"
}
```

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