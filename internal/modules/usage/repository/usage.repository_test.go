package repository_test

import (
	"context"
	"testing"
	"time"

	"claude-usage-server/internal/modules/usage/domain"
	"claude-usage-server/internal/modules/usage/repository"
	"claude-usage-server/internal/modules/usage/testdb"
)

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func session(id, userID string, newContext int64) domain.Session {
	return domain.Session{
		ID:             id,
		UserID:         userID,
		Machine:        "PC",
		Project:        "proj",
		Title:          "title",
		Model:          "claude-opus-5-5",
		ContextWindow:  1000000,
		StartContext:   21000,
		LatestContext:  180100,
		PeakContext:    210000,
		NewContext:     newContext,
		Requests:       150,
		Compactions:    1,
		FirstRequestAt: at("2026-10-30T02:11:09Z"),
		LastRequestAt:  at("2026-10-30T06:58:40Z"),
	}
}

func TestSaveSessionUpsertsAbsoluteValues(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	userID := testdb.User(t, pool, "a@test.local", false)
	repo := repository.New(pool)

	hours := []domain.HourUsage{
		{Hour: at("2026-10-30T02:00:00Z"), Model: "opus", NewContext: 40000, Requests: 120},
		{Hour: at("2026-10-30T06:00:00Z"), Model: "sonnet", NewContext: 8000, Requests: 30},
	}
	if owned, err := repo.SaveSession(ctx, session("s1", userID, 64000), hours); err != nil || !owned {
		t.Fatalf("first save: owned=%v err=%v", owned, err)
	}

	hours[1].NewContext = 14000
	if owned, err := repo.SaveSession(ctx, session("s1", userID, 70000), hours); err != nil || !owned {
		t.Fatalf("second save: owned=%v err=%v", owned, err)
	}

	var sessions, newContext int64
	if err := pool.QueryRow(ctx, `SELECT COUNT(*), MAX(new_context) FROM usage.sessions`).Scan(&sessions, &newContext); err != nil {
		t.Fatal(err)
	}
	if sessions != 1 || newContext != 70000 {
		t.Fatalf("sessions=%d new_context=%d, want 1 and 70000", sessions, newContext)
	}

	var rows, sum, sonnet int64
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*), SUM(new_context),
		       MAX(new_context) FILTER (WHERE model = 'sonnet' AND hour = '2026-10-30T06:00:00Z')
		FROM usage.session_hours WHERE session_id = 's1'`).Scan(&rows, &sum, &sonnet); err != nil {
		t.Fatal(err)
	}
	if rows != 2 || sum != 54000 || sonnet != 14000 {
		t.Fatalf("rows=%d sum=%d sonnet=%d, want 2, 54000, 14000", rows, sum, sonnet)
	}
}

func TestSaveSessionRefusesOtherOwner(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	a := testdb.User(t, pool, "a@test.local", false)
	b := testdb.User(t, pool, "b@test.local", false)
	repo := repository.New(pool)

	hours := []domain.HourUsage{{Hour: at("2026-10-30T02:00:00Z"), Model: "opus", NewContext: 40000, Requests: 120}}
	if owned, err := repo.SaveSession(ctx, session("s1", a, 40000), hours); err != nil || !owned {
		t.Fatalf("save as A: owned=%v err=%v", owned, err)
	}

	stolen := []domain.HourUsage{{Hour: at("2026-10-30T02:00:00Z"), Model: "opus", NewContext: 1, Requests: 1}}
	owned, err := repo.SaveSession(ctx, session("s1", b, 1), stolen)
	if err != nil {
		t.Fatal(err)
	}
	if owned {
		t.Fatal("save as B: owned=true, want false")
	}

	var owner string
	var newContext int64
	if err := pool.QueryRow(ctx, `SELECT user_id::text, new_context FROM usage.sessions WHERE id = 's1'`).Scan(&owner, &newContext); err != nil {
		t.Fatal(err)
	}
	if owner != a || newContext != 40000 {
		t.Fatalf("owner=%s new_context=%d, want %s and 40000", owner, newContext, a)
	}

	var hourContext, hourRequests int64
	if err := pool.QueryRow(ctx, `SELECT new_context, requests FROM usage.session_hours WHERE session_id = 's1'`).Scan(&hourContext, &hourRequests); err != nil {
		t.Fatal(err)
	}
	if hourContext != 40000 || hourRequests != 120 {
		t.Fatalf("hour row = %d/%d, want 40000/120", hourContext, hourRequests)
	}
}

func TestSaveSessionDropsHoursMissingFromPayload(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	userID := testdb.User(t, pool, "a@test.local", false)
	repo := repository.New(pool)

	first := []domain.HourUsage{
		{Hour: at("2026-10-30T01:00:00Z"), Model: "old", NewContext: 100, Requests: 1},
		{Hour: at("2026-10-30T02:00:00Z"), Model: "m", NewContext: 200, Requests: 1},
	}
	if owned, err := repo.SaveSession(ctx, session("s1", userID, 300), first); err != nil || !owned {
		t.Fatalf("first save: owned=%v err=%v", owned, err)
	}
	second := []domain.HourUsage{
		{Hour: at("2026-10-30T01:00:00Z"), Model: "m", NewContext: 100, Requests: 1},
		{Hour: at("2026-10-30T02:00:00Z"), Model: "m", NewContext: 200, Requests: 1},
	}
	if owned, err := repo.SaveSession(ctx, session("s1", userID, 300), second); err != nil || !owned {
		t.Fatalf("second save: owned=%v err=%v", owned, err)
	}

	var rows, sum, old int64
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*), SUM(new_context), COUNT(*) FILTER (WHERE model = 'old')
		FROM usage.session_hours WHERE session_id = 's1'`).Scan(&rows, &sum, &old); err != nil {
		t.Fatal(err)
	}
	if rows != 2 || sum != 300 || old != 0 {
		t.Fatalf("rows=%d sum=%d old=%d, want 2, 300, 0", rows, sum, old)
	}
}

func TestSaveSessionToleratesDuplicateHours(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	userID := testdb.User(t, pool, "a@test.local", false)
	repo := repository.New(pool)

	hours := []domain.HourUsage{
		{Hour: at("2026-10-30T01:00:00Z"), Model: "m", NewContext: 100, Requests: 1},
		{Hour: at("2026-10-30T01:00:00Z"), Model: "m", NewContext: 100, Requests: 1},
	}
	if owned, err := repo.SaveSession(ctx, session("s1", userID, 100), hours); err != nil || !owned {
		t.Fatalf("save: owned=%v err=%v", owned, err)
	}
}
