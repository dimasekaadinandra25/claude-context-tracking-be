package service

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"claude-usage-server/internal/modules/usage/dto"
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

func payload(id string) dto.SessionPayload {
	return dto.SessionPayload{
		SessionID:     id,
		Project:       `C:\Code\Claude Usage Monitor`,
		Title:         "Usage server design",
		Model:         "claude-opus-5-5",
		ContextWindow: 1000000,
		StartContext:  21000,
		LatestContext: 180100,
		PeakContext:   210000,
		NewContext:    48000,
		Requests:      150,
		Compactions:   1,
		FirstAt:       at("2026-10-30T02:11:09Z"),
		LatestAt:      at("2026-10-30T06:58:40Z"),
		Hours: []dto.HourPayload{
			{Hour: at("2026-10-30T02:00:00Z"), Model: "claude-opus-5-5", NewContext: 40000, Requests: 120},
			{Hour: at("2026-10-30T06:00:00Z"), Model: "claude-sonnet-5-5", NewContext: 8000, Requests: 30},
		},
	}
}

func TestValidateRejectsBadSessions(t *testing.T) {
	cases := map[string]func(*dto.SessionPayload){
		"negative new_context":   func(s *dto.SessionPayload) { s.NewContext = -1 },
		"negative hour requests": func(s *dto.SessionPayload) { s.Hours[0].Requests = -1 },
		"hour not on the hour":   func(s *dto.SessionPayload) { s.Hours[0].Hour = at("2026-10-30T02:30:00Z") },
		"too many hours": func(s *dto.SessionPayload) {
			s.Hours = make([]dto.HourPayload, 2001)
			for i := range s.Hours {
				s.Hours[i] = dto.HourPayload{Hour: at("2026-10-30T02:00:00Z").Add(time.Duration(i) * time.Hour), Model: "m"}
			}
		},
		"empty session_id":      func(s *dto.SessionPayload) { s.SessionID = "" },
		"long session_id":       func(s *dto.SessionPayload) { s.SessionID = string(make([]byte, 101)) },
		"empty model":           func(s *dto.SessionPayload) { s.Model = "" },
		"empty hour model":      func(s *dto.SessionPayload) { s.Hours[0].Model = "" },
		"first_at after latest": func(s *dto.SessionPayload) { s.FirstAt = s.LatestAt.Add(time.Second) },
		"missing timestamps":    func(s *dto.SessionPayload) { s.FirstAt, s.LatestAt = time.Time{}, time.Time{} },
		"NUL in title":          func(s *dto.SessionPayload) { s.Title = "a\x00b" },
		"NUL in hour model":     func(s *dto.SessionPayload) { s.Hours[0].Model = "m\x00" },
		"NUL in session_id":     func(s *dto.SessionPayload) { s.SessionID = "s\x00" },
		"NUL in project":        func(s *dto.SessionPayload) { s.Project = "p\x00" },
		"NUL in model":          func(s *dto.SessionPayload) { s.Model = "m\x00" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := payload("s1")
			mutate(&s)
			if err := validate(s); err == nil {
				t.Fatal("validate returned nil, want an error")
			}
		})
	}
	if err := validate(payload("s1")); err != nil {
		t.Fatalf("valid session: %v", err)
	}
}

func totals(t *testing.T, pool *pgxpool.Pool) [4]int64 {
	t.Helper()
	var out [4]int64
	err := pool.QueryRow(context.Background(), `
		SELECT (SELECT COUNT(*) FROM usage.sessions),
		       (SELECT COALESCE(SUM(new_context), 0) FROM usage.sessions),
		       (SELECT COUNT(*) FROM usage.session_hours),
		       (SELECT COALESCE(SUM(new_context), 0) FROM usage.session_hours)`).Scan(&out[0], &out[1], &out[2], &out[3])
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestIngestIsIdempotent(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	user := testdb.User(t, pool, "a@test.local", false)
	svc := New(repository.New(pool))
	req := dto.IngestRequest{Machine: "PC", Sessions: []dto.SessionPayload{payload("s1"), payload("s2")}}

	var seen [][4]int64
	for i := 0; i < 2; i++ {
		res, err := svc.Ingest(ctx, user, req)
		if err != nil {
			t.Fatal(err)
		}
		if res.Accepted != 2 || len(res.Rejected) != 0 {
			t.Fatalf("call %d: %+v, want 2 accepted", i+1, res)
		}
		seen = append(seen, totals(t, pool))
	}
	want := [4]int64{2, 96000, 4, 96000}
	if seen[0] != want || seen[1] != want {
		t.Fatalf("totals = %v, want %v after each call", seen, want)
	}
}

func TestIngestRejectsForeignSession(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	a := testdb.User(t, pool, "a@test.local", false)
	b := testdb.User(t, pool, "b@test.local", false)
	svc := New(repository.New(pool))

	if _, err := svc.Ingest(ctx, a, dto.IngestRequest{Machine: "A", Sessions: []dto.SessionPayload{payload("s1")}}); err != nil {
		t.Fatal(err)
	}
	before := totals(t, pool)

	foreign := payload("s1")
	foreign.NewContext = 1
	res, err := svc.Ingest(ctx, b, dto.IngestRequest{Machine: "B", Sessions: []dto.SessionPayload{foreign, payload("s2")}})
	if err != nil {
		t.Fatal(err)
	}
	want := []dto.Rejection{{SessionID: "s1", Reason: "session belongs to another user"}}
	if res.Accepted != 1 || !reflect.DeepEqual(res.Rejected, want) {
		t.Fatalf("got %+v, want accepted 1 and %v", res, want)
	}

	var owner string
	var newContext int64
	if err := pool.QueryRow(ctx, `SELECT user_id::text, new_context FROM usage.sessions WHERE id = 's1'`).Scan(&owner, &newContext); err != nil {
		t.Fatal(err)
	}
	if owner != a || newContext != 48000 {
		t.Fatalf("s1 owner=%s new_context=%d, want %s and 48000", owner, newContext, a)
	}
	after := totals(t, pool)
	if after[2] != before[2]+2 {
		t.Fatalf("hour rows %d -> %d, want only s2's 2 rows added", before[2], after[2])
	}
}

func TestIngestReportsInvalidSessions(t *testing.T) {
	pool := testdb.Open(t)
	user := testdb.User(t, pool, "a@test.local", false)
	svc := New(repository.New(pool))
	bad := payload("bad")
	bad.Requests = -5
	res, err := svc.Ingest(context.Background(), user, dto.IngestRequest{Machine: "PC", Sessions: []dto.SessionPayload{bad, payload("ok")}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Accepted != 1 || len(res.Rejected) != 1 || res.Rejected[0].SessionID != "bad" {
		t.Fatalf("got %+v, want ok accepted and bad rejected", res)
	}
}

func TestIngestRejectsNulBytes(t *testing.T) {
	pool := testdb.Open(t)
	user := testdb.User(t, pool, "a@test.local", false)
	svc := New(repository.New(pool))
	bad := payload("bad")
	bad.Title = "a\x00b"
	res, err := svc.Ingest(context.Background(), user, dto.IngestRequest{Machine: "PC", Sessions: []dto.SessionPayload{bad, payload("ok")}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Accepted != 1 || len(res.Rejected) != 1 || res.Rejected[0].SessionID != "bad" {
		t.Fatalf("got %+v, want ok accepted and bad rejected", res)
	}
}
