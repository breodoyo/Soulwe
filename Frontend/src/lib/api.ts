import type {
  AnonymousMeResponse,
  AnonymousSessionResponse,
  ApiErrorBody,
  BookingResponse,
  BookingsResponse,
  BreathingExerciseResponse,
  BreathingExercisesResponse,
  BreathingSessionResponse,
  BreathingSessionsResponse,
  CircleListResponse,
  CircleMessageResponse,
  CircleMessagesResponse,
  CircleResponse,
  CreateBookingPayload,
  CreateBreathingSessionPayload,
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
  TherapistResponse,
  TherapistsResponse,
  UpdateJournalPayload,
  UpdateProfilePayload,
} from '@/types'
import { ApiError, type CredentialSent } from '@/types'

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080'
const API_PREFIX = '/api/v1'

const ACCESS_TOKEN_KEY = 'sw_access_token'
const ANON_TOKEN_KEY = 'sw_anon_token'

// Stored in localStorage; never rendered or logged.
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

// Separate from the JWT so signing in cannot orphan a guest's rows.
export function getAnonToken(): string | null {
  return localStorage.getItem(ANON_TOKEN_KEY)
}

export function setAnonToken(token: string): void {
  localStorage.setItem(ANON_TOKEN_KEY, token)
}

export function clearAnonToken(): void {
  localStorage.removeItem(ANON_TOKEN_KEY)
}

type UnauthorizedHandler = () => void

let unauthorizedHandler: UnauthorizedHandler | null = null

export function setUnauthorizedHandler(handler: UnauthorizedHandler | null): void {
  unauthorizedHandler = handler
}

// Shared in-flight promise so StrictMode and concurrent callers mint exactly one session.
let anonSessionInFlight: Promise<void> | null = null

export async function ensureAnonSession(): Promise<void> {
  if (anonSessionInFlight) return anonSessionInFlight

  const run = (async () => {
    const existing = getAnonToken()
    if (existing) {
      try {
        await request<AnonymousMeResponse>('/auth/anonymous/me', { auth: 'anonymous' })
        return
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) {
          clearAnonToken()
        } else {
          // A fault must not discard a token that may still be good.
          throw err
        }
      }
    }
    const { anonymous_token } = await request<AnonymousSessionResponse>('/auth/anonymous', {
      method: 'POST',
      auth: false,
    })
    setAnonToken(anonymous_token)
  })()

  anonSessionInFlight = run
  try {
    return await run
  } finally {
    anonSessionInFlight = null
  }
}

// true = JWT, 'anonymous' = anon token, 'either' = JWT else anon, false = public.
interface RequestOptions {
  method?: 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE'
  body?: unknown
  auth?: boolean | 'anonymous' | 'either'
}

// The JWT wins so an account never mixes with an anonymous session's rows.
async function attachEitherCredential(
  headers: Record<string, string>,
): Promise<'registered' | 'anonymous' | 'none'> {
  const accessToken = getAccessToken()
  if (accessToken) {
    headers['Authorization'] = `Bearer ${accessToken}`
    return 'registered'
  }
  await ensureAnonSession()
  const anonToken = getAnonToken()
  if (anonToken) {
    headers['Authorization'] = `Bearer ${anonToken}`
    return 'anonymous'
  }
  return 'none'
}

