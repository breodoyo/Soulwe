package config

import (
	"log"
	"os"

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

// getEnv retrieves the value of the environment variable named by key,
// or returns fallback if the variable is empty or not present.
func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return fallback
}
