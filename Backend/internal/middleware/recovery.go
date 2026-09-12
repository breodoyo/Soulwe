package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
)

// Recovery catches any panic in downstream handlers, logs the stack trace safely,
// and sends a clean 500 JSON response instead of crashing the server process.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("Unhandled Panic Recovered",
					slog.Any("error", r),
					slog.String("stack", string(debug.Stack())),
				)

				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": gin.H{
						"code":    "INTERNAL_SERVER_ERROR",
						"message": "An unexpected server error occurred",
					},
				})
			}
		}()

		c.Next()
	}
}
