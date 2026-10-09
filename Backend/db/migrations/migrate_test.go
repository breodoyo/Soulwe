//go:build integration

package migrations

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
)

var migrationsPath = func() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("cannot resolve current source file path")
	}
	return "file://" + filepath.ToSlash(filepath.Dir(filename))
}()

func TestMigrationsRoundTrip(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	m, err := migrate.New(migrationsPath, databaseURL)
	if err != nil {
		t.Fatalf("failed to initialize migrations: %v", err)
	}
	defer m.Close()

	_ = m.Down()

	// Destructive: rolls the configured database back before applying.
	err = m.Up()
	if err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	expectedTables := []string{
		"users",
		"anon_identities",
		"journal_entries",
		"mood_logs",
		"circles",
		"circle_members",
		"circle_messages",
		"message_flags",
		"therapists",
		"therapist_languages",
		"breathing_sessions",
		"bookings",
	}
	for _, table := range expectedTables {
		var exists bool
		err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = $1
			)`, table).Scan(&exists)
		if err != nil {
			t.Fatalf("failed to check table %s: %v", table, err)
		}
		if !exists {
			t.Errorf("expected table %s to exist after migrations", table)
		}
	}

	var seedCount int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM circles`).Scan(&seedCount)
	if err != nil {
		t.Fatalf("failed to count circles: %v", err)
	}
	if seedCount != 8 {
		t.Errorf("expected 8 seeded circles, got %d", seedCount)
	}

	err = m.Down()
	if err != nil {
		t.Fatalf("failed to roll back migrations: %v", err)
	}

	for _, table := range expectedTables {
		var exists bool
		err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = $1
			)`, table).Scan(&exists)
		if err != nil {
			t.Fatalf("failed to check table %s: %v", table, err)
		}
		if exists {
			t.Errorf("expected table %s to be dropped after migration down", table)
		}
	}
}
