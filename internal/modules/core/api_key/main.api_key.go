package api_key

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"claude-usage-server/internal/middleware"
	"claude-usage-server/internal/modules/core/api_key/handler"
	"claude-usage-server/internal/modules/core/api_key/repository"
	"claude-usage-server/internal/modules/core/api_key/service"
)

// ApiKeyModule holds all components of the API key module
type ApiKeyModule struct {
	Handler    *handler.ApiKeyHandler
	Service    *service.ApiKeyService
	Repository *repository.ApiKeyRepository
}

// Initialize creates and wires all dependencies for the API key module
func Initialize(db *pgxpool.Pool, roleRepo service.RoleRepository) *ApiKeyModule {
	repo := repository.NewApiKeyRepository(db)
	svc := service.NewApiKeyService(repo, roleRepo)
	hdlr := handler.NewApiKeyHandler(svc)

	return &ApiKeyModule{
		Handler:    hdlr,
		Service:    svc,
		Repository: repo,
	}
}

// SetupRoutes registers all routes for the API key module
func (m *ApiKeyModule) SetupRoutes(router *gin.RouterGroup) {
	// All routes require JWT auth
	apiKeys := router.Group("/api-keys")
	apiKeys.Use(middleware.JWTAuth(), middleware.RejectApiKey())
	{
		// Create a new API key
		apiKeys.POST("", m.Handler.Create)

		// List all API keys for the user
		apiKeys.GET("", m.Handler.List)

		// Get a specific API key
		apiKeys.GET("/:id", m.Handler.GetByID)

		// Update an API key
		apiKeys.PATCH("/:id", m.Handler.Update)

		// Revoke an API key
		apiKeys.DELETE("/:id", m.Handler.Revoke)
	}

	admin := router.Group("/admin", middleware.JWTAuth(), middleware.RejectApiKey(), middleware.RequireRole("super_admin"))
	admin.POST("/users/:id/api-keys", m.Handler.CreateForUser)
	admin.GET("/users/:id/api-keys", m.Handler.ListForUser)
	admin.POST("/api-keys/:id/revoke", m.Handler.AdminRevoke)
}
