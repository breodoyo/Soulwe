package therapists

import (
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"Backend/internal/middleware"

	"github.com/gin-gonic/gin"
)

// Handler owns the HTTP surface of the therapists domain. It validates input,
// calls the service layer, and writes responses — never the database. Reading
// the directory is a registered-user feature, so the authenticated JWT is
// required; the directory itself is public and involves no ownership.
type Handler struct {
	svc Service
}

// NewHandler returns a Handler bound to the given service.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// uuidPattern accepts the canonical 8-4-4-4-12 UUID layout, matching the shape
// used across the API. It is a shape check, not a cryptographic guarantee.
var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// List handles GET /api/v1/therapists. It returns the public therapist
// directory newest first, honoring optional ?limit, ?before (cursor),
// ?language, and ?specialty query parameters.
func (h *Handler) List(c *gin.Context) {
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
	before, err := parseBefore(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"before must be an ISO 8601 timestamp", "before")
		return
	}

	opts := ListOptions{
		Language:  strings.TrimSpace(c.Query("language")),
		Specialty: strings.TrimSpace(c.Query("specialty")),
	}

	therapists, err := h.svc.List(c.Request.Context(), opts, limit, before)
	if err != nil {
		slog.Error("therapists list failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"therapists": therapists, "next_cursor": nextCursor(therapists, limit)})
}

// Get handles GET /api/v1/therapists/:id. It returns the therapist's public
// profile, or 404 for a missing one.
func (h *Handler) Get(c *gin.Context) {
	if _, ok := middleware.UserIDFromContext(c); !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}

	id := c.Param("id")
	if !uuidPattern.MatchString(id) {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"therapist id must be a valid UUID", "id")
		return
	}

	therapist, err := h.svc.Get(c.Request.Context(), id)
	switch {
	case errors.Is(err, ErrTherapistNotFound):
		respondError(c, http.StatusNotFound, "NOT_FOUND", "therapist not found", "")
	case err != nil:
		slog.Error("therapists get failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
	default:
		c.JSON(http.StatusOK, gin.H{"therapist": therapist})
	}
}

// nextCursor computes the keyset pagination cursor for the next page. It is
// the last therapist's created_at when the page is full, otherwise null.
func nextCursor(therapists []Therapist, limit int) *time.Time {
	if len(therapists) == 0 {
		return nil
	}
	if len(therapists) < ClampLimit(limit) {
		return nil
	}
	last := therapists[len(therapists)-1].CreatedAt
	return &last
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

// parseBefore reads the optional ?before cursor (ISO 8601 timestamp).
func parseBefore(c *gin.Context) (*time.Time, error) {
	raw := c.Query("before")
	if raw == "" {
		return nil, nil
	}
	before, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return nil, err
	}
	return &before, nil
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
