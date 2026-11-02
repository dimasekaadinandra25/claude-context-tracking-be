package api_key_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"claude-usage-server/internal/middleware"
	"claude-usage-server/internal/modules/core/api_key"
	roleRepository "claude-usage-server/internal/modules/core/role/repository"
	"claude-usage-server/internal/modules/core/user"
	"claude-usage-server/internal/testutil/testdb"
	jwtpkg "claude-usage-server/pkg/jwt"
	"claude-usage-server/pkg/keycrypt"
)

func TestApiKeyCannotCreateApiKeys(t *testing.T) {
	t.Setenv("JWT_SECRET", strings.Repeat("k", 48))
	t.Setenv(keycrypt.EnvName, strings.Repeat("ab", 32))
	pool := testdb.Open(t)
	ctx := context.Background()
	admin := testdb.User(t, pool, "boss@x.test", true)
	member := testdb.User(t, pool, "m@x.test", false)

	module := api_key.Initialize(pool, roleRepository.NewRoleRepository(pool))
	middleware.SetApiKeyValidator(module.Service)
	middleware.SetUserValidator(user.Initialize(pool).Service)
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

func TestRevealRequiresAdminJWT(t *testing.T) {
	t.Setenv("JWT_SECRET", strings.Repeat("k", 48))
	t.Setenv(keycrypt.EnvName, strings.Repeat("ab", 32))
	pool := testdb.Open(t)
	ctx := context.Background()
	admin := testdb.User(t, pool, "boss@x.test", true)
	member := testdb.User(t, pool, "m@x.test", false)

	module := api_key.Initialize(pool, roleRepository.NewRoleRepository(pool))
	middleware.SetApiKeyValidator(module.Service)
	middleware.SetUserValidator(user.Initialize(pool).Service)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	module.SetupRoutes(engine.Group("/core/v1"))

	memberKey, issued, err := module.Service.CreateForUser(ctx, admin, member, "desktop")
	if err != nil {
		t.Fatal(err)
	}
	adminKey, _, err := module.Service.CreateForUser(ctx, admin, admin, "admin-desktop")
	if err != nil {
		t.Fatal(err)
	}
	memberToken, err := jwtpkg.GenerateToken(member, "m@x.test", "m", "m", false, []string{"member"})
	if err != nil {
		t.Fatal(err)
	}
	adminToken, err := jwtpkg.GenerateToken(admin, "boss@x.test", "boss", "boss", true, []string{"super_admin"})
	if err != nil {
		t.Fatal(err)
	}

	reveal := func(id, header, value string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/core/v1/admin/api-keys/"+id+"/reveal", nil)
		req.Header.Set(header, value)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w
	}

	if w := reveal(issued.ID, "Authorization", "Bearer "+memberToken); w.Code != http.StatusForbidden {
		t.Fatalf("member JWT: status %d, want 403", w.Code)
	}
	if w := reveal(issued.ID, "X-API-Key", adminKey); w.Code != http.StatusForbidden {
		t.Fatalf("admin API key: status %d, want 403", w.Code)
	}
	w := reveal(issued.ID, "Authorization", "Bearer "+adminToken)
	if w.Code != http.StatusOK {
		t.Fatalf("admin JWT: status %d, want 200", w.Code)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control %q, want no-store", got)
	}
	var body struct {
		Data struct {
			Key string `json:"key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Key != memberKey {
		t.Fatal("revealed key differs from the issued key")
	}
	if w := reveal("not-a-uuid", "Authorization", "Bearer "+adminToken); w.Code != http.StatusNotFound {
		t.Fatalf("non-UUID id: status %d, want 404", w.Code)
	}
}
