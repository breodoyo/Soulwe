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

type Handler struct {
	svc Service
}

func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

type createRequest struct {
	Content    string   `json:"content"`
	MoodTags   []string `json:"mood_tags"`
	PromptUsed *string  `json:"prompt_used"`
}

type updateRequest struct {
	Content    *string   `json:"content"`
	MoodTags   *[]string `json:"mood_tags"`
	PromptUsed *string   `json:"prompt_used"`
}

func (h *Handler) Create(c *gin.Context) {
	owner, ok := middleware.OwnerFromContext(c)
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

	entry, err := h.svc.Create(c.Request.Context(), owner, req.Content, req.MoodTags, req.PromptUsed)
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

func (h *Handler) List(c *gin.Context) {
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
	before, err := parseBefore(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"before must be an ISO 8601 timestamp", "before")
		return
	}

	entries, err := h.svc.List(c.Request.Context(), owner, limit, before)
	if err != nil {
		slog.Error("journal list failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"entries": entries, "next_cursor": nextCursor(entries, limit)})
}


func (h *Handler) Get(c *gin.Context) {
	owner, ok := middleware.OwnerFromContext(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}

	entry, err := h.svc.Get(c.Request.Context(), owner, c.Param("id"))
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

func (h *Handler) Update(c *gin.Context) {
	owner, ok := middleware.OwnerFromContext(c)
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

	entry, err := h.svc.Update(c.Request.Context(), owner, c.Param("id"), Update{
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

func (h *Handler) Delete(c *gin.Context) {
	owner, ok := middleware.OwnerFromContext(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}

	err := h.svc.Delete(c.Request.Context(), owner, c.Param("id"))
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

func (h *Handler) Reflect(c *gin.Context) {
	owner, ok := middleware.OwnerFromContext(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}

	entry, err := h.svc.Reflect(c.Request.Context(), owner, c.Param("id"))
	switch {
	case errors.Is(err, ErrJournalEntryNotFound):
		respondError(c, http.StatusNotFound, "NOT_FOUND", "journal entry not found", "")
	case errors.Is(err, ErrAIReflectionUnavailable):
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
