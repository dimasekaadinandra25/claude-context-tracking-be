package main

import (
	"context"
	"os"
	"time"

	"claude-usage-server/internal/config"
	"claude-usage-server/internal/database"
	auditDomain "claude-usage-server/internal/modules/core/audit/domain"
	auditRepo "claude-usage-server/internal/modules/core/audit/repository"
	authRepo "claude-usage-server/internal/modules/core/auth/repository"
	"claude-usage-server/internal/router"
	"claude-usage-server/pkg/jwt"
	"claude-usage-server/pkg/keycrypt"
	"claude-usage-server/pkg/logger"

	"github.com/gin-gonic/gin"
)

func init() {
	// Force UTC timezone for the entire application
	// This ensures all time.Now() calls return UTC time
	time.Local = time.UTC
}

func main() {
	// Load configuration
	cfg := config.Load()

	// Initialize logger
	if err := logger.Initialize(cfg.Server.Env); err != nil {
		panic("Failed to initialize logger: " + err.Error())
	}
	defer logger.Sync()

	logger.Info("Starting Claude Usage API")

	// Validate JWT secret configuration
	if err := jwt.ValidateSecret(os.Getenv("JWT_SECRET")); err != nil {
		logger.Fatal("JWT secret validation failed: " + err.Error())
	}
	logger.Info("JWT secret validation passed")

	if err := keycrypt.ValidateKey(os.Getenv(keycrypt.EnvName)); err != nil {
		logger.Fatal("API key encryption key validation failed: " + err.Error())
	}

	// Initialize database
	db, err := database.New(cfg.Database.GetDSN())
	if err != nil {
		logger.Fatal("Failed to connect to database")
	}
	defer db.Close()

	logger.Info("Database connected successfully")

	go pruneRefreshTokens(authRepo.NewRefreshTokenRepository(db.Pool), auditRepo.NewAuditRepository(db.Pool))

	// Initialize Gin router
	ginRouter := gin.New() // Use gin.New() instead of gin.Default() since we use custom middleware
	if err := ginRouter.SetTrustedProxies([]string{"127.0.0.1", "::1", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}); err != nil {
		logger.Fatal("Invalid trusted proxies", logger.Err(err))
	}

	// Setup routes (all modules initialized inside router)
	router.Setup(ginRouter, db.Pool)

	// Start server
	serverAddr := cfg.Server.Host + ":" + cfg.Server.Port
	logger.Info("Starting server on " + serverAddr + " (Environment: " + cfg.Server.Env + ")")

	if err := ginRouter.Run(serverAddr); err != nil {
		logger.Fatal("Failed to start server")
	}
}

func pruneRefreshTokens(repo *authRepo.RefreshTokenRepository, audits *auditRepo.AuditRepository) {
	for {
		_ = repo.DeleteExpired(context.Background())
		if _, err := audits.Prune(context.Background(), time.Now().Add(-auditDomain.Retention)); err != nil {
			logger.Error("Failed to prune audit logs", logger.Err(err))
		}
		time.Sleep(time.Hour)
	}
}
