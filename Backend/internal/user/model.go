package user

import (
	"errors"
	"time"
)

const (
	// MinPasswordLength is the minimum accepted raw password length.
	// Mirrors the 12-character minimum documented in Docs/API.md.
	MinPasswordLength = 12

	// MaxPasswordLength caps raw passwords at bcrypt's 72-byte input limit.
	// Longer inputs are rejected rather than silently truncated.
	MaxPasswordLength = 72
)

// Sentinel errors returned by the users domain. Handlers map these to
// safe, client-facing HTTP responses; they never contain user input.
var (
	ErrEmailTaken      = errors.New("email already registered")
	ErrInvalidEmail    = errors.New("invalid email address")
	ErrInvalidPassword = errors.New("invalid password")
	ErrUserNotFound    = errors.New("user not found")
	ErrBadCredentials  = errors.New("invalid email or password")
)

// User mirrors the `users` table from db/migrations/001_create_users.up.sql.
// PasswordHash is never serialized to JSON or returned to API clients.
type User struct {
	ID           string     `json:"id"`
	Email        string     `json:"email"`
	PasswordHash string     `json:"-"`
	DisplayName  *string    `json:"display_name"`
	LanguagePref string     `json:"language_pref"`
	IsVerified   bool       `json:"is_verified"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    *time.Time `json:"deleted_at"`
}
