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