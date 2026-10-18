package anon

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"Backend/internal/middleware"

	"github.com/gin-gonic/gin"
)

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
		// A genuine conflict is a retryable condition, not a server fault, and
		// carries no identity values to log.
		if errors.Is(err, ErrIdentityConflict) {
			slog.Warn("anon session creation hit an identity conflict")
			respondError(c, http.StatusConflict, "SESSION_CONFLICT",
				"We couldn't start a new session just now. Please try again.", "")
			return
		}
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
