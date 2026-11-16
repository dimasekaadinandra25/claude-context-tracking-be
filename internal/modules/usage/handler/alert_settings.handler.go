package handler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	auditDomain "claude-usage-server/internal/modules/core/audit/domain"
	"claude-usage-server/internal/modules/usage/dto"
	"claude-usage-server/internal/modules/usage/service"
	"claude-usage-server/internal/shared/response"
)

func (h *Handler) AlertSettings(c *gin.Context) {
	who, ok := caller(c)
	if !ok {
		return
	}
	res, err := h.service.AlertSettings(c.Request.Context(), who)
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Alert settings retrieved", res)
}

func (h *Handler) SetAlertSettings(c *gin.Context) {
	who, ok := caller(c)
	if !ok {
		return
	}
	var req dto.AlertSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", err.Error())
		return
	}
	res, err := h.service.SetAlertSettings(c.Request.Context(), who, req)
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		response.Error(c, http.StatusBadRequest, "Invalid request payload", err.Error())
	case err != nil:
		fail(c, err)
	default:
		summary := fmt.Sprintf("alerts %q, runaway %d%%, project %d%%", *res.Alerts, res.RunawayShare, res.ProjectShare)
		h.audit.Record(c, auditDomain.ActionAlertSettingsUpdate, auditDomain.TargetAlertSettings, "", summary)
		response.Success(c, http.StatusOK, "Alert settings saved", res)
	}
}
