package service

import (
	"context"

	"github.com/gin-gonic/gin"

	"claude-usage-server/internal/middleware"
	"claude-usage-server/internal/modules/core/audit/domain"
	"claude-usage-server/internal/modules/core/audit/dto"
	"claude-usage-server/internal/modules/core/audit/repository"
	"claude-usage-server/pkg/logger"
)

type AuditService struct {
	repo *repository.AuditRepository
}

func NewAuditService(repo *repository.AuditRepository) *AuditService {
	return &AuditService{repo: repo}
}

func (s *AuditService) Record(c *gin.Context, action, targetType, targetID, summary string) {
	if s == nil {
		return
	}
	entry := domain.AuditLog{Action: action, TargetType: targetType, TargetID: targetID, Summary: summary, IP: c.ClientIP()}
	if claims, err := middleware.GetUserFromContext(c); err == nil {
		entry.ActorID, entry.ActorEmail = &claims.UserID, claims.Email
	}
	if err := s.repo.Create(context.WithoutCancel(c.Request.Context()), &entry); err != nil {
		logger.Error("Failed to record audit log", logger.String("action", action), logger.Err(err))
	}
}

func (s *AuditService) List(ctx context.Context, q *dto.AuditLogQuery) ([]domain.AuditLog, int64, error) {
	return s.repo.List(ctx, q)
}
