package bookings

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeRepo embeds the Repository interface so service tests only stub the
// methods under test.
type fakeRepo struct {
	Repository
	createFunc    func(ctx context.Context, b *Booking) error
	getFunc       func(ctx context.Context, userID, bookingID string) (*Booking, error)
	listFunc      func(ctx context.Context, userID string) ([]Booking, error)
	overlapFunc   func(ctx context.Context, userID string, start time.Time) (bool, error)
	statusFunc    func(ctx context.Context, therapistID string) (found, active bool, err error)
	setStatusFunc func(ctx context.Context, userID, bookingID, expected, next string) (*Booking, error)
}

func (f *fakeRepo) Create(ctx context.Context, b *Booking) error {
	if f.createFunc == nil {
		return errors.New("createFunc not configured")
	}
	return f.createFunc(ctx, b)
}

func (f *fakeRepo) GetByID(ctx context.Context, userID, bookingID string) (*Booking, error) {
	if f.getFunc == nil {
		return nil, errors.New("getFunc not configured")
	}
	return f.getFunc(ctx, userID, bookingID)
}

func (f *fakeRepo) ListByUserID(ctx context.Context, userID string) ([]Booking, error) {
	if f.listFunc == nil {
		return nil, errors.New("listFunc not configured")
	}
	return f.listFunc(ctx, userID)
}

func (f *fakeRepo) HasActiveOverlap(ctx context.Context, userID string, start time.Time) (bool, error) {
	if f.overlapFunc == nil {
		return false, errors.New("overlapFunc not configured")
	}
	return f.overlapFunc(ctx, userID, start)
}

func (f *fakeRepo) TherapistStatus(ctx context.Context, therapistID string) (found, active bool, err error) {
	if f.statusFunc == nil {
		return false, false, errors.New("statusFunc not configured")
	}
	return f.statusFunc(ctx, therapistID)
}

func (f *fakeRepo) SetStatus(ctx context.Context, userID, bookingID, expected, next string) (*Booking, error) {
	if f.setStatusFunc == nil {
		return nil, errors.New("setStatusFunc not configured")
	}
	return f.setStatusFunc(ctx, userID, bookingID, expected, next)
}

const (
	svcUserID     = "11111111-1111-1111-1111-111111111111"
	svcTherapist  = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	svcFutureTime = "2026-10-01T10:00:00Z"
)

func TestServiceCreate(t *testing.T) {
	future, err := time.Parse(time.RFC3339Nano, svcFutureTime)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	past := time.Now().Add(-time.Hour)

	t.Run("rejects a past scheduled_at before touching the repository", func(t *testing.T) {
		repo := &fakeRepo{}
		svc := NewService(repo)
		_, err := svc.Create(context.Background(), svcUserID, svcTherapist, past)
		if !errors.Is(err, ErrScheduledInPast) {
			t.Fatalf("expected ErrScheduledInPast, got %v", err)
		}
	})

	t.Run("rejects a missing therapist", func(t *testing.T) {
		repo := &fakeRepo{
			statusFunc: func(context.Context, string) (bool, bool, error) {
				return false, false, nil
			},
		}
		svc := NewService(repo)
		_, err := svc.Create(context.Background(), svcUserID, svcTherapist, future)
		if !errors.Is(err, ErrTherapistMissing) {
			t.Fatalf("expected ErrTherapistMissing, got %v", err)
		}
	})

	t.Run("rejects an inactive therapist", func(t *testing.T) {
		repo := &fakeRepo{
			statusFunc: func(context.Context, string) (bool, bool, error) {
				return true, false, nil
			},
		}
		svc := NewService(repo)
		_, err := svc.Create(context.Background(), svcUserID, svcTherapist, future)
		if !errors.Is(err, ErrTherapistInactive) {
			t.Fatalf("expected ErrTherapistInactive, got %v", err)
		}
	})

	t.Run("rejects a time overlapping an existing booking", func(t *testing.T) {
		repo := &fakeRepo{
			statusFunc: func(context.Context, string) (bool, bool, error) {
				return true, true, nil
			},
			overlapFunc: func(context.Context, string, time.Time) (bool, error) {
				return true, nil
			},
		}
		svc := NewService(repo)
		_, err := svc.Create(context.Background(), svcUserID, svcTherapist, future)
		if !errors.Is(err, ErrBookingConflict) {
			t.Fatalf("expected ErrBookingConflict, got %v", err)
		}
		if repo.createFunc != nil {
			t.Fatal("expected Create not to be called on overlap")
		}
	})

	t.Run("surfaces repository failures unchanged", func(t *testing.T) {
		repo := &fakeRepo{
			statusFunc: func(context.Context, string) (bool, bool, error) {
				return true, true, nil
			},
			overlapFunc: func(context.Context, string, time.Time) (bool, error) {
				return false, nil
			},
			createFunc: func(context.Context, *Booking) error {
				return errors.New("database connection lost")
			},
		}
		svc := NewService(repo)
		_, err := svc.Create(context.Background(), svcUserID, svcTherapist, future)
		if err == nil || err.Error() != "database connection lost" {
			t.Fatalf("expected the repo error to propagate, got %v", err)
		}
	})

	t.Run("creates a pending booking with the caller's identity", func(t *testing.T) {
		repo := &fakeRepo{
			statusFunc: func(ctx context.Context, therapistID string) (bool, bool, error) {
				if therapistID != svcTherapist {
					t.Errorf("expected therapist id %q, got %q", svcTherapist, therapistID)
				}
				return true, true, nil
			},
			overlapFunc: func(ctx context.Context, userID string, start time.Time) (bool, error) {
				if userID != svcUserID {
					t.Errorf("expected user id %q, got %q", svcUserID, userID)
				}
				if !start.Equal(future) {
					t.Errorf("expected the requested time to reach the overlap check, got %v", start)
				}
				return false, nil
			},
			createFunc: func(_ context.Context, b *Booking) error {
				b.ID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
				b.CreatedAt = time.Now()
				b.UpdatedAt = b.CreatedAt
				return nil
			},
		}
		svc := NewService(repo)
		booking, err := svc.Create(context.Background(), svcUserID, svcTherapist, future)
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if booking.Status != StatusPending {
			t.Errorf("expected status %q, got %q", StatusPending, booking.Status)
		}
		if booking.UserID != svcUserID || booking.TherapistID != svcTherapist {
			t.Errorf("expected booking for user/therapist pair, got %+v", booking)
		}
		if !booking.ScheduledAt.Equal(future) {
			t.Errorf("expected scheduled_at to be preserved, got %v", booking.ScheduledAt)
		}
	})
}

