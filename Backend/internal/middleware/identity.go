package middleware

import (
	"log/slog"
	"net/http"
	"strings"

	"Backend/internal/auth"

	"github.com/gin-gonic/gin"
)

// OwnerKey is the Gin context key under which IdentityRequired stores the
// resolved owner. Handlers read it via OwnerFromContext.
const OwnerKey = "owner"

// Owner is the resolved identity behind a request. Exactly one of the two IDs
// is ever populated, so a private record can be scoped to whoever made it
// without the record itself carrying a "which kind of user am I?" flag.
//
// The zero value is invalid: callers must use IdentityFromOwner, which rejects
// an owner with both or neither ID set.
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

// ID returns whichever identity ID is set, for use as a cryptographic
// associated-data key. It is empty only for the invalid zero Owner, which
// IdentityFromOwner refuses to produce.
func (o Owner) ID() string {
	if o.UserID != "" {
		return o.UserID
	}
	return o.AnonIdentityID
}

// IdentityFromOwner validates an owner has exactly one identity and returns it.
// A caller that somehow constructs an ambiguous or empty owner must not reach
// the database with it, so this is the single place that invariant is enforced.
func IdentityFromOwner(o Owner) (string, bool) {
	if o.Registered() == o.Anonymous() {
		return "", false
	}
	if o.Registered() {
		return o.UserID, true
	}
	return o.AnonIdentityID, true
}

// RegisteredVerifier is the slice of the auth token manager that
// IdentityRequired needs. It is an interface so the middleware package depends
// only on the interface's shape.
type RegisteredVerifier interface {
	ParseAccessToken(token string) (string, error)
}

// IdentityRequired returns Gin middleware that authenticates the request with
// EITHER a registered access token or an anonymous session token, and stores
// the resulting Owner in the request context.
//
// This is how Soulwe keeps authentication action-based rather than
// page-based: a personal feature (journal, mood check-in, breathing history)
// is usable by both a registered user and an anonymous session, and the row
// is scoped to whichever identity authenticated. Neither credential is ever
// accepted for the other domain's data — a registered JWT resolves to a
// UserID and an anonymous token to an AnonIdentityID, and never both.
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

		// Registered access token first: it is the stronger identity, and a
		// caller who has one should never be treated as anonymous.
		if userID, err := tokenManager.ParseAccessToken(token); err == nil && strings.TrimSpace(userID) != "" {
			c.Set(OwnerKey, Owner{UserID: userID})
			c.Next()
			return
		}

		// Without an anonymous verifier (a server built with no anon service)
		// only the registered path above is available.
		if anon == nil {
			abortUnauthorized(c)
			return
		}

		identityID, ok, err := anon.Authenticate(c.Request.Context(), token)
		if err != nil {
			// A repository failure is a server problem, not bad credentials:
			// report it as such so a client doesn't discard a valid token and
			// mint a new anonymous session because of a transient fault.
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

// OwnerFromContext returns the owner stored by IdentityRequired. The boolean
// reports whether a well-formed owner was present.
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
