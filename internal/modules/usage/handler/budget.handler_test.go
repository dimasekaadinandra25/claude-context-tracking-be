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

func TestSetBudgetRejects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const id = "d6cb9822-0d8e-4361-9e47-97ef72633428"
	for name, tc := range map[string]struct {
		admin  bool
		userID string
		body   string
		want   int
	}{
		"member":       {false, id, `{"monthly_new_context":1000}`, http.StatusForbidden},
		"zero":         {true, id, `{"monthly_new_context":0}`, http.StatusBadRequest},
		"negative":     {true, id, `{"monthly_new_context":-5}`, http.StatusBadRequest},
		"missing":      {true, id, `{}`, http.StatusBadRequest},
		"not a number": {true, id, `{"monthly_new_context":"lots"}`, http.StatusBadRequest},
		"bad id":       {true, "nope", `{"monthly_new_context":1000}`, http.StatusNotFound},
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPut, "/usage/v1/budgets/"+tc.userID, strings.NewReader(tc.body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Params = gin.Params{{Key: "user_id", Value: tc.userID}}
		middleware.SetUserContext(c, &jwtpkg.Claims{UserID: "u", IsSuperAdmin: tc.admin})

		New(service.New(nil)).SetBudget(c)

		if w.Code != tc.want {
			t.Fatalf("%s: status %d, want %d: %s", name, w.Code, tc.want, w.Body.String())
		}
	}
}

func TestDeleteBudgetMemberForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/usage/v1/budgets/x", nil)
	c.Params = gin.Params{{Key: "user_id", Value: "d6cb9822-0d8e-4361-9e47-97ef72633428"}}
	middleware.SetUserContext(c, &jwtpkg.Claims{UserID: "u"})

	New(service.New(nil)).DeleteBudget(c)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403: %s", w.Code, w.Body.String())
	}
}
