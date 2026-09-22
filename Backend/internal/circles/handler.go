package circles

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"Backend/internal/middleware"

	"github.com/gin-gonic/gin"
)

// Handler owns the HTTP surface of the circles domain. It validates input,
// calls the service layer, and writes responses — never the database. The
// authenticated anonymous identity always comes from the anonymous-session
// middleware; identity is never accepted from a request body or path, and no
// response carries anon_identity_ids, device UUIDs, or token hashes.
type Handler struct {
	svc Service
}

// NewHandler returns a Handler bound to the given service.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// sendRequest is the body of POST /api/v1/circles/:id/messages.
type sendRequest struct {
	Content string `json:"content"`
}

// List handles GET /api/v1/circles. It returns all active circles with live
// member counts. Discovery is open to any authenticated anonymous session.
func (h *Handler) List(c *gin.Context) {
	if _, ok := middleware.AnonIdentityIDFromContext(c); !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}

	circles, err := h.svc.List(c.Request.Context())
	if err != nil {
		slog.Error("circles list failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"circles": circles})
}

// Get handles GET /api/v1/circles/:id. It returns the circle's details,
// member count, and whether the authenticated identity is a member.
func (h *Handler) Get(c *gin.Context) {
	identityID, ok := middleware.AnonIdentityIDFromContext(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}

	circle, err := h.svc.Get(c.Request.Context(), identityID, c.Param("id"))
	switch {
	case errors.Is(err, ErrCircleNotFound):
		respondError(c, http.StatusNotFound, "NOT_FOUND", "circle not found", "")
	case err != nil:
		slog.Error("circles get failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
	default:
		c.JSON(http.StatusOK, gin.H{"circle": circle})
	}
}

// Join handles POST /api/v1/circles/:id/join. It adds the authenticated
// identity to the circle. Joining the same circle twice conflicts.
func (h *Handler) Join(c *gin.Context) {
	identityID, ok := middleware.AnonIdentityIDFromContext(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}

	err := h.svc.Join(c.Request.Context(), identityID, c.Param("id"))
	switch {
	case errors.Is(err, ErrCircleNotFound):
		respondError(c, http.StatusNotFound, "NOT_FOUND", "circle not found", "")
	case errors.Is(err, ErrAlreadyMember):
		respondError(c, http.StatusConflict, "ALREADY_MEMBER", "you have already joined this circle", "")
	case err != nil:
		slog.Error("circles join failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
	default:
		c.Status(http.StatusNoContent)
	}
}

// Leave handles DELETE /api/v1/circles/:id/leave. It removes the authenticated
// identity's membership and is idempotent: leaving a circle never joined still
// succeeds.
func (h *Handler) Leave(c *gin.Context) {
	identityID, ok := middleware.AnonIdentityIDFromContext(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}

	err := h.svc.Leave(c.Request.Context(), identityID, c.Param("id"))
	switch {
	case errors.Is(err, ErrCircleNotFound):
		respondError(c, http.StatusNotFound, "NOT_FOUND", "circle not found", "")
	case err != nil:
		slog.Error("circles leave failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
	default:
		c.Status(http.StatusNoContent)
	}
}

// ListMessages handles GET /api/v1/circles/:id/messages. Members only: the
// circle's messages come back newest first, honoring optional ?limit and
// ?before (cursor) query parameters.
func (h *Handler) ListMessages(c *gin.Context) {
	identityID, ok := middleware.AnonIdentityIDFromContext(c)
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
	before, err := parseBefore(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"before must be an ISO 8601 timestamp", "before")
		return
	}

	messages, err := h.svc.ListMessages(c.Request.Context(), identityID, c.Param("id"), limit, before)
	switch {
	case errors.Is(err, ErrCircleNotFound):
		respondError(c, http.StatusNotFound, "NOT_FOUND", "circle not found", "")
	case errors.Is(err, ErrNotMember):
		respondError(c, http.StatusForbidden, "NOT_A_MEMBER",
			"you must join the circle before reading its messages", "")
	case err != nil:
		slog.Error("circles list messages failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
	default:
		c.JSON(http.StatusOK, gin.H{"messages": messages, "next_cursor": nextCursor(messages, limit)})
	}
}

// SendMessage handles POST /api/v1/circles/:id/messages. Members only: the
// message is validated and stored with the caller's anonymous identity. The
// response exposes the server-generated anon_name, never the identity UUID.
func (h *Handler) SendMessage(c *gin.Context) {
	identityID, ok := middleware.AnonIdentityIDFromContext(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}

	var req sendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"Request body must be a valid JSON message", "")
		return
	}

	message, err := h.svc.SendMessage(c.Request.Context(), identityID, c.Param("id"), req.Content)
	switch {
	case errors.Is(err, ErrInvalidContent):
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"content is required and must be at most 1000 characters", "content")
	case errors.Is(err, ErrCircleNotFound):
		respondError(c, http.StatusNotFound, "NOT_FOUND", "circle not found", "")
	case errors.Is(err, ErrNotMember):
		respondError(c, http.StatusForbidden, "NOT_A_MEMBER",
			"you must join the circle before sending messages", "")
	case err != nil:
		slog.Error("circles send message failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
	default:
		c.JSON(http.StatusCreated, gin.H{"message": message})
	}
}

// nextCursor computes the keyset pagination cursor for the next page. It is
// the last message's created_at when the page is full, otherwise null.
func nextCursor(messages []CircleMessage, limit int) *time.Time {
	if len(messages) == 0 {
		return nil
	}
	if len(messages) < ClampLimit(limit) {
		return nil
	}
	last := messages[len(messages)-1].CreatedAt
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
