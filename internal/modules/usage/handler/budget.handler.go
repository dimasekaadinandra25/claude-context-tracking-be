package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"claude-usage-server/internal/modules/usage/dto"
	"claude-usage-server/internal/shared/response"
)

func (h *Handler) Budgets(c *gin.Context) {
	who, ok := caller(c)
	if !ok {
		return
	}
	res, err := h.service.Budgets(c.Request.Context(), who, c.Query("tz"))
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Budgets retrieved", res)
}

func (h *Handler) SetBudget(c *gin.Context) {
	who, ok := caller(c)
	if !ok {
		return
	}
	var req dto.SetBudgetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", err.Error())
		return
	}
	if err := h.service.SetBudget(c.Request.Context(), who, c.Param("user_id"), req.MonthlyNewContext); err != nil {
		fail(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Budget saved", nil)
}

func (h *Handler) DeleteBudget(c *gin.Context) {
	who, ok := caller(c)
	if !ok {
		return
	}
	if err := h.service.DeleteBudget(c.Request.Context(), who, c.Param("user_id")); err != nil {
		fail(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Budget deleted", nil)
}
