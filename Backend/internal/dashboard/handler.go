package dashboard

import (
	"errors"
	"log/slog"
	"net/http"

	"Backend/internal/middleware"
	"Backend/internal/user"

	"github.com/gin-gonic/gin"
)

// Handler owns the HTTP surface of the dashboard domain.
type Handler struct {
	svc Service
}

func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// Get handles GET /api/v1/dashboard. With no check-ins the response carries
// latest_mood null, recent_moods [], and a count of 0.
func (h *Handler) Get(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}

	dash, err := h.svc.Get(c.Request.Context(), userID)
	switch {
	case errors.Is(err, user.ErrUserNotFound):
		respondError(c, http.StatusNotFound, "NOT_FOUND", "user not found", "")
	case err != nil:
		slog.Error("dashboard get failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
	default:
		c.JSON(http.StatusOK, gin.H{
			"user":                dash.User,
			"latest_mood":         dash.LatestMood,
			"recent_moods":        dash.RecentMoods,
			"mood_checkins_count": dash.MoodCheckinsCount,
		})
	}
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
