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

export interface RegisterResponse {
  user: User
}

export interface MeResponse {
  user_id: string
}

export interface ProfileResponse {
  user: User
}

export type Mood = 'Heavy' | 'Okay' | 'Better' | 'At peace' | 'Grateful'

export type LanguagePref = 'en' | 'sw' | 'luo' | 'kik'

export interface MoodLog {
  id: string
  mood: Mood
  logged_at: string
}

export interface CreateMoodPayload {
  mood: Mood
}

export interface MoodsResponse {
  moods: MoodLog[]
}

// Empty display_name clears the stored name; omitted fields are untouched.
export interface UpdateProfilePayload {
  display_name?: string
  language_pref?: LanguagePref
}

export interface DashboardResponse {
  user: User
  latest_mood: MoodLog | null
  recent_moods: MoodLog[]
  mood_checkins_count: number
}

// Content is decrypted only on single-entry endpoints; there is no updated_at.
export interface JournalEntry {
  id: string
  content?: string
  mood_tags: string[]
  prompt_used: string | null
  ai_reflection: string | null
  word_count: number
  created_at: string
}

export interface CreateJournalPayload {
  content: string
  mood_tags?: string[]
  prompt_used?: string
}

// All fields optional, but at least one is required.
export interface UpdateJournalPayload {
  content?: string
  mood_tags?: string[]
  prompt_used?: string
}

export interface JournalEntryResponse {
  entry: JournalEntry
}

export interface JournalListResponse {
  entries: JournalEntry[]
  next_cursor: string | null
}

// The raw token is returned once; only its hash is stored; anonymous_id is internal.
export interface AnonymousSessionResponse {
  anonymous_token: string
  anonymous_id: string
}

export interface AnonymousMeResponse {
  anonymous_id: string
}

// Stable key (grief|work|family|relationships|growth), not a glyph.
export interface Circle {
  id: string
  slug: string
  name: string
  description: string | null
  icon: string | null
  member_count: number
  is_member: boolean
}

// author_name is a derived per-circle label, not an identity.
export interface CircleMessage {
  id: string
  author_name: string
  is_mine: boolean
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

// Internal fields are never serialized by the backend.
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

export interface TherapistsResponse {
  therapists: Therapist[]
  next_cursor: string | null
}

export interface TherapistResponse {
  therapist: Therapist
}

// Must match the backend CHECK constraint.
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

// Must be in the future; sessions are a fixed 60-minute window.
export interface CreateBookingPayload {
  scheduled_at: string
}

export interface BookingResponse {
  booking: Booking
}

export interface BookingsResponse {
  bookings: Booking[]
}

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

export interface BreathingExercisesResponse {
  exercises: BreathingExercise[]
}

export interface BreathingExerciseResponse {
  exercise: BreathingExercise
}

// exercise_id is null only for legacy device sessions.
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

export interface CreateBreathingSessionPayload {
  exercise_id: string
  breaths: number
  duration_s: number
  completed?: boolean
}

export interface BreathingSessionResponse {
  session: BreathingSession
}

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

// Storage cannot say which session ended; this stamps the failed request.
export type CredentialSent = 'registered' | 'anonymous' | 'none'

// message is always safe to display.
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