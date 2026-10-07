package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// AnonIdentityIDKey is the Gin context key AnonymousAuthRequired stores the
// identity ID under.
const AnonIdentityIDKey = "anon_identity_id"

// AnonymousVerifier authenticates an anonymous bearer token; it keeps the
// middleware free of a dependency back onto anon. The raw token must never be
// stored or logged.
type AnonymousVerifier interface {
	Authenticate(ctx context.Context, rawToken string) (identityID string, ok bool, err error)
}

// AnonymousAuthRequired authenticates a Bearer anonymous token and stores the
// identity ID in the context. Unknown credentials abort with a generic 401,
// repository failures with a 500. The raw token never enters the context.
func AnonymousAuthRequired(verifier AnonymousVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c)
		if !ok {
			abortUnauthorized(c)
			return
		}

		identityID, ok, err := verifier.Authenticate(c.Request.Context(), token)
		if err != nil {
			slog.Error("anonymous authentication failed", slog.String("error", err.Error()))
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": gin.H{
					"code":    "INTERNAL_SERVER_ERROR",
					"message": "An unexpected server error occurred",
				},
			})
			return
		}
		if !ok {
			abortUnauthorized(c)
			return
		}

		c.Set(AnonIdentityIDKey, identityID)
		c.Next()
	}
}

// AnonIdentityIDFromContext returns the ID stored by AnonymousAuthRequired.
func AnonIdentityIDFromContext(c *gin.Context) (string, bool) {
	value, ok := c.Get(AnonIdentityIDKey)
	if !ok {
		return "", false
	}
	identityID, ok := value.(string)
	if !ok || strings.TrimSpace(identityID) == "" {
		return "", false
	}
	return identityID, true
}

// abortUnauthorized writes the generic 401 shared by both auth middlewares.
func abortUnauthorized(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"error": gin.H{
			"code":    "UNAUTHORIZED",
			"message": "authentication required",
		},
	})
}