func TestServiceList(t *testing.T) {
	t.Run("returns the repository list", func(t *testing.T) {
		bookings := []Booking{{ID: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"}}
		repo := &fakeRepo{
			listFunc: func(ctx context.Context, userID string) ([]Booking, error) {
				if userID != svcUserID {
					t.Errorf("expected user id %q, got %q", svcUserID, userID)
				}
				return bookings, nil
			},
		}
		svc := NewService(repo)
		got, err := svc.List(context.Background(), svcUserID)
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if len(got) != 1 || got[0].ID != "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb" {
			t.Errorf("expected the repo list back, got %+v", got)
		}
	})
}

func TestServiceGet(t *testing.T) {
	t.Run("forwards to the repository with scoping", func(t *testing.T) {
		repo := &fakeRepo{
			getFunc: func(_ context.Context, userID, bookingID string) (*Booking, error) {
				if userID != svcUserID {
					t.Errorf("expected scoped user id, got %q", userID)
				}
				return &Booking{ID: bookingID, UserID: svcUserID}, nil
			},
		}
		svc := NewService(repo)
		id := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
		got, err := svc.Get(context.Background(), svcUserID, id)
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if got.ID != id {
			t.Errorf("expected booking %q, got %q", id, got.ID)
		}
	})

	t.Run("propagates not found", func(t *testing.T) {
		repo := &fakeRepo{
			getFunc: func(context.Context, string, string) (*Booking, error) {
				return nil, ErrBookingNotFound
			},
		}
		svc := NewService(repo)
		_, err := svc.Get(context.Background(), svcUserID, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
		if !errors.Is(err, ErrBookingNotFound) {
			t.Fatalf("expected ErrBookingNotFound, got %v", err)
		}
	})
}

func TestServiceCancel(t *testing.T) {
	t.Run("requests a pending→cancelled transition", func(t *testing.T) {
		repo := &fakeRepo{
			setStatusFunc: func(_ context.Context, userID, bookingID, expected, next string) (*Booking, error) {
				if userID != svcUserID {
					t.Errorf("expected scoped user id, got %q", userID)
				}
				if expected != StatusPending || next != StatusCancelled {
					t.Errorf("expected pending→cancelled transition, got %q→%q", expected, next)
				}
				return &Booking{ID: bookingID, UserID: userID, Status: StatusCancelled}, nil
			},
		}
		svc := NewService(repo)
		id := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
		got, err := svc.Cancel(context.Background(), svcUserID, id)
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if got.Status != StatusCancelled {
			t.Errorf("expected cancelled booking, got %+v", got)
		}
	})

	t.Run("propagates the status conflict", func(t *testing.T) {
		repo := &fakeRepo{
			setStatusFunc: func(context.Context, string, string, string, string) (*Booking, error) {
				return nil, ErrBookingStatusConflict
			},
		}
		svc := NewService(repo)
		_, err := svc.Cancel(context.Background(), svcUserID, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
		if !errors.Is(err, ErrBookingStatusConflict) {
			t.Fatalf("expected ErrBookingStatusConflict, got %v", err)
		}
	})
}
