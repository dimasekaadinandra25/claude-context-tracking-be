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

func weekly(t *testing.T, used float64, resetsAt, capturedAt time.Time) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(domain.PlanLimits{
		SevenDay:   &domain.PlanWindow{UsedPercentage: used, ResetsAt: resetsAt.Unix()},
		CapturedAt: capturedAt.Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestDailyUsageJoinsTokensWithLimitUsedPerDay(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	a := testdb.User(t, pool, "a@test.local", false)
	svc := New(repository.New(pool))
	send := func(raw json.RawMessage) {
		t.Helper()
		if _, err := svc.Ingest(ctx, a, dto.IngestRequest{Machine: "PC", PlanLimits: raw}); err != nil {
			t.Fatal(err)
		}
	}

	ingest(t, svc, a, withHours("s1",
		dto.HourPayload{Hour: at("2026-10-29T09:00:00Z"), Model: "m", Requests: 3, InputTokens: 10, CacheWriteTokens: 20, CacheReadTokens: 300, OutputTokens: 40},
		dto.HourPayload{Hour: at("2026-10-30T23:00:00Z"), Model: "m", Requests: 1, InputTokens: 1, OutputTokens: 2},
	))
	window := at("2026-11-02T09:00:00Z")
	send(weekly(t, 4, window, at("2026-10-28T20:00:00Z")))
	send(weekly(t, 10, window, at("2026-10-29T08:00:00Z")))
	send(weekly(t, 12, window, at("2026-10-29T22:00:00Z")))
	send(weekly(t, 30, window, at("2026-10-30T22:00:00Z")))
	send(weekly(t, 5, window.Add(7*24*time.Hour), at("2026-10-31T10:00:00Z")))

	days, err := svc.Daily(ctx, dto.Caller{UserID: a}, week())
	if err != nil {
		t.Fatal(err)
	}
	f := func(v float64) *float64 { return &v }
	want := []dto.DailyUsage{
		{UserID: a, Day: "2026-10-29", InputTokens: 10, CacheWriteTokens: 20, CacheReadTokens: 300, OutputTokens: 40, Requests: 3, SevenDayUsed: f(12), LimitUsed: f(8)},
		{UserID: a, Day: "2026-10-30", InputTokens: 1, OutputTokens: 2, Requests: 1, SevenDayUsed: f(30), LimitUsed: f(18)},
		{UserID: a, Day: "2026-10-31", SevenDayUsed: f(5), LimitUsed: f(5)},
	}
	if len(days) != len(want) {
		t.Fatalf("days = %+v", days)
	}
	for i := range want {
		got, w := days[i], want[i]
		if got.Day != w.Day || got.InputTokens != w.InputTokens || got.CacheWriteTokens != w.CacheWriteTokens ||
			got.CacheReadTokens != w.CacheReadTokens || got.OutputTokens != w.OutputTokens || got.Requests != w.Requests ||
			*got.SevenDayUsed != *w.SevenDayUsed || *got.LimitUsed != *w.LimitUsed {
			t.Fatalf("day %d = %+v used %v delta %v, want %+v", i, got, *got.SevenDayUsed, *got.LimitUsed, w)
		}
	}

	q := week()
	q.TZ = "Asia/Jakarta"
	days, err = svc.Daily(ctx, dto.Caller{UserID: a}, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 3 || days[0].Day != "2026-10-29" || *days[0].LimitUsed != 6 || days[1].Day != "2026-10-30" ||
		*days[1].LimitUsed != 2 || days[2].Day != "2026-10-31" || days[2].OutputTokens != 2 || *days[2].LimitUsed != 23 || *days[2].SevenDayUsed != 5 {
		t.Fatalf("local days = %+v", days)
	}
}

func TestClaudeAccountIsReportedAndBadOnesIgnored(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	a := testdb.User(t, pool, "a@test.local", false)
	svc := New(repository.New(pool))
	send := func(raw string) {
		t.Helper()
		res, err := svc.Ingest(ctx, a, dto.IngestRequest{Machine: "PC", Sessions: encode(payload("s1")), Account: json.RawMessage(raw)})
		if err != nil || res.Accepted != 1 {
			t.Fatalf("%s: res=%+v err=%v", raw, res, err)
		}
	}
	person := func() dto.PersonSummary {
		t.Helper()
		ov, err := svc.Overview(ctx, dto.Caller{UserID: a}, week())
		if err != nil {
			t.Fatal(err)
		}
		return ov.People[0]
	}

	send(`"oops"`)
	send(`{"email":"","plan":"Max","tier":""}`)
	if p := person(); p.ClaudeEmail != nil || p.ClaudeTier != nil {
		t.Fatalf("bad account stored: %+v", p)
	}
	send(`{"email":" me@claude.test ","plan":"Max","tier":"default_claude_max_20x"}`)
	p := person()
	if p.ClaudeEmail == nil || *p.ClaudeEmail != "me@claude.test" || *p.ClaudePlan != "Max" || *p.ClaudeTier != "default_claude_max_20x" {
		t.Fatalf("account = %+v", p)
	}
	long := make([]byte, 101)
	for i := range long {
		long[i] = 'x'
	}
	send(`{"email":"me@claude.test","plan":"` + string(long) + `","tier":"t"}`)
	if p := person(); *p.ClaudePlan != "Max" {
		t.Fatalf("overlong plan stored: %v", *p.ClaudePlan)
	}
}

func TestFirstReadingCountsOnlyNearTheWindowStart(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	early := testdb.User(t, pool, "early@test.local", false)
	late := testdb.User(t, pool, "late@test.local", false)
	svc := New(repository.New(pool))
	window := at("2026-11-05T09:00:00Z")
	for user, capturedAt := range map[string]time.Time{
		early: at("2026-10-29T15:00:00Z"),
		late:  at("2026-10-31T15:00:00Z"),
	} {
		if _, err := svc.Ingest(ctx, user, dto.IngestRequest{Machine: "PC", PlanLimits: weekly(t, 20, window, capturedAt)}); err != nil {
			t.Fatal(err)
		}
	}

	for user, want := range map[string]*float64{early: func() *float64 { v := 20.0; return &v }(), late: nil} {
		days, err := svc.Daily(ctx, dto.Caller{UserID: user}, week())
		if err != nil || len(days) != 1 {
			t.Fatalf("days = %+v err = %v", days, err)
		}
		got := days[0].LimitUsed
		if (want == nil) != (got == nil) || (want != nil && *got != *want) {
			t.Fatalf("%s limit used = %v, want %v", days[0].Day, got, want)
		}
	}
}

func TestSharedClaudeAccountMergesReadingsAndReportsItsWeight(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	owner := testdb.User(t, pool, "owner@test.local", false)
	guest := testdb.User(t, pool, "guest@test.local", false)
	solo := testdb.User(t, pool, "solo@test.local", false)
	svc := New(repository.New(pool))
	send := func(user string, req dto.IngestRequest) {
		t.Helper()
		req.Machine = "PC"
		if _, err := svc.Ingest(ctx, user, req); err != nil {
			t.Fatal(err)
		}
	}
	shared := json.RawMessage(`{"email":"Team@Claude.test","plan":"Max","tier":"default_claude_max_20x"}`)
	send(owner, dto.IngestRequest{Account: shared, Sessions: encode(withHours("o1",
		dto.HourPayload{Hour: at("2026-10-30T02:00:00Z"), Model: "m", Requests: 1, CacheReadTokens: 900, OutputTokens: 20}))})
	send(guest, dto.IngestRequest{Account: json.RawMessage(`{"email":"team@claude.test","plan":"Max","tier":"default_claude_max_20x"}`),
		Sessions: encode(withHours("g1", dto.HourPayload{Hour: at("2026-10-30T03:00:00Z"), Model: "m", Requests: 1, InputTokens: 100}))})
	send(solo, dto.IngestRequest{Account: json.RawMessage(`{"email":"solo@claude.test","tier":"default_claude_pro"}`)})

	window := at("2026-11-02T09:00:00Z")
	send(owner, dto.IngestRequest{PlanLimits: weekly(t, 10, window, at("2026-10-29T12:00:00Z"))})
	send(guest, dto.IngestRequest{PlanLimits: weekly(t, 14, window, at("2026-10-30T12:00:00Z"))})
	send(owner, dto.IngestRequest{PlanLimits: weekly(t, 20, window, at("2026-10-30T20:00:00Z"))})

	days, err := svc.Daily(ctx, dto.Caller{IsAdmin: true}, week())
	if err != nil {
		t.Fatal(err)
	}
	byUser := map[string][]dto.DailyUsage{}
	for _, d := range days {
		byUser[d.UserID] = append(byUser[d.UserID], d)
	}
	if len(byUser[solo]) != 0 {
		t.Fatalf("solo has no usage, got %+v", byUser[solo])
	}
	for _, user := range []string{owner, guest} {
		rows := byUser[user]
		if len(rows) != 2 || rows[1].Day != "2026-10-30" {
			t.Fatalf("%s rows = %+v", user, rows)
		}
		day := rows[1]
		if *day.LimitUsed != 10 || *day.SevenDayUsed != 20 || day.AccountWeight != 1100 || day.AccountMembers != 2 {
			t.Fatalf("%s day = %+v used %v delta %v", user, day, *day.SevenDayUsed, *day.LimitUsed)
		}
	}

	own, err := svc.Daily(ctx, dto.Caller{UserID: guest}, week())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range own {
		if d.UserID != guest {
			t.Fatalf("member saw another member's row: %+v", d)
		}
	}

	ov, err := svc.Overview(ctx, dto.Caller{UserID: guest}, week())
	if err != nil {
		t.Fatal(err)
	}
	if p := ov.People[0]; p.LimitsCapturedAt == nil || !p.LimitsCapturedAt.Equal(at("2026-10-30T20:00:00Z")) {
		t.Fatalf("guest should see the account's latest limits: %+v", p)
	}
}

func TestModelsReportInputAndCacheTokens(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	a := testdb.User(t, pool, "a@test.local", false)
	svc := New(repository.New(pool))
	ingest(t, svc, a, withHours("s1",
		dto.HourPayload{Hour: at("2026-10-30T02:00:00Z"), Model: "claude-haiku-5-5", Requests: 2, InputTokens: 5, CacheWriteTokens: 40, CacheReadTokens: 300, OutputTokens: 9},
		dto.HourPayload{Hour: at("2026-10-30T03:00:00Z"), Model: "claude-haiku-5-5", Requests: 1, InputTokens: 1, CacheReadTokens: 100},
	))

	models, err := svc.Models(ctx, dto.Caller{UserID: a}, week())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].InputTokens != 6 || models[0].CacheWriteTokens != 40 || models[0].CacheReadTokens != 400 || models[0].OutputTokens != 9 {
		t.Fatalf("models = %+v", models)
	}
}
