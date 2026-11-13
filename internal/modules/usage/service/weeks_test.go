package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"claude-usage-server/internal/modules/usage/dto"
	"claude-usage-server/internal/modules/usage/repository"
	"claude-usage-server/internal/testutil/testdb"
)

func TestWeeksSummariseEachWindowPerMemberWithTheTopModel(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	owner := testdb.User(t, pool, "owner@test.local", false)
	guest := testdb.User(t, pool, "guest@test.local", false)
	svc := New(repository.New(pool))
	send := func(user string, req dto.IngestRequest) {
		t.Helper()
		req.Machine = "PC"
		if _, err := svc.Ingest(ctx, user, req); err != nil {
			t.Fatal(err)
		}
	}
	account := json.RawMessage(`{"email":"team@claude.test","tier":"default_claude_max_20x"}`)
	send(owner, dto.IngestRequest{Account: account, Sessions: encode(withHours("o1",
		dto.HourPayload{Hour: at("2026-10-28T02:00:00Z"), Model: "claude-opus-5-5", Requests: 3, CacheReadTokens: 900, OutputTokens: 20},
		dto.HourPayload{Hour: at("2026-10-29T02:00:00Z"), Model: "claude-haiku-5-5", Requests: 1, InputTokens: 50},
	))})
	send(guest, dto.IngestRequest{Account: account, Sessions: encode(withHours("g1",
		dto.HourPayload{Hour: at("2026-10-30T03:00:00Z"), Model: "claude-haiku-5-5", Requests: 2, InputTokens: 100, SubagentRequests: 1, SubagentOutputTokens: 4},
	))})
	window := at("2026-11-02T09:00:00Z")
	send(owner, dto.IngestRequest{PlanLimits: weekly(t, 10, window, at("2026-10-29T12:00:00Z"))})
	send(guest, dto.IngestRequest{PlanLimits: weekly(t, 25, window.Add(20*time.Minute), at("2026-10-31T12:00:00Z"))})

	rows, err := svc.Weeks(ctx, dto.Caller{IsAdmin: true}, week())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %+v", rows)
	}
	first, second := rows[0], rows[1]
	if first.Used != 25 || !first.ResetsAt.Equal(window) || first.TopModel == nil || *first.TopModel != "claude-opus-5-5" ||
		first.ClaudeEmail == nil || *first.ClaudeEmail != "team@claude.test" || first.ClaudeTier == nil {
		t.Fatalf("window = %+v", first)
	}
	if *first.UserID != owner || first.InputTokens != 950 || first.OutputTokens != 20 || first.Requests != 4 || first.Weight != 1050 {
		t.Fatalf("owner = %+v", first)
	}
	if *second.UserID != guest || second.InputTokens != 100 || second.OutputTokens != 4 || second.Requests != 3 || second.Weight != 120 {
		t.Fatalf("guest = %+v", second)
	}

	own, err := svc.Weeks(ctx, dto.Caller{UserID: guest}, dto.ReportQuery{From: week().From, To: week().To, TZ: "UTC", UserID: owner})
	if err != nil {
		t.Fatal(err)
	}
	if len(own) != 1 || *own[0].UserID != guest || own[0].Used != 25 {
		t.Fatalf("member sees = %+v", own)
	}
}
