package middleware

import (
	"strings"

	"Backend/internal/auth"

	"github.com/gin-gonic/gin"
)

// UserIDKey is the Gin context key AuthRequired stores the user ID under.
const UserIDKey = "user_id"

const bearerPrefix = "Bearer "

// AuthRequired authenticates the request's Bearer access token and stores the
// user ID in the context. Every failure aborts with a generic 401, never
// sending parsing details back.
func AuthRequired(tokenManager *auth.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := parseBearerToken(c, tokenManager)
		if !ok {
			abortUnauthorized(c)
			return
		}

		c.Set(UserIDKey, userID)
		c.Next()
	}
}

// bearerToken extracts a non-empty Bearer credential. Failures are
// deliberately indistinguishable and the token is never logged.
func bearerToken(c *gin.Context) (string, bool) {
	header := c.GetHeader("Authorization")
	if header == "" {
		return "", false
	}
	if !strings.HasPrefix(header, bearerPrefix) {
		return "", false
	}

	token := strings.TrimSpace(strings.TrimPrefix(header, bearerPrefix))
	if token == "" {
		return "", false
	}

	return token, true
}

// parseBearerToken validates a Bearer credential as a JWT access token.
func parseBearerToken(c *gin.Context, tokenManager *auth.Manager) (string, bool) {
	token, ok := bearerToken(c)
	if !ok {
		return "", false
	}

	userID, err := tokenManager.ParseAccessToken(token)
	if err != nil {
		return "", false
	}
	return userID, true
}

// UserIDFromContext returns the ID stored by AuthRequired; ok is false when absent.
func UserIDFromContext(c *gin.Context) (string, bool) {
	value, ok := c.Get(UserIDKey)
	if !ok {
		return "", false
	}
	userID, ok := value.(string)
	if !ok || strings.TrimSpace(userID) == "" {
		return "", false
	}
	return userID, true
}
