# Architecture

This document explains how Soulwe is structured and why each decision was made. Read this before touching any code.

---

## System overview

```text
                        ┌─────────────────────┐
                        │   React Frontend    │
                        │      (Vercel)       │
                        └──────────┬──────────┘
                                   │ HTTPS / JSON
                        ┌──────────▼──────────┐
                        │    Go API Server    │
                        │       (Render)      │
                        │                     │
                        │  ┌───────────────┐  │
                        │  │  Chi Router   │  │
                        │  └──────┬────────┘  │
                        │         │           │
                        │  ┌──────▼────────┐  │
                        │  │  Middleware    │  │
                        │  │ Auth/CORS/     │  │
                        │  │ RateLimit      │  │
                        │  └──────┬────────┘  │
                        │         │           │
                        │  ┌──────▼────────┐  │
                        │  │   Handlers     │  │
                        │  └──────┬────────┘  │
                        │         │           │
                        │  ┌──────▼────────┐  │
                        │  │   Services     │  │
                        │  └──────┬────────┘  │
                        │         │           │
                        │  ┌──────▼────────┐  │
                        │  │  Repository    │  │
                        │  └──────┬────────┘  │
                        └─────────┼───────────┘
                                  │
                  ┌───────────────┼───────────────┐
                  │               │               │
         ┌────────▼──────┐ ┌─────▼──────┐ ┌─────▼──────┐
         │  PostgreSQL   │ │ Claude API │ │  (Future)  │
         │    (Render)   │ │ (Anthropic)│ │    Redis    │
         └───────────────┘ └────────────┘ └────────────┘
```

---

## Backend layers

The backend uses a strict 3-layer architecture. Each layer has one job.

### Handler layer (`internal/*/handler.go`)

* Receives HTTP requests
* Validates input
* Calls the service layer
* Writes HTTP responses

Handlers never access the database directly or contain business logic.

```text
Request → Handler → Service → Repository → Database

Response ← Handler ← Service ← Repository ← Database
```

### Service layer (`internal/*/service.go`)

* Contains business logic
* Calls repositories
* Calls external services such as Claude
* Orchestrates multi-step operations

For example, saving a journal entry may involve:

1. Validate the entry
2. Save it through the repository
3. Request a reflection from Claude
4. Return the saved entry and reflection

The handler only needs to call something like `service.SaveEntry()`.

### Repository layer (`internal/*/repository.go`)

* Communicates only with the database
* Contains SQL queries
* Returns Go structs
* Contains no business logic

Keeping database access here makes the rest of the application independent of PostgreSQL-specific details.

---

## Frontend structure

### Pages vs Components

**Pages** (`src/pages/`) are route-level components. They fetch data, manage top-level state, and compose components.

**Components** (`src/components/`) are reusable UI building blocks. They receive data through props and generally do not fetch application data directly.

### Data flow

```text
Page
 ├── fetches data through hooks
 ├── passes data to components
 └── handles user actions
        └── calls API through lib/api.ts
```

### Custom hooks (`src/hooks/`)

Feature-specific hooks encapsulate:

* API calls
* Loading and error states
* Local state management

This keeps pages clean and makes feature logic easier to test.

---

## Authentication design

Soulwe has two identity modes.

### Anonymous users

* Receive a random anonymous name such as `Anon Baobab`
* Can use circles, breathing, and limited journaling
* No email or phone required
* Anonymous token stored in `localStorage`

### Registered users

* Email + password
* Passwords hashed with bcrypt
* Full journal access
* Encrypted journal entries
* Therapist matching
* JWT access token — 15 minutes
* Refresh token — 30 days

The anonymous system allows users to experience Soulwe before sharing personal information.

---

## Privacy decisions

### Journal encryption

Journal entries are encrypted at rest using **AES-256-GCM**. The encryption key is derived from the user's password using **Argon2id**.

This means:

* A database breach should not expose readable journal entries.
* If a user permanently loses their password, encrypted journal data may be unrecoverable.

This trade-off must be clearly explained before account creation.

### Circle anonymity

Circle messages store a user ID for moderation, but that ID maps to an anonymous identity rather than a real identity.

The mapping is stored separately and is accessible only by the backend.

### AI data handling

Journal content sent to Claude is not stored by Soulwe after the reflection is returned.

Anthropic's data policies are disclosed to users through Soulwe's privacy notice.

---

## Technology choices

* **Go** — Backend language; chosen for its simplicity, performance, concurrency, and straightforward deployment.
* **Chi** — HTTP router; lightweight, idiomatic Go, and built on `net/http`.
* **PostgreSQL** — Primary database; provides reliable relational storage and strong querying capabilities.
* **React** — Frontend framework for the web application and mobile-first PWA.
* **Claude API** — Provides AI-powered journal reflections.
* **Redis** — Planned for future caching, rate limiting, and real-time features.

---

## Deployment

| Component      | Platform             |
| -------------- | -------------------- |
| React Frontend | Vercel               |
| Go API         | Render               |
| PostgreSQL     | Render               |
| AI             | Anthropic Claude API |
| Redis          | Future               |

---

## What We Are Not Building Yet

### Real-time circles — V2

Messages use polling initially. WebSockets will be introduced when real-time communication becomes necessary.

### SMS / USSD — V2

Africa's Talking integration will be considered for users with limited internet access.

### Voice journaling — V3

Voice recording and transcription will be added after the core journaling experience is stable.

### Native mobile app — V3

The initial application is a mobile-first React PWA. Native Android/iOS development comes later.

---

## Architectural principles

1. **Keep layers separate** — handlers handle HTTP, services handle business logic, repositories handle persistence.
2. **Privacy by design** — minimize, protect, and control sensitive data.
3. **Keep infrastructure simple** — introduce complexity only when needed.
4. **Design for testing** — dependencies should be easy to mock and replace.
5. **Build incrementally** — prioritize real user needs over premature features.
