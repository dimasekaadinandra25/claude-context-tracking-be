package service

import (
	"context"
	"strings"
	"testing"

	"claude-usage-server/internal/modules/core/api_key/repository"
	roleRepository "claude-usage-server/internal/modules/core/role/repository"
	"claude-usage-server/internal/modules/usage/testdb"
)

func TestCreateForUserKeyAuthenticatesAsThatUser(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	admin := testdb.User(t, pool, "boss@x.test", true)
	member := testdb.User(t, pool, "m@x.test", false)
	svc := NewApiKeyService(repository.NewApiKeyRepository(pool), roleRepository.NewRoleRepository(pool))

	fullKey, key, err := svc.CreateForUser(ctx, admin, member, "laptop")
	if err != nil {
		t.Fatal(err)
	}
	if key.UserID != member || key.Name != "laptop" || !strings.HasPrefix(fullKey, "wiz_live_"+key.KeyID+"_") {
		t.Fatalf("unexpected key %q %+v", fullKey, key)
	}

	parts := strings.Split(fullKey, "_")
	claims, err := svc.ValidateAndGetClaims(ctx, parts[2], parts[3], "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != member {
		t.Fatalf("claims user %s, want %s", claims.UserID, member)
	}
	if len(claims.Roles) != 1 || claims.Roles[0] != "member" {
		t.Fatalf("claims roles %v", claims.Roles)
	}
}
