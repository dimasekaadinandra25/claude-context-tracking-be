package audit

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"claude-usage-server/internal/middleware"
	"claude-usage-server/internal/modules/core/audit/handler"
	"claude-usage-server/internal/modules/core/audit/repository"
	"claude-usage-server/internal/modules/core/audit/service"
)

type AuditModule struct {
	Handler *handler.AuditHandler
	Service *service.AuditService
}

func Initialize(db *pgxpool.Pool) *AuditModule {
	svc := service.NewAuditService(repository.NewAuditRepository(db))
	return &AuditModule{Handler: handler.NewAuditHandler(svc), Service: svc}
}

func (m *AuditModule) SetupRoutes(router *gin.RouterGroup) {
	admin := router.Group("/admin", middleware.JWTAuth(), middleware.RejectApiKey(), middleware.RequireRole("super_admin"))
	admin.GET("/audit-logs", m.Handler.List)
}
