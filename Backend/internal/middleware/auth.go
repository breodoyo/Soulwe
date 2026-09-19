package middleware

import (
	"strings"

	"Backend/internal/auth"

	"github.com/gin-gonic/gin"
)

// UserIDKey is the Gin context key under which the authenticated user ID is
// stored by AuthRequired. Handlers read it via UserIDFromContext.
const UserIDKey = "user_id"

const bearerPrefix = "Bearer "

// AuthRequired returns Gin middleware that authenticates the request's
// Bearer access token. On success it stores the authenticated user ID in the
// request context and lets the handler continue. On any failure it aborts the
// request with a safe, generic 401 — parsing details are never sent back.
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

// bearerToken extracts the raw credentials from the Authorization header and
// requires the Bearer scheme with a non-empty token. The boolean reports
// success; failures are deliberately indistinguishable. It never logs the
// header or the token. Callers interpret the raw token (JWT vs anonymous).
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

// parseBearerToken extracts the token from the Authorization header, requires
// the Bearer scheme, and validates it against the token manager. The boolean
// reports success; failures are deliberately indistinguishable. It never logs
// the header or the token.
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

// UserIDFromContext returns the authenticated user ID stored by AuthRequired.
// The boolean reports whether a non-empty ID was present.
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
