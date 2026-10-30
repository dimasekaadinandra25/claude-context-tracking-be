package user

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"claude-usage-server/internal/middleware"
	"claude-usage-server/internal/modules/core/user/handler"
	"claude-usage-server/internal/modules/core/user/repository"
	"claude-usage-server/internal/modules/core/user/service"
)

type UserModule struct {
	Handler *handler.UserHandler
	Service *service.UserService
}

// Initialize initializes the user module with all dependencies
func Initialize(db *pgxpool.Pool) *UserModule {
	userRepo := repository.NewUserRepository(db)
	userService := service.NewUserService(userRepo)
	userHandler := handler.NewUserHandler(userService)

	return &UserModule{
		Handler: userHandler,
		Service: userService,
	}
}

// SetupRoutes registers all user module routes
func (m *UserModule) SetupRoutes(router *gin.RouterGroup) {
	users := router.Group("/users")
	users.Use(middleware.JWTAuth(), middleware.RejectApiKey())
	{
		// Current user endpoints (must be before /:id to avoid conflict)
		users.GET("/me", m.Handler.GetMe)
		users.PUT("/me", m.Handler.UpdateMe)
		users.PUT("/me/password", m.Handler.ChangePassword)

		// User management endpoints
		users.GET("/:id", middleware.RequirePermission("user-management:read"), m.Handler.GetByID)
	}

	admin := router.Group("/admin/users", middleware.JWTAuth(), middleware.RejectApiKey(), middleware.RequireRole("super_admin"))
	admin.POST("", m.Handler.Create)
	admin.GET("", m.Handler.GetAll)
	admin.POST("/:id/deactivate", m.Handler.Deactivate)
	admin.POST("/:id/activate", m.Handler.Activate)
	admin.POST("/:id/reset-password", m.Handler.ResetPassword)
}
