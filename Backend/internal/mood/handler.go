package mood

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"Backend/internal/middleware"

	"github.com/gin-gonic/gin"
)

// Handler owns the HTTP surface of the mood domain. It validates input, calls
// the service layer, and writes responses — never the database.
type Handler struct {
	svc Service
}

// NewHandler returns a Handler bound to the given service.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// createRequest is the body of POST /api/v1/moods. Ownership is never taken
// from the body: the authenticated user ID comes from the JWT context.
type createRequest struct {
	Mood string `json:"mood"`
}

// Create handles POST /api/v1/moods. It records a mood check-in for the
// authenticated user.
func (h *Handler) Create(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}

	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"Request body must include a valid JSON mood", "")
		return
	}

	log, err := h.svc.Create(c.Request.Context(), userID, req.Mood)
	switch {
	case errors.Is(err, ErrInvalidMood):
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"mood must be one of: Heavy, Okay, Better, At peace, Grateful", "mood")
	case err != nil:
		slog.Error("mood create failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
	default:
		c.JSON(http.StatusCreated, log)
	}
}

// List handles GET /api/v1/moods. It returns the authenticated user's mood
// history, newest first, honoring an optional ?limit= query parameter. The
// user ID always comes from the JWT context, so this can never list another
// user's check-ins.
func (h *Handler) List(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}

	limit, err := parseLimit(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"limit must be a positive integer", "limit")
		return
	}

	logs, err := h.svc.List(c.Request.Context(), userID, limit)
	if err != nil {
		slog.Error("mood list failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"moods": logs})
}

// parseLimit reads the optional ?limit query parameter. Absent or empty means
// "use the service default"; a non-integer value is a client error.
func parseLimit(c *gin.Context) (int, error) {
	raw := c.Query("limit")
	if raw == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	if limit < 1 {
		return 0, errors.New("limit must be positive")
	}
	return limit, nil
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
