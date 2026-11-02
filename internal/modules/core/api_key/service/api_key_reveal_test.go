package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"claude-usage-server/internal/modules/core/api_key/repository"
	roleRepository "claude-usage-server/internal/modules/core/role/repository"
	"claude-usage-server/internal/modules/usage/testdb"
	"claude-usage-server/pkg/keycrypt"
)

func revealSetup(t *testing.T) (*ApiKeyService, *pgxpool.Pool, string, string) {
	t.Helper()
	t.Setenv(keycrypt.EnvName, strings.Repeat("ab", 32))
	pool := testdb.Open(t)
	admin := testdb.User(t, pool, "boss@x.test", true)
	member := testdb.User(t, pool, "m@x.test", false)
	return NewApiKeyService(repository.NewApiKeyRepository(pool), roleRepository.NewRoleRepository(pool)), pool, admin, member
}

func TestRevealReturnsIssuedKey(t *testing.T) {
	svc, _, admin, member := revealSetup(t)
	ctx := context.Background()

	fullKey, key, err := svc.CreateForUser(ctx, admin, member, "laptop")
	if err != nil {
		t.Fatal(err)
	}
	revealed, err := svc.Reveal(ctx, key.ID)
	if err != nil {
		t.Fatal(err)
	}
	if revealed != fullKey {
		t.Fatal("revealed key differs from the issued key")
	}
	parts := strings.Split(revealed, "_")
	claims, err := svc.ValidateAndGetClaims(ctx, parts[2], parts[3], "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != member {
		t.Fatalf("claims user %s, want %s", claims.UserID, member)
	}
}

func TestRevealRefusesRevokedKey(t *testing.T) {
	svc, _, admin, member := revealSetup(t)
	ctx := context.Background()

	_, key, err := svc.CreateForUser(ctx, admin, member, "laptop")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.AdminRevoke(ctx, key.ID, admin, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Reveal(ctx, key.ID); !errors.Is(err, ErrApiKeyNotActive) {
		t.Fatalf("Reveal revoked key: %v, want ErrApiKeyNotActive", err)
	}
}

func TestRevealRefusesLegacyKey(t *testing.T) {
	svc, pool, admin, member := revealSetup(t)
	ctx := context.Background()

	_, key, err := svc.CreateForUser(ctx, admin, member, "laptop")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE core.api_keys SET secret_encrypted = NULL WHERE id = $1`, key.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Reveal(ctx, key.ID); !errors.Is(err, ErrApiKeyNotRevealable) {
		t.Fatalf("Reveal legacy key: %v, want ErrApiKeyNotRevealable", err)
	}
}

func TestRevealUnknownKeyIsNotFound(t *testing.T) {
	svc, _, _, _ := revealSetup(t)
	ctx := context.Background()

	for _, id := range []string{"69e4e903-5425-42d2-9994-bb467324dce7", "not-a-uuid"} {
		if _, err := svc.Reveal(ctx, id); !errors.Is(err, ErrApiKeyNotFound) {
			t.Fatalf("Reveal %s: %v, want ErrApiKeyNotFound", id, err)
		}
	}
}

func TestRevealAfterEncryptionKeyRotationIsNotRevealable(t *testing.T) {
	svc, _, admin, member := revealSetup(t)
	ctx := context.Background()

	_, key, err := svc.CreateForUser(ctx, admin, member, "laptop")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(keycrypt.EnvName, strings.Repeat("cd", 32))
	if _, err := svc.Reveal(ctx, key.ID); !errors.Is(err, ErrApiKeyNotRevealable) {
		t.Fatalf("Reveal after rotation: %v, want ErrApiKeyNotRevealable", err)
	}
}
