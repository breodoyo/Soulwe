package anon

import (
	"log/slog"
	"net/http"
	"strings"

	"Backend/internal/middleware"

	"github.com/gin-gonic/gin"
)

// Handler owns the HTTP surface of anonymous sessions. It validates the
// optional X-Device-ID header, delegates to the service, and writes responses
// — it never touches the database.
type Handler struct {
	svc Service
}

// NewHandler returns a Handler bound to the given service.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// Create handles POST /api/v1/auth/anonymous. It mints a fresh anonymous
// session (or rotates the token of an existing device identity) and returns
// the raw token exactly once.
func (h *Handler) Create(c *gin.Context) {
	deviceID := strings.TrimSpace(c.GetHeader("X-Device-ID"))
	if deviceID != "" && !ValidDeviceUUID(deviceID) {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"X-Device-ID must be a valid UUID", "device_id")
		return
	}

	session, err := h.svc.CreateSession(c.Request.Context(), deviceID)
	if err != nil {
		slog.Error("anon session creation failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"anonymous_token": session.Token,
		"anonymous_id":    session.AnonymousID,
	})
}

// Me handles GET /api/v1/auth/anonymous/me. The anonymous auth middleware has
// already verified the bearer token and stored the identity ID in the request
// context, so this handler only echoes it back — a minimal demonstration that
// anonymous protection works.
func (h *Handler) Me(c *gin.Context) {
	anonymousID, ok := middleware.AnonIdentityIDFromContext(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"anonymous_id": anonymousID})
}

// respondError writes the documented error envelope:
// {"error": {"code": "...", "message": "...", "field": "..."}}.
func respondError(c *gin.Context, status int, code, message, field string) {
	body := gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	}
	if field != "" {
		body["error"].(gin.H)["field"] = field
	}
	c.AbortWithStatusJSON(status, body)
}
