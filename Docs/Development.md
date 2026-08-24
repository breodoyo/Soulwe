# Development guide

How to run Soulwe on your machine from scratch.

---

## Prerequisites

Install these before anything else:

| Tool        | Version  | Install                                |
|-------------|----------|----------------------------------------|
| Go          | 1.22+    | https://go.dev/dl                      |
| Node.js     | 20+      | https://nodejs.org                     |
| PostgreSQL  | 15+      | https://www.postgresql.org/download    |
| Git         | any      | already installed on most systems      |

Verify your installs:
```bash
go version       # should print go1.22.x or higher
node --version   # should print v20.x.x or higher
psql --version   # should print psql (PostgreSQL) 15.x or higher
```

---

## 1. Clone the repo

```bash
git clone https://github.com/breodoyo/soulwe.git
cd soulwe
```

---

## 2. Set up the database

Create the database locally:

```bash
psql -U postgres
```

Inside the psql shell:
```sql
CREATE DATABASE soulwe_dev;
CREATE USER soulwe WITH PASSWORD 'devpassword';
GRANT ALL PRIVILEGES ON DATABASE soulwe_dev TO soulwe;
\q
```

Run migrations (once we have them):
```bash
cd backend
go run ./cmd/migrate up
```

---

## 3. Configure environment variables

Copy the example file and fill in your values:

```bash
cp backend/.env.example backend/.env
```

Edit `backend/.env`:

```env
# Server
PORT=8080
ENV=development

# Database
DATABASE_URL=postgres://soulwe:devpassword@localhost:5432/soulwe_dev?sslmode=disable

# JWT
JWT_SECRET=replace-this-with-a-random-64-char-string
JWT_REFRESH_SECRET=replace-this-with-a-different-random-64-char-string

# Anthropic
ANTHROPIC_API_KEY=sk-ant-...

# Encryption (for journal entries)
JOURNAL_ENCRYPTION_KEY=replace-with-32-random-bytes-hex
```

Generate a random JWT secret:
```bash
openssl rand -hex 32
```

Get your Anthropic API key at: https://console.anthropic.com

---

## 4. Run the backend

```bash
cd backend
go mod download        # install dependencies
go run ./cmd/server    # start the server
```

You should see:
```
Soulwe API starting on :8080
Database connected
Ready.
```

Test it:
```bash
curl http://localhost:8080/health
# {"status": "ok"}
```

---

## 5. Set up the frontend

```bash
cd frontend
npm install
cp .env.example .env.local
```

Edit `frontend/.env.local`:
```env
VITE_API_URL=http://localhost:8080
```

Run the dev server:
```bash
npm run dev
```

Open http://localhost:5173 in your browser.

---

## 6. Common commands

```bash
# Backend
go run ./cmd/server          # start server
go test ./...                # run all tests
go test ./internal/journal/  # test one package
go build ./cmd/server        # build binary

# Frontend
npm run dev          # dev server with hot reload
npm run build        # production build
npm run typecheck    # TypeScript check without building
npm run lint         # ESLint

# Database
go run ./cmd/migrate up      # apply all migrations
go run ./cmd/migrate down 1  # roll back last migration
go run ./cmd/migrate status  # see what's applied
```

---

## 7. Project conventions

### Git branches

```
main          ← production, always deployable
dev           ← integration branch, merge features here first
feat/journal  ← feature branches
fix/auth-bug  ← fix branches
```

Never commit directly to `main`.

### Commit messages

```
feat: add journal entry encryption
fix: handle Claude API timeout gracefully
docs: update API reference for /mood endpoint
refactor: extract journal service from handler
test: add unit tests for auth middleware
```

### Go conventions

- One package per feature (`internal/journal`, `internal/auth`, etc.)
- Files named by role: `handler.go`, `service.go`, `repository.go`, `types.go`
- Errors wrapped with context: `fmt.Errorf("journal.Save: %w", err)`
- No global state — everything passed through function arguments or context

### TypeScript conventions

- Components in PascalCase: `JournalEntry.tsx`
- Hooks prefixed with `use`: `useJournal.ts`
- Types in `types/` or colocated if used by one component only
- No `any` — if you don't know the type yet, use `unknown`

---

## 8. Folder quick-reference

```
backend/cmd/server/main.go       ← start here when reading backend code
backend/internal/auth/           ← JWT, login, register
backend/internal/journal/        ← entries, AI reflection
backend/db/migrations/           ← SQL files
frontend/src/pages/              ← one file per screen
frontend/src/components/ui/      ← Button, Input, Badge etc.
frontend/src/lib/api.ts          ← all API calls in one place
frontend/src/hooks/              ← useJournal, useCircle etc.
```

---

## 9. Troubleshooting

**`dial tcp: connection refused` on startup**
Your PostgreSQL isn't running. Start it:
```bash
sudo service postgresql start   # Linux
brew services start postgresql  # macOS
```

**`pq: role "soulwe" does not exist`**
You skipped step 2. Run the database setup commands again.

**`invalid API key` from Anthropic**
Check that `ANTHROPIC_API_KEY` in your `.env` starts with `sk-ant-` and has
no extra spaces or quotes.

**Frontend shows blank page**
Open browser devtools (F12) → Console tab. The error message there will tell
you what's wrong. Common cause: `VITE_API_URL` is pointing at the wrong port.