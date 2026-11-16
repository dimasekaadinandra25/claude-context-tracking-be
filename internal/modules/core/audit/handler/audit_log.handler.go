package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"claude-usage-server/internal/modules/core/audit/dto"
	"claude-usage-server/internal/modules/core/audit/service"
	"claude-usage-server/internal/shared/response"
	"claude-usage-server/pkg/logger"
)

type AuditHandler struct {
	service *service.AuditService
}

func NewAuditHandler(svc *service.AuditService) *AuditHandler {
	return &AuditHandler{service: svc}
}

func (h *AuditHandler) List(c *gin.Context) {
	var q dto.AuditLogQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid query parameters", err.Error())
		return
	}
	logs, total, err := h.service.List(c.Request.Context(), &q)
	if err != nil {
		logger.Error("Failed to list audit logs", logger.Err(err))
		response.Error(c, http.StatusInternalServerError, "Internal server error", "")
		return
	}
	response.SuccessWithPagination(c, http.StatusOK, "Audit logs retrieved", logs, q.Page, q.Limit, total)
}
