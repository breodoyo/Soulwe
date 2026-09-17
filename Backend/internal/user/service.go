package user

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
)

// Service is the authentication business-logic boundary for registered users.
// It is the foundation the future login step will build on.
type Service interface {
	// Register hashes the password, creates the user, and returns the
	// persisted user (never including the password hash).
	Register(ctx context.Context, email, password string) (*User, error)

	// FindByEmail returns the user matching the normalized email.
	FindByEmail(ctx context.Context, email string) (*User, error)

	// VerifyPassword finds the user by email and checks the password.
	// It returns ErrBadCredentials when the email is unknown or the
	// password does not match, avoiding user enumeration.
	VerifyPassword(ctx context.Context, email, password string) (*User, error)
}

type service struct {
	users Repository
}

// NewService wires the authentication service to a user repository.
func NewService(users Repository) *service {
	return &service{users: users}
}

func (s *service) Register(ctx context.Context, email, password string) (*User, error) {
	email = normalizeEmail(email)
	if !validEmail(email) {
		return nil, ErrInvalidEmail
	}
	if !validPassword(password) {
		return nil, ErrInvalidPassword
	}

	hash, err := hashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("user register: hash password: %w", err)
	}

	u := &User{
		Email:        email,
		PasswordHash: hash,
		LanguagePref: "en",
	}
	if err := s.users.Create(ctx, u); err != nil {
		if errors.Is(err, ErrEmailTaken) {
			return nil, ErrEmailTaken
		}
		return nil, fmt.Errorf("user register: %w", err)
	}

	return u, nil
}

func (s *service) FindByEmail(ctx context.Context, email string) (*User, error) {
	return s.users.FindByEmail(ctx, normalizeEmail(email))
}

func (s *service) VerifyPassword(ctx context.Context, email, password string) (*User, error) {
	u, err := s.users.FindByEmail(ctx, normalizeEmail(email))
	if errors.Is(err, ErrUserNotFound) {
		return nil, ErrBadCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("user verify password: %w", err)
	}
	if !checkPassword(u.PasswordHash, password) {
		return nil, ErrBadCredentials
	}
	return u, nil
}

// normalizeEmail trims surrounding whitespace and lowercases the address so
// the unique email constraint behaves predictably.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// validEmail rejects empty, oversized, and malformed addresses.
func validEmail(email string) bool {
	if email == "" || len(email) > 320 {
		return false
	}
	addr, err := mail.ParseAddress(email)
	return err == nil && addr.Address == email
}

// validPassword enforces the documented minimum length and bcrypt's input cap.
func validPassword(password string) bool {
	return len(password) >= MinPasswordLength && len(password) <= MaxPasswordLength
}
