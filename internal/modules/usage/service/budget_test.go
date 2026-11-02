package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"claude-usage-server/internal/modules/usage/dto"
	"claude-usage-server/internal/modules/usage/repository"
	"claude-usage-server/internal/testutil/testdb"
)

func TestBudgetUsedFollowsMonthAndTimeZone(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	a := testdb.User(t, pool, "a@test.local", false)
	svc := New(repository.New(pool))
	ingest(t, svc, a, withHours("s",
		dto.HourPayload{Hour: at("2026-10-31T16:00:00Z"), Model: "claude-opus-5-5", NewContext: 5, Requests: 1},
		dto.HourPayload{Hour: at("2026-10-31T17:00:00Z"), Model: "claude-opus-5-5", NewContext: 7, Requests: 1},
		dto.HourPayload{Hour: at("2026-11-01T00:00:00Z"), Model: "claude-opus-5-5", NewContext: 11, Requests: 1},
		dto.HourPayload{Hour: at("2026-11-20T00:00:00Z"), Model: "claude-opus-5-5", NewContext: 13, Requests: 1},
	))
	now := at("2026-11-15T12:30:00Z")

	for tz, want := range map[string]struct {
		used  int64
		start string
	}{
		"":             {11, "2026-11-01T00:00:00Z"},
		"UTC":          {11, "2026-11-01T00:00:00Z"},
		"Asia/Jakarta": {18, "2026-11-01T00:00:00+07:00"},
	} {
		list, err := svc.budgets(ctx, dto.Caller{UserID: a}, tz, now)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 1 || list[0].UsedNewContext != want.used || !list[0].MonthStart.Equal(at(want.start)) || list[0].MonthlyNewContext != nil {
			t.Fatalf("tz %q: %+v, want used %d from %s and no budget", tz, list, want.used, want.start)
		}
	}

	if _, err := svc.budgets(ctx, dto.Caller{UserID: a}, "Mars/Base", now); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Mars/Base err = %v, want ErrInvalidInput", err)
	}
}

func TestBudgetsScopeAndChanges(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	a := testdb.User(t, pool, "a@test.local", false)
	b := testdb.User(t, pool, "b@test.local", false)
	gone := testdb.User(t, pool, "gone@test.local", false)
	if _, err := pool.Exec(ctx, `UPDATE core.users SET is_active = FALSE WHERE id = $1`, gone); err != nil {
		t.Fatal(err)
	}
	admin := dto.Caller{UserID: testdb.User(t, pool, "boss@test.local", true), IsAdmin: true}
	member := dto.Caller{UserID: a}
	svc := New(repository.New(pool))

	budgetOf := func(c dto.Caller) map[string]*int64 {
		t.Helper()
		list, err := svc.Budgets(ctx, c, "")
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]*int64{}
		for _, row := range list {
			got[row.UserID] = row.MonthlyNewContext
		}
		return got
	}

	if err := svc.SetBudget(ctx, admin, a, 1000); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetBudget(ctx, admin, a, 2000); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetBudget(ctx, admin, b, 500); err != nil {
		t.Fatal(err)
	}

	mine := budgetOf(member)
	if len(mine) != 1 || mine[a] == nil || *mine[a] != 2000 {
		t.Fatalf("member sees %v, want only own budget 2000", mine)
	}
	all := budgetOf(admin)
	if _, ok := all[gone]; ok || all[b] == nil || *all[b] != 500 || all[admin.UserID] != nil {
		t.Fatalf("admin sees %v, want b 500, admin without budget, no inactive user", all)
	}

	if err := svc.SetBudget(ctx, member, a, 9999); !errors.Is(err, ErrForbidden) {
		t.Fatalf("member set err = %v, want ErrForbidden", err)
	}
	if err := svc.DeleteBudget(ctx, member, a); !errors.Is(err, ErrForbidden) {
		t.Fatalf("member delete err = %v, want ErrForbidden", err)
	}
	if err := svc.SetBudget(ctx, admin, uuid.NewString(), 1000); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("unknown user set err = %v, want ErrUserNotFound", err)
	}
	if err := svc.DeleteBudget(ctx, admin, uuid.NewString()); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("unknown user delete err = %v, want ErrUserNotFound", err)
	}

	if err := svc.DeleteBudget(ctx, admin, a); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteBudget(ctx, admin, a); err != nil {
		t.Fatalf("second delete err = %v", err)
	}
	if got := budgetOf(member); got[a] != nil {
		t.Fatalf("budget after delete = %d, want none", *got[a])
	}
}
