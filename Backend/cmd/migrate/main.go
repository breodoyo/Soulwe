package main

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/joho/godotenv"
)

const migrationsPath = "file://db/migrations"

func main() {
	_ = godotenv.Load()

	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	command := os.Args[1]

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required (set it in .env or the environment)")
	}

	m, err := migrate.New(migrationsPath, databaseURL)
	if err != nil {
		log.Fatalf("failed to initialize migrations: %v", err)
	}
	defer m.Close()

	switch command {
	case "up":
		err = m.Up()
		if errors.Is(err, migrate.ErrNoChange) {
			fmt.Println("no migrations to apply; database is up to date")
			return
		}
	case "down":
		if len(os.Args) > 2 && os.Args[2] == "-all" {
			err = m.Down()
		} else {
			err = m.Steps(-1)
		}
	case "version":
		version, dirty, verr := m.Version()
		if errors.Is(verr, migrate.ErrNilVersion) {
			fmt.Println("no migrations applied yet")
			return
		}
		if verr != nil {
			log.Fatalf("failed to read migration version: %v", verr)
		}
		fmt.Printf("version %d%s\n", version, dirtyIndicator(dirty))
		return
	default:
		usage()
		os.Exit(1)
	}

	if err != nil {
		log.Fatalf("migration failed: %v", err)
	}
	fmt.Printf("migrations %s completed successfully\n", command)
}

func dirtyIndicator(dirty bool) string {
	if dirty {
		return " (dirty)"
	}
	return ""
}

func usage() {
	fmt.Println(`Usage:
  go run ./cmd/migrate up          Apply all pending migrations
  go run ./cmd/migrate down        Roll back the last batch of migrations
  go run ./cmd/migrate down -all   Roll back all migrations
  go run ./cmd/migrate version     Print the current migration version`)
}