package dashboard

import (
	"context"
	"fmt"

	"Backend/internal/middleware"
	"Backend/internal/mood"
	"Backend/internal/user"
)

// RecentMoodLimit is how many recent check-ins sit beside the latest one.
const RecentMoodLimit = 5

// Dashboard is the wellness snapshot returned by GET /api/v1/dashboard,
// assembled only from the user's own records.
type Dashboard struct {
	User              *user.User
	LatestMood        *mood.MoodLog
	RecentMoods       []mood.MoodLog
	MoodCheckinsCount int64
}

// Service is the dashboard business-logic boundary, composing the user and
// mood services so handlers never call multiple services directly.
type Service interface {
	// Get builds the wellness snapshot for a user ID taken from the JWT
	// context, and returns ErrUserNotFound when the account no longer exists.
	Get(ctx context.Context, userID string) (*Dashboard, error)
}

type service struct {
	users user.Service
	moods mood.Service
}

func NewService(users user.Service, moods mood.Service) *service {
	return &service{users: users, moods: moods}
}

func (s *service) Get(ctx context.Context, userID string) (*Dashboard, error) {
	u, err := s.users.GetProfile(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Scoped to this user so no other identity's check-ins leak in.
	owner := middleware.Owner{UserID: userID}

	latest, err := s.moods.Latest(ctx, owner)
	if err != nil {
		return nil, fmt.Errorf("dashboard: latest mood: %w", err)
	}

	recent, err := s.moods.List(ctx, owner, RecentMoodLimit)
	if err != nil {
		return nil, fmt.Errorf("dashboard: recent moods: %w", err)
	}

	count, err := s.moods.Count(ctx, owner)
	if err != nil {
		return nil, fmt.Errorf("dashboard: mood count: %w", err)
	}

	return &Dashboard{
		User:              u,
		LatestMood:        latest,
		RecentMoods:       recent,
		MoodCheckinsCount: count,
	}, nil
}
