// Central API client for Soulwe.

// Usage:
//   import { api } from '@/lib/api'
//   const entries = await api.journal.list()

import type {
  AuthTokens, CurrentUser,
  CreateEntryPayload, JournalEntry, PaginatedEntries,
  Mood, MoodLog,
  Circle, CircleList, CircleMessage, MessageList,
  Therapist, TherapistList, BookingRequest, Booking,
  BreathTechnique, BreathingSession,
} from '@/types'

const BASE_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080'

// ── Token management ─────────────────────────────────────────────────────────

function getToken(): string | null {
  return localStorage.getItem('sw_access_token')
}

function setTokens(tokens: AuthTokens): void {
  localStorage.setItem('sw_access_token', tokens.access_token)
  if (tokens.refresh_token) {
    localStorage.setItem('sw_refresh_token', tokens.refresh_token)
  }
}

function clearTokens(): void {
  localStorage.removeItem('sw_access_token')
  localStorage.removeItem('sw_refresh_token')
}

// ── Core fetch wrapper ────────────────────────────────────────────────────────

interface RequestOptions {
  method?: 'GET' | 'POST' | 'PUT' | 'DELETE'
  body?: unknown
  anonymous?: boolean
}

async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = 'GET', body, anonymous = false } = options

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  }

  if (!anonymous) {
    const token = getToken()
    if (token) {
      headers['Authorization'] = `Bearer ${token}`
    }
  }

  const response = await fetch(`${BASE_URL}${path}`, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })

  // Handle 401 — try to refresh the token once
  if (response.status === 401 && !anonymous) {
    const refreshed = await tryRefreshToken()
    if (refreshed) {
      // Retry the original request with the new token
      return request<T>(path, options)
    } else {
      clearTokens()
      // In Phase 4 we'll redirect to login here
      throw new Error('Session expired. Please log in again.')
    }
  }

  // Parse the response
  const data = await response.json()

  if (!response.ok) {
    // Throw the API error so callers can handle it
    throw data
  }

  return data as T
}

async function tryRefreshToken(): Promise<boolean> {
  const refreshToken = localStorage.getItem('sw_refresh_token')
  if (!refreshToken) return false

  try {
    const data = await fetch(`${BASE_URL}/auth/refresh`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: refreshToken }),
    }).then(r => r.json())

    if (data.access_token) {
      localStorage.setItem('sw_access_token', data.access_token)
      return true
    }
    return false
  } catch {
    return false
  }
}

// ── API methods ───────────────────────────────────────────────────────────────

export const api = {

  auth: {
    register: (email: string, password: string) =>
      request<{ access_token: string; refresh_token: string; user: CurrentUser }>(
        '/auth/register', { method: 'POST', body: { email, password }, anonymous: true }
      ).then(data => { setTokens(data); return data }),

    login: (email: string, password: string) =>
      request<{ access_token: string; refresh_token: string; user: CurrentUser }>(
        '/auth/login', { method: 'POST', body: { email, password }, anonymous: true }
      ).then(data => { setTokens(data); return data }),

    anonymous: () =>
      request<{ access_token: string; anon_name: string }>(
        '/auth/anonymous', { method: 'POST', anonymous: true }
      ).then(data => {
        localStorage.setItem('sw_access_token', data.access_token)
        return data
      }),

    logout: () => clearTokens(),
  },

  journal: {
    list: (cursor?: string) => {
      const params = cursor ? `?before=${encodeURIComponent(cursor)}` : ''
      return request<PaginatedEntries>(`/journal${params}`)
    },

    get: (id: string) =>
      request<{ entry: JournalEntry }>(`/journal/${id}`),

    create: (payload: CreateEntryPayload) =>
      request<{ entry: JournalEntry }>(
        '/journal', { method: 'POST', body: payload }
      ),

    delete: (id: string) =>
      request<void>(`/journal/${id}`, { method: 'DELETE' }),
  },

  mood: {
    log: (mood: Mood) =>
      request<MoodLog>('/mood', { method: 'POST', body: { mood } }),
  },

  circles: {
    list: () =>
      request<CircleList>('/circles'),

    messages: (slug: string, cursor?: string) => {
      const params = cursor ? `?before=${encodeURIComponent(cursor)}` : ''
      return request<MessageList>(`/circles/${slug}/messages${params}`)
    },

    send: (slug: string, content: string) =>
      request<{ message: CircleMessage }>(
        `/circles/${slug}/messages`, { method: 'POST', body: { content } }
      ),

    react: (messageId: string, emoji: string) =>
      request<{ reaction_counts: Record<string, number> }>(
        `/circles/messages/${messageId}/react`, { method: 'POST', body: { emoji } }
      ),

    flag: (messageId: string, reason?: string) =>
      request<{ flagged: boolean }>(
        `/circles/messages/${messageId}/flag`, { method: 'POST', body: { reason } }
      ),
  },

  therapists: {
    list: (filters?: {
      language?: string
      specialty?: string
      max_price?: number
      free_only?: boolean
      online_only?: boolean
    }) => {
      const params = new URLSearchParams()
      if (filters?.language) params.set('language', filters.language)
      if (filters?.specialty) params.set('specialty', filters.specialty)
      if (filters?.max_price) params.set('max_price', String(filters.max_price))
      if (filters?.free_only) params.set('free_only', 'true')
      if (filters?.online_only) params.set('online_only', 'true')
      const qs = params.toString()
      return request<TherapistList>(`/therapists${qs ? '?' + qs : ''}`)
    },

    book: (therapistId: string, payload: BookingRequest) =>
      request<Booking>(
        `/therapists/${therapistId}/book`, { method: 'POST', body: payload }
      ),
  },

  breathing: {
    logSession: (session: {
      technique: BreathTechnique
      breaths: number
      duration_s: number
      completed: boolean
    }) =>
      request<BreathingSession>(
        '/breathing/sessions', { method: 'POST', body: session }
      ),
  },
}