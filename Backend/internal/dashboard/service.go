package dashboard

import (
	"context"
	"fmt"

	"Backend/internal/mood"
	"Backend/internal/user"
)

// RecentMoodLimit is how many recent check-ins the dashboard surfaces beside
// the latest one. A deliberately small "recent activity" snapshot.
const RecentMoodLimit = 5

// Dashboard is the authenticated wellness snapshot returned by GET
// /api/v1/dashboard. It is assembled only from the user's own records.
type Dashboard struct {
	User              *user.User
	LatestMood        *mood.MoodLog
	RecentMoods       []mood.MoodLog
	MoodCheckinsCount int64
}

// Service is the dashboard business-logic boundary. It composes the existing
// user and mood services and contains no SQL; it exists so handlers never
// call multiple services directly.
type Service interface {
	// Get builds the wellness snapshot for the authenticated user's ID. The
	// user ID always comes from the JWT context, never from client input.
	// It returns the user's ErrUserNotFound when the account no longer exists.
	Get(ctx context.Context, userID string) (*Dashboard, error)
}

type service struct {
	users user.Service
	moods mood.Service
}

// NewService wires a dashboard service to the user and mood services.
func NewService(users user.Service, moods mood.Service) *service {
	return &service{users: users, moods: moods}
}

func (s *service) Get(ctx context.Context, userID string) (*Dashboard, error) {
	u, err := s.users.GetProfile(ctx, userID)
	if err != nil {
		return nil, err
	}

	latest, err := s.moods.Latest(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("dashboard: latest mood: %w", err)
	}

	recent, err := s.moods.List(ctx, userID, RecentMoodLimit)
	if err != nil {
		return nil, fmt.Errorf("dashboard: recent moods: %w", err)
	}

	count, err := s.moods.Count(ctx, userID)
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
