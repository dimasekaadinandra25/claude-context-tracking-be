package router

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	apiKeyRepository "claude-usage-server/internal/modules/core/api_key/repository"
	apiKeyService "claude-usage-server/internal/modules/core/api_key/service"
	"claude-usage-server/internal/testutil/testdb"
	jwtpkg "claude-usage-server/pkg/jwt"
	"claude-usage-server/pkg/keycrypt"
)

type stack struct {
	engine *gin.Engine
	pool   *pgxpool.Pool
	keys   *apiKeyService.ApiKeyService
	admin  string
	member string
	token  string
}

func newStack(t *testing.T) stack {
	t.Helper()
	t.Setenv("JWT_SECRET", strings.Repeat("k", 48))
	t.Setenv(keycrypt.EnvName, strings.Repeat("ab", 32))
	pool := testdb.Open(t)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	Setup(engine, pool)
	admin := testdb.User(t, pool, "boss@x.test", true)
	token, err := jwtpkg.GenerateToken(admin, "boss@x.test", "boss", "boss", true, []string{"super_admin"})
	if err != nil {
		t.Fatal(err)
	}
	return stack{
		engine: engine,
		pool:   pool,
		keys:   apiKeyService.NewApiKeyService(apiKeyRepository.NewApiKeyRepository(pool), nil),
		admin:  admin,
		member: testdb.User(t, pool, "m@x.test", false),
		token:  token,
	}
}

func (s stack) do(method, path, header, value string, body io.Reader) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", "application/json")
	if header != "" {
		req.Header.Set(header, value)
	}
	w := httptest.NewRecorder()
	s.engine.ServeHTTP(w, req)
	return w
}

func (s stack) issueKey(t *testing.T, userID string) string {
	t.Helper()
	key, _, err := s.keys.CreateForUser(context.Background(), s.admin, userID, "desktop")
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestIngestBodyLimit(t *testing.T) {
	s := newStack(t)
	key := s.issueKey(t, s.member)

	small := s.do(http.MethodPost, "/usage/v1/sessions", "X-API-Key", key, strings.NewReader(`{"machine":"PC","sessions":[]}`))
	if small.Code != http.StatusOK {
		t.Fatalf("small body: status %d, want 200: %s", small.Code, small.Body.String())
	}

	padding := bytes.Repeat([]byte(" "), 64<<20)
	body := io.MultiReader(strings.NewReader(`{"machine":"PC","sessions":[]`), bytes.NewReader(padding), strings.NewReader(`}`))
	if w := s.do(http.MethodPost, "/usage/v1/sessions", "X-API-Key", key, body); w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("body over 64 MB: status %d, want 413", w.Code)
	}
}

func TestApiKeyFailuresAreGeneric(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	good := s.issueKey(t, s.member)
	parts := strings.Split(good, "_")

	revoked := s.issueKey(t, s.member)
	if _, err := s.pool.Exec(ctx, `UPDATE core.api_keys SET revoked_at = NOW() WHERE key_id = $1`, strings.Split(revoked, "_")[2]); err != nil {
		t.Fatal(err)
	}
	other := testdb.User(t, s.pool, "o@x.test", false)
	inactive := s.issueKey(t, other)
	if _, err := s.pool.Exec(ctx, `UPDATE core.users SET is_active = FALSE WHERE id = $1`, other); err != nil {
		t.Fatal(err)
	}

	for name, key := range map[string]string{
		"bad format":     "garbage",
		"unknown key id": "wiz_live_zzzzzzzz_" + parts[3],
		"wrong secret":   strings.Join([]string{parts[0], parts[1], parts[2], strings.Repeat("0", len(parts[3]))}, "_"),
		"revoked":        revoked,
		"inactive owner": inactive,
	} {
		w := s.do(http.MethodPost, "/usage/v1/sessions", "X-API-Key", key, strings.NewReader(`{"machine":"PC","sessions":[]}`))
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		_, hasDetail := body["errors"]
		if w.Code != http.StatusUnauthorized || body["message"] != "Invalid API key" || hasDetail {
			t.Fatalf("%s: status %d body %s, want 401 with only message Invalid API key", name, w.Code, w.Body.String())
		}
	}
}

func TestIDRoutesReturn404(t *testing.T) {
	s := newStack(t)
	routes := []struct {
		method, path, body string
	}{
		{http.MethodGet, "/core/v1/users/%s", ""},
		{http.MethodPost, "/core/v1/admin/users/%s/deactivate", ""},
		{http.MethodPost, "/core/v1/admin/users/%s/activate", ""},
		{http.MethodPost, "/core/v1/admin/users/%s/reset-password", `{"password":"Reset1234!"}`},
		{http.MethodPost, "/core/v1/admin/users/%s/api-keys", `{"name":"desktop"}`},
		{http.MethodGet, "/core/v1/admin/users/%s/api-keys", ""},
		{http.MethodPost, "/core/v1/admin/api-keys/%s/revoke", ""},
		{http.MethodGet, "/core/v1/admin/api-keys/%s/reveal", ""},
		{http.MethodGet, "/core/v1/api-keys/%s", ""},
		{http.MethodPatch, "/core/v1/api-keys/%s", `{"name":"renamed"}`},
		{http.MethodDelete, "/core/v1/api-keys/%s", ""},
		{http.MethodGet, "/usage/v1/sessions/%s", ""},
	}
	for _, r := range routes {
		for _, id := range []string{"not-a-uuid", uuid.NewString()} {
			path := fmt.Sprintf(r.path, id)
			if w := s.do(r.method, path, "Authorization", "Bearer "+s.token, strings.NewReader(r.body)); w.Code != http.StatusNotFound {
				t.Errorf("%s %s: status %d, want 404: %s", r.method, path, w.Code, w.Body.String())
			}
		}
	}
}
