package service

import (
	"context"
	"encoding/json"
	"testing"

	"claude-usage-server/internal/modules/usage/dto"
	"claude-usage-server/internal/modules/usage/repository"
	"claude-usage-server/internal/testutil/testdb"
)

func TestIngestStoresOutputAndSubagentTotals(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	a := testdb.User(t, pool, "a@test.local", false)
	admin := dto.Caller{UserID: testdb.User(t, pool, "boss@test.local", true), IsAdmin: true}
	svc := New(repository.New(pool))

	mixed := payload("mixed")
	mixed.Hours[0].OutputTokens, mixed.Hours[0].SubagentRequests, mixed.Hours[0].SubagentOutputTokens = 1200, 3, 700
	mixed.Hours[1].OutputTokens, mixed.Hours[1].SubagentRequests, mixed.Hours[1].SubagentOutputTokens = 300, 1, 100
	quiet := withHours("quiet", dto.HourPayload{Hour: at("2026-10-30T04:00:00Z"), Model: "claude-opus-5-5", OutputTokens: 50, SubagentRequests: 2, SubagentOutputTokens: 20})
	ingest(t, svc, a, mixed, quiet)

	ov, err := svc.Overview(ctx, admin, week())
	if err != nil {
		t.Fatal(err)
	}
	if ov.TotalOutputTokens != 1550 || ov.TotalSubagentRequests != 6 || ov.TotalSubagentOutputTokens != 820 {
		t.Fatalf("overview totals = %+v, want 1550 / 6 / 820", ov)
	}
	for _, p := range ov.People {
		if p.UserID == a && (p.OutputTokens != 1550 || p.SubagentRequests != 6 || p.SubagentOutputTokens != 820) {
			t.Fatalf("person A = %+v, want 1550 / 6 / 820", p)
		}
	}

	models, err := svc.Models(ctx, admin, week())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][3]int64{}
	for _, m := range models {
		got[m.Model] = [3]int64{m.OutputTokens, m.SubagentRequests, m.SubagentOutputTokens}
	}
	if got["claude-opus-5-5"] != [3]int64{1250, 5, 720} || got["claude-sonnet-5-5"] != [3]int64{300, 1, 100} {
		t.Fatalf("models = %v", got)
	}

	points, err := svc.Timeseries(ctx, admin, week(), "day", "user")
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 1 || points[0].OutputTokens != 1550 || points[0].SubagentRequests != 6 || points[0].SubagentOutputTokens != 820 {
		t.Fatalf("timeseries = %+v, want one point 1550 / 6 / 820", points)
	}

	rows, total, err := svc.Sessions(ctx, admin, dto.SessionQuery{ReportQuery: week()})
	if err != nil {
		t.Fatal(err)
	}
	sessions := map[string][3]int64{}
	for _, r := range rows {
		sessions[r.ID] = [3]int64{r.OutputTokens, r.SubagentRequests, r.SubagentOutputTokens}
	}
	if total != 2 || sessions["mixed"] != [3]int64{1500, 4, 800} || sessions["quiet"] != [3]int64{50, 2, 20} {
		t.Fatalf("sessions total=%d rows=%v, want mixed and quiet (no new context) with range sums", total, sessions)
	}

	detail, err := svc.Session(ctx, admin, "mixed")
	if err != nil {
		t.Fatal(err)
	}
	h := detail.Hours[0]
	if h.OutputTokens != 1200 || h.SubagentRequests != 3 || h.SubagentOutputTokens != 700 {
		t.Fatalf("detail hour = %+v, want 1200 / 3 / 700", h)
	}
}

func TestIngestWithoutNewFieldsDefaultsToZero(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	a := testdb.User(t, pool, "a@test.local", false)
	svc := New(repository.New(pool))

	var req dto.IngestRequest
	if err := json.Unmarshal([]byte(`{"machine":"PC","sessions":[{"session_id":"old","model":"claude-opus-5-5","new_context":500,"requests":2,
		"first_at":"2026-10-30T02:11:09Z","latest_at":"2026-10-30T02:58:40Z",
		"hours":[{"hour":"2026-10-30T02:00:00Z","model":"claude-opus-5-5","new_context":500,"requests":2}]}]}`), &req); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Ingest(ctx, a, req)
	if err != nil || res.Accepted != 1 {
		t.Fatalf("ingest: %+v %v", res, err)
	}

	var output, subRequests, subOutput int64
	if err := pool.QueryRow(ctx, `SELECT output_tokens, subagent_requests, subagent_output_tokens FROM usage.session_hours WHERE session_id = 'old'`).Scan(&output, &subRequests, &subOutput); err != nil {
		t.Fatal(err)
	}
	if output != 0 || subRequests != 0 || subOutput != 0 {
		t.Fatalf("stored %d / %d / %d, want zeros", output, subRequests, subOutput)
	}
	ov, err := svc.Overview(ctx, dto.Caller{UserID: a}, week())
	if err != nil {
		t.Fatal(err)
	}
	if ov.TotalNewContext != 500 || ov.TotalOutputTokens != 0 || ov.TotalSubagentRequests != 0 || ov.TotalSubagentOutputTokens != 0 {
		t.Fatalf("overview = %+v, want 500 new context and zero output/subagent totals", ov)
	}
}
