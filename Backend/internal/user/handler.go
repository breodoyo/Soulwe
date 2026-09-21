package user

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"Backend/internal/auth"
	"Backend/internal/middleware"

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

// promoteRequest is the body of POST /api/v1/auth/anonymous/promote. The
// anonymous identity comes from the request's bearer token, not the body.
type promoteRequest struct {
	Email       string  `json:"email"`
	Password    string  `json:"password"`
	DisplayName *string `json:"display_name"`
}

// Me handles GET /api/v1/auth/me. The auth middleware has already validated
// the Bearer token and stored the user ID in the request context, so this
// handler only echoes it back — a minimal demonstration that protection works.
func (h *Handler) Me(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"user_id": userID})
}

// Login handles POST /api/v1/auth/login.
func (h *Handler) Login(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"Request body must include a valid JSON email and password", "")
		return
	}

	result, err := h.svc.Login(c.Request.Context(), strings.TrimSpace(req.Email), req.Password)
	switch {
	case errors.Is(err, ErrBadCredentials):
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid email or password", "")
	case err != nil:
		slog.Error("user login failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
	default:
		c.JSON(http.StatusOK, gin.H{
			"access_token": result.AccessToken,
			"token_type":   "Bearer",
			"expires_in":   int(auth.AccessTokenTTL.Seconds()),
			"user":         result.User,
		})
	}
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

// Promote handles POST /api/v1/auth/anonymous/promote. The anonymous auth
// middleware has already validated the Bearer token and stored the identity ID
// in the request context, so the handler only reads it from there — it never
// sees or logs the raw anonymous token.
func (h *Handler) Promote(c *gin.Context) {
	identityID, ok := middleware.AnonIdentityIDFromContext(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}

	var req promoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"Request body must include a valid JSON email and password", "")
		return
	}

	req.Email = strings.TrimSpace(req.Email)

	result, err := h.svc.Promote(c.Request.Context(), identityID, req.Email, req.Password, req.DisplayName)
	switch {
	case errors.Is(err, ErrInvalidEmail):
		respondError(c, http.StatusBadRequest, "INVALID_INPUT", "provide a valid email address", "email")
	case errors.Is(err, ErrInvalidPassword):
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"password must be between 12 and 72 characters", "password")
	case errors.Is(err, ErrEmailTaken):
		respondError(c, http.StatusConflict, "CONFLICT", "an account with this email already exists", "email")
	case errors.Is(err, ErrIdentityAlreadyPromoted):
		respondError(c, http.StatusConflict, "CONFLICT", "anonymous identity already promoted", "")
	case err != nil:
		slog.Error("user promote failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
	default:
		c.JSON(http.StatusCreated, gin.H{
			"access_token": result.AccessToken,
			"token_type":   "Bearer",
			"expires_in":   int(auth.AccessTokenTTL.Seconds()),
			"user":         result.User,
		})
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
