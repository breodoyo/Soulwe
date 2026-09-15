//go:build integration

package db

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestOpenPing verifies the connection pool can connect to a running PostgreSQL.
// Requires a real DATABASE_URL and is skipped when the variable is missing.
func TestOpenPing(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to open pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("failed to ping database: %v", err)
	}
}