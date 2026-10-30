package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	apiKeyRepository "claude-usage-server/internal/modules/core/api_key/repository"
	apiKeyService "claude-usage-server/internal/modules/core/api_key/service"
	"claude-usage-server/internal/modules/core/user/dto"
	"claude-usage-server/internal/modules/core/user/repository"
	"claude-usage-server/internal/modules/usage/testdb"
	"claude-usage-server/pkg/crypto"
)

func TestInactiveUserKeyRejected(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	admin := testdb.User(t, pool, "boss@x.test", true)
	member := testdb.User(t, pool, "m@x.test", false)
	other := testdb.User(t, pool, "o@x.test", false)

	keys := apiKeyService.NewApiKeyService(apiKeyRepository.NewApiKeyRepository(pool), nil)
	users := NewUserService(repository.NewUserRepository(pool))
	users.SetKeyRevoker(keys)

	first, _, err := keys.CreateForUser(ctx, admin, member, "desktop")
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := keys.CreateForUser(ctx, admin, member, "laptop")
	if err != nil {
		t.Fatal(err)
	}
	otherKey, _, err := keys.CreateForUser(ctx, admin, other, "desktop")
	if err != nil {
		t.Fatal(err)
	}

	if err := users.SetActive(ctx, admin, member, false); err != nil {
		t.Fatal(err)
	}

	var revoked int
	var reason string
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*), MIN(revoke_reason) FROM core.api_keys
		WHERE user_id = $1 AND revoked_at IS NOT NULL AND revoked_by = $2`, member, admin).Scan(&revoked, &reason); err != nil {
		t.Fatal(err)
	}
	if revoked != 2 || reason != "user deactivated" {
		t.Fatalf("revoked %d reason %q", revoked, reason)
	}

	for _, k := range []string{first, second} {
		parts := strings.Split(k, "_")
		if _, err := keys.ValidateAndGetClaims(ctx, parts[2], parts[3], "127.0.0.1"); err == nil {
			t.Fatal("deactivated user's key still authenticates")
		}
	}

	parts := strings.Split(otherKey, "_")
	if _, err := keys.ValidateAndGetClaims(ctx, parts[2], parts[3], "127.0.0.1"); err != nil {
		t.Fatalf("other user's key rejected: %v", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE core.users SET is_active = FALSE WHERE id = $1`, other); err != nil {
		t.Fatal(err)
	}
	_, err = keys.ValidateAndGetClaims(ctx, parts[2], parts[3], "127.0.0.1")
	if !errors.Is(err, apiKeyService.ErrApiKeyUserInactive) || err.Error() != "user is inactive" {
		t.Fatalf("unrevoked key of inactive user: got %v", err)
	}
}

func TestResetPasswordAllowsSignin(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	admin := testdb.User(t, pool, "boss@x.test", true)
	member := testdb.User(t, pool, "m@x.test", false)
	users := NewUserService(repository.NewUserRepository(pool))

	if err := users.ResetPassword(ctx, admin, member, "NewPass123!"); err != nil {
		t.Fatal(err)
	}

	var hash string
	if err := pool.QueryRow(ctx, `SELECT password_hash FROM core.users WHERE id = $1`, member).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if !crypto.ComparePassword(hash, "NewPass123!") {
		t.Fatal("new password does not match stored hash")
	}

	if err := users.ResetPassword(ctx, admin, "00000000-0000-0000-0000-0000000000ff", "NewPass123!"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("unknown user: got %v", err)
	}
}

func TestCreateMemberAssignsMemberRole(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	admin := testdb.User(t, pool, "boss@x.test", true)
	users := NewUserService(repository.NewUserRepository(pool))

	name := "Mia"
	created, err := users.CreateMember(ctx, &dto.CreateUserRequest{Email: "mia@x.test", Username: "mia", Password: "Secret123!", FullName: &name}, admin)
	if err != nil {
		t.Fatal(err)
	}

	list, err := users.GetAll(ctx, &dto.UserQueryParams{Page: 1, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range list.Users {
		if u.ID == created.ID {
			if len(u.Roles) != 1 || u.Roles[0] != "member" || !u.IsActive {
				t.Fatalf("created user %+v", u)
			}
			return
		}
	}
	t.Fatal("created user not listed")
}

func TestResetPasswordRevokesRefreshTokens(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	admin := testdb.User(t, pool, "boss@x.test", true)
	member := testdb.User(t, pool, "m@x.test", false)
	other := testdb.User(t, pool, "o@x.test", false)
	users := NewUserService(repository.NewUserRepository(pool))

	if _, err := pool.Exec(ctx, `
		INSERT INTO core.refresh_tokens (user_id, token_hash, expires_at)
		VALUES ($1, 'member-1', NOW() + INTERVAL '1 day'), ($1, 'member-2', NOW() + INTERVAL '1 day'), ($2, 'other-1', NOW() + INTERVAL '1 day')`, member, other); err != nil {
		t.Fatal(err)
	}

	if err := users.ResetPassword(ctx, admin, member, "NewPass123!"); err != nil {
		t.Fatal(err)
	}

	var memberLive, otherLive int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE user_id = $1 AND revoked_at IS NULL),
		       COUNT(*) FILTER (WHERE user_id = $2 AND revoked_at IS NULL)
		FROM core.refresh_tokens`, member, other).Scan(&memberLive, &otherLive); err != nil {
		t.Fatal(err)
	}
	if memberLive != 0 || otherLive != 1 {
		t.Fatalf("live tokens member=%d other=%d, want 0 and 1", memberLive, otherLive)
	}
}
