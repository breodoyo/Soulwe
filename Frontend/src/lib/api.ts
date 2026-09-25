// Central API client for Soulwe.
//
// The backend serves every route under /api/v1 (see Backend/cmd/server/main.go).
// The origin (scheme + host + port) is configured once with the
// VITE_API_BASE_URL environment variable (see .env.example); it falls back to
// the local development backend when unset.
//
// Usage:
//   import { api } from '@/lib/api'
//   const { user } = await api.auth.login(email, password)

import type {
  AnonymousMeResponse,
  AnonymousSessionResponse,
  ApiErrorBody,
  CircleListResponse,
  CircleMessageResponse,
  CircleMessagesResponse,
  CircleResponse,
  CreateCircleMessagePayload,
  CreateJournalPayload,
  CreateMoodPayload,
  DashboardResponse,
  JournalEntryResponse,
  JournalListResponse,
  LoginResponse,
  MeResponse,
  MoodLog,
  MoodsResponse,
  ProfileResponse,
  RegisterResponse,
  UpdateJournalPayload,
  UpdateProfilePayload,
} from '@/types'
import { ApiError } from '@/types'

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080'
const API_PREFIX = '/api/v1'

const ACCESS_TOKEN_KEY = 'sw_access_token'
const ANON_TOKEN_KEY = 'sw_anon_token'

// ── Registered token management ──────────────────────────────────────────────
// The registered access token lives in localStorage. It is attached to every
// authenticated request and is never rendered in the UI or logged.

export function getAccessToken(): string | null {
  return localStorage.getItem(ACCESS_TOKEN_KEY)
}

export function hasAccessToken(): boolean {
  return localStorage.getItem(ACCESS_TOKEN_KEY) !== null
}

export function setAccessToken(token: string): void {
  localStorage.setItem(ACCESS_TOKEN_KEY, token)
}

export function clearAuth(): void {
  localStorage.removeItem(ACCESS_TOKEN_KEY)
}

// ── Anonymous token management ───────────────────────────────────────────────
// The anonymous session token is deliberately stored separately from the
// registered JWT: circles authenticate through the anonymous system, and the
// two flows must never share a credential. Persisting the token is what lets
// the same anonymous identity keep its circle memberships across page reloads.
// Only the raw token is stored — never the anonymous_id, never the registered
// JWT here, and never any device UUID.

export function getAnonToken(): string | null {
  return localStorage.getItem(ANON_TOKEN_KEY)
}

export function setAnonToken(token: string): void {
  localStorage.setItem(ANON_TOKEN_KEY, token)
}

export function clearAnonToken(): void {
  localStorage.removeItem(ANON_TOKEN_KEY)
}

// Registered by the auth provider so that any authenticated request that is
// rejected with 401 (expired/invalid token) can return the app to the
// unauthenticated state immediately.
type UnauthorizedHandler = () => void

let unauthorizedHandler: UnauthorizedHandler | null = null

export function setUnauthorizedHandler(handler: UnauthorizedHandler | null): void {
  unauthorizedHandler = handler
}

// ── Request options ──────────────────────────────────────────────────────────

interface RequestOptions {
  method?: 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE'
  body?: unknown
  // Which credential, if any, the request carries:
  //   auth: true         → the registered JWT (default)
  //   auth: 'anonymous'  → the anonymous sessions token (circles, anonymous/me)
  //   auth: false        → public endpoints (register, login, anonymous create)
  auth?: boolean | 'anonymous'
}

async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = 'GET', body, auth = true } = options

  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (auth === 'anonymous') {
    const token = getAnonToken()
    if (token) headers['Authorization'] = `Bearer ${token}`
  } else if (auth === true) {
    const token = getAccessToken()
    if (token) headers['Authorization'] = `Bearer ${token}`
  }

  let response: Response
  try {
    response = await fetch(`${API_BASE_URL}${API_PREFIX}${path}`, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    })
  } catch {
    throw new ApiError(
      0,
      'NETWORK_ERROR',
      'Could not reach the Soulwe server. Please check your connection and try again.',
    )
  }

  // Expired or invalid credentials on a registered request: clear the stored
  // token and bounce through the normal auth flow. Anonymous requests never
  // touch the registered JWT — they just surface the 401 for the caller to
  // renew its anonymous session.
  if (response.status === 401 && auth === true) {
    clearAuth()
    unauthorizedHandler?.()
    throw await readError(response)
  }

  if (response.status === 204) {
    return undefined as T
  }

  if (!response.ok) {
    throw await readError(response)
  }

  try {
    return (await response.json()) as T
  } catch {
    throw new ApiError(
      500,
      'INTERNAL_SERVER_ERROR',
      'The server returned an unexpected response. Please try again.',
    )
  }
}

