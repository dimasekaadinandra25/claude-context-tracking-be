package service

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"claude-usage-server/internal/modules/usage/dto"
	"claude-usage-server/internal/modules/usage/repository"
	"claude-usage-server/internal/modules/usage/testdb"
)

func TestScope(t *testing.T) {
	s := &Service{}
	admin := dto.Caller{UserID: "admin", IsAdmin: true}
	member := dto.Caller{UserID: "a"}
	if got := s.scope(admin, ""); got != "" {
		t.Fatalf("admin all: got %q", got)
	}
	if got := s.scope(admin, "b"); got != "b" {
		t.Fatalf("admin b: got %q", got)
	}
	if got := s.scope(member, "b"); got != "a" {
		t.Fatalf("member asking b: got %q", got)
	}
	if got := s.scope(member, ""); got != "a" {
		t.Fatalf("member all: got %q", got)
	}
}

func week() dto.ReportQuery {
	return dto.ReportQuery{From: at("2026-10-29T00:00:00Z"), To: at("2026-11-01T00:00:00Z"), TZ: "UTC"}
}

func ingest(t *testing.T, svc *Service, userID string, sessions ...dto.SessionPayload) {
	t.Helper()
	res, err := svc.Ingest(context.Background(), userID, dto.IngestRequest{Machine: "PC", Sessions: sessions})
	if err != nil || res.Accepted != len(sessions) {
		t.Fatalf("ingest: %+v %v", res, err)
	}
}

func withHours(id string, hours ...dto.HourPayload) dto.SessionPayload {
	p := payload(id)
	p.Hours = hours
	p.NewContext = 0
	for _, h := range hours {
		p.NewContext += h.NewContext
	}
	p.FirstAt = hours[0].Hour
	p.LatestAt = hours[len(hours)-1].Hour.Add(30 * time.Minute)
	return p
}

func TestMemberCannotReadOthers(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	a := testdb.User(t, pool, "a@test.local", false)
	b := testdb.User(t, pool, "b@test.local", false)
	svc := New(repository.New(pool))
	ingest(t, svc, a, payload("a1"))
	ingest(t, svc, b, payload("b1"), withHours("b2", dto.HourPayload{Hour: at("2026-10-30T03:00:00Z"), Model: "claude-haiku-5", NewContext: 999, Requests: 9}))

	caller := dto.Caller{UserID: a}
	q := week()
	q.UserID = b

	ov, err := svc.Overview(ctx, caller, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(ov.People) != 1 || ov.People[0].UserID != a || ov.TotalNewContext != 48000 || ov.TotalSessions != 1 {
		t.Fatalf("overview leaked: %+v", ov)
	}

	models, err := svc.Models(ctx, caller, q)
	if err != nil {
		t.Fatal(err)
	}
	var modelTotal int64
	for _, m := range models {
		if m.Model == "claude-haiku-5" {
			t.Fatalf("models leaked B's model: %+v", models)
		}
		modelTotal += m.NewContext
	}
	if modelTotal != 48000 {
		t.Fatalf("models total = %d, want 48000", modelTotal)
	}

	rows, total, err := svc.Sessions(ctx, caller, dto.SessionQuery{ReportQuery: q})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(rows) != 1 || rows[0].ID != "a1" {
		t.Fatalf("sessions leaked: total=%d rows=%+v", total, rows)
	}

	points, err := svc.Timeseries(ctx, caller, q, "day", "user")
	if err != nil {
		t.Fatal(err)
	}
	if len(points) == 0 {
		t.Fatal("timeseries returned nothing for A")
	}
	for _, p := range points {
		if p.Key != a {
			t.Fatalf("timeseries leaked: %+v", points)
		}
	}

	if _, err := svc.Session(ctx, caller, "b1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Session(b1) err = %v, want ErrNotFound", err)
	}
	if _, err := svc.Session(ctx, caller, "a1"); err != nil {
		t.Fatalf("Session(a1) err = %v", err)
	}
	if _, err := svc.People(ctx, caller); !errors.Is(err, ErrForbidden) {
		t.Fatalf("People err = %v, want ErrForbidden", err)
	}
}

