package service

import (
	"context"
	"errors"
	"testing"

	"claude-usage-server/internal/modules/usage/dto"
	"claude-usage-server/internal/modules/usage/repository"
	"claude-usage-server/internal/testutil/testdb"
)

func TestNormalizeAlerts(t *testing.T) {
	for in, want := range map[string]string{
		"":                                     "",
		"80,90,pace":                           "80,90,pace",
		" 90 , Pace  summary,90":               "90,pace,summary",
		"1,100,07":                             "1,100,7",
		"stale,runaway,project,budget,version": "stale,runaway,project,budget,version",
	} {
		got, err := normalizeAlerts(in)
		if err != nil || got != want {
			t.Errorf("normalizeAlerts(%q) = %q, %v, want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"0", "101", "80.5", "-5", "abc", "80,weekly"} {
		if _, err := normalizeAlerts(in); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("normalizeAlerts(%q) err = %v, want ErrInvalidInput", in, err)
		}
	}
}

func TestAlertSettingsAccessAndRoundTrip(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	admin := dto.Caller{UserID: testdb.User(t, pool, "boss@test.local", true), IsAdmin: true, ViewAll: true}
	viewer := dto.Caller{UserID: testdb.User(t, pool, "viewer@test.local", false), ViewAll: true}
	member := dto.Caller{UserID: testdb.User(t, pool, "a@test.local", false)}
	svc := New(repository.New(pool))

	got, err := svc.AlertSettings(ctx, viewer)
	if err != nil || got != nil {
		t.Fatalf("nothing stored: %+v, %v, want nil", got, err)
	}
	if _, err := svc.AlertSettings(ctx, member); !errors.Is(err, ErrForbidden) {
		t.Fatalf("member GET err = %v, want ErrForbidden", err)
	}

	alerts := "90, 80,PACE"
	req := dto.AlertSettings{Alerts: &alerts, RunawayShare: 7, ProjectShare: 30}
	if _, err := svc.SetAlertSettings(ctx, viewer, req); !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer PUT err = %v, want ErrForbidden", err)
	}
	saved, err := svc.SetAlertSettings(ctx, admin, req)
	if err != nil || *saved.Alerts != "90,80,pace" {
		t.Fatalf("admin PUT: %+v, %v", saved, err)
	}

	none := ""
	if _, err := svc.SetAlertSettings(ctx, admin, dto.AlertSettings{Alerts: &none, RunawayShare: 5, ProjectShare: 40}); err != nil {
		t.Fatal(err)
	}
	got, err = svc.AlertSettings(ctx, viewer)
	if err != nil || got == nil || *got.Alerts != "" || got.RunawayShare != 5 || got.ProjectShare != 40 {
		t.Fatalf("after second PUT: %+v, %v", got, err)
	}

	bad := "80,loud"
	if _, err := svc.SetAlertSettings(ctx, admin, dto.AlertSettings{Alerts: &bad, RunawayShare: 5, ProjectShare: 40}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("bad word err = %v, want ErrInvalidInput", err)
	}
	var rows int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM usage.alert_settings`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("rows = %d, %v, want a single row", rows, err)
	}
}
