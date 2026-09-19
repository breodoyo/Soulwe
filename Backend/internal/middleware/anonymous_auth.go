package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// AnonIdentityIDKey is the Gin context key under which the authenticated
// anonymous identity ID is stored by AnonymousAuthRequired. Handlers read it
// via AnonIdentityIDFromContext.
const AnonIdentityIDKey = "anon_identity_id"

// AnonymousVerifier authenticates an anonymous bearer token. It is satisfied
// by the anon domain's Service; the interface keeps the middleware package
// free of a dependency back onto anon. The raw token must never be stored or
// logged.
type AnonymousVerifier interface {
	Authenticate(ctx context.Context, rawToken string) (identityID string, ok bool, err error)
}

// AnonymousAuthRequired returns Gin middleware that authenticates a Bearer
// anonymous token. On success it stores the anonymous identity ID in the
// request context and lets the handler continue. Unknown credentials abort
// with a safe, generic 401; repository failures abort with a 500. The raw
// token is never placed in the context, only the identity ID.
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

// AnonIdentityIDFromContext returns the anonymous identity ID stored by
// AnonymousAuthRequired. The boolean reports whether a non-empty ID was
// present.
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

// abortUnauthorized writes the shared, deliberately generic 401 response used
// by both authentication middlewares.
func abortUnauthorized(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"error": gin.H{
			"code":    "UNAUTHORIZED",
			"message": "authentication required",
		},
	})
}
