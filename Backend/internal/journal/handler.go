package journal

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"Backend/internal/middleware"

	"github.com/gin-gonic/gin"
)

// Handler owns the HTTP surface of the journal domain. It validates input,
// calls the service layer, and writes responses — never the database, and
// never the ciphertext. Ownership always comes from the authenticated JWT;
// the user ID is never accepted from a request body or path.
type Handler struct {
	svc Service
}

// NewHandler returns a Handler bound to the given service.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// createRequest is the body of POST /api/v1/journal. There is deliberately no
// "type" field: the schema has no journal-type column, so the product defines
// a single regular journal entry and no types are invented. Unknown fields
// (like a stray "type") are ignored by JSON binding.
type createRequest struct {
	Content    string   `json:"content"`
	MoodTags   []string `json:"mood_tags"`
	PromptUsed *string  `json:"prompt_used"`
}

// updateRequest is the body of PATCH /api/v1/journal/:id. Pointer fields
// distinguish "omitted" from "explicit value"; prompt_used follows the
// profile convention where "" clears the stored value.
type updateRequest struct {
	Content    *string   `json:"content"`
	MoodTags   *[]string `json:"mood_tags"`
	PromptUsed *string   `json:"prompt_used"`
}

// Create handles POST /api/v1/journal.
func (h *Handler) Create(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}

	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"Request body must be a valid JSON journal entry", "")
		return
	}

	entry, err := h.svc.Create(c.Request.Context(), userID, req.Content, req.MoodTags, req.PromptUsed)
	switch {
	case errors.Is(err, ErrInvalidContent):
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"content is required and must be at most 10000 characters", "content")
	case errors.Is(err, ErrInvalidMoodTags):
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"mood_tags must contain at most 10 tags of at most 50 characters each", "mood_tags")
	case errors.Is(err, ErrInvalidPromptUsed):
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"prompt_used must be at most 200 characters", "prompt_used")
	case err != nil:
		slog.Error("journal create failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
	default:
		c.JSON(http.StatusCreated, gin.H{"entry": entry})
	}
}

// List handles GET /api/v1/journal. It returns the authenticated user's
// entries newest first, honoring optional ?limit and ?before (cursor) query
// parameters. Content is never returned in list responses.
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
	before, err := parseBefore(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"before must be an ISO 8601 timestamp", "before")
		return
	}

	entries, err := h.svc.List(c.Request.Context(), userID, limit, before)
	if err != nil {
		slog.Error("journal list failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"entries": entries, "next_cursor": nextCursor(entries, limit)})
}

// Get handles GET /api/v1/journal/:id. The content is decrypted server-side
// and returned only to the entry's owner.
func (h *Handler) Get(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}

	entry, err := h.svc.Get(c.Request.Context(), userID, c.Param("id"))
	switch {
	case errors.Is(err, ErrJournalEntryNotFound):
		respondError(c, http.StatusNotFound, "NOT_FOUND", "journal entry not found", "")
	case err != nil:
		slog.Error("journal get failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
	default:
		c.JSON(http.StatusOK, gin.H{"entry": entry})
	}
}

// Update handles PATCH /api/v1/journal/:id. Only the authenticated owner can
// edit an entry; content edits clear any stale AI reflection.
func (h *Handler) Update(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}

	var req updateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"Request body must be a valid JSON patch", "")
		return
	}

	entry, err := h.svc.Update(c.Request.Context(), userID, c.Param("id"), Update{
		Content:    req.Content,
		MoodTags:   req.MoodTags,
		PromptUsed: req.PromptUsed,
	})
	switch {
	case errors.Is(err, ErrJournalEntryNotFound):
		respondError(c, http.StatusNotFound, "NOT_FOUND", "journal entry not found", "")
	case errors.Is(err, ErrNothingToUpdate):
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"at least one of content, mood_tags or prompt_used is required", "")
	case errors.Is(err, ErrInvalidContent):
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"content is required and must be at most 10000 characters", "content")
	case errors.Is(err, ErrInvalidMoodTags):
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"mood_tags must contain at most 10 tags of at most 50 characters each", "mood_tags")
	case errors.Is(err, ErrInvalidPromptUsed):
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"prompt_used must be at most 200 characters", "prompt_used")
	case err != nil:
		slog.Error("journal update failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
	default:
		c.JSON(http.StatusOK, gin.H{"entry": entry})
	}
}

// Delete handles DELETE /api/v1/journal/:id. Delete is immediate and
// irreversible.
func (h *Handler) Delete(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}

	err := h.svc.Delete(c.Request.Context(), userID, c.Param("id"))
	switch {
	case errors.Is(err, ErrJournalEntryNotFound):
		respondError(c, http.StatusNotFound, "NOT_FOUND", "journal entry not found", "")
	case err != nil:
		slog.Error("journal delete failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
	default:
		c.Status(http.StatusNoContent)
	}
}

// Reflect handles POST /api/v1/journal/:id/reflect. It generates (and stores)
// a fresh AI reflection for the owner's entry.
func (h *Handler) Reflect(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}

	entry, err := h.svc.Reflect(c.Request.Context(), userID, c.Param("id"))
	switch {
	case errors.Is(err, ErrJournalEntryNotFound):
		respondError(c, http.StatusNotFound, "NOT_FOUND", "journal entry not found", "")
	case errors.Is(err, ErrAIReflectionUnavailable):
		// Safe by design: reveals only that reflections are unavailable, never
		// why (no key material, no upstream details).
		respondError(c, http.StatusServiceUnavailable, "AI_REFLECTION_UNAVAILABLE",
			"AI reflection is temporarily unavailable", "")
	case err != nil:
		slog.Error("journal reflect failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
	default:
		c.JSON(http.StatusOK, gin.H{"entry": entry})
	}
}

// nextCursor computes the keyset pagination cursor for the next page. It is
// the last entry's created_at when the page is full, otherwise null.
func nextCursor(entries []JournalEntry, limit int) *time.Time {
	if len(entries) == 0 {
		return nil
	}
	if len(entries) < ClampLimit(limit) {
		return nil
	}
	last := entries[len(entries)-1].CreatedAt
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
