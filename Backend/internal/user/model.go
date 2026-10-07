package user

import (
	"errors"
	"time"
)

const (
	// MinPasswordLength is the minimum accepted raw password length (Docs/API.md).
	MinPasswordLength = 12

	// MaxPasswordLength caps raw passwords at bcrypt's 72-byte input limit.
	MaxPasswordLength = 72

	// MaxDisplayNameLength caps the profile display name length.
	MaxDisplayNameLength = 100
)

// Sentinel errors of the users domain, mapped by handlers to safe HTTP responses.
var (
	ErrEmailTaken      = errors.New("email already registered")
	ErrInvalidEmail    = errors.New("invalid email address")
	ErrInvalidPassword = errors.New("invalid password")
	ErrUserNotFound    = errors.New("user not found")
	ErrBadCredentials  = errors.New("invalid email or password")
	// ErrIdentityAlreadyPromoted reports an identity already linked to an account.
	ErrIdentityAlreadyPromoted = errors.New("anonymous identity already promoted")
	// ErrInvalidDisplayName reports a name longer than MaxDisplayNameLength.
	ErrInvalidDisplayName = errors.New("invalid display name")
	// ErrInvalidLanguagePref reports a language_pref outside the supported set.
	ErrInvalidLanguagePref = errors.New("invalid language preference")
)

// validLanguagePrefs holds the supported codes; the column itself is free-form.
var validLanguagePrefs = map[string]bool{
	"en":  true,
	"sw":  true,
	"luo": true,
	"kik": true,
}

// User mirrors the users table; PasswordHash is never serialized to JSON.
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
