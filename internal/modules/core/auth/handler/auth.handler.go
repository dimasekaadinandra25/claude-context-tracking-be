package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"claude-usage-server/internal/middleware"
	"claude-usage-server/internal/modules/core/auth/dto"
	"claude-usage-server/internal/modules/core/auth/service"
	"claude-usage-server/internal/shared/response"
)

type AuthHandler struct {
	authService *service.AuthService
}

func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

func (h *AuthHandler) SignIn(c *gin.Context) {
	var req dto.SignInRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", err.Error())
		return
	}

	// Get device info from user agent
	userAgent := c.GetHeader("User-Agent")
	deviceInfo := service.GetDeviceInfoFromUserAgent(userAgent)

	// Get IP address
	ipAddress := c.ClientIP()

	ctx := c.Request.Context()
	result, err := h.authService.SignIn(ctx, &req, deviceInfo, ipAddress)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidCredentials):
			response.Error(c, http.StatusUnauthorized, "Invalid credentials", "")
		case errors.Is(err, service.ErrUserNotActive):
			response.Error(c, http.StatusUnauthorized, "User account is not active", "")
		case errors.Is(err, service.ErrUserLocked):
			response.Error(c, http.StatusUnauthorized, "User account is locked. Try again later.", "")
		default:
			response.Error(c, http.StatusInternalServerError, "Failed to sign in", err.Error())
		}
		return
	}

	response.Success(c, http.StatusOK, "Sign in successful", result)
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	var req dto.RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", err.Error())
		return
	}

	ctx := c.Request.Context()
	result, err := h.authService.RefreshToken(ctx, req.RefreshToken)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidRefreshToken):
			response.Error(c, http.StatusUnauthorized, "Invalid refresh token", "")
		case errors.Is(err, service.ErrRefreshTokenExpired):
			response.Error(c, http.StatusUnauthorized, "Refresh token expired", "")
		case errors.Is(err, service.ErrRefreshTokenRevoked):
			response.Error(c, http.StatusUnauthorized, "Refresh token revoked", "")
		case errors.Is(err, service.ErrUserNotActive):
			response.Error(c, http.StatusUnauthorized, "User account is not active", "")
		default:
			response.Error(c, http.StatusInternalServerError, "Failed to refresh token", err.Error())
		}
		return
	}

	response.Success(c, http.StatusOK, "Token refreshed successfully", result)
}

func (h *AuthHandler) Logout(c *gin.Context) {
	var req dto.LogoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", err.Error())
		return
	}

	ctx := c.Request.Context()
	err := h.authService.Logout(ctx, req.RefreshToken)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to logout", err.Error())
		return
	}

	response.Success(c, http.StatusOK, "Logged out successfully", nil)
}

func (h *AuthHandler) LogoutAll(c *gin.Context) {
	userID := middleware.MustGetUserID(c)

	ctx := c.Request.Context()
	err := h.authService.LogoutAll(ctx, userID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to logout from all devices", err.Error())
		return
	}

	response.Success(c, http.StatusOK, "Logged out from all devices successfully", nil)
}

func (h *AuthHandler) GetMe(c *gin.Context) {
	claims := middleware.MustGetUserFromContext(c)

	ctx := c.Request.Context()
	result, err := h.authService.GetMe(ctx, claims.UserID, claims.IsSuperAdmin)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrUserNotActive):
			response.Error(c, http.StatusUnauthorized, "User not active", err.Error())
		default:
			response.Error(c, http.StatusInternalServerError, "Failed to load profile", err.Error())
		}
		return
	}

	response.Success(c, http.StatusOK, "Profile retrieved successfully", result)
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req dto.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", err.Error())
		return
	}

	err := h.authService.ChangePassword(c.Request.Context(), middleware.MustGetUserID(c), &req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidPassword):
			response.Error(c, http.StatusBadRequest, "Current password is incorrect", "")
		case errors.Is(err, service.ErrUserNotActive):
			response.Error(c, http.StatusUnauthorized, "User account is not active", "")
		default:
			response.Error(c, http.StatusBadRequest, "Failed to change password", err.Error())
		}
		return
	}

	response.Success(c, http.StatusOK, "Password changed successfully", nil)
}
