// Shared frontend types for the Soulwe API.
//
// These mirror the backend response shapes documented in Docs/API.md and the
// Go structs under Backend/internal/. Keep them in sync with the backend when
// contracts change.

// A registered Soulwe user as returned by the auth/profile endpoints. Mirrors
// Backend/internal/user/model.go (password_hash is never serialized).
export interface User {
  id: string
  email: string
  display_name: string | null
  language_pref: string
  is_verified: boolean
  created_at: string
  updated_at: string
  deleted_at: string | null
}

// POST /api/v1/auth/login
export interface LoginResponse {
  access_token: string
  token_type: string
  expires_in: number
  user: User
}

// POST /api/v1/auth/register — no tokens; the caller signs in afterwards.
export interface RegisterResponse {
  user: User
}

// GET /api/v1/auth/me — token validation that returns the user's id.
export interface MeResponse {
  user_id: string
}

// GET /api/v1/users/me
export interface ProfileResponse {
  user: User
}

// The exact mood vocabulary accepted/stored by the backend
// (Backend/internal/mood/model.go).
export type Mood = 'Heavy' | 'Okay' | 'Better' | 'At peace' | 'Grateful'

// language_pref values accepted by PATCH /users/me
// (Backend/internal/user/model.go).
export type LanguagePref = 'en' | 'sw' | 'luo' | 'kik'

// A mood check-in as returned by the mood/dashboard endpoints
// (Backend/internal/mood/model.go: id, mood, logged_at).
export interface MoodLog {
  id: string
  mood: Mood
  logged_at: string
}

// POST /api/v1/moods
export interface CreateMoodPayload {
  mood: Mood
}

// GET /api/v1/moods — newest first.
export interface MoodsResponse {
  moods: MoodLog[]
}

// PATCH /api/v1/users/me — both fields optional; an empty/whitespace
// display_name clears the stored name; language_pref must be one of
// en/sw/luo/kik.
export interface UpdateProfilePayload {
  display_name?: string
  language_pref?: LanguagePref
}

// GET /api/v1/dashboard — latest_mood is null and recent_moods is [] when the
// user has no check-ins (Backend/internal/dashboard/service.go).
export interface DashboardResponse {
  user: User
  latest_mood: MoodLog | null
  recent_moods: MoodLog[]
  mood_checkins_count: number
}

// A journal entry as returned by the journal endpoints
// (Backend/internal/journal/model.go).
//
// `content` is intentionally absent from list and create responses — the
// backend only decrypts and returns it on the single-entry endpoints
// (GET/PATCH/reflect). Ciphertext, ownership, and encryption fields are never
// serialized. There is no `updated_at` field in the backend contract.
export interface JournalEntry {
  id: string
  content?: string
  mood_tags: string[]
  prompt_used: string | null
  ai_reflection: string | null
  word_count: number
  created_at: string
}

// POST /api/v1/journal — content required; mood_tags and prompt_used optional.
export interface CreateJournalPayload {
  content: string
  mood_tags?: string[]
  prompt_used?: string
}

// PATCH /api/v1/journal/:id — all fields optional, at least one required.
// Content edits clear any stored ai_reflection.
export interface UpdateJournalPayload {
  content?: string
  mood_tags?: string[]
  prompt_used?: string
}

// {entry} envelope shared by POST, GET /:id, PATCH /:id and POST /:id/reflect.
export interface JournalEntryResponse {
  entry: JournalEntry
}

// GET /api/v1/journal — newest first. next_cursor is the created_at of the
// last entry in a full page, otherwise null.
export interface JournalListResponse {
  entries: JournalEntry[]
  next_cursor: string | null
}

// POST /api/v1/auth/anonymous — mints a fresh anonymous session. The raw
// token is returned exactly once; only its SHA-256 hash is stored server-side
// (Backend/internal/anon/). `anonymous_id` is the internal identity row — the
// frontend stores only the token and never renders this id.
export interface AnonymousSessionResponse {
  anonymous_token: string
  anonymous_id: string
}

// GET /api/v1/auth/anonymous/me — validates the anonymous token and echoes the
// authenticated identity back.
export interface AnonymousMeResponse {
  anonymous_id: string
}

// A peer circle as returned by GET /circles and GET /circles/:id
// (Backend/internal/circles/model.go). description/icon are nullable;
// is_member is always present but only meaningful on the detail endpoint (the
// list always reports false).
export interface Circle {
  id: string
  slug: string
  name: string
  description: string | null
  icon: string | null
  member_count: number
  is_member: boolean
}

// A circle chat message. The author is exposed only as the server-generated
// anon_name; identity UUIDs, device UUIDs, token hashes, and user IDs never
// leave the API. reaction_counts is always an object (possibly empty).
export interface CircleMessage {
  id: string
  anon_name: string
  content: string
  reaction_counts: Record<string, number>
  created_at: string
}

// GET /api/v1/circles
export interface CircleListResponse {
  circles: Circle[]
}

// GET /api/v1/circles/:id
export interface CircleResponse {
  circle: Circle
}

// GET /api/v1/circles/:id/messages — newest first. next_cursor is the created_at
// cursor for the previous (older) page when one exists, otherwise null.
export interface CircleMessagesResponse {
  messages: CircleMessage[]
  next_cursor: string | null
}

// POST /api/v1/circles/:id/messages
export interface CreateCircleMessagePayload {
  content: string
}

// POST /api/v1/circles/:id/messages → 201
export interface CircleMessageResponse {
  message: CircleMessage
}

// The documented error envelope: {"error": {"code", "message", "field"?}}.
export interface ApiErrorBody {
  error: {
    code: string
    message: string
    field?: string
  }
}

// Application-level error thrown by the API client for every failed request.
// `message` is always safe to display to the user.
export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly field?: string

  constructor(status: number, code: string, message: string, field?: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.field = field
  }
}

export function isApiError(err: unknown): err is ApiError {
  return err instanceof ApiError
}