// Turns a failed response into a user-safe ApiError. The backend already keeps
// its messages free of internal details; as a safety net, 5xx responses always
// fall back to a generic message regardless of the body.
async function readError(response: Response): Promise<ApiError> {
  if (response.status >= 500) {
    return new ApiError(
      response.status,
      'INTERNAL_SERVER_ERROR',
      'Something went wrong on our end. Please try again.',
    )
  }

  let code = 'UNKNOWN'
  let message = 'Something went wrong. Please try again.'
  let field: string | undefined

  try {
    const body = (await response.json()) as ApiErrorBody
    if (body?.error) {
      code = body.error.code || code
      message = body.error.message || message
      field = body.error.field
    }
  } catch {
    // Non-JSON body — keep the generic fallbacks above.
  }

  return new ApiError(response.status, code, message, field)
}

// ── API methods ──────────────────────────────────────────────────────────────

export const api = {
  auth: {
    register: (email: string, password: string): Promise<RegisterResponse> =>
      request<RegisterResponse>('/auth/register', {
        method: 'POST',
        body: { email, password },
        auth: false,
      }),

    login: async (email: string, password: string): Promise<LoginResponse> => {
      const data = await request<LoginResponse>('/auth/login', {
        method: 'POST',
        body: { email, password },
        auth: false,
      })
      setAccessToken(data.access_token)
      return data
    },

    // GET /auth/me — validates the stored token and returns the user's id.
    me: (): Promise<MeResponse> => request<MeResponse>('/auth/me'),

    // GET /users/me — the full authenticated profile used to restore a session.
    profile: (): Promise<ProfileResponse> => request<ProfileResponse>('/users/me'),
  },

  users: {
    // GET /users/me — the authenticated user's profile.
    me: (): Promise<ProfileResponse> => request<ProfileResponse>('/users/me'),

    // PATCH /users/me — updates display_name/language_pref (omitted fields are
    // left untouched; a blank display_name clears the stored name).
    updateProfile: (payload: UpdateProfilePayload): Promise<ProfileResponse> =>
      request<ProfileResponse>('/users/me', { method: 'PATCH', body: payload }),
  },

  moods: {
    // GET /moods — the user's check-ins, newest first. `limit` is optional
    // (backend default 20 / max 50).
    list: (params?: { limit?: number }): Promise<MoodsResponse> => {
      const query = params?.limit ? `?limit=${params.limit}` : ''
      return request<MoodsResponse>(`/moods${query}`)
    },

    // POST /moods — records a mood check-in.
    create: (payload: CreateMoodPayload): Promise<MoodLog> =>
      request<MoodLog>('/moods', { method: 'POST', body: payload }),
  },

  dashboard: {
    // GET /dashboard — the user's wellness snapshot (profile, latest + recent
    // moods, and total check-in count).
    get: (): Promise<DashboardResponse> => request<DashboardResponse>('/dashboard'),
  },

  journal: {
    // GET /journal — the user's entries, newest first. `limit` (default 20,
    // max 50) and `before` (created_at cursor, exclusive) are optional.
    list: (params?: { limit?: number; before?: string }): Promise<JournalListResponse> => {
      const query = new URLSearchParams()
      if (params?.limit) query.set('limit', String(params.limit))
      if (params?.before) query.set('before', params.before)
      const qs = query.toString()
      return request<JournalListResponse>(`/journal${qs ? `?${qs}` : ''}`)
    },

    // GET /journal/:id — a single entry including its decrypted content.
    get: (id: string): Promise<JournalEntryResponse> =>
      request<JournalEntryResponse>(`/journal/${encodeURIComponent(id)}`),

    // POST /journal — saves an entry; the server attempts an AI reflection
    // on create (best-effort, ai_reflection may be null).
    create: (payload: CreateJournalPayload): Promise<JournalEntryResponse> =>
      request<JournalEntryResponse>('/journal', { method: 'POST', body: payload }),

    // PATCH /journal/:id — updates content/mood_tags/prompt_used.
    update: (id: string, payload: UpdateJournalPayload): Promise<JournalEntryResponse> =>
      request<JournalEntryResponse>(`/journal/${encodeURIComponent(id)}`, {
        method: 'PATCH',
        body: payload,
      }),

    // DELETE /journal/:id — permanent, returns 204.
    delete: (id: string): Promise<void> =>
      request<void>(`/journal/${encodeURIComponent(id)}`, { method: 'DELETE' }),

    // POST /journal/:id/reflect — generates and stores a fresh AI reflection.
    // May throw a 503 ApiError (code AI_REFLECTION_UNAVAILABLE).
    reflect: (id: string): Promise<JournalEntryResponse> =>
      request<JournalEntryResponse>(`/journal/${encodeURIComponent(id)}/reflect`, {
        method: 'POST',
      }),
  },

  anon: {
    // POST /auth/anonymous — public. Mints an anonymous session; the raw token
    // is returned once and only its hash is stored server-side. Optional
    // X-Device-ID is intentionally not sent: the stored token alone keeps the
    // same identity across reloads, so no device identifier is invented.
    create: (): Promise<AnonymousSessionResponse> =>
      request<AnonymousSessionResponse>('/auth/anonymous', { method: 'POST', auth: false }),

    // GET /auth/anonymous/me — validates the stored anonymous token.
    me: (): Promise<AnonymousMeResponse> =>
      request<AnonymousMeResponse>('/auth/anonymous/me', { auth: 'anonymous' }),
  },

  circles: {
    // GET /circles — every active circle with a live member count.
    list: (): Promise<CircleListResponse> =>
      request<CircleListResponse>('/circles', { auth: 'anonymous' }),

    // GET /circles/:id — one circle plus the caller's membership.
    get: (id: string): Promise<CircleResponse> =>
      request<CircleResponse>(`/circles/${encodeURIComponent(id)}`, { auth: 'anonymous' }),

    // POST /circles/:id/join — 204; 409 ALREADY_MEMBER when already a member.
    join: (id: string): Promise<void> =>
      request<void>(`/circles/${encodeURIComponent(id)}/join`, {
        method: 'POST',
        auth: 'anonymous',
      }),

    // DELETE /circles/:id/leave — 204. Idempotent: leaving a circle you were
    // never in still succeeds.
    leave: (id: string): Promise<void> =>
      request<void>(`/circles/${encodeURIComponent(id)}/leave`, {
        method: 'DELETE',
        auth: 'anonymous',
      }),

    // GET /circles/:id/messages — members only. Newest first; `limit` (default
    // 20, max 50) and `before` (created_at cursor, exclusive) are optional.
    messages: (
      id: string,
      params?: { limit?: number; before?: string },
    ): Promise<CircleMessagesResponse> => {
      const query = new URLSearchParams()
      if (params?.limit) query.set('limit', String(params.limit))
      if (params?.before) query.set('before', params.before)
      const qs = query.toString()
      return request<CircleMessagesResponse>(
        `/circles/${encodeURIComponent(id)}/messages${qs ? `?${qs}` : ''}`,
        { auth: 'anonymous' },
      )
    },

    // POST /circles/:id/messages — members only. 201 returns the stored message
    // with its server-generated anon_name.
    sendMessage: (id: string, content: string): Promise<CircleMessageResponse> => {
      const payload: CreateCircleMessagePayload = { content }
      return request<CircleMessageResponse>(`/circles/${encodeURIComponent(id)}/messages`, {
        method: 'POST',
        body: payload,
        auth: 'anonymous',
      })
    },
  },

  // Future domain clients (therapists, breathing) plug
  // in here using the same `request` helper.
}