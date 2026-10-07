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
	"Backend/internal/breathing"
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

func setupRouter(cfg *config.Config, pool *pgxpool.Pool, authHandler *user.Handler, tokenManager *auth.Manager, anonHandler *anon.Handler, anonService anon.Service, moodHandler *mood.Handler, dashboardHandler *dashboard.Handler, journalHandler *journal.Handler, circlesHandler *circles.Handler, therapistsHandler *therapists.Handler, bookingsHandler *bookings.Handler, breathingHandler *breathing.Handler) *gin.Engine {
	gin.SetMode(cfg.GinMode)

	// Blank engine: the custom logger/recovery below replace the defaults.
	r := gin.New()
	r.Use(middleware.Logger())
	r.Use(middleware.Recovery())
	r.Use(middleware.CORS(cfg.FrontendURL))

	// Liveness: deliberately no database dependency.
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": "soulwe-api",
			"env":     cfg.Env,
			"time":    time.Now().UTC().Format(time.RFC3339),
		})
	})

	// Readiness: pings the database with a 2s budget.
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

	v1 := r.Group("/api/v1")
	{
		v1.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"status":  "ok",
				"version": "v1",
			})
		})

		// Nil guards let unit tests pass nil handlers.
		if authHandler != nil {
			auth := v1.Group("/auth")
			{
				auth.POST("/register", authHandler.Register)
				auth.POST("/login", authHandler.Login)

				if tokenManager != nil {
					auth.GET("/me", middleware.AuthRequired(tokenManager), authHandler.Me)
				}
			}
		}

		if anonHandler != nil && anonService != nil {
			anonGroup := v1.Group("/auth")
			{
				// Public: this is what mints the anonymous token.
				anonGroup.POST("/anonymous", anonHandler.Create)

				anonGroup.GET("/anonymous/me", middleware.AnonymousAuthRequired(anonService), anonHandler.Me)

				// Promote authenticates with the anonymous middleware so the identity
				// is recovered from the token; the user handler does the promote.
				if authHandler != nil {
					anonGroup.POST("/anonymous/promote",
						middleware.AnonymousAuthRequired(anonService), authHandler.Promote)
				}
			}
		}

		if tokenManager != nil && authHandler != nil {
			users := v1.Group("/users")
			{
				users.GET("/me", middleware.AuthRequired(tokenManager), authHandler.GetProfile)
				users.PATCH("/me", middleware.AuthRequired(tokenManager), authHandler.UpdateProfile)
			}
		}
		// IdentityRequired never confuses credentials: a JWT yields a user, an
		// anonymous token an anonymous session.
		identity := middleware.IdentityRequired(tokenManager, anonService)
		if tokenManager != nil && moodHandler != nil {
			moods := v1.Group("/moods", identity)
			{
				moods.POST("", moodHandler.Create)
				moods.GET("", moodHandler.List)
			}
		}
		if tokenManager != nil && dashboardHandler != nil {
			// Registered-only: it includes the account profile.
			dashboard := v1.Group("/dashboard")
			{
				dashboard.GET("", middleware.AuthRequired(tokenManager), dashboardHandler.Get)
			}
		}
		if tokenManager != nil && journalHandler != nil {
			// Ownership comes from the resolved identity in the context, and the
			// ciphertext is bound to that owner.
			journalGroup := v1.Group("/journal", identity)
			{
				journalGroup.POST("", journalHandler.Create)
				journalGroup.GET("", journalHandler.List)
				journalGroup.GET("/:id", journalHandler.Get)
				journalGroup.PATCH("/:id", journalHandler.Update)
				journalGroup.DELETE("/:id", journalHandler.Delete)
				journalGroup.POST("/:id/reflect", journalHandler.Reflect)
			}
		}
		// tokenManager is in the guard because identity parses a registered token
		// first; without one these routes must not be wired.
		if tokenManager != nil && anonService != nil && circlesHandler != nil {
			// Registrants post under their display name, anonymous sessions under
			// their pseudonym, and the two are never mixed.
			circlesGroup := v1.Group("/circles", identity)
			{
				circlesGroup.GET("", circlesHandler.List)
				circlesGroup.GET("/:id", circlesHandler.Get)
				circlesGroup.POST("/:id/join", circlesHandler.Join)
				circlesGroup.DELETE("/:id/leave", circlesHandler.Leave)
				circlesGroup.GET("/:id/messages", circlesHandler.ListMessages)
				circlesGroup.POST("/:id/messages", circlesHandler.SendMessage)
			}
		}
		if therapistsHandler != nil {
			// Public catalog data with no owner, so discovery is browsable by everyone.
			therapistsGroup := v1.Group("/therapists")
			{
				therapistsGroup.GET("", therapistsHandler.List)
				therapistsGroup.GET("/:id", therapistsHandler.Get)
			}
		}
		if tokenManager != nil && bookingsHandler != nil {
			// Registered-user JWT only; anonymous tokens are rejected. Routes are scoped
			// to the authenticated user so one person's bookings stay private.
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
		if breathingHandler != nil {
			// Shared public catalog; sessions accept either credential.
			breathingGroup := v1.Group("/breathing")
			{
				breathingGroup.GET("/exercises", breathingHandler.ListExercises)
				breathingGroup.GET("/exercises/:id", breathingHandler.GetExercise)
				if tokenManager != nil {
					breathingGroup.POST("/sessions", identity, breathingHandler.RecordSession)
					breathingGroup.GET("/sessions", identity, breathingHandler.ListSessions)
				}
			}
		}
	}

	return r
}

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	// Migrations are NOT run at startup.
	ctx := context.Background()
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer pool.Close()
	log.Println("🌿 Database connection pool established")

	tokenManager, err := auth.NewManager(cfg.JWTSecret)
	if err != nil {
		log.Fatalf("JWT signing secret error: %v", err)
	}
	userRepo := user.NewPostgresRepository(pool)
	userService := user.NewService(userRepo, tokenManager)
	userHandler := user.NewHandler(userService)

	// Session tokens are opaque; only their SHA-256 hashes are stored.
	anonRepo := anon.NewPostgresRepository(pool)
	anonService := anon.NewService(anonRepo)
	anonHandler := anon.NewHandler(anonService)

	moodRepo := mood.NewPostgresRepository(pool)
	moodService := mood.NewService(moodRepo)
	moodHandler := mood.NewHandler(moodService)

	dashboardService := dashboard.NewService(userService, moodService)
	dashboardHandler := dashboard.NewHandler(dashboardService)

	// Content is AES-256-GCM encrypted before it reaches the repository. A missing
	// or failing ANTHROPIC_API_KEY only disables AI reflections.
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

	// Memberships and messages are keyed to anonymous identities.
	circlesHandler := circles.NewHandler(circles.NewService(circles.NewPostgresRepository(pool)))

	therapistsHandler := therapists.NewHandler(therapists.NewService(therapists.NewPostgresRepository(pool)))

	// Both the service (overlap of different-but-adjacent times) and the schema
	// (partial unique indexes for exact-minute races) prevent double-booking.
	bookingsHandler := bookings.NewHandler(bookings.NewService(bookings.NewPostgresRepository(pool)))

	breathingHandler := breathing.NewHandler(breathing.NewService(breathing.NewPostgresRepository(pool)))

	router := setupRouter(cfg, pool, userHandler, tokenManager, anonHandler, anonService, moodHandler, dashboardHandler, journalHandler, circlesHandler, therapistsHandler, bookingsHandler, breathingHandler)

	serverAddr := ":" + cfg.Port
	srv := &http.Server{
		Addr:           serverAddr,
		Handler:        router,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	go func() {
		log.Printf("🌿 Soulwe API server listening on %s [%s mode]", serverAddr, cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server startup failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	sig := <-quit
	log.Printf("Received signal '%v'. Initiating graceful shutdown...", sig)

	// Up to 5 seconds for in-flight requests to finish.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Soulwe API server stopped cleanly")
}
