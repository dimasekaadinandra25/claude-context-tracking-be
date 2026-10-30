package usage

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"claude-usage-server/internal/middleware"
	"claude-usage-server/internal/modules/usage/handler"
	"claude-usage-server/internal/modules/usage/repository"
	"claude-usage-server/internal/modules/usage/service"
)

type Module struct {
	Handler *handler.Handler
	Service *service.Service
}

func Initialize(db *pgxpool.Pool) *Module {
	svc := service.New(repository.New(db))
	return &Module{Handler: handler.New(svc), Service: svc}
}

func (m *Module) SetupRoutes(rg *gin.RouterGroup) {
	rg.Use(middleware.JWTAuth())
	rg.POST("/sessions", m.Handler.Ingest)
	rg.GET("/overview", m.Handler.Overview)
	rg.GET("/timeseries", m.Handler.Timeseries)
	rg.GET("/models", m.Handler.Models)
	rg.GET("/sessions", m.Handler.Sessions)
	rg.GET("/sessions/:id", m.Handler.Session)
	rg.GET("/people", m.Handler.People)
}
