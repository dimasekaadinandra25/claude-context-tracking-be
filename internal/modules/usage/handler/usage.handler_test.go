package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"claude-usage-server/internal/middleware"
	"claude-usage-server/internal/modules/usage/repository"
	"claude-usage-server/internal/modules/usage/service"
	"claude-usage-server/internal/testutil/testdb"
	jwtpkg "claude-usage-server/pkg/jwt"
)

func TestIngestRejectsNulMachine(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/usage/v1/sessions", strings.NewReader(`{"machine":"PC\u0000","sessions":[]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	middleware.SetUserContext(c, &jwtpkg.Claims{UserID: "u"})
	middleware.SetAuthType(c, "api_key")

	New(service.New(nil)).Ingest(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400: %s", w.Code, w.Body.String())
	}
}

func TestIngestStoresAppVersionAndBilledTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	pool := testdb.Open(t)
	ctx := context.Background()
	user := testdb.User(t, pool, "a@test.local", false)
	if _, err := pool.Exec(ctx, `INSERT INTO core.api_keys (user_id, key_id, secret_hash, key_prefix, name) VALUES ($1, 'k1', 'h1', 'p', 'PC')`, user); err != nil {
		t.Fatal(err)
	}
	h := New(service.New(repository.New(pool)))
	const body = `{"machine":"PC","sessions":[{"session_id":"s1","model":"claude-opus-5-5",
		"first_at":"2026-10-30T02:11:09Z","latest_at":"2026-10-30T02:58:40Z",
		"hours":[{"hour":"2026-10-30T02:00:00Z","model":"claude-opus-5-5","input_tokens":10,"cache_write_tokens":20,"cache_read_tokens":30}]}]}`

	for _, tc := range []struct {
		header string
		want   *string
	}{
		{"", nil},
		{" 0.2.0 ", ptr("0.2.0")},
		{"", ptr("0.2.0")},
		{"0.3.0; drop", ptr("0.2.0")},
		{strings.Repeat("1", 33), ptr("0.2.0")},
		{"0.3.0-beta.1", ptr("0.3.0-beta.1")},
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/usage/v1/sessions", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		if tc.header != "" {
			c.Request.Header.Set("X-App-Version", tc.header)
		}
		middleware.SetUserContext(c, &jwtpkg.Claims{UserID: user})
		middleware.SetAuthType(c, "api_key")
		middleware.SetApiKeyID(c, "k1")

		h.Ingest(c)

		var got *string
		if err := pool.QueryRow(ctx, `SELECT app_version FROM core.api_keys WHERE key_id = 'k1'`).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if w.Code != http.StatusOK || (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
			t.Fatalf("header %q: status %d app_version %v, want 200 and %v", tc.header, w.Code, got, tc.want)
		}
	}

	var input, cacheWrite, cacheRead int64
	if err := pool.QueryRow(ctx, `SELECT input_tokens, cache_write_tokens, cache_read_tokens FROM usage.session_hours WHERE session_id = 's1'`).Scan(&input, &cacheWrite, &cacheRead); err != nil {
		t.Fatal(err)
	}
	if input != 10 || cacheWrite != 20 || cacheRead != 30 {
		t.Fatalf("stored %d / %d / %d, want 10 / 20 / 30", input, cacheWrite, cacheRead)
	}
}

func ptr(s string) *string {
	return &s
}
