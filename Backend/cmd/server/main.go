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
	"Backend/internal/ai"
	"Backend/internal/anon"
	"Backend/internal/auth"
	"Backend/internal/bookings"
	"Backend/internal/cipher"
	"Backend/internal/circles"
	"Backend/internal/config"
	"Backend/internal/dashboard"
	"Backend/internal/journal"
	"Backend/internal/middleware"
	"Backend/internal/mood"
	"Backend/internal/therapists"
	"Backend/internal/user"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// setupRouter initializes the Gin engine, global middleware, and foundational routes.
func setupRouter(cfg *config.Config, pool *pgxpool.Pool, authHandler *user.Handler, tokenManager *auth.Manager, anonHandler *anon.Handler, anonService anon.Service, moodHandler *mood.Handler, dashboardHandler *dashboard.Handler, journalHandler *journal.Handler, circlesHandler *circles.Handler, therapistsHandler *therapists.Handler, bookingsHandler *bookings.Handler) *gin.Engine {
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

		// Anonymous session routes. Guarded so unit tests can pass nil values.
		if anonHandler != nil && anonService != nil {
			anonGroup := v1.Group("/auth")
			{
				// Creating a session is public: it mints the anonymous token.
				anonGroup.POST("/anonymous", anonHandler.Create)

				// The /anonymous/me endpoint requires a valid anonymous Bearer
				// token (distinct from registered-user JWTs).
				anonGroup.GET("/anonymous/me", middleware.AnonymousAuthRequired(anonService), anonHandler.Me)

				// Promoting an anonymous identity to a registered account also
				// authenticates with the anonymous middleware so the identity is
				// recovered from the token; the user handler does the promote.
				if authHandler != nil {
					anonGroup.POST("/anonymous/promote",
						middleware.AnonymousAuthRequired(anonService), authHandler.Promote)
				}
			}
		}

		// Phase 4 wellness routes. All require a registered-user JWT; the
		// identical profile/mood/dashboard routes reject anonymous tokens.
		if tokenManager != nil && authHandler != nil {
			users := v1.Group("/users")
			{
				users.GET("/me", middleware.AuthRequired(tokenManager), authHandler.GetProfile)
				users.PATCH("/me", middleware.AuthRequired(tokenManager), authHandler.UpdateProfile)
			}
		}
		if tokenManager != nil && moodHandler != nil {
			moods := v1.Group("/moods")
			{
				moods.POST("", middleware.AuthRequired(tokenManager), moodHandler.Create)
				moods.GET("", middleware.AuthRequired(tokenManager), moodHandler.List)
			}
		}
		if tokenManager != nil && dashboardHandler != nil {
			dashboard := v1.Group("/dashboard")
			{
				dashboard.GET("", middleware.AuthRequired(tokenManager), dashboardHandler.Get)
			}
		}
		if tokenManager != nil && journalHandler != nil {
			// All journal routes require a registered-user JWT; anonymous
			// tokens are rejected by AuthRequired. Ownership never comes from
			// the request: each handler derives the user from the context.
			journalGroup := v1.Group("/journal")
			{
				journalGroup.POST("", middleware.AuthRequired(tokenManager), journalHandler.Create)
				journalGroup.GET("", middleware.AuthRequired(tokenManager), journalHandler.List)
				journalGroup.GET("/:id", middleware.AuthRequired(tokenManager), journalHandler.Get)
				journalGroup.PATCH("/:id", middleware.AuthRequired(tokenManager), journalHandler.Update)
				journalGroup.DELETE("/:id", middleware.AuthRequired(tokenManager), journalHandler.Delete)
				journalGroup.POST("/:id/reflect", middleware.AuthRequired(tokenManager), journalHandler.Reflect)
			}
		}
		if anonService != nil && circlesHandler != nil {
			// Phase 6.1 peer support circles. Circles are an anonymous-session
			// feature: memberships and messages are keyed to anon_identities,
			// and every route (discovery through messaging) requires a valid
			// anonymous bearer token. Registered-user JWTs are rejected by
			// AnonymousAuthRequired.
			circlesGroup := v1.Group("/circles", middleware.AnonymousAuthRequired(anonService))
			{
				circlesGroup.GET("", circlesHandler.List)
				circlesGroup.GET("/:id", circlesHandler.Get)
				circlesGroup.POST("/:id/join", circlesHandler.Join)
				circlesGroup.DELETE("/:id/leave", circlesHandler.Leave)
				circlesGroup.GET("/:id/messages", circlesHandler.ListMessages)
				circlesGroup.POST("/:id/messages", circlesHandler.SendMessage)
			}
		}
		if tokenManager != nil && therapistsHandler != nil {
			// Phase 6.2 therapist discovery. The directory is public catalog
			// data, but browsing it is a registered-user feature: every route
			// requires a valid JWT (AuthRequired). Anonymous tokens are
			// rejected. Profiles expose only public fields, never personal or
			// credential material.
			therapistsGroup := v1.Group("/therapists")
			{
				therapistsGroup.GET("", middleware.AuthRequired(tokenManager), therapistsHandler.List)
				therapistsGroup.GET("/:id", middleware.AuthRequired(tokenManager), therapistsHandler.Get)
			}
		}
		if tokenManager != nil && bookingsHandler != nil {
			// Phase 6.3 therapist bookings. Every route requires a registered
			// user JWT; anonymous tokens are rejected. Create lives under a
			// therapist, and the list/get/cancel routes are scoped to the
			// authenticated user so one person's bookings are never exposed
			// to another.
			therapistBookingsGroup := v1.Group("/therapists")
			{
				therapistBookingsGroup.POST("/:id/bookings", middleware.AuthRequired(tokenManager), bookingsHandler.Create)
			}
			bookingsGroup := v1.Group("/bookings", middleware.AuthRequired(tokenManager))
			{
				bookingsGroup.GET("", bookingsHandler.List)
				bookingsGroup.GET("/:id", bookingsHandler.Get)
				bookingsGroup.PATCH("/:id/cancel", bookingsHandler.Cancel)
			}
		}
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

	// 4. Compose the anonymous session stack. Session tokens are opaque and
	// only their SHA-256 hashes are stored; the same service marks last_seen_at
	// on every authenticated anonymous request.
	anonRepo := anon.NewPostgresRepository(pool)
	anonService := anon.NewService(anonRepo)
	anonHandler := anon.NewHandler(anonService)

	// 4b. Compose the Phase 4 wellness stacks: mood check-ins and the
	// dashboard, which reuses the existing user and mood services.
	moodRepo := mood.NewPostgresRepository(pool)
	moodService := mood.NewService(moodRepo)
	moodHandler := mood.NewHandler(moodService)

	dashboardService := dashboard.NewService(userService, moodService)
	dashboardHandler := dashboard.NewHandler(dashboardService)

	// 4c. Compose the Phase 5 journal stack. Journal content is encrypted with
	// AES-256-GCM before it ever reaches the repository, using the server-side
	// JOURNAL_ENCRYPTION_KEY. A failing or missing ANTHROPIC_API_KEY only
	// disables AI reflections; the journal itself keeps working.
	journalCodec, err := cipher.NewAESGCM([]byte(cfg.JournalKey))
	if err != nil {
		log.Fatalf("Journal encryption key error: %v", err)
	}
	var reflection journal.ReflectionGenerator
	if cfg.AnthropicKey != "" {
		reflection = ai.NewClient(cfg.AnthropicKey, ai.DefaultModel)
	}
	journalService := journal.NewService(journal.NewPostgresRepository(pool), journalCodec, reflection)
	journalHandler := journal.NewHandler(journalService)

	// 4d. Compose the Phase 6.1 circles stack. Memberships and messages are
	// keyed to anonymous identities; the same anonymous-session service that
	// mints tokens authenticates every circle request.
	circlesHandler := circles.NewHandler(circles.NewService(circles.NewPostgresRepository(pool)))

	// 4e. Compose the Phase 6.2 therapist discovery stack. Browsing is
	// registered-user only; profiles are public catalog data, so no ownership
	// or authorization decisions live in the handlers themselves.
	therapistsHandler := therapists.NewHandler(therapists.NewService(therapists.NewPostgresRepository(pool)))

	// 4f. Compose the Phase 6.3 therapist booking stack. Bookings tie a
	// registered user to an active therapist's slot; both the service (overlap
	// of different-but-adjacent times) and the schema (partial unique indexes
	// for exact-minute races) defend against double-booking.
	bookingsHandler := bookings.NewHandler(bookings.NewService(bookings.NewPostgresRepository(pool)))

	// 5. Setup router and middleware; the same token manager validates the
	// Bearer tokens on the protected routes.
	router := setupRouter(cfg, pool, userHandler, tokenManager, anonHandler, anonService, moodHandler, dashboardHandler, journalHandler, circlesHandler, therapistsHandler, bookingsHandler)

	// 6. Configure HTTP server
	serverAddr := ":" + cfg.Port
	srv := &http.Server{
		Addr:           serverAddr,
		Handler:        router,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	// 7. Start HTTP server in a separate goroutine
	go func() {
		log.Printf("🌿 Soulwe API server listening on %s [%s mode]", serverAddr, cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server startup failed: %v", err)
		}
	}()

	// 8. Graceful shutdown listening on OS interrupt signals
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
