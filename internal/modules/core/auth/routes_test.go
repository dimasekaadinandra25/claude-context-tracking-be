package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"claude-usage-server/internal/middleware"
	"claude-usage-server/internal/modules/core/auth"
	roleRepository "claude-usage-server/internal/modules/core/role/repository"
	"claude-usage-server/internal/modules/core/user"
	"claude-usage-server/internal/shared/authz"
	"claude-usage-server/internal/testutil/testdb"
	jwtpkg "claude-usage-server/pkg/jwt"
)

func TestJWTRejectedAfterDeactivation(t *testing.T) {
	t.Setenv("JWT_SECRET", strings.Repeat("k", 48))
	pool := testdb.Open(t)
	ctx := context.Background()
	member := testdb.User(t, pool, "m@x.test", false)

	roles := roleRepository.NewRoleRepository(pool)
	middleware.SetUserValidator(user.Initialize(pool).Service)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	auth.Initialize(pool, roles, authz.NewService(roles)).SetupRoutes(engine.Group("/core/v1"))

	token, err := jwtpkg.GenerateToken(member, "m@x.test", "m", "m", false, []string{"member"})
	if err != nil {
		t.Fatal(err)
	}
	me := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/core/v1/auth/me", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w
	}

	if w := me(); w.Code != http.StatusOK {
		t.Fatalf("active user: status %d, want 200: %s", w.Code, w.Body.String())
	}

	for name, stmt := range map[string]string{
		"deactivated": `UPDATE core.users SET is_active = FALSE WHERE id = $1`,
		"deleted":     `UPDATE core.users SET is_active = TRUE, deleted_at = NOW() WHERE id = $1`,
	} {
		if _, err := pool.Exec(ctx, stmt, member); err != nil {
			t.Fatal(err)
		}
		w := me()
		var body struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if w.Code != http.StatusUnauthorized || body.Message != "User account is not active" {
			t.Fatalf("%s: status %d message %q, want 401 User account is not active", name, w.Code, body.Message)
		}
	}
}
