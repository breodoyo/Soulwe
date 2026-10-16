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

// This is a shape check, not a cryptographic guarantee.
var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Handler struct {
	svc Service
}

func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

type recordSessionRequest struct {
	ExerciseID string `json:"exercise_id"`
	Breaths    int    `json:"breaths"`
	DurationS  int    `json:"duration_s"`
	Completed  *bool  `json:"completed"`
}

func (h *Handler) ListExercises(c *gin.Context) {
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

func (h *Handler) GetExercise(c *gin.Context) {
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

func (h *Handler) RecordSession(c *gin.Context) {
	owner, ok := middleware.OwnerFromContext(c)
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

	session, err := h.svc.RecordSession(c.Request.Context(), owner, req.ExerciseID,
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

func (h *Handler) ListSessions(c *gin.Context) {
	owner, ok := middleware.OwnerFromContext(c)
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

	sessions, err := h.svc.ListSessions(c.Request.Context(), owner, limit)
	if err != nil {
		slog.Error("breathing sessions list failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"sessions": sessions})
}

// parseLimit reads the optional ?limit query parameter; empty means "use the
// service default" and a non-integer value is a client error.
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
