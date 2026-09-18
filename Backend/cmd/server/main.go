package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"Backend/db"
	"Backend/internal/auth"
	"Backend/internal/config"
	"Backend/internal/middleware"
	"Backend/internal/user"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// setupRouter initializes the Gin engine, global middleware, and foundational routes.
func setupRouter(cfg *config.Config, pool *pgxpool.Pool, authHandler *user.Handler, tokenManager *auth.Manager) *gin.Engine {
	// Set Gin mode (debug or release)
	gin.SetMode(cfg.GinMode)

	// Create a new blank Gin engine without default logger/recovery (we attach our custom ones)
	r := gin.New()

	// Attach custom middleware
	r.Use(middleware.Logger())
	r.Use(middleware.Recovery())
	r.Use(middleware.CORS(cfg.FrontendURL))

	// Base Health Check endpoint (liveness — no database dependency)
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": "soulwe-api",
			"env":     cfg.Env,
			"time":    time.Now().UTC().Format(time.RFC3339),
		})
	})

	// Database-aware readiness endpoint
	r.GET("/readyz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		if err := pool.Ping(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not ready",
				"db":     "disconnected",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"db":     "connected",
		})
	})

	// /api/v1 root route group
	v1 := r.Group("/api/v1")
	{
		v1.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"status":  "ok",
				"version": "v1",
			})
		})

		// Authentication routes. Guarded so unit tests can pass a nil handler.
		if authHandler != nil {
			auth := v1.Group("/auth")
			{
				auth.POST("/register", authHandler.Register)
				auth.POST("/login", authHandler.Login)

				// /me is the only protected route in this phase. It requires a
				// valid Bearer access token; /register and /login stay public.
				if tokenManager != nil {
					auth.GET("/me", middleware.AuthRequired(tokenManager), authHandler.Me)
				}
			}
		}

		// Domain route groups (/users, /dashboard, /moods, /affirmations,
		// /journal, /circles, /therapists, /breathing) will be registered here
		// in subsequent phases as their Handler -> Service -> Repository layers are implemented.
	}

	return r
}

func main() {
	// 1. Load application configuration and validate required values
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	// 2. Open the database connection pool (migrations are NOT run at startup)
	ctx := context.Background()
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer pool.Close()
	log.Println("🌿 Database connection pool established")

	// 3. Compose the users auth stack (Handler → Service → Repository), with
	// a JWT manager that signs access tokens using the validated JWT_SECRET.
	tokenManager, err := auth.NewManager(cfg.JWTSecret)
	if err != nil {
		log.Fatalf("JWT signing secret error: %v", err)
	}
	userRepo := user.NewPostgresRepository(pool)
	userService := user.NewService(userRepo, tokenManager)
	userHandler := user.NewHandler(userService)

	// 4. Setup router and middleware; the same token manager validates the
	// Bearer tokens on the protected routes.
	router := setupRouter(cfg, pool, userHandler, tokenManager)

	// 5. Configure HTTP server
	serverAddr := ":" + cfg.Port
	srv := &http.Server{
		Addr:           serverAddr,
		Handler:        router,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	// 6. Start HTTP server in a separate goroutine
	go func() {
		log.Printf("🌿 Soulwe API server listening on %s [%s mode]", serverAddr, cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server startup failed: %v", err)
		}
	}()

	// 7. Graceful shutdown listening on OS interrupt signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Block until a shutdown signal is received
	sig := <-quit
	log.Printf("Received signal '%v'. Initiating graceful shutdown...", sig)

	// Allow up to 5 seconds for in-flight requests to finish
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Soulwe API server stopped cleanly")
}
