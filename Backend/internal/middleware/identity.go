package middleware

import (
	"log/slog"
	"net/http"
	"strings"

	"Backend/internal/auth"

	"github.com/gin-gonic/gin"
)

// OwnerKey is the Gin context key IdentityRequired stores the Owner under.
const OwnerKey = "owner"

// Owner is the resolved identity behind a request. Exactly one ID is populated,
// so a private record can be scoped to whoever made it without carrying a
// "which kind of user am I?" flag. The zero value is invalid; use
// IdentityFromOwner, which rejects an owner with both or neither ID set.
type Owner struct {
	// UserID is set when the caller presented a registered access token.
	UserID string
	// AnonIdentityID is set when the caller presented an anonymous session
	// token.
	AnonIdentityID string
}

// Registered reports whether this owner is a signed-in user.
func (o Owner) Registered() bool { return o.UserID != "" }

// Anonymous reports whether this owner is an anonymous session.
func (o Owner) Anonymous() bool { return o.AnonIdentityID != "" }

// ID returns whichever identity ID is set, for use as associated-data key
// material. It is empty only for the invalid zero Owner.
func (o Owner) ID() string {
	if o.UserID != "" {
		return o.UserID
	}
	return o.AnonIdentityID
}

// IdentityFromOwner validates that an owner has exactly one identity. This is
// the single place that invariant is enforced, so an ambiguous or empty owner
// never reaches the database.
func IdentityFromOwner(o Owner) (string, bool) {
	if o.Registered() == o.Anonymous() {
		return "", false
	}
	if o.Registered() {
		return o.UserID, true
	}
	return o.AnonIdentityID, true
}

// RegisteredVerifier is the slice of the auth token manager IdentityRequired
// needs, so the middleware depends only on the interface's shape.
type RegisteredVerifier interface {
	ParseAccessToken(token string) (string, error)
}

// IdentityRequired authenticates the request with EITHER a registered access
// token or an anonymous session token and stores the resolved Owner in the
// context.
//
// This keeps authentication action-based rather than page-based: a personal
// feature (journal, mood check-in, breathing history) is usable by both a
// registered user and an anonymous session, and the row is scoped to whichever
// identity authenticated. A registered JWT resolves to a UserID and an
// anonymous token to an AnonIdentityID, never both.
//
// A request with no usable credential aborts with the same generic 401 as the
// other middlewares. Raw tokens are never placed in the context or logged.
func IdentityRequired(tokenManager RegisteredVerifier, anon AnonymousVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c)
		if !ok {
			abortUnauthorized(c)
			return
		}

		// Registered token first: it is the stronger identity, so a caller who has
		// one is never treated as anonymous.
		if userID, err := tokenManager.ParseAccessToken(token); err == nil && strings.TrimSpace(userID) != "" {
			c.Set(OwnerKey, Owner{UserID: userID})
			c.Next()
			return
		}

		// No anon verifier (a server built without the anon service): only the
		// registered path above is available.
		if anon == nil {
			abortUnauthorized(c)
			return
		}

		identityID, ok, err := anon.Authenticate(c.Request.Context(), token)
		if err != nil {
			// A repository failure is a server problem, not bad credentials, so it is
			// reported as a 500: a client must not discard a valid token and mint
			// a new anonymous session over a transient fault.
			slog.Error("identity authentication failed", slog.String("error", err.Error()))
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

		c.Set(OwnerKey, Owner{AnonIdentityID: identityID})
		c.Next()
	}
}

// OwnerFromContext returns the Owner stored by IdentityRequired; ok is false
// unless a well-formed owner was present.
func OwnerFromContext(c *gin.Context) (Owner, bool) {
	value, ok := c.Get(OwnerKey)
	if !ok {
		return Owner{}, false
	}
	owner, ok := value.(Owner)
	if !ok {
		return Owner{}, false
	}
	if _, valid := IdentityFromOwner(owner); !valid {
		return Owner{}, false
	}
	return owner, true
}

// Compile-time proof that the real token manager satisfies RegisteredVerifier.
var _ RegisteredVerifier = (*auth.Manager)(nil)
