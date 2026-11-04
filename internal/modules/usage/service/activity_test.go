package service

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"claude-usage-server/internal/modules/usage/domain"
	"claude-usage-server/internal/modules/usage/dto"
	"claude-usage-server/internal/modules/usage/repository"
	"claude-usage-server/internal/testutil/testdb"
)

func withTools(id string, tools ...dto.ToolPayload) dto.SessionPayload {
	p := payload(id)
	p.Tools = tools
	return p
}

func tool(hour, name string, calls int32, added, removed int64, commits int32) dto.ToolPayload {
	return dto.ToolPayload{Hour: at(hour), Tool: name, Calls: calls, LinesAdded: added, LinesRemoved: removed, Commits: commits}
}

func TestIngestStoresToolActivityAndReportsIt(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	a := testdb.User(t, pool, "a@test.local", false)
	svc := New(repository.New(pool))

	ingest(t, svc, a, withTools("s1",
		tool("2026-10-30T02:00:00Z", "Edit", 4, 30, 10, 0),
		tool("2026-10-30T02:00:00Z", "Bash", 6, 0, 0, 2),
		tool("2026-10-30T03:00:00Z", "Edit", 1, 5, 0, 0),
	))
	caller := dto.Caller{UserID: a}

	ov, err := svc.Overview(ctx, caller, week())
	if err != nil {
		t.Fatal(err)
	}
	if ov.TotalLinesAdded != 35 || ov.TotalLinesRemoved != 10 || ov.TotalCommits != 2 {
		t.Fatalf("totals = +%d -%d commits %d, want +35 -10 commits 2", ov.TotalLinesAdded, ov.TotalLinesRemoved, ov.TotalCommits)
	}
	p := ov.People[0]
	if p.LinesAdded != 35 || p.LinesRemoved != 10 || p.Commits != 2 {
		t.Fatalf("person = %+v", p)
	}

	want := []dto.ToolSummary{
		{Tool: "Bash", Calls: 6, Commits: 2},
		{Tool: "Edit", Calls: 5, LinesAdded: 35, LinesRemoved: 10},
	}
	tools, err := svc.Tools(ctx, caller, week())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tools, want) {
		t.Fatalf("tools = %+v, want %+v", tools, want)
	}
	detail, err := svc.Session(ctx, caller, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(detail.Tools, want) {
		t.Fatalf("session tools = %+v, want %+v", detail.Tools, want)
	}

	narrow := week()
	narrow.From, narrow.To = at("2026-10-30T03:00:00Z"), at("2026-10-30T04:00:00Z")
	tools, err = svc.Tools(ctx, caller, narrow)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0].Calls != 1 {
		t.Fatalf("range filter ignored: %+v", tools)
	}
}

func TestIngestToolRowsReplaceButOldAppsKeepThem(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	a := testdb.User(t, pool, "a@test.local", false)
	svc := New(repository.New(pool))
	caller := dto.Caller{UserID: a}
	calls := func() int64 {
		t.Helper()
		tools, err := svc.Tools(ctx, caller, week())
		if err != nil {
			t.Fatal(err)
		}
		var total int64
		for _, row := range tools {
			total += row.Calls
		}
		return total
	}

	ingest(t, svc, a, withTools("s1", tool("2026-10-30T02:00:00Z", "Edit", 4, 1, 1, 0)))
	ingest(t, svc, a, payload("s1"))
	if got := calls(); got != 4 {
		t.Fatalf("calls after old app sync = %d, want 4 kept", got)
	}
	ingest(t, svc, a, withTools("s1", tool("2026-10-30T02:00:00Z", "Edit", 9, 1, 1, 0)))
	if got := calls(); got != 9 {
		t.Fatalf("calls after resend = %d, want 9 not doubled", got)
	}
	ingest(t, svc, a, withTools("s1", []dto.ToolPayload{}...))
	if got := calls(); got != 0 {
		t.Fatalf("empty tools must clear rows, got %d", got)
	}
}

