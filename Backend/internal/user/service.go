package user

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
)

// TokenManager issues signed JWT access tokens for authenticated users.
// It is satisfied by *auth.Manager; the interface keeps the JWT package out
// of the users domain so the layers stay decoupled.
type TokenManager interface {
	SignAccessToken(userID string) (string, error)
}

// Service is the authentication business-logic boundary for registered users.
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

	// Login verifies the credentials and returns the authenticated user
	// together with a freshly signed JWT access token.
	Login(ctx context.Context, email, password string) (*LoginResult, error)

	// Promote upgrades an anonymous identity to a registered account: it
	// validates the credentials, persists the user and the identity link
	// atomically, and returns the persisted user with a freshly signed JWT.
	// It returns ErrIdentityAlreadyPromoted when the identity is already
	// linked and ErrEmailTaken when the email is already registered.
	Promote(ctx context.Context, identityID, email, password string, displayName *string) (*PromotionResult, error)
}

// LoginResult is the successful outcome of a login: the authenticated user
// and the access token they must send on subsequent requests.
type LoginResult struct {
	User        *User
	AccessToken string
}

// PromotionResult is the successful outcome of promoting an anonymous
// identity: the newly created registered user and the access token.
type PromotionResult struct {
	User        *User
	AccessToken string
}

type service struct {
	users  Repository
	tokens TokenManager
}

// NewService wires the authentication service to a user repository.
func NewService(users Repository, tokens TokenManager) *service {
	return &service{users: users, tokens: tokens}
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

// Login reuses VerifyPassword for credential checks, so the anti-enumeration
// behaviour is identical: unknown emails and wrong passwords both surface as
// ErrBadCredentials. Only after a successful check is an access token signed.
func (s *service) Login(ctx context.Context, email, password string) (*LoginResult, error) {
	u, err := s.VerifyPassword(ctx, email, password)
	if err != nil {
		return nil, err
	}

	accessToken, err := s.tokens.SignAccessToken(u.ID)
	if err != nil {
		return nil, fmt.Errorf("user login: sign access token: %w", err)
	}

	return &LoginResult{User: u, AccessToken: accessToken}, nil
}

// Promote turns an authenticated anonymous identity into a registered account.
// The service owns the business rules (email/password validation, optional
// display_name normalization, password hashing) and only issues a JWT after
// the repository confirms the user and the identity link were persisted
// atomically. It never deletes or mutates the anonymous identity beyond the
// user_id link performed by the repository.
func (s *service) Promote(ctx context.Context, identityID, email, password string, displayName *string) (*PromotionResult, error) {
	email = normalizeEmail(email)
	if !validEmail(email) {
		return nil, ErrInvalidEmail
	}
	if !validPassword(password) {
		return nil, ErrInvalidPassword
	}

	var name *string
	if displayName != nil {
		if trimmed := strings.TrimSpace(*displayName); trimmed != "" {
			name = &trimmed
		}
	}

	hash, err := hashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("user promote: hash password: %w", err)
	}

	u, err := s.users.Promote(ctx, identityID, email, hash, name, "en")
	if errors.Is(err, ErrEmailTaken) {
		return nil, ErrEmailTaken
	}
	if errors.Is(err, ErrIdentityAlreadyPromoted) {
		return nil, ErrIdentityAlreadyPromoted
	}
	if err != nil {
		return nil, fmt.Errorf("user promote: %w", err)
	}

	accessToken, err := s.tokens.SignAccessToken(u.ID)
	if err != nil {
		return nil, fmt.Errorf("user promote: sign access token: %w", err)
	}

	return &PromotionResult{User: u, AccessToken: accessToken}, nil
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
