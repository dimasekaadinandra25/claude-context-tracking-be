package router

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	jwtpkg "claude-usage-server/pkg/jwt"
)

type auditPage struct {
	Data []struct {
		ActorEmail  string `json:"actor_email"`
		Action      string `json:"action"`
		TargetType  string `json:"target_type"`
		TargetID    string `json:"target_id"`
		TargetLabel string `json:"target_label"`
		Summary     string `json:"summary"`
		IP          string `json:"ip"`
	} `json:"data"`
	Meta struct {
		Pagination struct {
			Total int `json:"total"`
		} `json:"pagination"`
	} `json:"meta"`
}

func (s stack) must(t *testing.T, method, path, token, body string, want int) []byte {
	t.Helper()
	w := s.do(method, path, "Authorization", "Bearer "+token, strings.NewReader(body))
	if w.Code != want {
		t.Fatalf("%s %s: status %d, want %d: %s", method, path, w.Code, want, w.Body.String())
	}
	return w.Body.Bytes()
}

func (s stack) auditLogs(t *testing.T, query string) auditPage {
	t.Helper()
	var page auditPage
	if err := json.Unmarshal(s.must(t, http.MethodGet, "/core/v1/admin/audit-logs"+query, s.token, "", http.StatusOK), &page); err != nil {
		t.Fatal(err)
	}
	return page
}

func TestAdminActionsAreAudited(t *testing.T) {
	s := newStack(t)
	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(s.must(t, http.MethodPost, "/core/v1/admin/users", s.token,
		`{"email":"rin@x.test","username":"rin","password":"Secret123!"}`, http.StatusCreated), &created); err != nil {
		t.Fatal(err)
	}
	rin := created.Data.ID
	s.must(t, http.MethodPost, "/core/v1/admin/users/"+rin+"/deactivate", s.token, "", http.StatusOK)
	s.must(t, http.MethodPost, "/core/v1/admin/users/"+rin+"/activate", s.token, "", http.StatusOK)
	s.must(t, http.MethodPost, "/core/v1/admin/users/"+rin+"/reset-password", s.token, `{"password":"Reset1234!"}`, http.StatusOK)
	s.must(t, http.MethodPost, "/core/v1/admin/users/"+s.admin+"/deactivate", s.token, "", http.StatusBadRequest)

	var issued struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(s.must(t, http.MethodPost, "/core/v1/admin/users/"+rin+"/api-keys", s.token, `{"name":"desk"}`, http.StatusCreated), &issued); err != nil {
		t.Fatal(err)
	}
	s.must(t, http.MethodGet, "/core/v1/admin/api-keys/"+issued.Data.ID+"/reveal", s.token, "", http.StatusOK)
	s.must(t, http.MethodPost, "/core/v1/admin/api-keys/"+issued.Data.ID+"/revoke", s.token, `{"reason":"lost laptop"}`, http.StatusOK)
	s.must(t, http.MethodPut, "/usage/v1/alert-settings", s.token, `{"alerts":"80,pace","runaway_share":5,"project_share":40}`, http.StatusOK)

	rinToken, err := jwtpkg.GenerateToken(rin, "rin@x.test", "rin", "rin", false, []string{"member"})
	if err != nil {
		t.Fatal(err)
	}
	s.must(t, http.MethodPost, "/core/v1/auth/change-password", rinToken, `{"current_password":"Reset1234!","new_password":"Newer1234!"}`, http.StatusOK)
	s.must(t, http.MethodPut, "/core/v1/users/me/password", rinToken, `{"current_password":"Newer1234!","new_password":"Newest1234!"}`, http.StatusOK)

	all := s.auditLogs(t, "?limit=50")
	want := []string{
		"user.change_password", "user.change_password", "alert_settings.update", "api_key.revoke", "api_key.reveal",
		"api_key.issue", "user.reset_password", "user.activate", "user.deactivate", "user.create",
	}
	if len(all.Data) != len(want) || all.Meta.Pagination.Total != len(want) {
		t.Fatalf("got %d entries (total %d), want %d: %+v", len(all.Data), all.Meta.Pagination.Total, len(want), all.Data)
	}
	for i, action := range want {
		if all.Data[i].Action != action {
			t.Fatalf("entry %d action %q, want %q", i, all.Data[i].Action, action)
		}
	}
	create := all.Data[9]
	if create.ActorEmail != "boss@x.test" || create.TargetType != "user" || create.TargetID != rin || create.TargetLabel != "rin@x.test" || create.IP == "" {
		t.Fatalf("create entry %+v", create)
	}
	if own := all.Data[0]; own.ActorEmail != "rin@x.test" || own.TargetID != rin {
		t.Fatalf("own password change entry %+v", own)
	}
	if settings := all.Data[2]; settings.Summary != `alerts "80,pace", runaway 5%, project 40%` {
		t.Fatalf("settings summary %q", settings.Summary)
	}

	revoked := s.auditLogs(t, "?action=api_key.revoke")
	if len(revoked.Data) != 1 || revoked.Data[0].Summary != "lost laptop" || revoked.Data[0].TargetLabel != "rin@x.test · desk" {
		t.Fatalf("revoke filter %+v", revoked.Data)
	}
	if mine := s.auditLogs(t, "?actor=RIN@x"); len(mine.Data) != 2 {
		t.Fatalf("actor filter got %d entries, want 2", len(mine.Data))
	}
	page := s.auditLogs(t, "?limit=3&page=2")
	if len(page.Data) != 3 || page.Data[0].Action != "api_key.revoke" || page.Meta.Pagination.Total != len(want) {
		t.Fatalf("page 2 %+v", page)
	}
}

