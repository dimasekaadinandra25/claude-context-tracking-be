package router

import (
	"time"

	"claude-usage-server/internal/middleware"

	// Core modules
	"claude-usage-server/internal/modules/core/api_key"
	"claude-usage-server/internal/modules/core/auth"
	"claude-usage-server/internal/modules/core/role"
	"claude-usage-server/internal/modules/core/user"
	"claude-usage-server/internal/modules/usage"

	"claude-usage-server/internal/shared/audit"
	"claude-usage-server/internal/shared/authz"

	"claude-usage-server/pkg/logger"

	"github.com/gin-contrib/cors"
	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

func Setup(router *gin.Engine, db *pgxpool.Pool) {
	// Get logger instance
	log := logger.GetLogger()

	// Ginzap middleware for logging HTTP requests
	router.Use(ginzap.Ginzap(log, time.RFC3339, true))

	// Recovery middleware with Zap (handles panics)
	router.Use(ginzap.RecoveryWithZap(log, true))

	// CORS middleware configuration
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173", "http://localhost:3000"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-API-Key"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"message": "Claude Usage API is running",
		})
	})

	// Core v1 routes
	coreV1 := router.Group("/core/v1")
	{
		roleModule := role.Initialize(db)
		roleModule.SetupRoutes(coreV1)

		authzService := authz.NewService(roleModule.Repository)
		middleware.SetAuthzService(authzService)

		authModule := auth.Initialize(db, roleModule.Repository, authzService)
		authModule.SetupRoutes(coreV1)

		userModule := user.Initialize(db)
		userModule.SetupRoutes(coreV1)

		apiKeyModule := api_key.Initialize(db, roleModule.Repository)
		apiKeyModule.SetupRoutes(coreV1)
		middleware.SetApiKeyValidator(apiKeyModule.Service)
		userModule.Service.SetKeyRevoker(apiKeyModule.Service)
	}

	// Shared audit module (polymorphic, used by any feature that wants
	// per-entity history).
	auditModule := audit.Initialize(db)
	auditModule.SetupRoutes(coreV1)

	usage.Initialize(db).SetupRoutes(router.Group("/usage/v1"))

	log.Info("Routes setup completed", zap.Int("routes", len(router.Routes())))
}
