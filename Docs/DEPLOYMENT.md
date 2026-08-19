# Deployment

Soulwe deploys the backend to Render and the frontend to Vercel.
Both have generous free tiers suitable for an MVP.

---

## Backend → Render

### First-time setup

1. Push your code to GitHub
2. Go to https://render.com → New → Web Service
3. Connect your GitHub repo
4. Set these values:

| Field            | Value                              |
|------------------|------------------------------------|
| Name             | soulwe-api                    |
| Environment      | Go                                 |
| Build command    | `go build -o server ./cmd/server`  |
| Start command    | `./server`                         |
| Plan             | Free                               |

5. Add environment variables (same as your local `.env`, with real values):
   - `DATABASE_URL` — Render provides this when you add a PostgreSQL database
   - `JWT_SECRET`
   - `JWT_REFRESH_SECRET`
   - `ANTHROPIC_API_KEY`
   - `JOURNAL_ENCRYPTION_KEY`
   - `ENV=production`
   - `PORT=8080`

### Add a PostgreSQL database

Inside Render dashboard → New → PostgreSQL

Copy the **Internal Database URL** and set it as `DATABASE_URL` in your
web service environment variables.

### Run migrations on deploy

Add this to your `render.yaml` (root of repo):

```yaml
services:
  - type: web
    name: soulwe-api
    env: go
    buildCommand: go build -o server ./cmd/server
    startCommand: ./server
    envVars:
      - key: ENV
        value: production

  - type: pserv
    name: soulwe-db
    env: postgresql
    plan: free
```

Create a migration script at `backend/scripts/migrate.sh`:
```bash
#!/bin/bash
go run ./cmd/migrate up
```

---

## Frontend → Vercel

### First-time setup

1. Go to https://vercel.com → New Project
2. Import your GitHub repo
3. Set root directory to `frontend`
4. Vercel auto-detects Vite — accept the defaults

### Environment variables

In Vercel → Project → Settings → Environment Variables:

| Variable        | Value                              |
|-----------------|------------------------------------|
| `VITE_API_URL`  | `https://soulwe-api.onrender.com` |

### Auto-deploy

Every push to `main` triggers a deploy on Vercel automatically.
Pushes to other branches create preview deployments at temporary URLs
— useful for testing before merging.

---

## After deployment checklist

- [ ] Visit `https://your-api.onrender.com/health` → should return `{"status":"ok"}`
- [ ] Open the Vercel URL → app should load
- [ ] Try registering an account end-to-end
- [ ] Write a journal entry and confirm AI reflection appears
- [ ] Open a circle and send a message
- [ ] Check Render logs for any errors: Dashboard → your service → Logs

---

## Free tier limits (know these)

### Render free tier
- Web service spins down after 15 minutes of inactivity
- First request after spin-down takes ~30 seconds (cold start)
- 750 hours/month compute (enough for one always-on service)
- PostgreSQL: 1GB storage, 97 connection limit

### Vercel free tier
- 100GB bandwidth/month
- Unlimited deployments
- Custom domain supported

### Anthropic API
- Pay-as-you-go, no free tier
- Journal reflection costs ~$0.001 per entry (Claude Sonnet)
- Budget alert: set a $10 limit in the Anthropic console while in development

---

## Custom domain (optional)

Buy a `.co.ke` domain from Kenya Network Information Centre (KeNIC) at
approximately KES 1,500/year. Point it to Vercel following their
custom domain docs.

For the API subdomain (`api.soulwe.co.ke`), add it to your Render
service under Settings → Custom Domains.

---

## Monitoring

Free options to add once deployed:

**Uptime monitoring:** UptimeRobot (free) — pings your `/health` endpoint
every 5 minutes and emails you if it's down. Also keeps Render from
spinning down (set ping interval to 14 minutes).

**Error tracking:** Sentry free tier — add the Go SDK to the backend and
the React SDK to the frontend. Captures stack traces from production errors.

**Logs:** Render shows the last 500 lines of logs in the dashboard.
For persistent logging, add a Papertrail drain (free tier: 50MB/month).