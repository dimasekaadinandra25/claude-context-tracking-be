package service

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"claude-usage-server/internal/modules/core/auth/domain"
	"claude-usage-server/internal/modules/core/auth/dto"
	"claude-usage-server/internal/modules/core/auth/repository"
	roleRepository "claude-usage-server/internal/modules/core/role/repository"
	userRepo "claude-usage-server/internal/modules/core/user/repository"
	"claude-usage-server/internal/shared/authz"
	"claude-usage-server/internal/testutil/testdb"
	"claude-usage-server/pkg/crypto"
)

func newAuth(pool *pgxpool.Pool) *AuthService {
	roles := roleRepository.NewRoleRepository(pool)
	return NewAuthService(userRepo.NewUserRepository(pool), repository.NewRefreshTokenRepository(pool), roles, authz.NewService(roles))
}

func setPassword(t *testing.T, pool *pgxpool.Pool, id, password string, mustChange bool) {
	t.Helper()
	hash, err := crypto.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE core.users SET password_hash = $2, must_change_password = $3 WHERE id = $1`, id, hash, mustChange); err != nil {
		t.Fatal(err)
	}
}

func TestChangePasswordClearsFlag(t *testing.T) {
	pool := testdb.Open(t)
	member := testdb.User(t, pool, "m@x.test", false)
	setPassword(t, pool, member, "OldPass123!", true)

	if err := newAuth(pool).ChangePassword(context.Background(), member, &dto.ChangePasswordRequest{CurrentPassword: "OldPass123!", NewPassword: "NewPass456!"}); err != nil {
		t.Fatal(err)
	}
	var flag bool
	if err := pool.QueryRow(context.Background(), `SELECT must_change_password FROM core.users WHERE id = $1`, member).Scan(&flag); err != nil {
		t.Fatal(err)
	}
	if flag {
		t.Fatal("must_change_password still true after change-password")
	}
}

func TestSignInAndMeExposeMustChangePassword(t *testing.T) {
	t.Setenv("JWT_SECRET", strings.Repeat("k", 48))
	pool := testdb.Open(t)
	ctx := context.Background()
	member := testdb.User(t, pool, "m@x.test", false)
	svc := newAuth(pool)

	for _, want := range []bool{true, false} {
		setPassword(t, pool, member, "OldPass123!", want)
		signed, err := svc.SignIn(ctx, &dto.SignInRequest{Email: "m@x.test", Password: "OldPass123!"}, domain.DeviceInfo{}, "127.0.0.1")
		if err != nil {
			t.Fatal(err)
		}
		if signed.User.MustChangePassword != want {
			t.Fatalf("signin must_change_password = %v, want %v", signed.User.MustChangePassword, want)
		}
		me, err := svc.GetMe(ctx, member, false)
		if err != nil {
			t.Fatal(err)
		}
		if me.User.MustChangePassword != want {
			t.Fatalf("me must_change_password = %v, want %v", me.User.MustChangePassword, want)
		}
	}
}
