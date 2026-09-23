package bookings

import (
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"Backend/internal/middleware"

	"github.com/gin-gonic/gin"
)

// Handler owns the HTTP surface of the bookings domain. It validates input,
// calls the service layer, and writes responses — never the database. Every
// route is a registered-user feature (JWT required) and every read is scoped
// to the authenticated user, so one person's bookings are never exposed to
// another.
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

// createBookingRequest is the body of POST /therapists/:id/bookings.
// scheduled_at is captured as a raw string so a malformed timestamp maps to a
// precise 400 with the field name, rather than a generic bind error.
type createBookingRequest struct {
	ScheduledAt string `json:"scheduled_at"`
}

// Create handles POST /api/v1/therapists/:id/bookings. It validates the
// therapist id and the scheduled time, then books a pending session for the
// authenticated user.
func (h *Handler) Create(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}

	therapistID := c.Param("id")
	if !uuidPattern.MatchString(therapistID) {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"therapist id must be a valid UUID", "id")
		return
	}

	var body createBookingRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"request body must be a valid JSON object", "")
		return
	}
	raw := strings.TrimSpace(body.ScheduledAt)
	if raw == "" {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"scheduled_at is required", "scheduled_at")
		return
	}
	scheduledAt, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"scheduled_at must be an ISO 8601 timestamp", "scheduled_at")
		return
	}

	booking, err := h.svc.Create(c.Request.Context(), userID, therapistID, scheduledAt)
	switch {
	case errors.Is(err, ErrScheduledInPast):
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"scheduled_at must be in the future", "scheduled_at")
	case errors.Is(err, ErrTherapistMissing):
		respondError(c, http.StatusNotFound, "NOT_FOUND", "therapist not found", "")
	case errors.Is(err, ErrTherapistInactive):
		respondError(c, http.StatusConflict, "THERAPIST_UNAVAILABLE",
			"therapist is not accepting bookings", "")
	case errors.Is(err, ErrBookingConflict):
		respondError(c, http.StatusConflict, "BOOKING_CONFLICT",
			"that time slot is no longer available", "scheduled_at")
	case err != nil:
		slog.Error("bookings create failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
	default:
		c.JSON(http.StatusCreated, gin.H{"booking": booking})
	}
}

// List handles GET /api/v1/bookings. It returns the authenticated user's own
// bookings newest first.
func (h *Handler) List(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}

	bookings, err := h.svc.List(c.Request.Context(), userID)
	if err != nil {
		slog.Error("bookings list failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"bookings": bookings})
}

// Get handles GET /api/v1/bookings/:id. It returns the booking only when it
// belongs to the authenticated user; anyone else's id is indistinguishable
// from a missing one (404).
func (h *Handler) Get(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}

	id := c.Param("id")
	if !uuidPattern.MatchString(id) {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"booking id must be a valid UUID", "id")
		return
	}

	booking, err := h.svc.Get(c.Request.Context(), userID, id)
	switch {
	case errors.Is(err, ErrBookingNotFound):
		respondError(c, http.StatusNotFound, "NOT_FOUND", "booking not found", "")
	case err != nil:
		slog.Error("bookings get failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
	default:
		c.JSON(http.StatusOK, gin.H{"booking": booking})
	}
}

// Cancel handles PATCH /api/v1/bookings/:id/cancel. Only a pending booking can
// be cancelled; a cancelled/completed one is a conflict, and anything that is
// not the caller's is a 404.
func (h *Handler) Cancel(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}

	id := c.Param("id")
	if !uuidPattern.MatchString(id) {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"booking id must be a valid UUID", "id")
		return
	}

	booking, err := h.svc.Cancel(c.Request.Context(), userID, id)
	switch {
	case errors.Is(err, ErrBookingNotFound):
		respondError(c, http.StatusNotFound, "NOT_FOUND", "booking not found", "")
	case errors.Is(err, ErrBookingStatusConflict):
		respondError(c, http.StatusConflict, "BOOKING_STATUS_CONFLICT",
			"only pending bookings can be cancelled", "")
	case err != nil:
		slog.Error("bookings cancel failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
	default:
		c.JSON(http.StatusOK, gin.H{"booking": booking})
	}
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
