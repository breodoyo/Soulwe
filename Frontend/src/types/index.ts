// Mirror the backend response shapes in Docs/API.md; keep them in sync when contracts change.

// User mirrors Backend/internal/user/model.go (password_hash is never serialized).
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

export interface MeResponse {
  user_id: string
}

export interface ProfileResponse {
  user: User
}

// The exact mood vocabulary the backend accepts and stores.
export type Mood = 'Heavy' | 'Okay' | 'Better' | 'At peace' | 'Grateful'

// Language-preference values accepted by PATCH /users/me.
export type LanguagePref = 'en' | 'sw' | 'luo' | 'kik'

export interface MoodLog {
  id: string
  mood: Mood
  logged_at: string
}

export interface CreateMoodPayload {
  mood: Mood
}

// Newest first.
export interface MoodsResponse {
  moods: MoodLog[]
}

// Both fields optional; an empty/whitespace display_name clears the stored name.
export interface UpdateProfilePayload {
  display_name?: string
  language_pref?: LanguagePref
}

// latest_mood is null and recent_moods is [] when the user has no check-ins.
export interface DashboardResponse {
  user: User
  latest_mood: MoodLog | null
  recent_moods: MoodLog[]
  mood_checkins_count: number
}

// `content` is intentionally absent from list and create responses; the backend only decrypts it on the single-entry endpoints, and there is no `updated_at`.
export interface JournalEntry {
  id: string
  content?: string
  mood_tags: string[]
  prompt_used: string | null
  ai_reflection: string | null
  word_count: number
  created_at: string
}

// Content required; mood_tags and prompt_used optional.
export interface CreateJournalPayload {
  content: string
  mood_tags?: string[]
  prompt_used?: string
}

// All fields optional, at least one required.
export interface UpdateJournalPayload {
  content?: string
  mood_tags?: string[]
  prompt_used?: string
}

export interface JournalEntryResponse {
  entry: JournalEntry
}

// Newest first; next_cursor is the created_at of the last entry in a full page, otherwise null.
export interface JournalListResponse {
  entries: JournalEntry[]
  next_cursor: string | null
}

// The raw token is returned once; only its hash is stored. `anonymous_id` is internal and never rendered.
export interface AnonymousSessionResponse {
  anonymous_token: string
  anonymous_id: string
}

export interface AnonymousMeResponse {
  anonymous_id: string
}

// `icon` is a stable key (grief|work|family|relationships|growth), not a glyph; it was emoji until migration 019_circle_icon_keys.
export interface Circle {
  id: string
  slug: string
  name: string
  description: string | null
  icon: string | null
  member_count: number
  is_member: boolean
}

// The author is a resolved display name: their own when registered, a server pseudonym when anonymous (IsAnonymous says which); identities never leave the API.
export interface CircleMessage {
  id: string
  author_name: string
  is_anonymous: boolean
  content: string
  reaction_counts: Record<string, number>
  created_at: string
}

export interface CircleListResponse {
  circles: Circle[]
}

export interface CircleResponse {
  circle: Circle
}

// Newest first; next_cursor is the created_at cursor for the previous page when one exists, otherwise null.
export interface CircleMessagesResponse {
  messages: CircleMessage[]
  next_cursor: string | null
}

export interface CreateCircleMessagePayload {
  content: string
}

export interface CircleMessageResponse {
  message: CircleMessage
}

// bio and session_price are nullable; currency is the fixed "KES" constant; internal fields are never serialized by the backend.
export interface Therapist {
  id: string
  display_name: string
  bio: string | null
  languages: string[]
  specialties: string[]
  session_price: number | null
  currency: string
  is_active: boolean
  is_online_only: boolean
}

// Newest first; next_cursor is the created_at of the last therapist in a full page, otherwise null.
export interface TherapistsResponse {
  therapists: Therapist[]
  next_cursor: string | null
}

export interface TherapistResponse {
  therapist: Therapist
}

// Booking statuses, matching the backend CHECK constraint.
export type BookingStatus = 'pending' | 'confirmed' | 'cancelled' | 'completed'

// The owner is never serialized; the therapist is id plus display_name.
export interface Booking {
  id: string
  therapist_id: string
  display_name: string
  scheduled_at: string
  status: BookingStatus
  created_at: string
  updated_at: string
}

// ISO 8601 timestamp, must be in the future; sessions are a fixed 60-minute window.
export interface CreateBookingPayload {
  scheduled_at: string
}

export interface BookingResponse {
  booking: Booking
}

// The authenticated user's own bookings, newest first.
export interface BookingsResponse {
  bookings: Booking[]
}

// Seed catalog ('478' and 'box'); durations are in seconds; created_at is never serialized.
export interface BreathingExercise {
  id: string
  slug: string
  name: string
  description: string
  technique: string
  inhale_s: number
  hold_s: number
  exhale_s: number
}

// In the catalog's defined order.
export interface BreathingExercisesResponse {
  exercises: BreathingExercise[]
}

export interface BreathingExerciseResponse {
  exercise: BreathingExercise
}

// exercise_id is null only for legacy device sessions; duration is client-supplied seconds.
export interface BreathingSession {
  id: string
  exercise_id: string | null
  technique: string
  name?: string | null
  breaths: number
  duration_s: number
  completed: boolean
  created_at: string
}

// breaths and duration_s must be positive integers; completed is optional.
export interface CreateBreathingSessionPayload {
  exercise_id: string
  breaths: number
  duration_s: number
  completed?: boolean
}

export interface BreathingSessionResponse {
  session: BreathingSession
}

// The user's history, newest first (default limit 20, max 50).
export interface BreathingSessionsResponse {
  sessions: BreathingSession[]
}

export interface ApiErrorBody {
  error: {
    code: string
    message: string
    field?: string
  }
}

// Stamped so a 401 handler knows which session ended; storage cannot answer it.
export type CredentialSent = 'registered' | 'anonymous' | 'none'

// Thrown by the API client for every failed request; `message` is always safe to display.
export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly field?: string
  readonly credential: CredentialSent

  constructor(
    status: number,
    code: string,
    message: string,
    field?: string,
    credential: CredentialSent = 'none',
  ) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.field = field
    this.credential = credential
  }
}

export function isApiError(err: unknown): err is ApiError {
  return err instanceof ApiError
}