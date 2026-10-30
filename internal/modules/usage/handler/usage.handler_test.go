package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"claude-usage-server/internal/middleware"
	"claude-usage-server/internal/modules/usage/service"
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