func TestValidateRejectsBadTools(t *testing.T) {
	const hour = "2026-10-30T02:00:00Z"
	for name, tools := range map[string][]dto.ToolPayload{
		"missing name":  {tool(hour, "", 1, 0, 0, 0)},
		"negative":      {tool(hour, "Edit", -1, 0, 0, 0)},
		"partial hour":  {tool("2026-10-30T02:30:00Z", "Edit", 1, 0, 0, 0)},
		"duplicate key": {tool(hour, "Edit", 1, 0, 0, 0), tool(hour, "Edit", 1, 0, 0, 0)},
		"nul byte":      {tool(hour, "Ed\x00it", 1, 0, 0, 0)},
	} {
		if err := validate(withTools("s1", tools...)); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	if err := validate(withTools("s1", tool(hour, "Edit", 1, 2, 3, 4), tool(hour, "Bash", 1, 0, 0, 1))); err != nil {
		t.Fatalf("valid tools rejected: %v", err)
	}
}

func limits(t *testing.T, used float64, resetsAt, capturedAt time.Time) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(domain.PlanLimits{
		FiveHour:   &domain.PlanWindow{UsedPercentage: used, ResetsAt: resetsAt.Unix()},
		SevenDay:   &domain.PlanWindow{UsedPercentage: used + 1, ResetsAt: resetsAt.Unix()},
		CapturedAt: capturedAt.Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestPlanLimitsKeepNewestAndHideExpiredWindows(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	a := testdb.User(t, pool, "a@test.local", false)
	svc := New(repository.New(pool))
	now := time.Now().Truncate(time.Second)
	send := func(raw json.RawMessage) {
		t.Helper()
		if _, err := svc.Ingest(ctx, a, dto.IngestRequest{Machine: "PC", PlanLimits: raw}); err != nil {
			t.Fatal(err)
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

	if p := person(); p.FiveHourUsed != nil || p.LimitsCapturedAt != nil {
		t.Fatalf("no limits yet, got %+v", p)
	}
	send(limits(t, 40, now.Add(time.Hour), now))
	p := person()
	if p.FiveHourUsed == nil || *p.FiveHourUsed != 40 || p.SevenDayUsed == nil || *p.SevenDayUsed != 41 {
		t.Fatalf("stored limits = %+v", p)
	}
	if p.FiveHourResetsAt == nil || !p.FiveHourResetsAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("resets at = %v", p.FiveHourResetsAt)
	}

	send(limits(t, 10, now.Add(time.Hour), now.Add(-time.Minute)))
	if p := person(); *p.FiveHourUsed != 40 {
		t.Fatalf("older machine overwrote newer limits: %v", *p.FiveHourUsed)
	}

	send(limits(t, 70, now.Add(-time.Minute), now.Add(time.Minute)))
	if p := person(); p.FiveHourUsed != nil || p.SevenDayUsed != nil || p.LimitsCapturedAt == nil {
		t.Fatalf("expired windows should read as nil: %+v", p)
	}
}

func TestBadPlanLimitsAreIgnoredWithoutFailingTheSync(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	a := testdb.User(t, pool, "a@test.local", false)
	svc := New(repository.New(pool))
	now := time.Now()

	for name, raw := range map[string]json.RawMessage{
		"not an object":  json.RawMessage(`"oops"`),
		"over 100":       limits(t, 150, now.Add(time.Hour), now),
		"negative":       limits(t, -1, now.Add(time.Hour), now),
		"no capture":     limits(t, 10, now.Add(time.Hour), time.Unix(0, 0)),
		"future capture": limits(t, 10, now.Add(time.Hour), now.Add(48*time.Hour)),
		"no windows":     json.RawMessage(`{"captured_at":1}`),
	} {
		res, err := svc.Ingest(ctx, a, dto.IngestRequest{Machine: "PC", Sessions: encode(payload("s1")), PlanLimits: raw})
		if err != nil || res.Accepted != 1 {
			t.Fatalf("%s: res=%+v err=%v", name, res, err)
		}
	}
	ov, err := svc.Overview(ctx, dto.Caller{UserID: a}, week())
	if err != nil {
		t.Fatal(err)
	}
	if p := ov.People[0]; p.LimitsCapturedAt != nil {
		t.Fatalf("bad limits were stored: %+v", p)
	}
}