func TestTimeseriesUsesTimeZone(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	a := testdb.User(t, pool, "a@test.local", false)
	svc := New(repository.New(pool))
	ingest(t, svc, a, withHours("late", dto.HourPayload{Hour: at("2026-10-29T23:00:00Z"), Model: "claude-opus-5-5", NewContext: 5000, Requests: 3}))
	caller := dto.Caller{UserID: a}

	for tz, day := range map[string]string{"Asia/Jakarta": "2026-10-30", "UTC": "2026-10-29"} {
		q := week()
		q.TZ = tz
		points, err := svc.Timeseries(ctx, caller, q, "day", "model")
		if err != nil {
			t.Fatal(err)
		}
		loc, _ := time.LoadLocation(tz)
		if len(points) != 1 || points[0].Bucket.In(loc).Format("2006-01-02 15:04") != day+" 00:00" || points[0].NewContext != 5000 {
			t.Fatalf("tz %s: %+v, want one bucket on %s with 5000", tz, points, day)
		}
	}

	q := week()
	q.TZ = "Mars/Base"
	if _, err := svc.Timeseries(ctx, caller, q, "day", "user"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Mars/Base err = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.Timeseries(ctx, caller, week(), "week", "user"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("interval week err = %v, want ErrInvalidInput", err)
	}
}

func TestOverviewTotals(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	a := testdb.User(t, pool, "a@test.local", false)
	b := testdb.User(t, pool, "b@test.local", false)
	admin := testdb.User(t, pool, "boss@test.local", true)
	svc := New(repository.New(pool))
	ingest(t, svc, a, payload("a1"))
	ingest(t, svc, b,
		withHours("b1", dto.HourPayload{Hour: at("2026-10-30T03:00:00Z"), Model: "claude-opus-5-5", NewContext: 5000, Requests: 5}),
		withHours("b2", dto.HourPayload{Hour: at("2026-10-20T03:00:00Z"), Model: "claude-opus-5-5", NewContext: 7000, Requests: 7}),
	)

	ov, err := svc.Overview(ctx, dto.Caller{UserID: admin, IsAdmin: true}, week())
	if err != nil {
		t.Fatal(err)
	}
	if ov.TotalNewContext != 53000 || ov.TotalSessions != 2 || ov.TotalRequests != 155 || ov.ActivePeople != 2 {
		t.Fatalf("totals = %+v, want 53000 new context, 2 sessions, 155 requests, 2 active", ov)
	}
	got := map[string]int64{}
	for _, p := range ov.People {
		got[p.UserID] = p.NewContext
	}
	if got[a] != 48000 || got[b] != 5000 {
		t.Fatalf("per person = %v, want A 48000 and B 5000", got)
	}
}

func TestSessionsListIncludesAllModels(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	a := testdb.User(t, pool, "a@test.local", false)
	svc := New(repository.New(pool))
	bare := payload("bare")
	bare.Hours = nil
	ingest(t, svc, a, payload("mixed"), bare)
	caller := dto.Caller{UserID: a}

	rows, _, err := svc.Sessions(ctx, caller, dto.SessionQuery{ReportQuery: week()})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]string{}
	for _, r := range rows {
		got[r.ID] = r.Models
	}
	want := map[string][]string{
		"mixed": {"claude-opus-5-5", "claude-sonnet-5-5"},
		"bare":  {"claude-opus-5-5"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("models = %v, want %v", got, want)
	}

	detail, err := svc.Session(ctx, caller, "mixed")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(detail.Models, want["mixed"]) {
		t.Fatalf("detail models = %v, want %v", detail.Models, want["mixed"])
	}
}

func TestSessionsFilterMatchesAnyModel(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	a := testdb.User(t, pool, "a@test.local", false)
	svc := New(repository.New(pool))
	ingest(t, svc, a, payload("mixed"),
		withHours("opus-only", dto.HourPayload{Hour: at("2026-10-30T03:00:00Z"), Model: "claude-opus-5-5", NewContext: 5000, Requests: 5}))

	q := dto.SessionQuery{ReportQuery: week(), Model: "claude-sonnet-5-5"}
	rows, total, err := svc.Sessions(ctx, dto.Caller{UserID: a}, q)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(rows) != 1 || rows[0].ID != "mixed" || rows[0].Model != "claude-opus-5-5" {
		t.Fatalf("sonnet filter: total=%d rows=%+v, want only mixed", total, rows)
	}
}
