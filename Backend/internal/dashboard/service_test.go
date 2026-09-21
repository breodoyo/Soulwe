package dashboard

import (
	"context"
	"errors"
	"testing"
	"time"

	"Backend/internal/mood"
	"Backend/internal/user"
)

const testUserID = "11111111-1111-1111-1111-111111111111"

// fakeUserService stubs only the user.Service methods the dashboard uses.
type fakeUserService struct {
	user.Service
	getProfileFunc func(ctx context.Context, userID string) (*user.User, error)
}

func (f *fakeUserService) GetProfile(ctx context.Context, userID string) (*user.User, error) {
	return f.getProfileFunc(ctx, userID)
}

// fakeMoodService stubs only the mood.Service methods the dashboard uses.
type fakeMoodService struct {
	mood.Service
	latestFunc func(ctx context.Context, userID string) (*mood.MoodLog, error)
	listFunc   func(ctx context.Context, userID string, limit int) ([]mood.MoodLog, error)
	countFunc  func(ctx context.Context, userID string) (int64, error)
}

func (f *fakeMoodService) Latest(ctx context.Context, userID string) (*mood.MoodLog, error) {
	return f.latestFunc(ctx, userID)
}

func (f *fakeMoodService) List(ctx context.Context, userID string, limit int) ([]mood.MoodLog, error) {
	return f.listFunc(ctx, userID, limit)
}

func (f *fakeMoodService) Count(ctx context.Context, userID string) (int64, error) {
	return f.countFunc(ctx, userID)
}

func testUser() *user.User {
	return &user.User{
		ID:           testUserID,
		Email:        "bree@example.com",
		DisplayName:  nil,
		LanguagePref: "en",
		IsVerified:   false,
	}
}

func testMood(movement string) mood.MoodLog {
	return mood.MoodLog{
		ID:       "22222222-2222-2222-2222-222222222222",
		UserID:   testUserID,
		Mood:     movement,
		LoggedAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
	}
}

func TestServiceGet(t *testing.T) {
	t.Run("returns the full snapshot when the user has check-ins", func(t *testing.T) {
		users := &fakeUserService{
			getProfileFunc: func(context.Context, string) (*user.User, error) { return testUser(), nil },
		}
		recent := []mood.MoodLog{testMood("Grateful"), testMood("Better")}
		moods := &fakeMoodService{
			latestFunc: func(context.Context, string) (*mood.MoodLog, error) { return &recent[0], nil },
			listFunc: func(_ context.Context, userID string, limit int) ([]mood.MoodLog, error) {
				if limit != RecentMoodLimit {
					t.Errorf("expected recent mood limit %d, got %d", RecentMoodLimit, limit)
				}
				return recent, nil
			},
			countFunc: func(context.Context, string) (int64, error) { return 2, nil },
		}

		dash, err := NewService(users, moods).Get(context.Background(), testUserID)
		if err != nil {
			t.Fatalf("Get returned error: %v", err)
		}
		if dash.User == nil || dash.User.Email != "bree@example.com" {
			t.Error("expected the user profile in the dashboard")
		}
		if dash.LatestMood == nil || dash.LatestMood.Mood != "Grateful" {
			t.Errorf("expected the latest mood 'Grateful', got %+v", dash.LatestMood)
		}
		if len(dash.RecentMoods) != 2 || dash.RecentMoods[1].Mood != "Better" {
			t.Errorf("unexpected recent moods: %+v", dash.RecentMoods)
		}
		if dash.MoodCheckinsCount != 2 {
			t.Errorf("expected count 2, got %d", dash.MoodCheckinsCount)
		}
	})

	t.Run("returns nulls and zeros when the user has no check-ins", func(t *testing.T) {
		users := &fakeUserService{
			getProfileFunc: func(context.Context, string) (*user.User, error) { return testUser(), nil },
		}
		moods := &fakeMoodService{
			latestFunc: func(context.Context, string) (*mood.MoodLog, error) { return nil, nil },
			listFunc: func(context.Context, string, int) ([]mood.MoodLog, error) {
				return []mood.MoodLog{}, nil
			},
			countFunc: func(context.Context, string) (int64, error) { return 0, nil },
		}

		dash, err := NewService(users, moods).Get(context.Background(), testUserID)
		if err != nil {
			t.Fatalf("Get returned error: %v", err)
		}
		if dash.LatestMood != nil {
			t.Errorf("expected latest_mood nil, got %+v", dash.LatestMood)
		}
		if len(dash.RecentMoods) != 0 {
			t.Errorf("expected no recent moods, got %d", len(dash.RecentMoods))
		}
		if dash.RecentMoods == nil {
			t.Error("expected a non-nil empty slice so JSON renders []")
		}
		if dash.MoodCheckinsCount != 0 {
			t.Errorf("expected count 0, got %d", dash.MoodCheckinsCount)
		}
	})

	t.Run("propagates the user not found error", func(t *testing.T) {
		users := &fakeUserService{
			getProfileFunc: func(context.Context, string) (*user.User, error) {
				return nil, user.ErrUserNotFound
			},
		}
		moods := &fakeMoodService{}
		_, err := NewService(users, moods).Get(context.Background(), testUserID)
		if !errors.Is(err, user.ErrUserNotFound) {
			t.Fatalf("expected ErrUserNotFound, got %v", err)
		}
	})

	t.Run("wraps mood repository failures", func(t *testing.T) {
		users := &fakeUserService{
			getProfileFunc: func(context.Context, string) (*user.User, error) { return testUser(), nil },
		}
		moods := &fakeMoodService{
			latestFunc: func(context.Context, string) (*mood.MoodLog, error) {
				return nil, errors.New("connection lost")
			},
		}
		_, err := NewService(users, moods).Get(context.Background(), testUserID)
		if err == nil || errors.Is(err, user.ErrUserNotFound) {
			t.Fatalf("expected a wrapped mood repository error, got %v", err)
		}
	})

	t.Run("the user id passed to every service is the authenticated id", func(t *testing.T) {
		users := &fakeUserService{
			getProfileFunc: func(_ context.Context, userID string) (*user.User, error) {
				if userID != testUserID {
					t.Errorf("expected authenticated user id, got %q", userID)
				}
				return testUser(), nil
			},
		}
		moods := &fakeMoodService{
			latestFunc: func(_ context.Context, userID string) (*mood.MoodLog, error) {
				if userID != testUserID {
					t.Errorf("latest: expected authenticated user id, got %q", userID)
				}
				return nil, nil
			},
			listFunc: func(_ context.Context, userID string, limit int) ([]mood.MoodLog, error) {
				if userID != testUserID {
					t.Errorf("list: expected authenticated user id, got %q", userID)
				}
				return []mood.MoodLog{}, nil
			},
			countFunc: func(_ context.Context, userID string) (int64, error) {
				if userID != testUserID {
					t.Errorf("count: expected authenticated user id, got %q", userID)
				}
				return 0, nil
			},
		}
		if _, err := NewService(users, moods).Get(context.Background(), testUserID); err != nil {
			t.Fatalf("Get returned error: %v", err)
		}
	})
}
