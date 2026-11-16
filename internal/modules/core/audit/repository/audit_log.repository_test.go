package repository

import (
	"context"
	"testing"
	"time"

	"claude-usage-server/internal/modules/core/audit/domain"
	"claude-usage-server/internal/testutil/testdb"
)

func TestPruneDropsEntriesOlderThanRetention(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO core.audit_logs (action, created_at) VALUES
			('user.create', NOW() - INTERVAL '400 days'),
			('user.activate', NOW() - INTERVAL '364 days'),
			('user.deactivate', NOW())`); err != nil {
		t.Fatal(err)
	}

	deleted, err := NewAuditRepository(pool).Prune(ctx, time.Now().Add(-domain.Retention))
	if err != nil || deleted != 1 {
		t.Fatalf("deleted %d, %v, want 1", deleted, err)
	}
	var left []string
	rows, err := pool.Query(ctx, `SELECT action FROM core.audit_logs ORDER BY created_at`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var action string
		if err := rows.Scan(&action); err != nil {
			t.Fatal(err)
		}
		left = append(left, action)
	}
	if len(left) != 2 || left[0] != "user.activate" || left[1] != "user.deactivate" {
		t.Fatalf("left %v", left)
	}
}
