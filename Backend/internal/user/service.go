package user

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
)

// TokenManager issues signed JWT access tokens; the interface keeps the auth
// package out of the users domain.
type TokenManager interface {
	SignAccessToken(userID string) (string, error)
}

// Service is the authentication business-logic boundary for registered users.
type Service interface {
	Register(ctx context.Context, email, password string) (*User, error)

	FindByEmail(ctx context.Context, email string) (*User, error)

	// VerifyPassword returns ErrBadCredentials for an unknown email *and* a bad
	// password, so responses cannot be used to enumerate accounts.
	VerifyPassword(ctx context.Context, email, password string) (*User, error)

	Login(ctx context.Context, email, password string) (*LoginResult, error)

	// Promote validates the credentials, then persists the user and the identity
	// link atomically; it returns ErrIdentityAlreadyPromoted or ErrEmailTaken.
	Promote(ctx context.Context, identityID, email, password string, displayName *string) (*PromotionResult, error)

	GetProfile(ctx context.Context, userID string) (*User, error)

	// UpdateProfile applies validated changes; nil pointer fields are left unchanged.
	UpdateProfile(ctx context.Context, userID string, displayName, languagePref *string) (*User, error)
}

type LoginResult struct {
	User        *User
	AccessToken string
}

type PromotionResult struct {
	User        *User
	AccessToken string
}

type service struct {
	users  Repository
	tokens TokenManager
}

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

func (s *service) GetProfile(ctx context.Context, userID string) (*User, error) {
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// UpdateProfile validates and applies changes; a blank display name clears it.
func (s *service) UpdateProfile(ctx context.Context, userID string, displayName, languagePref *string) (*User, error) {
	var name *string
	if displayName != nil {
		trimmed := strings.TrimSpace(*displayName)
		if trimmed == "" {
			empty := ""
			name = &empty // explicit clear → repository stores NULL
		} else {
			if len(trimmed) > MaxDisplayNameLength {
				return nil, ErrInvalidDisplayName
			}
			name = &trimmed
		}
	}

	var lang *string
	if languagePref != nil {
		code := strings.ToLower(strings.TrimSpace(*languagePref))
		if !validLanguagePrefs[code] {
			return nil, ErrInvalidLanguagePref
		}
		lang = &code
	}

	u, err := s.users.UpdateProfile(ctx, userID, name, lang)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// normalizeEmail lowercases and trims so the unique email constraint behaves predictably.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

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
