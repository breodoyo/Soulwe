package dashboard

import (
	"context"
	"fmt"

	"Backend/internal/middleware"
	"Backend/internal/mood"
	"Backend/internal/user"
)

const RecentMoodLimit = 5

type Dashboard struct {
	User              *user.User
	LatestMood        *mood.MoodLog
	RecentMoods       []mood.MoodLog
	MoodCheckinsCount int64
}

type Service interface {
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
