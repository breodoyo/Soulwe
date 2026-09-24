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
  ApiErrorBody,
  CreateMoodPayload,
  DashboardResponse,
  LoginResponse,
  MeResponse,
  MoodLog,
  MoodsResponse,
  ProfileResponse,
  RegisterResponse,
  UpdateProfilePayload,
} from '@/types'
import { ApiError } from '@/types'

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080'
const API_PREFIX = '/api/v1'

const ACCESS_TOKEN_KEY = 'sw_access_token'

// ── Token management ─────────────────────────────────────────────────────────
// The access token lives in localStorage. It is attached to every
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
  // auth=false labels public endpoints (register, login) that must not carry
  // the Authorization header even when a token is present.
  auth?: boolean
}

async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = 'GET', body, auth = true } = options

  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (auth) {
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

  // Expired or invalid credentials on an authenticated request: clear the
  // stored token and bounce through the normal auth flow.
  if (response.status === 401 && auth) {
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

  // Future domain clients (journal, circles, therapists, breathing) plug
  // in here using the same `request` helper.
}