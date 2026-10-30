package auth

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"claude-usage-server/internal/middleware"
	"claude-usage-server/internal/modules/core/auth/handler"
	"claude-usage-server/internal/modules/core/auth/repository"
	"claude-usage-server/internal/modules/core/auth/service"
	userRepo "claude-usage-server/internal/modules/core/user/repository"
)

type AuthModule struct {
	Handler *handler.AuthHandler
	Service *service.AuthService
}

// Initialize initializes the auth module with all dependencies
func Initialize(db *pgxpool.Pool, roleRepo service.RoleRepository, permissionReader service.PermissionReader) *AuthModule {
	userRepository := userRepo.NewUserRepository(db)
	refreshTokenRepo := repository.NewRefreshTokenRepository(db)
	authService := service.NewAuthService(userRepository, refreshTokenRepo, roleRepo, permissionReader)
	authHandler := handler.NewAuthHandler(authService)

	return &AuthModule{
		Handler: authHandler,
		Service: authService,
	}
}

// SetupRoutes registers all auth module routes
func (m *AuthModule) SetupRoutes(router *gin.RouterGroup) {
	auth := router.Group("/auth")
	{
		// Public routes
		auth.POST("/signin", m.Handler.SignIn)
		auth.POST("/refresh", m.Handler.Refresh)

		// Protected routes
		auth.POST("/logout", middleware.JWTAuth(), middleware.RejectApiKey(), m.Handler.Logout)
		auth.POST("/logout-all", middleware.JWTAuth(), middleware.RejectApiKey(), m.Handler.LogoutAll)
		auth.GET("/me", middleware.JWTAuth(), middleware.RejectApiKey(), m.Handler.GetMe)
		auth.POST("/change-password", middleware.JWTAuth(), middleware.RejectApiKey(), m.Handler.ChangePassword)
	}
}
