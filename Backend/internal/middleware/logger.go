package middleware

import (
	"log/slog"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

// Logger returns a Gin middleware that records structured request logs.
// Each log entry contains the HTTP method, path, response status, duration, client IP, and user-agent.
func Logger() gin.HandlerFunc {
	// Use standard library slog with JSON handler for structured output
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// Execute next handlers in chain
		c.Next()

		// Compute request duration and gather response details
		latency := time.Since(start)
		status := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method

		if raw != "" {
			path = path + "?" + raw
		}

		logger.Info("HTTP Request",
			slog.String("method", method),
			slog.String("path", path),
			slog.Int("status", status),
			slog.Duration("latency", latency),
			slog.String("client_ip", clientIP),
			slog.String("user_agent", c.Request.UserAgent()),
		)
	}
}
