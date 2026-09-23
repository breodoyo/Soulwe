//go:build integration

package bookings

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestPostgresRepositoryIntegration exercises the bookings repository against a
// running PostgreSQL. It is excluded from the default build via the
// "integration" tag and skipped when DATABASE_URL is not set. Users and
// therapists are seeded via SQL with fixed ids; the deferred cleanup removes
// them and the bookings rows cascade away.
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

	const (
		u1 = "11111111-1111-1111-1111-111111111111"
		u2 = "22222222-2222-2222-2222-222222222222"
		u3 = "33333333-3333-3333-3333-333333333333"
		t1 = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		t2 = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	)
	userIDs := []string{u1, u2, u3}

	seedUser := func(id, email string) {
		if _, err := pool.Exec(ctx, `
			INSERT INTO users (id, email, password_hash, display_name)
			VALUES ($1, $2, 'x', $3)`, id, email, "User "+id); err != nil {
			t.Fatalf("failed to seed user %s: %v", id, err)
		}
	}
	seedTherapist := func(id, name string, active bool) {
		if _, err := pool.Exec(ctx, `
			INSERT INTO therapists (id, full_name, credentials, years_exp, bio, location,
				is_online_only, price_kes, free_sessions, specialties, is_active, created_at)
			VALUES ($1, $2, 'MA', 5, 'bio', 'Nairobi', FALSE, 800, 1, ARRAY['Grief'], $3, NOW())`,
			id, name, active); err != nil {
			t.Fatalf("failed to seed therapist %s: %v", id, err)
		}
	}

	for _, id := range userIDs {
		seedUser(id, id+"@example.test")
	}
	seedTherapist(t1, "Dr. Amina Korir", true)
	seedTherapist(t2, "Dr. Bora Njoroge", false)

	defer func() {
		cleanupCtx := context.Background()
		for _, id := range userIDs {
			if _, err := pool.Exec(cleanupCtx, "DELETE FROM users WHERE id = $1", id); err != nil {
				t.Errorf("cleanup failed to DELETE user %s: %v", id, err)
			}
		}
		for _, id := range []string{t1, t2} {
			if _, err := pool.Exec(cleanupCtx, "DELETE FROM therapists WHERE id = $1", id); err != nil {
				t.Errorf("cleanup failed to DELETE therapist %s: %v", id, err)
			}
		}
	}()

	at := func(hour int) time.Time {
		return time.Date(2026, 10, 1, hour, 0, 0, 0, time.UTC)
	}

	// cancelledID points at the booking cancelled in the SetStatus subtest; the
	// following subtests reuse it as a known non-pending, foreign-scoped row.
	var cancelledID string

	t.Run("Create inserts a pending booking and fills generated fields", func(t *testing.T) {
		b := &Booking{UserID: u1, TherapistID: t1, ScheduledAt: at(10), Status: StatusPending}
		if err := repo.Create(ctx, b); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		if b.ID == "" {
			t.Error("expected the database to assign an id")
		}
		if b.CreatedAt.IsZero() || b.UpdatedAt.IsZero() {
			t.Error("expected the database to assign timestamps")
		}
		if b.Status != StatusPending {
			t.Errorf("expected status %q, got %q", StatusPending, b.Status)
		}
		if b.TherapistID != t1 || b.UserID != u1 {
			t.Errorf("expected the seeded identity, got %+v", b)
		}
	})

	t.Run("Create forbids the same therapist slot twice", func(t *testing.T) {
		b := &Booking{UserID: u2, TherapistID: t1, ScheduledAt: at(10), Status: StatusPending}
		if err := repo.Create(ctx, b); !errors.Is(err, ErrBookingConflict) {
			t.Fatalf("expected ErrBookingConflict for the taken slot, got %v", err)
		}
	})

	t.Run("Create lets a cancelled slot be rebooked", func(t *testing.T) {
		if _, err := repo.SetStatus(ctx, u1, mustBookingFor(ctx, t, repo, u1, t1), StatusPending, StatusCancelled); err != nil {
			t.Fatalf("failed to cancel the slot owner: %v", err)
		}
		b := &Booking{UserID: u1, TherapistID: t1, ScheduledAt: at(10), Status: StatusPending}
		if err := repo.Create(ctx, b); err != nil {
			t.Fatalf("expected the freed slot to be rebookable, got %v", err)
		}
	})

	t.Run("HasActiveOverlap detects overlapping sessions for the same user", func(t *testing.T) {
		// u1 holds 10:00 (rebooked above). 10:30 overlaps it; 11:00 is exactly
		// the boundary and does not; 12:00 is clean.
		overlap, err := repo.HasActiveOverlap(ctx, u1, at(10).Add(30*time.Minute))
		if err != nil {
			t.Fatalf("HasActiveOverlap returned error: %v", err)
		}
		if !overlap {
			t.Error("expected a 10:30 session to overlap the active 10:00 booking")
		}
		boundary, err := repo.HasActiveOverlap(ctx, u1, at(11))
		if err != nil {
			t.Fatalf("HasActiveOverlap returned error: %v", err)
		}
		if boundary {
			t.Error("expected an 11:00 session (adjacent, non-overlapping) to be allowed")
		}
		clean, err := repo.HasActiveOverlap(ctx, u1, at(12))
		if err != nil {
			t.Fatalf("HasActiveOverlap returned error: %v", err)
		}
		if clean {
			t.Error("expected a 12:00 session to be clean")
		}
		other, err := repo.HasActiveOverlap(ctx, u2, at(10).Add(30*time.Minute))
		if err != nil {
			t.Fatalf("HasActiveOverlap returned error: %v", err)
		}
		if other {
			t.Error("another user's calendar must not overlap u1's booking")
		}
	})

	t.Run("GetByID joins the therapist display name for the owner", func(t *testing.T) {
		id := mustBookingFor(ctx, t, repo, u1, t1)
		b, err := repo.GetByID(ctx, u1, id)
		if err != nil {
			t.Fatalf("GetByID returned error: %v", err)
		}
		if b.ID != id || b.UserID != u1 {
			t.Errorf("expected the owner's booking, got %+v", b)
		}
		if b.DisplayName != "Dr. Amina Korir" {
			t.Errorf("expected the therapist display name to be joined, got %q", b.DisplayName)
		}
		if b.TherapistID != t1 {
			t.Errorf("expected therapist id %q, got %q", t1, b.TherapistID)
		}
	})

	t.Run("GetByID hides a booking from a different user", func(t *testing.T) {
		id := mustBookingFor(ctx, t, repo, u1, t1)
		_, err := repo.GetByID(ctx, u2, id)
		if !errors.Is(err, ErrBookingNotFound) {
			t.Fatalf("expected ErrBookingNotFound for a foreign booking, got %v", err)
		}
	})

	t.Run("ListByUserID returns only the user's bookings newest-first", func(t *testing.T) {
		// Seed a second user's booking (u2) and multiple u1 bookings, with
		// explicit created_at so the ordering is deterministic.
		seed := func(user, therapist string, at time.Time, created time.Time, status string) string {
			var id string
			if err := pool.QueryRow(ctx, `
				INSERT INTO bookings (user_id, therapist_id, scheduled_at, status, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $5) RETURNING id`,
				user, therapist, at, status, created).Scan(&id); err != nil {
				t.Fatalf("failed to seed booking: %v", err)
			}
			return id
		}
		seed(u2, t2, at(13), time.Date(2026, 10, 1, 13, 30, 0, 0, time.UTC), StatusPending)
		seed(u1, t1, at(15), time.Date(2026, 10, 1, 15, 40, 0, 0, time.UTC), StatusPending)
		seed(u1, t1, at(14), time.Date(2026, 10, 1, 14, 50, 0, 0, time.UTC), StatusPending)

		bookings, err := repo.ListByUserID(ctx, u1)
		if err != nil {
			t.Fatalf("ListByUserID returned error: %v", err)
		}
		if len(bookings) < 2 {
			t.Fatalf("expected at least the 2 seeded u1 bookings, got %d", len(bookings))
		}
		for i := 1; i < len(bookings); i++ {
			if bookings[i-1].CreatedAt.Before(bookings[i].CreatedAt) {
				t.Errorf("list not newest-first at index %d", i)
			}
		}
		foreign := bookings[0]
		if foreign.UserID != u1 {
			t.Errorf("a booking belonging to %s leaked into u1's list", foreign.UserID)
		}
		u2List, err := repo.ListByUserID(ctx, u2)
		if err != nil {
			t.Fatalf("ListByUserID returned error: %v", err)
		}
		for _, b := range u2List {
			if b.UserID != u2 {
				t.Errorf("a booking belonging to %s leaked into u2's list", b.UserID)
			}
		}
		if len(u2List) != 1 {
			t.Errorf("expected exactly the 1 seeded u2 booking, got %d", len(u2List))
		}
	})

	t.Run("TherapistStatus reports existence and activity", func(t *testing.T) {
		found, active, err := repo.TherapistStatus(ctx, t1)
		if err != nil {
			t.Fatalf("TherapistStatus returned error: %v", err)
		}
		if !found || !active {
			t.Errorf("expected t1 found+active, got found=%v active=%v", found, active)
		}
		found, active, err = repo.TherapistStatus(ctx, t2)
		if err != nil {
			t.Fatalf("TherapistStatus returned error: %v", err)
		}
		if !found || active {
			t.Errorf("expected t2 found+inactive, got found=%v active=%v", found, active)
		}
		found, active, err = repo.TherapistStatus(ctx, "00000000-0000-0000-0000-000000000000")
		if err != nil {
			t.Fatalf("TherapistStatus returned error: %v", err)
		}
		if found || active {
			t.Errorf("expected an unknown therapist to be missing, got found=%v", found)
		}
	})

	t.Run("SetStatus transitions pending to cancelled", func(t *testing.T) {
		created := &Booking{UserID: u1, TherapistID: t1, ScheduledAt: at(16), Status: StatusPending}
		if err := repo.Create(ctx, created); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		cancelledID = created.ID
		b, err := repo.SetStatus(ctx, u1, cancelledID, StatusPending, StatusCancelled)
		if err != nil {
			t.Fatalf("SetStatus returned error: %v", err)
		}
		if b.Status != StatusCancelled {
			t.Errorf("expected status %q, got %q", StatusCancelled, b.Status)
		}
		if b.UpdatedAt.Before(created.CreatedAt) {
			t.Error("expected updated_at to refresh past created_at")
		}
	})

	t.Run("SetStatus rejects cancelling a non-pending booking", func(t *testing.T) {
		_, err := repo.SetStatus(ctx, u1, cancelledID, StatusPending, StatusCancelled)
		if !errors.Is(err, ErrBookingStatusConflict) {
			t.Fatalf("expected ErrBookingStatusConflict for a non-pending booking, got %v", err)
		}
	})

	t.Run("SetStatus returns ErrBookingNotFound for a foreign booking", func(t *testing.T) {
		_, err := repo.SetStatus(ctx, u2, cancelledID, StatusPending, StatusCancelled)
		if !errors.Is(err, ErrBookingNotFound) {
			t.Fatalf("expected ErrBookingNotFound for a foreign booking, got %v", err)
		}
	})

	t.Run("concurrent creates for the same slot admit exactly one", func(t *testing.T) {
		slot := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)
		const attempts = 5
		users := []string{u1, u2, u3}
		start := make(chan struct{})
		var wg sync.WaitGroup
		errs := make([]error, attempts)
		for i := 0; i < attempts; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				b := &Booking{
					UserID:      users[i%len(users)],
					TherapistID: t2,
					ScheduledAt: slot,
					Status:      StatusPending,
				}
				errs[i] = repo.Create(ctx, b)
			}(i)
		}
		close(start)
		wg.Wait()

		successes := 0
		conflicts := 0
		for _, err := range errs {
			switch {
			case err == nil:
				successes++
			case errors.Is(err, ErrBookingConflict):
				conflicts++
			default:
				t.Fatalf("unexpected error from concurrent create: %v", err)
			}
		}
		if successes != 1 {
			t.Fatalf("expected exactly 1 successful create, got %d", successes)
		}
		if conflicts != attempts-1 {
			t.Fatalf("expected %d conflicts, got %d", attempts-1, conflicts)
		}

		// Clean the race winner so later subtests are unaffected.
		if _, err := pool.Exec(ctx, "DELETE FROM bookings WHERE user_id = ANY($1) AND therapist_id = $2 AND scheduled_at = $3",
			users, t2, slot); err != nil {
			t.Fatalf("failed to clean raced booking: %v", err)
		}
	})
}

