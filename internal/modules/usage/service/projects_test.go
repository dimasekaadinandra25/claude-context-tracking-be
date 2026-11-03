package service

import (
	"context"
	"reflect"
	"testing"

	"claude-usage-server/internal/modules/usage/domain"
	"claude-usage-server/internal/modules/usage/dto"
	"claude-usage-server/internal/modules/usage/repository"
	"claude-usage-server/internal/testutil/testdb"
)

func detailOverhead(t *testing.T, svc *Service, userID, id string) *domain.Overhead {
	t.Helper()
	detail, err := svc.Session(context.Background(), dto.Caller{UserID: userID}, id)
	if err != nil {
		t.Fatal(err)
	}
	return detail.Overhead
}

func TestIngestStoresOverhead(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	a := testdb.User(t, pool, "a@test.local", false)
	svc := New(repository.New(pool))

	first := payload("s1")
	first.Overhead = &domain.Overhead{SystemTokens: 100, MemoryTokens: 200, AgentsTokens: 300, SkillsTokens: 400}
	ingest(t, svc, a, first, payload("bare"))
	if got := detailOverhead(t, svc, a, "s1"); !reflect.DeepEqual(got, first.Overhead) {
		t.Fatalf("stored overhead = %+v, want %+v", got, first.Overhead)
	}
	if got := detailOverhead(t, svc, a, "bare"); got != nil {
		t.Fatalf("bare overhead = %+v, want nil", got)
	}

	updated := payload("s1")
	updated.Overhead = &domain.Overhead{SystemTokens: 1, MemoryTokens: 2, AgentsTokens: 3, SkillsTokens: 4}
	ingest(t, svc, a, updated)
	if got := detailOverhead(t, svc, a, "s1"); !reflect.DeepEqual(got, updated.Overhead) {
		t.Fatalf("updated overhead = %+v, want %+v", got, updated.Overhead)
	}

	ingest(t, svc, a, payload("s1"))
	if got := detailOverhead(t, svc, a, "s1"); !reflect.DeepEqual(got, updated.Overhead) {
		t.Fatalf("overhead after omitted = %+v, want unchanged %+v", got, updated.Overhead)
	}

	bad := payload("bad")
	bad.Overhead = &domain.Overhead{MemoryTokens: -1}
	res, err := svc.Ingest(ctx, a, dto.IngestRequest{Machine: "PC", Sessions: encode(bad, payload("ok"))})
	if err != nil {
		t.Fatal(err)
	}
	want := []dto.Rejection{{SessionID: "bad", Reason: "numbers must be >= 0"}}
	if res.Accepted != 1 || !reflect.DeepEqual(res.Rejected, want) {
		t.Fatalf("got %+v, want ok accepted and %v", res, want)
	}
}

func TestOverviewBilledTokensNearFullAndCompactions(t *testing.T) {
	pool := testdb.Open(t)
	a := testdb.User(t, pool, "a@test.local", false)
	b := testdb.User(t, pool, "b@test.local", false)
	admin := dto.Caller{UserID: testdb.User(t, pool, "boss@test.local", true), IsAdmin: true}
	svc := New(repository.New(pool))

	full := withHours("full", dto.HourPayload{Hour: at("2026-10-30T03:00:00Z"), Model: "m", InputTokens: 10, CacheWriteTokens: 20, CacheReadTokens: 30})
	full.PeakContext, full.Compactions = 900000, 2
	roomy := withHours("roomy", dto.HourPayload{Hour: at("2026-10-30T04:00:00Z"), Model: "m", InputTokens: 1, CacheWriteTokens: 2, CacheReadTokens: 3})
	noWindow := withHours("no-window", dto.HourPayload{Hour: at("2026-10-30T05:00:00Z"), Model: "m", NewContext: 1})
	noWindow.ContextWindow, noWindow.PeakContext, noWindow.Compactions = 0, 5, 0
	old := withHours("old", dto.HourPayload{Hour: at("2026-10-20T03:00:00Z"), Model: "m", InputTokens: 1000})
	old.PeakContext, old.Compactions = 950000, 5
	ingest(t, svc, a, full, roomy, noWindow, old)
	edge := withHours("edge", dto.HourPayload{Hour: at("2026-10-30T03:00:00Z"), Model: "m", InputTokens: 100, CacheWriteTokens: 200, CacheReadTokens: 300})
	edge.PeakContext, edge.Compactions = 800000, 3
	ingest(t, svc, b, edge)

	ov, err := svc.Overview(context.Background(), admin, week())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][5]int64{}
	for _, p := range ov.People {
		got[p.UserID] = [5]int64{p.InputTokens, p.CacheWriteTokens, p.CacheReadTokens, p.SessionsNearFull, p.Compactions}
	}
	if got[a] != [5]int64{11, 22, 33, 1, 3} || got[b] != [5]int64{100, 200, 300, 1, 3} {
		t.Fatalf("per person = %v, want A 11/22/33/1/3 and B 100/200/300/1/3", got)
	}
	totals := [5]int64{ov.TotalInputTokens, ov.TotalCacheWriteTokens, ov.TotalCacheReadTokens, ov.TotalSessionsNearFull, ov.TotalCompactions}
	if totals != [5]int64{111, 222, 333, 2, 6} {
		t.Fatalf("totals = %v, want 111/222/333/2/6", totals)
	}
}

