package audit

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"claude-usage-server/internal/middleware"
)

// Module bundles the audit repository, service, and handler for both
// cross-module write access (feature services inject *Service) and the
// read-side global endpoint mounted under /core/v1/audit-logs.
type Module struct {
	Repository *Repository
	Service    *Service
	Handler    *Handler
}

// Initialize wires the audit module.
func Initialize(db *pgxpool.Pool) *Module {
	repo := NewRepository(db)
	svc := NewService(repo)
	h := NewHandler(svc)
	return &Module{
		Repository: repo,
		Service:    svc,
		Handler:    h,
	}
}

// SetupRoutes registers the global audit endpoint. The audit trail is
// polymorphic — one endpoint serves every feature via reff_type + reff_id
// filters. Feature modules never mount their own /history routes.
func (m *Module) SetupRoutes(router *gin.RouterGroup) {
	logs := router.Group("/audit-logs")
	logs.Use(middleware.JWTAuth(), middleware.RejectApiKey(), middleware.RequireRole("super_admin"))
	{
		logs.GET("", m.Handler.List)
	}
}
