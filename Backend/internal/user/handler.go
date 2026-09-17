package user

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Handler owns the HTTP surface of the users domain. It validates input,
// calls the service layer, and writes responses — never the database.
type Handler struct {
	svc Service
}

// NewHandler returns a Handler bound to the given service.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Register handles POST /api/v1/auth/register.
func (h *Handler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"Request body must include a valid JSON email and password", "")
		return
	}

	req.Email = strings.TrimSpace(req.Email)

	u, err := h.svc.Register(c.Request.Context(), req.Email, req.Password)
	switch {
	case errors.Is(err, ErrInvalidEmail):
		respondError(c, http.StatusBadRequest, "INVALID_INPUT", "provide a valid email address", "email")
	case errors.Is(err, ErrInvalidPassword):
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"password must be between 12 and 72 characters", "password")
	case errors.Is(err, ErrEmailTaken):
		respondError(c, http.StatusConflict, "CONFLICT", "an account with this email already exists", "email")
	case err != nil:
		slog.Error("user register failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
	default:
		c.JSON(http.StatusCreated, gin.H{"user": u})
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
