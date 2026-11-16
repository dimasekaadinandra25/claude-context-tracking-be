package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"claude-usage-server/internal/middleware"
	"claude-usage-server/internal/modules/core/audit/repository"
	jwtpkg "claude-usage-server/pkg/jwt"
)

func TestRecordNeverFailsTheCaller(t *testing.T) {
	gin.SetMode(gin.TestMode)
	pool, err := pgxpool.New(context.Background(), "postgres://nobody:none@127.0.0.1:1/none?connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/core/v1/admin/users", nil)
	middleware.SetUserContext(c, &jwtpkg.Claims{UserID: "d6cb9822-0d8e-4361-9e47-97ef72633428", Email: "boss@x.test"})

	var unset *AuditService
	unset.Record(c, "user.create", "user", "x", "")
	NewAuditService(repository.NewAuditRepository(pool)).Record(c, "user.create", "user", "x", "")

	if w.Code != http.StatusOK || w.Body.Len() != 0 {
		t.Fatalf("Record wrote a response: %d %q", w.Code, w.Body.String())
	}
}
