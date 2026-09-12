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

	"Backend/internal/config"
	"Backend/internal/middleware"

	"github.com/gin-gonic/gin"
)

// setupRouter initializes the Gin engine, global middleware, and foundational routes.
func setupRouter(cfg *config.Config) *gin.Engine {
	// Set Gin mode (debug or release)
	gin.SetMode(cfg.GinMode)

	// Create a new blank Gin engine without default logger/recovery (we attach our custom ones)
	r := gin.New()

	// Attach custom middleware
	r.Use(middleware.Logger())
	r.Use(middleware.Recovery())
	r.Use(middleware.CORS(cfg.FrontendURL))

	// Base Health Check endpoint
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": "soulwe-api",
			"env":     cfg.Env,
			"time":    time.Now().UTC().Format(time.RFC3339),
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

		// Domain route groups (/auth, /users, /dashboard, /moods, /affirmations,
		// /journal, /circles, /therapists, /breathing) will be registered here
		// in subsequent phases as their Handler -> Service -> Repository layers are implemented.
	}

	return r
}

func main() {
	// 1. Load application configuration
	cfg := config.Load()

	// 2. Setup router and middleware
	router := setupRouter(cfg)

	// 3. Configure HTTP server
	serverAddr := ":" + cfg.Port
	srv := &http.Server{
		Addr:           serverAddr,
		Handler:        router,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	// 4. Start HTTP server in a separate goroutine
	go func() {
		log.Printf("🌿 Soulwe API server listening on %s [%s mode]", serverAddr, cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server startup failed: %v", err)
		}
	}()

	// 5. Graceful shutdown listening on OS interrupt signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Block until a shutdown signal is received
	sig := <-quit
	log.Printf("Received signal '%v'. Initiating graceful shutdown...", sig)

	// Allow up to 5 seconds for in-flight requests to finish
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Soulwe API server stopped cleanly")
}
