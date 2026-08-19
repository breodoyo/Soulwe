# Architecture

This document explains how Soulwe is structured and why each decision
was made. Read this before touching any code.

---

## System overview

```
                        ┌─────────────────────┐
                        │   React Frontend     │
                        │   (Vercel)           │
                        └────────┬────────────┘
                                 │ HTTPS / JSON
                        ┌────────▼────────────┐
                        │   Go API Server      │
                        │   (Render)           │
                        │                     │
                        │  ┌───────────────┐  │
                        │  │  Chi Router   │  │
                        │  └──────┬────────┘  │
                        │         │           │
                        │  ┌──────▼────────┐  │
                        │  │  Middleware   │  │
                        │  │  Auth/CORS/   │  │
                        │  │  RateLimit    │  │
                        │  └──────┬────────┘  │
                        │         │           │
                        │  ┌──────▼────────┐  │
                        │  │   Handlers    │  │
                        │  └──────┬────────┘  │
                        │         │           │
                        │  ┌──────▼────────┐  │
                        │  │   Services    │  │
                        │  └──────┬────────┘  │
                        │         │           │
                        │  ┌──────▼────────┐  │
                        │  │  Repository   │  │
                        │  └──────┬────────┘  │
                        └─────────┼───────────┘
                                  │
                  ┌───────────────┼───────────────┐
                  │               │               │
         ┌────────▼──────┐ ┌─────▼──────┐ ┌─────▼──────┐
         │  PostgreSQL   │ │ Claude API │ │  (Future)  │
         │  (Render)     │ │ (Anthropic)│ │  Redis     │
         └───────────────┘ └────────────┘ └────────────┘
```

---

## Backend layers

The backend uses a strict 3-layer architecture. Each layer has one job.

### Handler layer (`internal/*/handler.go`)

- Receives HTTP requests
- Validates input (checks that required fields exist, types are right)
- Calls the service layer
- Writes the HTTP response

A handler never talks to the database directly. It never contains business
logic. Its only job is HTTP in, HTTP out.

```
Request → Handler → Service → Repository → Database
Response ← Handler ← Service ← Repository ← Database
```

### Service layer (`internal/*/service.go`)

- Contains all business logic
- Calls the repository for data
- Calls external APIs (Claude, etc.)
- Orchestrates multi-step operations

Example: saving a journal entry is a multi-step operation:
1. Validate the entry content
2. Save it to the database (repository)
3. Call Claude API for a reflection (external)
4. Return both the saved entry and the reflection

The service knows about all of this. The handler just calls `service.SaveEntry()`.

### Repository layer (`internal/*/repository.go`)

- Only talks to the database
- Contains all SQL queries
- Returns Go structs, not raw rows
- Never contains business logic

If you want to change from PostgreSQL to something else one day, you only
change the repository. Nothing else needs to touch.

---

## Frontend structure

### Pages vs Components

**Pages** (`src/pages/`) are route-level components. Each page maps to a URL.
They fetch data, manage top-level state, and compose components.

**Components** (`src/components/`) are reusable building blocks. They receive
props, they don't fetch their own data (with a few exceptions using hooks).

### Data flow

```
Page
 ├── fetches data via hook (useJournal, useCircle, etc.)
 ├── passes data down as props
 └── handles user actions (save, send, book)
      └── calls API via lib/api.ts
```

### Custom hooks (`src/hooks/`)

Each feature has its own hook that encapsulates:
- API calls
- Loading/error state
- Local state management

This keeps pages clean and makes logic testable.

---

## Authentication design

Soulwe has two identity modes:

### Anonymous users
- Get a random anonymous name ("Anon Baobab", "Anon Willow")
- Can use circles, breathe, and limited journaling
- No email or phone required
- Token stored in localStorage

### Registered users
- Email + password (hashed with bcrypt)
- Full journal access (encrypted entries)
- Can match with therapists
- JWT access token (15 min) + refresh token (30 days)

The anonymous token system is important. Many users will never register —
especially early on, when trust is being built. The app should be useful
before it asks for anything personal.

---

## Privacy decisions

### Journal encryption
Journal entries are encrypted at rest using AES-256-GCM. The encryption key
is derived from the user's password using Argon2id. This means:
- Even if the database is breached, entries are unreadable
- If a user forgets their password, their journal is permanently lost
  (this is a feature, not a bug — it's the privacy guarantee)

This is explained clearly to users before they create an account.

### Circle anonymity
Messages in circles store a user ID for moderation purposes (flagging abuse),
but the ID maps to an anonymous name, never a real identity. The mapping is
stored separately and only accessible to the backend — it never leaves the
server.

### AI data handling
Journal content sent to Claude is never stored by Soulwe after the
reflection is returned. The Anthropic API itself has its own data policies,
which are surfaced to users in the privacy notice.

---

## Why Go for the backend?

1. Single binary deployment — easy to run on Render's free tier
2. Strong standard library — net/http, crypto, encoding/json are all built in
3. Fast — handles concurrent requests well without much configuration
4. Great fit with your Zone01 background — you already know it

## Why Chi instead of Gin or Fiber?

Chi is idiomatic Go. It uses the standard `net/http` interfaces, which means:
- Middleware is just `func(http.Handler) http.Handler`
- You learn patterns that work anywhere in Go, not just Chi
- It's lightweight — just routing, nothing else opinionated

## Why PostgreSQL?

- Reliable, mature, feature-rich
- JSON support for flexible fields (therapist availability, preferences)
- Full-text search built in (useful for searching journal entries later)
- Render free tier includes PostgreSQL

---

## What we are not building (yet)

- **Real-time circles** — Messages are polled every few seconds for now.
  WebSockets are v2 once we have users.
- **SMS/USSD** — Africa's Talking integration is v2.
- **Voice journaling** — Whisper API transcription is v3.
- **Mobile app** — The React app is mobile-first PWA. Native is v3.