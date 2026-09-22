//go:build integration

package therapists

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestPostgresRepositoryIntegration exercises the therapists repository against
// a running PostgreSQL. It is excluded from the default build via the
// "integration" tag and skipped when DATABASE_URL is not set. Therapists and
// their language rows are seeded via SQL and removed by the deferred cleanup
// (therapist_languages cascades on therapist delete).
func TestPostgresRepositoryIntegration(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	repo := NewPostgresRepository(pool)

	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	seededIDs := make([]string, 0, 3)
	seedTherapist := func(name string, langs, specs []string, active bool, createdAt time.Time) string {
		var id string
		if err := pool.QueryRow(ctx, `
			INSERT INTO therapists (full_name, credentials, years_exp, bio, location,
				is_online_only, price_kes, free_sessions, specialties, is_active, created_at)
			VALUES ($1, 'MA, PhD', $2, $3, 'Nairobi', $4, $5, $6, $7, $8, $9)
			RETURNING id`,
			name, 8, "Test bio", false, 800, 1, specs, active, createdAt).Scan(&id); err != nil {
			t.Fatalf("failed to seed therapist: %v", err)
		}
		for _, lang := range langs {
			if _, err := pool.Exec(ctx, `
				INSERT INTO therapist_languages (therapist_id, language, proficiency)
				VALUES ($1, $2, 'fluent')`, id, lang); err != nil {
				t.Fatalf("failed to seed language: %v", err)
			}
		}
		seededIDs = append(seededIDs, id)
		return id
	}

	defer func() {
		for _, id := range seededIDs {
			if _, err := pool.Exec(context.Background(), "DELETE FROM therapists WHERE id = $1", id); err != nil {
				t.Errorf("cleanup failed to DELETE therapist %s: %v", id, err)
			}
		}
	}()

	seedTherapist("Dr. Amina Korir", []string{"English", "Swahili"}, []string{"Grief", "Trauma"}, true, base.Add(0*time.Hour))
	seedTherapist("Dr. Bora Njoroge", []string{"English"}, []string{"Anxiety", "Grief"}, false, base.Add(1*time.Hour))
	seedTherapist("Dr. Cici Mwangi", []string{"Kikuyu", "Swahili"}, []string{"Trauma"}, true, base.Add(2*time.Hour))

	t.Run("List returns the directory newest-first", func(t *testing.T) {
		therapists, err := repo.List(ctx, ListOptions{}, 10, nil)
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		if len(therapists) != 3 {
			t.Fatalf("expected 3 therapists, got %d", len(therapists))
		}
		if therapists[0].DisplayName != "Dr. Cici Mwangi" {
			t.Errorf("expected the newest (Dr. Cici Mwangi) first, got %q", therapists[0].DisplayName)
		}
		for i := 1; i < len(therapists); i++ {
			if therapists[i-1].CreatedAt.Before(therapists[i].CreatedAt) {
				t.Errorf("list not newest-first at index %d", i)
			}
		}
		last := therapists[len(therapists)-1]
		if last.SessionPrice == nil || *last.SessionPrice != 800 {
			t.Errorf("expected session price 800, got %v", last.SessionPrice)
		}
		if last.Currency != SessionCurrency {
			t.Errorf("expected currency %q, got %q", SessionCurrency, last.Currency)
		}
		if len(last.Languages) != 2 {
			t.Errorf("expected 2 languages aggregated for Dr. Amina Korir, got %v", last.Languages)
		}
		if len(last.Specialties) != 2 {
			t.Errorf("expected 2 specialties, got %v", last.Specialties)
		}
	})

	t.Run("List respects the limit", func(t *testing.T) {
		therapists, err := repo.List(ctx, ListOptions{}, 1, nil)
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		if len(therapists) != 1 {
			t.Errorf("expected 1 therapist, got %d", len(therapists))
		}
		if therapists[0].DisplayName != "Dr. Cici Mwangi" {
			t.Errorf("expected the newest therapist only, got %q", therapists[0].DisplayName)
		}
	})

	t.Run("List filters by language case-insensitively", func(t *testing.T) {
		therapists, err := repo.List(ctx, ListOptions{Language: "swahili"}, 10, nil)
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		if len(therapists) != 2 {
			t.Fatalf("expected 2 Swahili-speaking therapists, got %d", len(therapists))
		}
		for _, th := range therapists {
			if !containsAny(th.Languages, "Swahili") {
				t.Errorf("therapist %s does not actually speak Swahili: %v", th.ID, th.Languages)
			}
		}
	})

	t.Run("List filters by specialty case-insensitively", func(t *testing.T) {
		therapists, err := repo.List(ctx, ListOptions{Specialty: "grief"}, 10, nil)
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		if len(therapists) != 2 {
			t.Fatalf("expected 2 Grief-specialist therapists, got %d", len(therapists))
		}
		for _, th := range therapists {
			if !containsAny(th.Specialties, "Grief") {
				t.Errorf("therapist %s does not actually list Grief: %v", th.ID, th.Specialties)
			}
		}
	})

	t.Run("List combines filters", func(t *testing.T) {
		therapists, err := repo.List(ctx, ListOptions{Language: "Swahili", Specialty: "Trauma"}, 10, nil)
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		if len(therapists) != 2 {
			t.Fatalf("expected 2 therapists matching both filters, got %d", len(therapists))
		}
	})

	t.Run("List filters out everything with no matches", func(t *testing.T) {
		therapists, err := repo.List(ctx, ListOptions{Language: "French"}, 10, nil)
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		if len(therapists) != 0 {
			t.Errorf("expected no French-speaking therapists, got %d", len(therapists))
		}
	})

	t.Run("List honors the before cursor", func(t *testing.T) {
		// Cici is the newest; ask for rows strictly older than her created_at.
		cursor := base.Add(2 * time.Hour)
		therapists, err := repo.List(ctx, ListOptions{}, 10, &cursor)
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		if len(therapists) != 2 {
			t.Fatalf("expected the 2 older therapists, got %d", len(therapists))
		}
		if therapists[0].DisplayName != "Dr. Bora Njoroge" {
			t.Errorf("expected Dr. Bora Njoroge next in the page, got %q", therapists[0].DisplayName)
		}
	})

	t.Run("Get returns the public profile", func(t *testing.T) {
		therapists, err := repo.List(ctx, ListOptions{}, 1, nil)
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		th, err := repo.Get(ctx, therapists[0].ID)
		if err != nil {
			t.Fatalf("Get returned error: %v", err)
		}
		if th.ID != therapists[0].ID || th.DisplayName == "" {
			t.Errorf("Get returned the wrong profile: %+v", th)
		}
		if th.Currency != SessionCurrency {
			t.Errorf("expected currency %q, got %q", SessionCurrency, th.Currency)
		}
		if len(th.Languages) != 2 {
			t.Errorf("expected languages aggregated, got %v", th.Languages)
		}
		if len(th.Specialties) != 1 {
			t.Errorf("expected the seeded specialty, got %v", th.Specialties)
		}
		if th.IsOnlineOnly {
			t.Error("expected is_online_only false")
		}
		if !th.IsActive {
			t.Error("expected is_active true for the newest therapist")
		}
	})

	t.Run("Get returns ErrTherapistNotFound for an unknown id", func(t *testing.T) {
		_, err := repo.Get(ctx, "00000000-0000-0000-0000-000000000000")
		if !errors.Is(err, ErrTherapistNotFound) {
			t.Errorf("expected ErrTherapistNotFound, got %v", err)
		}
	})
}

func containsAny(items []string, needle string) bool {
	for _, item := range items {
		if fmt.Sprint(item) == needle {
			return true
		}
	}
	return false
}
