package breathing

import (
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"

	"Backend/internal/middleware"

	"github.com/gin-gonic/gin"
)

// uuidPattern accepts the canonical 8-4-4-4-12 UUID layout, matching the shape
// used across the API. It is a shape check, not a cryptographic guarantee.
var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Handler owns the HTTP surface of the breathing domain. It validates input,
// calls the service layer, and writes responses — never the database.
// Discovery and session history are registered-user features, so the
// authenticated JWT is required; session ownership is derived from the JWT
// context and is never taken from the request body.
type Handler struct {
	svc Service
}

// NewHandler returns a Handler bound to the given service.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// recordSessionRequest is the body of POST /breathing/sessions. Completed is
// optional and defaults to true: recording a session through this endpoint
// means the user finished the exercise. The user ID is never part of the
// body — it comes from the JWT context.
type recordSessionRequest struct {
	ExerciseID string `json:"exercise_id"`
	Breaths    int    `json:"breaths"`
	DurationS  int    `json:"duration_s"`
	Completed  *bool  `json:"completed"`
}

// ListExercises handles GET /breathing/exercises. It returns the curated
// catalog in its defined order, honoring an optional ?limit= query parameter.
func (h *Handler) ListExercises(c *gin.Context) {
	if _, ok := middleware.UserIDFromContext(c); !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}

	limit, err := parseLimit(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"limit must be a positive integer", "limit")
		return
	}

	exercises, err := h.svc.ListExercises(c.Request.Context(), limit)
	if err != nil {
		slog.Error("breathing exercises list failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"exercises": exercises})
}

// GetExercise handles GET /breathing/exercises/:id. It returns one catalog
// exercise, or 404 for a missing one.
func (h *Handler) GetExercise(c *gin.Context) {
	if _, ok := middleware.UserIDFromContext(c); !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}

	id := c.Param("id")
	if !uuidPattern.MatchString(id) {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"exercise id must be a valid UUID", "id")
		return
	}

	exercise, err := h.svc.GetExercise(c.Request.Context(), id)
	switch {
	case errors.Is(err, ErrExerciseNotFound):
		respondError(c, http.StatusNotFound, "NOT_FOUND", "breathing exercise not found", "")
	case err != nil:
		slog.Error("breathing exercises get failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
	default:
		c.JSON(http.StatusOK, gin.H{"exercise": exercise})
	}
}

// RecordSession handles POST /breathing/sessions. It records a completed
// exercise session for the authenticated user.
func (h *Handler) RecordSession(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}

	var req recordSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"request body must be a valid JSON object", "")
		return
	}
	if !uuidPattern.MatchString(req.ExerciseID) {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"exercise_id must be a valid UUID", "exercise_id")
		return
	}
	if req.Breaths < 1 {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"breaths must be a positive integer", "breaths")
		return
	}
	if req.DurationS < 1 {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"duration_s must be a positive integer", "duration_s")
		return
	}
	completed := true
	if req.Completed != nil {
		completed = *req.Completed
	}

	session, err := h.svc.RecordSession(c.Request.Context(), userID, req.ExerciseID,
		req.Breaths, req.DurationS, completed)
	switch {
	case errors.Is(err, ErrExerciseNotFound):
		respondError(c, http.StatusNotFound, "NOT_FOUND", "breathing exercise not found", "")
	case err != nil:
		slog.Error("breathing session record failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
	default:
		c.JSON(http.StatusCreated, gin.H{"session": session})
	}
}

// ListSessions handles GET /breathing/sessions. It returns the authenticated
// user's breathing history, newest first, honoring an optional ?limit= query
// parameter. The user ID always comes from the JWT context, so this can never
// list another user's sessions.
func (h *Handler) ListSessions(c *gin.Context) {
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

	sessions, err := h.svc.ListSessions(c.Request.Context(), userID, limit)
	if err != nil {
		slog.Error("breathing sessions list failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"sessions": sessions})
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