async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = 'GET', body, auth = true } = options

  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  let sent: 'registered' | 'anonymous' | 'none' = auth === false ? 'none' : 'registered'
  if (auth === 'anonymous') {
    sent = 'none'
    const token = getAnonToken()
    if (token) {
      headers['Authorization'] = `Bearer ${token}`
      sent = 'anonymous'
    }
  } else if (auth === 'either') {
    sent = await attachEitherCredential(headers)
  } else if (auth === true) {
    const token = getAccessToken()
    if (token) headers['Authorization'] = `Bearer ${token}`
    else sent = 'none'
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

  if (response.status === 401 && sent === 'registered') {
    clearAuth()
    unauthorizedHandler?.()
    throw await readError(response, sent)
  }

  if (response.status === 401 && sent === 'anonymous') {
    clearAnonToken()
    throw await readError(response, sent)
  }

  if (response.status === 204) {
    return undefined as T
  }

  if (!response.ok) {
    throw await readError(response, sent)
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

// Stamps the credential because storage cannot say which session ended.
async function readError(response: Response, credential: CredentialSent = 'none'): Promise<ApiError> {
  if (response.status >= 500) {
    return new ApiError(
      response.status,
      'INTERNAL_SERVER_ERROR',
      'Something went wrong on our end. Please try again.',
      undefined,
      credential,
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
    // Non-JSON body: keep the generic error above.
  }

  return new ApiError(response.status, code, message, field, credential)
}

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

    me: (): Promise<MeResponse> => request<MeResponse>('/auth/me'),

    profile: (): Promise<ProfileResponse> => request<ProfileResponse>('/users/me'),
  },

  users: {
    me: (): Promise<ProfileResponse> => request<ProfileResponse>('/users/me'),

    // Omitted fields are untouched; a blank display_name clears it.
    updateProfile: (payload: UpdateProfilePayload): Promise<ProfileResponse> =>
      request<ProfileResponse>('/users/me', { method: 'PATCH', body: payload }),
  },

  moods: {
    list: (params?: { limit?: number }): Promise<MoodsResponse> => {
      const query = params?.limit ? `?limit=${params.limit}` : ''
      return request<MoodsResponse>(`/moods${query}`, { auth: 'either' })
    },

    create: (payload: CreateMoodPayload): Promise<MoodLog> =>
      request<MoodLog>('/moods', { method: 'POST', body: payload, auth: 'either' }),
  },

  dashboard: {
    get: (): Promise<DashboardResponse> => request<DashboardResponse>('/dashboard'),
  },

  journal: {
    list: (params?: { limit?: number; before?: string }): Promise<JournalListResponse> => {
      const query = new URLSearchParams()
      if (params?.limit) query.set('limit', String(params.limit))
      if (params?.before) query.set('before', params.before)
      const qs = query.toString()
      return request<JournalListResponse>(`/journal${qs ? `?${qs}` : ''}`, { auth: 'either' })
    },

    get: (id: string): Promise<JournalEntryResponse> =>
      request<JournalEntryResponse>(`/journal/${encodeURIComponent(id)}`, { auth: 'either' }),

    create: (payload: CreateJournalPayload): Promise<JournalEntryResponse> =>
      request<JournalEntryResponse>('/journal', {
        method: 'POST',
        body: payload,
        auth: 'either',
      }),

    // Content edits clear any stored ai_reflection.
    update: (id: string, payload: UpdateJournalPayload): Promise<JournalEntryResponse> =>
      request<JournalEntryResponse>(`/journal/${encodeURIComponent(id)}`, {
        method: 'PATCH',
        body: payload,
        auth: 'either',
      }),

    delete: (id: string): Promise<void> =>
      request<void>(`/journal/${encodeURIComponent(id)}`, {
        method: 'DELETE',
        auth: 'either',
      }),

    reflect: (id: string): Promise<JournalEntryResponse> =>
      request<JournalEntryResponse>(`/journal/${encodeURIComponent(id)}/reflect`, {
        method: 'POST',
        auth: 'either',
      }),
  },

  anon: {
    // X-Device-ID is intentionally not sent; the stored token alone keeps the identity.
    create: (): Promise<AnonymousSessionResponse> =>
      request<AnonymousSessionResponse>('/auth/anonymous', { method: 'POST', auth: false }),

    me: (): Promise<AnonymousMeResponse> =>
      request<AnonymousMeResponse>('/auth/anonymous/me', { auth: 'anonymous' }),
  },

  // Either credential works; the two identities are never merged server-side.
  circles: {
    list: (): Promise<CircleListResponse> =>
      request<CircleListResponse>('/circles', { auth: 'either' }),

    get: (id: string): Promise<CircleResponse> =>
      request<CircleResponse>(`/circles/${encodeURIComponent(id)}`, { auth: 'either' }),

    join: (id: string): Promise<void> =>
      request<void>(`/circles/${encodeURIComponent(id)}/join`, {
        method: 'POST',
        auth: 'either',
      }),

    leave: (id: string, removeMyMessages = false): Promise<void> =>
      request<void>(
        `/circles/${encodeURIComponent(id)}/leave${removeMyMessages ? '?removeMyMessages=true' : ''}`,
        {
          method: 'DELETE',
          auth: 'either',
        },
      ),

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
        { auth: 'either' },
      )
    },

    sendMessage: (id: string, content: string): Promise<CircleMessageResponse> => {
      const payload: CreateCircleMessagePayload = { content }
      return request<CircleMessageResponse>(`/circles/${encodeURIComponent(id)}/messages`, {
        method: 'POST',
        body: payload,
        auth: 'either',
      })
    },

    deleteMessage: (id: string, messageId: string): Promise<void> =>
      request<void>(
        `/circles/${encodeURIComponent(id)}/messages/${encodeURIComponent(messageId)}`,
        {
          method: 'DELETE',
          auth: 'either',
        },
      ),
  },

  therapists: {
    list: (params?: { language?: string; specialty?: string; before?: string }): Promise<TherapistsResponse> => {
      const query = new URLSearchParams()
      if (params?.language) query.set('language', params.language)
      if (params?.specialty) query.set('specialty', params.specialty)
      if (params?.before) query.set('before', params.before)
      const qs = query.toString()
      return request<TherapistsResponse>(`/therapists${qs ? `?${qs}` : ''}`, { auth: false })
    },

    get: (id: string): Promise<TherapistResponse> =>
      request<TherapistResponse>(`/therapists/${encodeURIComponent(id)}`, { auth: false }),
  },

  bookings: {
    create: (therapistId: string, payload: CreateBookingPayload): Promise<BookingResponse> =>
      request<BookingResponse>(`/therapists/${encodeURIComponent(therapistId)}/bookings`, {
        method: 'POST',
        body: payload,
      }),

    list: (): Promise<BookingsResponse> => request<BookingsResponse>('/bookings'),

    get: (id: string): Promise<BookingResponse> =>
      request<BookingResponse>(`/bookings/${encodeURIComponent(id)}`),

    cancel: (id: string): Promise<BookingResponse> =>
      request<BookingResponse>(`/bookings/${encodeURIComponent(id)}/cancel`, {
        method: 'PATCH',
      }),
  },

  breathe: {
    exercises: (params?: { limit?: number }): Promise<BreathingExercisesResponse> => {
      const query = new URLSearchParams()
      if (params?.limit) query.set('limit', String(params.limit))
      const qs = query.toString()
      return request<BreathingExercisesResponse>(
        `/breathing/exercises${qs ? `?${qs}` : ''}`,
        { auth: false },
      )
    },

    exercise: (id: string): Promise<BreathingExerciseResponse> =>
      request<BreathingExerciseResponse>(
        `/breathing/exercises/${encodeURIComponent(id)}`,
        { auth: false },
      ),

    sessions: {
      record: (payload: CreateBreathingSessionPayload): Promise<BreathingSessionResponse> =>
        request<BreathingSessionResponse>('/breathing/sessions', {
          method: 'POST',
          body: payload,
          auth: 'either',
        }),

      list: (params?: { limit?: number }): Promise<BreathingSessionsResponse> => {
        const query = params?.limit ? `?limit=${params.limit}` : ''
        return request<BreathingSessionsResponse>(
          `/breathing/sessions${query ? `?${query}` : ''}`,
          { auth: 'either' },
        )
      },
    },
  },
}