package config

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Config encapsulates all environment configuration for the Soulwe backend.
type Config struct {
	Port             string
	Env              string
	GinMode          string
	FrontendURL      string
	DatabaseURL      string
	JWTSecret        string
	JWTRefreshSecret string
	JournalKey       string
	AnthropicKey     string
}

// Load reads settings from the .env file (if present) and system environment variables.
func Load() *Config {
	// Attempt to load .env; if it fails (e.g. production container), continue with environment variables.
	if err := godotenv.Load(); err != nil {
		log.Println("Note: No .env file loaded, reading configuration directly from system environment")
	}

	return &Config{
		Port:             getEnv("PORT", "8080"),
		Env:              getEnv("ENV", "development"),
		GinMode:          getEnv("GIN_MODE", "debug"),
		FrontendURL:      getEnv("FRONTEND_URL", "http://localhost:5173"),
		DatabaseURL:      getEnv("DATABASE_URL", ""),
		JWTSecret:        getEnv("JWT_SECRET", ""),
		JWTRefreshSecret: getEnv("JWT_REFRESH_SECRET", ""),
		JournalKey:       getEnv("JOURNAL_ENCRYPTION_KEY", ""),
		AnthropicKey:     getEnv("ANTHROPIC_API_KEY", ""),
	}
}

// Validate checks that required configuration values are present and valid.
func (c *Config) Validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required but not set; add it to your .env or environment")
	}

	parsed, err := url.Parse(c.DatabaseURL)
	if err != nil {
		return fmt.Errorf("DATABASE_URL is not a valid URL: %w", err)
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "postgres" && scheme != "postgresql" {
		return fmt.Errorf("DATABASE_URL must use postgres:// or postgresql:// scheme, got %s://", scheme)
	}

	if c.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET is required but not set; add it to your .env or environment")
	}

	return nil
}

// getEnv retrieves the value of the environment variable named by key,
// or returns fallback if the variable is empty or not present.
func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return fallback
}
