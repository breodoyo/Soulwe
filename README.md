# Soulwe

*Your soul. Your way. Your safe space.*

Soulwe is a culturally-aware mental health companion for East Africa.

The name comes from *soul* — universal, immediately felt — and *-we*, a
Luo and Bantu personal suffix meaning "yours" or "of you." Spoken aloud
it sounds like "soul-way": a path inward. That is exactly what this is.

Soulwe provides anonymous peer support circles, AI-assisted journaling,
therapist matching, and breathing exercises — designed specifically around
African cultural contexts, languages, and the stigma patterns that prevent
people from seeking help.

---

## Who this is for

People across East Africa who are dealing with grief, family pressure, work
burnout, anxiety, or simply need a private space to process their feelings —
without judgment, without exposing their identity, and without expensive
Western-modeled therapy as the only option.

---

## Tech stack

| Layer      | Technology                           | Why                                        |
|------------|--------------------------------------|--------------------------------------------|
| Backend    | Go (Chi router)                      | Fast, simple, great for REST APIs          |
| Database   | PostgreSQL                           | Relational, reliable, handles JSON well    |
| Auth       | JWT (access) + refresh tokens        | Stateless, works on mobile and web         |
| Frontend   | React + TypeScript + Vite            | Fast dev experience, type safety           |
| Styling    | CSS Modules + design tokens          | Scoped styles, no class conflicts          |
| AI         | Anthropic Claude API                 | Journal reflections, cultural context      |
| Deploy     | Render (backend) + Vercel (frontend) | Free tiers, easy CI/CD                    |

---

## Project structure

```
soulwe/
├── README.md                  ← You are here
├── docs/                      ← All documentation
│   ├── ARCHITECTURE.md        ← How the system fits together
│   ├── API.md                 ← Every endpoint documented
│   ├── DATABASE.md            ← Schema and design decisions
│   ├── DEVELOPMENT.md         ← How to run locally
│   └── DEPLOYMENT.md          ← How to ship to production
│
├── backend/                   ← Go API server
│   ├── cmd/server/main.go     ← Entry point
│   ├── internal/              ← Private application code
│   │   ├── auth/              ← JWT, sessions, anonymous tokens
│   │   ├── journal/           ← Journal entries + AI reflection
│   │   ├── circle/            ← Peer support groups + messages
│   │   ├── therapist/         ← Therapist profiles + booking
│   │   ├── breathing/         ← Session tracking
│   │   └── user/              ← Profile, mood, preferences
│   ├── db/
│   │   ├── migrations/        ← SQL files, versioned
│   │   └── queries/           ← SQL queries
│   ├── middleware/            ← Auth, logging, CORS, rate limit
│   └── config/                ← Environment config loader
│
└── frontend/                  ← React application
    └── src/
        ├── pages/             ← Route-level components
        ├── components/        ← Reusable UI pieces
        ├── hooks/             ← Custom React hooks
        ├── lib/api.ts         ← All API calls in one place
        ├── styles/            ← Global CSS, design tokens
        └── types/             ← TypeScript interfaces
```

---

## Core features

**Anonymous identity** — Every user gets an anonymous display name (e.g.
"Anon Baobab") for circles. Real identity is never required.

**Journal with AI reflection** — Private encrypted entries. Claude reads
each entry and responds with a warm, culturally-grounded reflection rooted
in Ubuntu and African wisdom.

**Peer support circles** — Topic-based anonymous chat rooms around real
African experiences: grief, family pressure, work burnout, young adult
identity.

**Therapist matching** — Kenyan therapists filterable by language (Swahili,
Dholuo, Kikuyu), specialty, price, and availability. First sessions
subsidised.

**Breathing exercises** — Guided 4-7-8 and box breathing with animation.
Sessions logged so users can track their practice.

---

## Guiding principles

**Privacy first.** No data is sold. No ads. Anonymous by default.

**Culturally grounded.** Content, language, and design are built around East
African lived experience — not adapted from Western mental health apps.

**Accessible.** Free tier for all. SMS/USSD fallback planned for v2 via
Africa's Talking, so people without smartphones can access peer circles.

**Honest about limits.** The AI companion is not a therapist. Soulwe makes
this clear and always offers a path to human support.

---

## Development phases

- **Phase 1 (done):** Structure, documentation, type definitions
- **Phase 2:** Backend — auth, user, journal endpoints
- **Phase 3:** Backend — circles, therapist, AI integration
- **Phase 4:** Frontend — shell, navigation, journal UI
- **Phase 5:** Frontend — circles, therapist matching, breathe
- **Phase 6:** Integration, testing, deployment

---

## Getting started

See [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) to run locally.

---

## Links

- Production: https://soulwe.vercel.app *(coming soon)*
- API: https://soulwe-api.onrender.com *(coming soon)*
- GitHub: https://github.com/breodoyo/soulwe