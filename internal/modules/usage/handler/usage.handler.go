package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"claude-usage-server/internal/middleware"
	"claude-usage-server/internal/modules/usage/dto"
	"claude-usage-server/internal/modules/usage/service"
	"claude-usage-server/internal/shared/response"
	"claude-usage-server/pkg/logger"
)

type Handler struct {
	service *service.Service
}

func New(svc *service.Service) *Handler {
	return &Handler{service: svc}
}

func (h *Handler) Ingest(c *gin.Context) {
	if !middleware.IsApiKeyAuth(c) {
		response.Error(c, http.StatusForbidden, "Forbidden", "ingest requires an API key")
		return
	}
	var req dto.IngestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", err.Error())
		return
	}
	if strings.ContainsRune(req.Machine, 0) {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", "machine must not contain NUL characters")
		return
	}
	res, err := h.service.Ingest(c.Request.Context(), middleware.MustGetUserID(c), req)
	if err != nil {
		logger.Error("Failed to save usage", logger.Err(err))
		response.Error(c, http.StatusInternalServerError, "Internal server error", "")
		return
	}
	response.Success(c, http.StatusOK, "Usage saved", res)
}

func caller(c *gin.Context) (dto.Caller, bool) {
	claims, err := middleware.GetUserFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized", err.Error())
		return dto.Caller{}, false
	}
	if middleware.IsApiKeyAuth(c) {
		response.Error(c, http.StatusForbidden, "Forbidden", "reports require a signed-in user, not an API key")
		return dto.Caller{}, false
	}
	return dto.Caller{UserID: claims.UserID, IsAdmin: claims.IsSuperAdmin}, true
}

func reportRequest[Q any](c *gin.Context) (dto.Caller, Q, bool) {
	var q Q
	who, ok := caller(c)
	if !ok {
		return who, q, false
	}
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid query parameters", err.Error())
		return who, q, false
	}
	return who, q, true
}

func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		response.Error(c, http.StatusBadRequest, "Invalid query parameters", err.Error())
	case errors.Is(err, service.ErrNotFound):
		response.Error(c, http.StatusNotFound, "Session not found", "")
	case errors.Is(err, service.ErrForbidden):
		response.Error(c, http.StatusForbidden, "Forbidden", err.Error())
	default:
		logger.Error("Failed to read usage", logger.Err(err))
		response.Error(c, http.StatusInternalServerError, "Internal server error", "")
	}
}

func (h *Handler) Overview(c *gin.Context) {
	who, q, ok := reportRequest[dto.ReportQuery](c)
	if !ok {
		return
	}
	res, err := h.service.Overview(c.Request.Context(), who, q)
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Overview retrieved", res)
}

func (h *Handler) Timeseries(c *gin.Context) {
	who, q, ok := reportRequest[dto.ReportQuery](c)
	if !ok {
		return
	}
	res, err := h.service.Timeseries(c.Request.Context(), who, q, c.DefaultQuery("interval", "day"), c.DefaultQuery("group_by", "user"))
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Timeseries retrieved", res)
}

func (h *Handler) Models(c *gin.Context) {
	who, q, ok := reportRequest[dto.ReportQuery](c)
	if !ok {
		return
	}
	res, err := h.service.Models(c.Request.Context(), who, q)
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Models retrieved", res)
}

func (h *Handler) Sessions(c *gin.Context) {
	who, sq, ok := reportRequest[dto.SessionQuery](c)
	if !ok {
		return
	}
	rows, total, err := h.service.Sessions(c.Request.Context(), who, sq)
	if err != nil {
		fail(c, err)
		return
	}
	page, perPage := sq.Paging()
	response.SuccessWithPagination(c, http.StatusOK, "Sessions retrieved", rows, page, perPage, total)
}

func (h *Handler) Session(c *gin.Context) {
	who, ok := caller(c)
	if !ok {
		return
	}
	res, err := h.service.Session(c.Request.Context(), who, c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Session retrieved", res)
}

func (h *Handler) People(c *gin.Context) {
	who, ok := caller(c)
	if !ok {
		return
	}
	res, err := h.service.People(c.Request.Context(), who)
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, http.StatusOK, "People retrieved", res)
}
