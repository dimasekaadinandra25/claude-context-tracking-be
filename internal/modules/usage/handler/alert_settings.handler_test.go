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

func TestSetAlertSettingsRejects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const valid = `{"alerts":"80,pace","runaway_share":5,"project_share":40}`
	for name, tc := range map[string]struct {
		admin bool
		body  string
		want  int
	}{
		"member":          {false, valid, http.StatusForbidden},
		"missing alerts":  {true, `{"runaway_share":5,"project_share":40}`, http.StatusBadRequest},
		"zero runaway":    {true, `{"alerts":"80","runaway_share":0,"project_share":40}`, http.StatusBadRequest},
		"project over":    {true, `{"alerts":"80","runaway_share":5,"project_share":101}`, http.StatusBadRequest},
		"unknown word":    {true, `{"alerts":"80,loud","runaway_share":5,"project_share":40}`, http.StatusBadRequest},
		"threshold 0":     {true, `{"alerts":"0","runaway_share":5,"project_share":40}`, http.StatusBadRequest},
		"share as string": {true, `{"alerts":"80","runaway_share":"5","project_share":40}`, http.StatusBadRequest},
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPut, "/usage/v1/alert-settings", strings.NewReader(tc.body))
		c.Request.Header.Set("Content-Type", "application/json")
		middleware.SetUserContext(c, &jwtpkg.Claims{UserID: "u", IsSuperAdmin: tc.admin})

		New(service.New(nil)).SetAlertSettings(c)

		if w.Code != tc.want {
			t.Fatalf("%s: status %d, want %d: %s", name, w.Code, tc.want, w.Body.String())
		}
	}
}

func TestAlertSettingsMemberForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/usage/v1/alert-settings", nil)
	middleware.SetUserContext(c, &jwtpkg.Claims{UserID: "u"})

	New(service.New(nil)).AlertSettings(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403: %s", w.Code, w.Body.String())
	}
}
