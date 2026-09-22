package therapists

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func therapist(name string, createdAt time.Time) *Therapist {
	price := 800
	return &Therapist{
		ID:           "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		DisplayName:  name,
		SessionPrice: &price,
		Currency:     SessionCurrency,
		CreatedAt:    createdAt,
	}
}

// fakeRepository implements Repository for service tests.
type fakeRepository struct {
	mu         sync.Mutex
	therapists []Therapist
	lastOpts   ListOptions
	lastLimit  int
	get        *Therapist
	getErr     error
}

func newFakeRepository(items ...Therapist) *fakeRepository {
	return &fakeRepository{therapists: items}
}

func (f *fakeRepository) List(_ context.Context, opts ListOptions, limit int, _ *time.Time) ([]Therapist, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastOpts = opts
	f.lastLimit = limit
	return f.therapists, nil
}

func (f *fakeRepository) Get(_ context.Context, _ string) (*Therapist, error) {
	return f.get, f.getErr
}

func TestService_ListClampsLimitAndPassesFilters(t *testing.T) {
	fake := newFakeRepository()
	fake.therapists = []Therapist{*therapist("Dr. Amina Korir", time.Now())}
	svc := NewService(fake)
	ctx := context.Background()

	opts := ListOptions{Language: "Swahili", Specialty: "Grief"}
	if _, err := svc.List(ctx, opts, 10_000, nil); err != nil {
		t.Fatalf("List: %v", err)
	}
	if fake.lastLimit != MaxListLimit {
		t.Errorf("expected the repository to receive the clamped limit %d, got %d", MaxListLimit, fake.lastLimit)
	}
	if fake.lastOpts.Language != "Swahili" || fake.lastOpts.Specialty != "Grief" {
		t.Errorf("expected the filters to be passed through, got %+v", fake.lastOpts)
	}

	if _, err := svc.List(ctx, ListOptions{}, 0, nil); err != nil {
		t.Fatalf("List default: %v", err)
	}
	if fake.lastLimit != DefaultListLimit {
		t.Errorf("expected the default limit %d, got %d", DefaultListLimit, fake.lastLimit)
	}
}

func TestService_GetReturnsProfileOrNotFound(t *testing.T) {
	svc := NewService(&fakeRepository{get: therapist("Dr. Bora", time.Now())})
	got, err := svc.Get(context.Background(), "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.DisplayName != "Dr. Bora" {
		t.Errorf("expected the seeded therapist, got %+v", got)
	}

	svc = NewService(&fakeRepository{getErr: ErrTherapistNotFound})
	if _, err := svc.Get(context.Background(), "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"); !errors.Is(err, ErrTherapistNotFound) {
		t.Errorf("expected ErrTherapistNotFound, got %v", err)
	}
}
