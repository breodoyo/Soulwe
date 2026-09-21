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

	// MaxDisplayNameLength caps the profile display name at a reasonable
	// displayable length. Blank names are allowed (stored as NULL).
	MaxDisplayNameLength = 100
)

// Sentinel errors returned by the users domain. Handlers map these to
// safe, client-facing HTTP responses; they never contain user input.
var (
	ErrEmailTaken      = errors.New("email already registered")
	ErrInvalidEmail    = errors.New("invalid email address")
	ErrInvalidPassword = errors.New("invalid password")
	ErrUserNotFound    = errors.New("user not found")
	ErrBadCredentials  = errors.New("invalid email or password")
	// ErrIdentityAlreadyPromoted reports that an anonymous identity has already
	// been linked to a registered account (anon_identities.user_id is set).
	ErrIdentityAlreadyPromoted = errors.New("anonymous identity already promoted")
	// ErrInvalidDisplayName reports a profile display name outside the
	// supported length, e.g. longer than MaxDisplayNameLength.
	ErrInvalidDisplayName = errors.New("invalid display name")
	// ErrInvalidLanguagePref reports a language_pref outside the supported set
	// ('en', 'sw', 'luo', 'kik') documented in Docs/DATABASE.md.
	ErrInvalidLanguagePref = errors.New("invalid language preference")
)

// validLanguagePrefs is the set of language codes the product supports. The
// schema stores the plain TEXT value; validation happens at the application
// boundary so the DB column stays free-form.
var validLanguagePrefs = map[string]bool{
	"en":  true,
	"sw":  true,
	"luo": true,
	"kik": true,
}

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
