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

// updateProfileRequest is the body of PATCH /api/v1/users/me. Both fields are
// optional pointers so an omitted field leaves the stored value untouched.
// Password, email, and other account attributes are intentionally not part of
// this request and are ignored even if a client sends them.
type updateProfileRequest struct {
	DisplayName  *string `json:"display_name"`
	LanguagePref *string `json:"language_pref"`
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

// GetProfile handles GET /api/v1/users/me. The registered-JWT middleware has
// already stored the authenticated user ID in the context; the handler never
// accepts a user ID from the request, so a user can only ever read their own
// profile.
func (h *Handler) GetProfile(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}

	u, err := h.svc.GetProfile(c.Request.Context(), userID)
	switch {
	case errors.Is(err, ErrUserNotFound):
		respondError(c, http.StatusNotFound, "NOT_FOUND", "user not found", "")
	case err != nil:
		slog.Error("user get profile failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
	default:
		c.JSON(http.StatusOK, gin.H{"user": u})
	}
}

// UpdateProfile handles PATCH /api/v1/users/me. Only the supported profile
// fields (display_name, language_pref) are read from the body; id, email,
// password, is_verified, and created_at are never accepted from the client.
func (h *Handler) UpdateProfile(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", "")
		return
	}

	var req updateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"Request body must be a valid JSON object", "")
		return
	}

	u, err := h.svc.UpdateProfile(c.Request.Context(), userID, req.DisplayName, req.LanguagePref)
	switch {
	case errors.Is(err, ErrUserNotFound):
		respondError(c, http.StatusNotFound, "NOT_FOUND", "user not found", "")
	case errors.Is(err, ErrInvalidDisplayName):
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"display name must be at most 100 characters", "display_name")
	case errors.Is(err, ErrInvalidLanguagePref):
		respondError(c, http.StatusBadRequest, "INVALID_INPUT",
			"language_pref must be one of: en, sw, luo, kik", "language_pref")
	case err != nil:
		slog.Error("user update profile failed", slog.String("error", err.Error()))
		respondError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR",
			"An unexpected server error occurred", "")
	default:
		c.JSON(http.StatusOK, gin.H{"user": u})
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
