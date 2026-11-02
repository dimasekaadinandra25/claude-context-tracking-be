package testdb

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const lockKey = 7_301_030

func Open(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	lock, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lock.Exec(ctx, `SELECT pg_advisory_lock($1)`, lockKey); err != nil {
		lock.Release()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = lock.Exec(ctx, `SELECT pg_advisory_unlock($1)`, lockKey)
		lock.Release()
	})

	if _, err := pool.Exec(ctx, `
		TRUNCATE usage.session_hours, usage.sessions, core.api_keys;
		DELETE FROM core.users WHERE username <> 'admin';`); err != nil {
		t.Fatal(err)
	}
	return pool
}

func User(t *testing.T, pool *pgxpool.Pool, email string, admin bool) string {
	t.Helper()
	role := "member"
	if admin {
		role = "super_admin"
	}
	name := strings.SplitN(email, "@", 2)[0]
	var id string
	err := pool.QueryRow(context.Background(), `
		WITH u AS (
			INSERT INTO core.users (email, username, password_hash, full_name, is_active)
			VALUES ($1, $2, 'x', $2, TRUE)
			RETURNING id
		)
		INSERT INTO core.user_roles (user_id, role_id)
		SELECT u.id, r.id FROM u, core.roles r WHERE r.code = $3
		RETURNING user_id::text`, email, name, role).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