func TestAuditLogsAdminOnly(t *testing.T) {
	s := newStack(t)
	memberToken, err := jwtpkg.GenerateToken(s.member, "m@x.test", "m", "m", false, []string{"member"})
	if err != nil {
		t.Fatal(err)
	}
	if w := s.do(http.MethodGet, "/core/v1/admin/audit-logs", "", "", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("no token: status %d, want 401", w.Code)
	}
	if w := s.do(http.MethodGet, "/core/v1/admin/audit-logs", "Authorization", "Bearer "+memberToken, nil); w.Code != http.StatusForbidden {
		t.Fatalf("member: status %d, want 403", w.Code)
	}
	if w := s.do(http.MethodGet, "/core/v1/admin/audit-logs", "X-API-Key", s.issueKey(t, s.admin), nil); w.Code != http.StatusForbidden {
		t.Fatalf("admin API key: status %d, want 403", w.Code)
	}
	if w := s.do(http.MethodGet, "/core/v1/admin/audit-logs?limit=500", "Authorization", "Bearer "+s.token, nil); w.Code != http.StatusBadRequest {
		t.Fatalf("limit 500: status %d, want 400", w.Code)
	}
	if page := s.auditLogs(t, ""); page.Data == nil || len(page.Data) != 0 {
		t.Fatalf("empty log: %+v, want an empty list", page.Data)
	}
}

func TestRoleChangesAndOwnKeysAreAudited(t *testing.T) {
	s := newStack(t)
	ctx := t.Context()
	if _, err := s.pool.Exec(ctx, `INSERT INTO core.roles (id, code, name, description, permissions, is_system, is_active)
		VALUES ('00000000-0000-0000-0000-000000000004', 'usage_viewer', 'Usage Viewer', '', '{}'::jsonb, TRUE, TRUE) ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	s.must(t, http.MethodPut, "/core/v1/admin/users/"+s.member+"/role", s.token, `{"role":"usage_viewer"}`, http.StatusOK)
	s.must(t, http.MethodPut, "/core/v1/admin/users/"+s.member+"/role", s.token, `{"role":"owner"}`, http.StatusBadRequest)
	s.must(t, http.MethodPut, "/core/v1/admin/users/"+s.admin+"/role", s.token, `{"role":"member"}`, http.StatusBadRequest)
	s.must(t, http.MethodPut, "/core/v1/admin/users/00000000-0000-0000-0000-00000000dead/role", s.token, `{"role":"member"}`, http.StatusNotFound)

	var codes []string
	rows, err := s.pool.Query(ctx, `SELECT r.code FROM core.user_roles ur JOIN core.roles r ON r.id = ur.role_id WHERE ur.user_id = $1`, s.member)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			t.Fatal(err)
		}
		codes = append(codes, code)
	}
	rows.Close()
	if len(codes) != 1 || codes[0] != "usage_viewer" {
		t.Fatalf("member roles %v, want [usage_viewer]", codes)
	}

	memberToken, err := jwtpkg.GenerateToken(s.member, "m@x.test", "m", "m", false, []string{"member"})
	if err != nil {
		t.Fatal(err)
	}
	var own struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(s.must(t, http.MethodPost, "/core/v1/api-keys", memberToken, `{"name":"laptop"}`, http.StatusCreated), &own); err != nil {
		t.Fatal(err)
	}
	s.must(t, http.MethodDelete, "/core/v1/api-keys/"+own.Data.ID, memberToken, `{"reason":"rotated"}`, http.StatusOK)

	all := s.auditLogs(t, "")
	want := []struct{ action, actor, summary string }{
		{"api_key.revoke", "m@x.test", "Own key: rotated"},
		{"api_key.issue", "m@x.test", "Own key"},
		{"user.change_role", "boss@x.test", "member → usage_viewer"},
	}
	if len(all.Data) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(all.Data), len(want), all.Data)
	}
	for i, w := range want {
		got := all.Data[i]
		if got.Action != w.action || got.ActorEmail != w.actor || got.Summary != w.summary {
			t.Fatalf("entry %d %+v, want %+v", i, got, w)
		}
	}
	if all.Data[2].TargetID != s.member {
		t.Fatalf("role change target %q, want %q", all.Data[2].TargetID, s.member)
	}
}
