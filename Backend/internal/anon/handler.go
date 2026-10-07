package anon

import (
	"log/slog"
	"net/http"
	"strings"

	"Backend/internal/middleware"

	"github.com/gin-gonic/gin"
)

// Handler owns the HTTP surface of anonymous sessions. It validates the
// optional X-Device-ID header and never touches the database.
type Handler struct {
	svc Service
}

func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

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

func (h *Handler) Me(c *gin.Context) {
	anonymousID, ok := middleware.AnonIdentityIDFromContext(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"anonymous_id": anonymousID})
}

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
