package api_key_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"claude-usage-server/internal/middleware"
	"claude-usage-server/internal/modules/core/api_key"
	roleRepository "claude-usage-server/internal/modules/core/role/repository"
	"claude-usage-server/internal/modules/usage/testdb"
	jwtpkg "claude-usage-server/pkg/jwt"
)

func TestApiKeyCannotCreateApiKeys(t *testing.T) {
	t.Setenv("JWT_SECRET", strings.Repeat("k", 48))
	pool := testdb.Open(t)
	ctx := context.Background()
	admin := testdb.User(t, pool, "boss@x.test", true)
	member := testdb.User(t, pool, "m@x.test", false)

	module := api_key.Initialize(pool, roleRepository.NewRoleRepository(pool))
	middleware.SetApiKeyValidator(module.Service)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	module.SetupRoutes(engine.Group("/core/v1"))

	key, _, err := module.Service.CreateForUser(ctx, admin, member, "desktop")
	if err != nil {
		t.Fatal(err)
	}
	token, err := jwtpkg.GenerateToken(member, "m@x.test", "m", "m", false, []string{"member"})
	if err != nil {
		t.Fatal(err)
	}

	post := func(header, value string) int {
		req := httptest.NewRequest(http.MethodPost, "/core/v1/api-keys", strings.NewReader(`{"name":"minted"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(header, value)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w.Code
	}

	if code := post("X-API-Key", key); code != http.StatusForbidden {
		t.Fatalf("with API key: status %d, want 403", code)
	}
	if code := post("Authorization", "Bearer "+token); code != http.StatusCreated {
		t.Fatalf("with JWT: status %d, want 201", code)
	}
}
