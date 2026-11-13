package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"claude-usage-server/internal/modules/usage/domain"
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

func TestCostPerMillionUsesTheWholeAccountsCurrentWeek(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	owner := testdb.User(t, pool, "owner@test.local", false)
	guest := testdb.User(t, pool, "guest@test.local", false)
	loner := testdb.User(t, pool, "loner@test.local", false)
	svc := New(repository.New(pool))
	now := time.Now().UTC().Truncate(time.Hour)
	account := json.RawMessage(`{"email":"team@claude.test","tier":"default_claude_max_20x"}`)
	for _, u := range []struct {
		id    string
		reads int64
	}{{owner, 7_000_000}, {guest, 5_000_000}} {
		if _, err := svc.Ingest(ctx, u.id, dto.IngestRequest{Machine: "PC", Account: account, Sessions: encode(withHours(u.id[:8],
			dto.HourPayload{Hour: now.Add(-2 * time.Hour), Model: "m", Requests: 1, CacheReadTokens: u.reads}))}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.Ingest(ctx, owner, dto.IngestRequest{Machine: "PC", PlanLimits: weekly(t, 26, now.Add(72*time.Hour), now)}); err != nil {
		t.Fatal(err)
	}
	for _, who := range []string{owner, guest} {
		rate, percent, err := svc.WeekRates(ctx, who)
		if err != nil || rate < 0.99 || rate > 1.01 || percent < 2.16 || percent > 2.17 {
			t.Fatalf("rates for %s = %v %v %v, want 1.00 and 2.17", who, rate, percent, err)
		}
	}
	if rate, percent, err := svc.WeekRates(ctx, loner); err != nil || rate != 0 || percent != 0 {
		t.Fatalf("no reading must give no rate, got %v %v %v", rate, percent, err)
	}
}

func TestHotSessionsFiveHourShareAndCostSplitUseTheWholeAccount(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	owner := testdb.User(t, pool, "owner@test.local", false)
	guest := testdb.User(t, pool, "guest@test.local", false)
	svc := New(repository.New(pool))
	now := time.Now().UTC().Truncate(time.Hour)
	account := json.RawMessage(`{"email":"team@claude.test","tier":"default_claude_max_20x"}`)
	for _, u := range []struct {
		id    string
		hour  time.Time
		reads int64
	}{{owner, now.Add(-time.Hour), 3_000_000}, {guest, now.Add(-30 * time.Hour), 9_000_000}} {
		if _, err := svc.Ingest(ctx, u.id, dto.IngestRequest{Machine: "PC", Account: account, Sessions: encode(withHours(u.id[:8],
			dto.HourPayload{Hour: u.hour, Model: "m", Requests: 1, CacheReadTokens: u.reads}))}); err != nil {
			t.Fatal(err)
		}
	}
	limits, err := json.Marshal(domain.PlanLimits{
		FiveHour:   &domain.PlanWindow{UsedPercentage: 30, ResetsAt: now.Add(3 * time.Hour).Unix()},
		SevenDay:   &domain.PlanWindow{UsedPercentage: 24, ResetsAt: now.Add(72 * time.Hour).Unix()},
		CapturedAt: time.Now().Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Ingest(ctx, owner, dto.IngestRequest{Machine: "PC", PlanLimits: limits}); err != nil {
		t.Fatal(err)
	}

	hot, err := svc.HotSessions(ctx, dto.Caller{IsAdmin: true}, 0)
	if err != nil || len(hot) != 1 || hot[0].SessionID != owner[:8] || hot[0].Share < 5.99 || hot[0].Share > 6.01 {
		t.Fatalf("hot = %+v %v, want the owner's session at 6%%", hot, err)
	}
	if hot, err := svc.HotSessions(ctx, dto.Caller{IsAdmin: true}, 7); err != nil || len(hot) != 0 {
		t.Fatalf("a higher threshold must hide it, got %+v %v", hot, err)
	}
	if hot, err := svc.HotSessions(ctx, dto.Caller{UserID: guest}, 0); err != nil || len(hot) != 0 {
		t.Fatalf("a member only sees their own sessions, got %+v %v", hot, err)
	}

	span := dto.ReportQuery{From: now.Add(-100 * time.Hour), To: now.Add(time.Hour), TZ: "UTC"}
	ov, err := svc.Overview(ctx, dto.Caller{IsAdmin: true}, span)
	if err != nil {
		t.Fatal(err)
	}
	shares := map[string]float64{}
	for _, p := range ov.People {
		if p.UserID != owner && p.UserID != guest {
			continue
		}
		if p.FiveHourShare == nil {
			t.Fatalf("five hour share missing for %s", p.Email)
		}
		shares[p.UserID] = *p.FiveHourShare
	}
	if shares[owner] != 30 || shares[guest] != 0 {
		t.Fatalf("five hour shares = %v, want owner 30 and guest 0", shares)
	}

	costs, err := svc.Costs(ctx, dto.Caller{IsAdmin: true}, span)
	if err != nil || len(costs) != 2 {
		t.Fatalf("costs = %+v %v", costs, err)
	}
	split := map[string]float64{}
	for _, c := range costs {
		split[c.UserID] = c.Cost
	}
	if split[owner] != 50 || split[guest] != 150 {
		t.Fatalf("cost split = %v, want 50 and 150 of the $200 plan", split)
	}
	own, err := svc.Costs(ctx, dto.Caller{UserID: guest}, span)
	if err != nil || len(own) != 1 || own[0].Cost != 150 {
		t.Fatalf("member costs = %+v %v", own, err)
	}
}
