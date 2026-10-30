package authz_test

import (
	"context"
	"slices"
	"testing"

	"claude-usage-server/internal/shared/authz"
)

type fakeFetcher struct {
	perms []string
	calls int
}

func (f *fakeFetcher) GetUserPermissions(ctx context.Context, userID string) ([]string, error) {
	f.calls++
	return f.perms, nil
}

func TestGetPermissionsReadsFetcherEveryCall(t *testing.T) {
	f := &fakeFetcher{perms: []string{"usage:read"}}
	s := authz.NewService(f)
	_, _ = s.GetPermissions(context.Background(), "u1")
	got, _ := s.GetPermissions(context.Background(), "u1")
	if f.calls != 2 || !slices.Equal(got, []string{"usage:read"}) {
		t.Fatalf("calls=%d got=%v", f.calls, got)
	}
}