// TestPostgresRepositoryWindowGuardIntegration exercises the migration 015
// guarantees: any two ACTIVE bookings whose [scheduled_at, +60m) windows
// overlap must be rejected by the database itself, so concurrent requests
// cannot sneak overlapping sessions in. It is excluded from the default build
// via the "integration" tag and skipped when DATABASE_URL is not set.
func TestPostgresRepositoryWindowGuardIntegration(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Keep the concurrent exclusion subtests fast: a lost GiST race resolves
	// via deadlock detection (default 1s), and the repository retries it as a
	// clean conflict. Bounding detection here avoids multi-second waits.
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("failed to parse DATABASE_URL: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["deadlock_timeout"] = "100ms"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	repo := NewPostgresRepository(pool)

	const (
		u1 = "11111111-1111-1111-1111-111111111111"
		u2 = "22222222-2222-2222-2222-222222222222"
		u3 = "33333333-3333-3333-3333-333333333333"
		u4 = "44444444-4444-4444-4444-444444444444"
		t1 = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		t2 = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
		t3 = "cccccccc-cccc-cccc-cccc-cccccccccccc"
	)
	userIDs := []string{u1, u2, u3, u4}
	therapistIDs := []string{t1, t2, t3}

	seedUser := func(id string) {
		if _, err := pool.Exec(ctx, `
			INSERT INTO users (id, email, password_hash)
			VALUES ($1, $2, 'x')`, id, id+"@guard.test"); err != nil {
			t.Fatalf("failed to seed user %s: %v", id, err)
		}
	}
	seedTherapist := func(id string) {
		if _, err := pool.Exec(ctx, `
			INSERT INTO therapists (id, full_name, credentials, is_active)
			VALUES ($1, $2, 'MA', TRUE)`, id, "Dr. Guard "+id); err != nil {
			t.Fatalf("failed to seed therapist %s: %v", id, err)
		}
	}
	for _, id := range userIDs {
		seedUser(id)
	}
	for _, id := range therapistIDs {
		seedTherapist(id)
	}
	defer func() {
		cleanupCtx := context.Background()
		for _, id := range userIDs {
			if _, err := pool.Exec(cleanupCtx, "DELETE FROM users WHERE id = $1", id); err != nil {
				t.Errorf("cleanup failed to DELETE user %s: %v", id, err)
			}
		}
		for _, id := range therapistIDs {
			if _, err := pool.Exec(cleanupCtx, "DELETE FROM therapists WHERE id = $1", id); err != nil {
				t.Errorf("cleanup failed to DELETE therapist %s: %v", id, err)
			}
		}
	}()

	// Each scenario lives on its own calendar day so one result never leaks
	// into the next.
	at := func(day, hour, minute int) time.Time {
		return time.Date(2026, 12, day, hour, minute, 0, 0, time.UTC)
	}
	create := func(user, therapist string, start time.Time) error {
		return repo.Create(ctx, &Booking{
			UserID:      user,
			TherapistID: therapist,
			ScheduledAt: start,
			Status:      StatusPending,
		})
	}

	t.Run("same therapist overlapping windows rejected sequentially", func(t *testing.T) {
		if err := create(u1, t1, at(1, 10, 0)); err != nil {
			t.Fatalf("baseline booking failed: %v", err)
		}
		if err := create(u2, t1, at(1, 10, 30)); !errors.Is(err, ErrBookingConflict) {
			t.Fatalf("expected ErrBookingConflict for the overlapping window, got %v", err)
		}
	})

	t.Run("same therapist adjacent windows allowed", func(t *testing.T) {
		if err := create(u1, t2, at(2, 10, 0)); err != nil {
			t.Fatalf("baseline booking failed: %v", err)
		}
		if err := create(u2, t2, at(2, 11, 0)); err != nil {
			t.Fatalf("expected the adjacent 11:00 window to be allowed, got %v", err)
		}
	})

	t.Run("same user overlapping windows rejected sequentially", func(t *testing.T) {
		if err := create(u1, t3, at(3, 10, 0)); err != nil {
			t.Fatalf("baseline booking failed: %v", err)
		}
		if err := create(u1, t1, at(3, 10, 30)); !errors.Is(err, ErrBookingConflict) {
			t.Fatalf("expected ErrBookingConflict for the user's overlapping window, got %v", err)
		}
	})

	t.Run("cancelling a booking frees its whole interval", func(t *testing.T) {
		b := &Booking{UserID: u1, TherapistID: t2, ScheduledAt: at(4, 10, 0), Status: StatusPending}
		if err := repo.Create(ctx, b); err != nil {
			t.Fatalf("baseline booking failed: %v", err)
		}
		if _, err := repo.SetStatus(ctx, u1, b.ID, StatusPending, StatusCancelled); err != nil {
			t.Fatalf("failed to cancel: %v", err)
		}
		if err := create(u2, t2, at(4, 10, 30)); err != nil {
			t.Fatalf("expected the cancelled interval to be rebookable, got %v", err)
		}
	})

	t.Run("exact duplicate slot still rejected", func(t *testing.T) {
		if err := create(u1, t3, at(5, 10, 0)); err != nil {
			t.Fatalf("baseline booking failed: %v", err)
		}
		if err := create(u2, t3, at(5, 10, 0)); !errors.Is(err, ErrBookingConflict) {
			t.Fatalf("expected ErrBookingConflict for the exact duplicate slot, got %v", err)
		}
	})

	t.Run("different therapist at the same time allowed", func(t *testing.T) {
		if err := create(u1, t1, at(6, 10, 0)); err != nil {
			t.Fatalf("first therapist booking failed: %v", err)
		}
		if err := create(u2, t2, at(6, 10, 0)); err != nil {
			t.Fatalf("expected a different therapist at the same time to be allowed, got %v", err)
		}
	})

	t.Run("concurrent overlapping windows for the same therapist admit exactly one", func(t *testing.T) {
		const attempts = 5
		users := []string{u1, u2, u3, u4, u1}
		start := make(chan struct{})
		var wg sync.WaitGroup
		errs := make([]error, attempts)
		for i := 0; i < attempts; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				// Slots 10:00, 10:12, 10:24, 10:36, 10:48 all sit inside the
				// [10:00, 11:00) baseline window (max spread 48 min < 60), so
				// every pair overlaps and at most one can win.
				<-start
				errs[i] = create(users[i%len(users)], t1, at(7, 10, 12*i))
			}(i)
		}
		close(start)
		wg.Wait()

		successes, conflicts := 0, 0
		for _, err := range errs {
			switch {
			case err == nil:
				successes++
			case errors.Is(err, ErrBookingConflict):
				conflicts++
			default:
				t.Fatalf("unexpected error from concurrent create: %v", err)
			}
		}
		if successes != 1 {
			t.Fatalf("expected exactly 1 successful create for the overlapping bursts, got %d", successes)
		}
		if conflicts != attempts-1 {
			t.Fatalf("expected %d conflicts, got %d", attempts-1, conflicts)
		}
	})

	t.Run("concurrent overlapping windows for the same user admit exactly one", func(t *testing.T) {
		const attempts = 5
		therapists := []string{t1, t2, t3, t1, t2}
		start := make(chan struct{})
		var wg sync.WaitGroup
		errs := make([]error, attempts)
		for i := 0; i < attempts; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				// u1 firing 10:00, 10:12, 10:24, ... at different therapists;
				// every window overlaps the [10:00, 11:00) baseline.
				<-start
				errs[i] = create(u1, therapists[i%len(therapists)], at(8, 10, 12*i))
			}(i)
		}
		close(start)
		wg.Wait()

		successes, conflicts := 0, 0
		for _, err := range errs {
			switch {
			case err == nil:
				successes++
			case errors.Is(err, ErrBookingConflict):
				conflicts++
			default:
				t.Fatalf("unexpected error from concurrent create: %v", err)
			}
		}
		if successes != 1 {
			t.Fatalf("expected exactly 1 successful create for the user, got %d", successes)
		}
		if conflicts != attempts-1 {
			t.Fatalf("expected %d conflicts, got %d", attempts-1, conflicts)
		}
	})
}

// mustBookingFor resolves a booking id for the given user+therapist pair.
func mustBookingFor(ctx context.Context, t *testing.T, repo Repository, userID, therapistID string) string {
	bookings, err := repo.ListByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("ListByUserID returned error: %v", err)
	}
	for _, b := range bookings {
		if b.TherapistID == therapistID {
			return b.ID
		}
	}
	t.Fatalf("no booking for user %s therapist %s", userID, therapistID)
	return ""
}
