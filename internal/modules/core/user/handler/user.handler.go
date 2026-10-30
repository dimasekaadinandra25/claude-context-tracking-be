package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"claude-usage-server/internal/middleware"
	"claude-usage-server/internal/modules/core/user/dto"
	"claude-usage-server/internal/modules/core/user/service"
	"claude-usage-server/internal/shared/response"
)

type UserHandler struct {
	userService *service.UserService
}

func NewUserHandler(userService *service.UserService) *UserHandler {
	return &UserHandler{
		userService: userService,
	}
}

func (h *UserHandler) GetAll(c *gin.Context) {
	var params dto.UserQueryParams
	if err := c.ShouldBindQuery(&params); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid query parameters", err.Error())
		return
	}

	ctx := c.Request.Context()
	result, err := h.userService.GetAll(ctx, &params)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to get users", err.Error())
		return
	}

	response.SuccessWithPagination(c, http.StatusOK, "Users retrieved successfully",
		result.Users, result.Page, result.Limit, result.Total)
}

func (h *UserHandler) GetByID(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		response.Error(c, http.StatusBadRequest, "User ID is required", "")
		return
	}

	ctx := c.Request.Context()
	result, err := h.userService.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			response.Error(c, http.StatusNotFound, "User not found", "")
			return
		}
		response.Error(c, http.StatusInternalServerError, "Failed to get user", err.Error())
		return
	}

	response.Success(c, http.StatusOK, "User retrieved successfully", result)
}

func (h *UserHandler) Create(c *gin.Context) {
	var req dto.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", err.Error())
		return
	}

	createdBy := middleware.MustGetUserID(c)
	ctx := c.Request.Context()

	result, err := h.userService.CreateMember(ctx, &req, createdBy)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrEmailAlreadyExists):
			response.Error(c, http.StatusConflict, "Email already exists", "")
		case errors.Is(err, service.ErrUsernameAlreadyExists):
			response.Error(c, http.StatusConflict, "Username already exists", "")
		default:
			response.Error(c, http.StatusInternalServerError, "Failed to create user", err.Error())
		}
		return
	}

	response.Success(c, http.StatusCreated, "User created successfully", result)
}

func (h *UserHandler) Deactivate(c *gin.Context) {
	h.setActive(c, false, "User deactivated successfully")
}

func (h *UserHandler) Activate(c *gin.Context) {
	h.setActive(c, true, "User activated successfully")
}

func (h *UserHandler) setActive(c *gin.Context, active bool, message string) {
	err := h.userService.SetActive(c.Request.Context(), middleware.MustGetUserID(c), c.Param("id"), active)
	switch {
	case err == nil:
		response.Success(c, http.StatusOK, message, nil)
	case errors.Is(err, service.ErrUserNotFound):
		response.Error(c, http.StatusNotFound, "User not found", "")
	case errors.Is(err, service.ErrDeactivateSelf):
		response.Error(c, http.StatusBadRequest, "Cannot deactivate yourself", "")
	default:
		response.Error(c, http.StatusInternalServerError, "Failed to update user", err.Error())
	}
}

func (h *UserHandler) ResetPassword(c *gin.Context) {
	var req dto.ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", err.Error())
		return
	}

	err := h.userService.ResetPassword(c.Request.Context(), middleware.MustGetUserID(c), c.Param("id"), req.Password)
	switch {
	case err == nil:
		response.Success(c, http.StatusOK, "Password reset successfully", nil)
	case errors.Is(err, service.ErrUserNotFound):
		response.Error(c, http.StatusNotFound, "User not found", "")
	default:
		response.Error(c, http.StatusInternalServerError, "Failed to reset password", err.Error())
	}
}

func (h *UserHandler) GetMe(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	ctx := c.Request.Context()

	result, err := h.userService.GetMe(ctx, userID)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			response.Error(c, http.StatusNotFound, "User not found", "")
			return
		}
		response.Error(c, http.StatusInternalServerError, "Failed to get user", err.Error())
		return
	}

	response.Success(c, http.StatusOK, "User retrieved successfully", result)
}

func (h *UserHandler) UpdateMe(c *gin.Context) {
	var req dto.UpdateMeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", err.Error())
		return
	}

	userID := middleware.MustGetUserID(c)
	ctx := c.Request.Context()

	result, err := h.userService.UpdateMe(ctx, userID, &req)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			response.Error(c, http.StatusNotFound, "User not found", "")
			return
		}
		response.Error(c, http.StatusInternalServerError, "Failed to update user", err.Error())
		return
	}

	response.Success(c, http.StatusOK, "User updated successfully", result)
}

func (h *UserHandler) ChangePassword(c *gin.Context) {
	var req dto.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", err.Error())
		return
	}

	userID := middleware.MustGetUserID(c)
	ctx := c.Request.Context()

	err := h.userService.ChangePassword(ctx, userID, &req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrUserNotFound):
			response.Error(c, http.StatusNotFound, "User not found", "")
		case errors.Is(err, service.ErrInvalidPassword):
			response.Error(c, http.StatusBadRequest, "Current password is incorrect", "")
		default:
			response.Error(c, http.StatusInternalServerError, "Failed to change password", err.Error())
		}
		return
	}

	response.Success(c, http.StatusOK, "Password changed successfully", nil)
}