func TestSessionsListBilledTokensInRange(t *testing.T) {
	pool := testdb.Open(t)
	a := testdb.User(t, pool, "a@test.local", false)
	svc := New(repository.New(pool))
	ingest(t, svc, a, withHours("spans",
		dto.HourPayload{Hour: at("2026-10-20T03:00:00Z"), Model: "m", NewContext: 1, InputTokens: 1000, CacheWriteTokens: 2000, CacheReadTokens: 3000},
		dto.HourPayload{Hour: at("2026-10-30T03:00:00Z"), Model: "m", NewContext: 1, InputTokens: 4, CacheWriteTokens: 5, CacheReadTokens: 6},
		dto.HourPayload{Hour: at("2026-10-30T04:00:00Z"), Model: "n", NewContext: 1, InputTokens: 10, CacheWriteTokens: 20, CacheReadTokens: 30}))

	rows, _, err := svc.Sessions(context.Background(), dto.Caller{UserID: a}, dto.SessionQuery{ReportQuery: week()})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].InputTokens != 14 || rows[0].CacheWriteTokens != 25 || rows[0].CacheReadTokens != 36 {
		t.Fatalf("rows = %+v, want 14/25/36 in range", rows)
	}
}

func project(id, name string, hours ...dto.HourPayload) dto.SessionPayload {
	p := withHours(id, hours...)
	p.Project = name
	p.PeakContext = 100
	return p
}

func TestProjects(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	a := testdb.User(t, pool, "a@test.local", false)
	b := testdb.User(t, pool, "b@test.local", false)
	admin := dto.Caller{UserID: testdb.User(t, pool, "boss@test.local", true), IsAdmin: true}
	svc := New(repository.New(pool))

	aOverhead := &domain.Overhead{SystemTokens: 1, MemoryTokens: 2, AgentsTokens: 3, SkillsTokens: 4}
	bOverhead := &domain.Overhead{SystemTokens: 5, MemoryTokens: 6, AgentsTokens: 7, SkillsTokens: 8}
	p1a := project("p1-a", "P1",
		dto.HourPayload{Hour: at("2026-10-30T01:00:00Z"), Model: "opus", NewContext: 1000, Requests: 1, OutputTokens: 10, InputTokens: 1, CacheWriteTokens: 2, CacheReadTokens: 3},
		dto.HourPayload{Hour: at("2026-10-30T01:00:00Z"), Model: "sonnet", NewContext: 3000, Requests: 2})
	p1a.PeakContext, p1a.Compactions, p1a.Overhead = 900000, 1, aOverhead
	p1b := project("p1-b", "P1", dto.HourPayload{Hour: at("2026-10-30T02:00:00Z"), Model: "opus", NewContext: 500, Requests: 3, OutputTokens: 5})
	p1b.Compactions, p1b.Overhead = 2, bOverhead
	p1Latest := project("p1-latest", "P1", dto.HourPayload{Hour: at("2026-10-30T05:00:00Z"), Model: "haiku", NewContext: 100})
	p1Old := project("p1-old", "P1", dto.HourPayload{Hour: at("2026-10-20T01:00:00Z"), Model: "haiku", NewContext: 99999})
	private := project("private", "", dto.HourPayload{Hour: at("2026-10-30T01:00:00Z"), Model: "opus", NewContext: 200})
	p2 := project("p2", "P2", dto.HourPayload{Hour: at("2026-10-30T01:00:00Z"), Model: "opus", NewContext: 10000})
	ingest(t, svc, a, p1a, p1Latest, p1Old, private)
	ingest(t, svc, b, p1b, p2)

	all, err := svc.Projects(ctx, admin, week())
	if err != nil {
		t.Fatal(err)
	}
	want := []dto.ProjectSummary{
		{Project: "P2", NewContext: 10000, Sessions: 1, Members: 1, Models: []string{"opus"}, Compactions: 1},
		{Project: "P1", NewContext: 4600, Requests: 6, OutputTokens: 15, InputTokens: 1, CacheWriteTokens: 2, CacheReadTokens: 3,
			Sessions: 3, Members: 2, Models: []string{"sonnet", "opus", "haiku"}, Compactions: 4, SessionsNearFull: 1, Overhead: bOverhead},
		{Project: "", NewContext: 200, Sessions: 1, Members: 1, Models: []string{"opus"}, Compactions: 1},
	}
	if !reflect.DeepEqual(all, want) {
		t.Fatalf("admin projects =\n%+v\nwant\n%+v", all, want)
	}

	q := week()
	q.UserID = b
	mine, err := svc.Projects(ctx, dto.Caller{UserID: a}, q)
	if err != nil {
		t.Fatal(err)
	}
	want = []dto.ProjectSummary{
		{Project: "P1", NewContext: 4100, Requests: 3, OutputTokens: 10, InputTokens: 1, CacheWriteTokens: 2, CacheReadTokens: 3,
			Sessions: 2, Members: 1, Models: []string{"sonnet", "opus", "haiku"}, Compactions: 2, SessionsNearFull: 1, Overhead: aOverhead},
		{Project: "", NewContext: 200, Sessions: 1, Members: 1, Models: []string{"opus"}, Compactions: 1},
	}
	if !reflect.DeepEqual(mine, want) {
		t.Fatalf("member projects =\n%+v\nwant\n%+v", mine, want)
	}
}
