package audit

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"claude-usage-server/pkg/logger"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

const auditColumns = `
	id,
	reff_type, reff_id, reff_no,
	action, summary, changes, metadata,
	actor_id, actor_name, actor_role,
	host(ip_address), user_agent, request_id,
	occurred_at, created_at
`

func scan(row pgx.Row, a *AuditLog) error {
	return row.Scan(
		&a.ID,
		&a.ReffType, &a.ReffID, &a.ReffNo,
		&a.Action, &a.Summary, &a.Changes, &a.Metadata,
		&a.ActorID, &a.ActorName, &a.ActorRole,
		&a.IPAddress, &a.UserAgent, &a.RequestID,
		&a.OccurredAt, &a.CreatedAt,
	)
}

// InsertTx writes one audit row inside the caller's transaction. Failures
// bubble up so the caller can abort the business transaction — an audit
// write must not be silently dropped.
func (r *Repository) InsertTx(ctx context.Context, tx pgx.Tx, a *AuditLog) error {
	query := `
		INSERT INTO core.audit_logs (
			id,
			reff_type, reff_id, reff_no,
			action, summary, changes, metadata,
			actor_id, actor_name, actor_role,
			ip_address, user_agent, request_id,
			occurred_at, created_at
		) VALUES (
			$1,
			$2,$3,$4,
			$5,$6,$7,$8,
			$9,$10,$11,
			$12::inet,$13,$14,
			$15,$16
		)
	`
	_, err := tx.Exec(ctx, query,
		a.ID,
		a.ReffType, a.ReffID, a.ReffNo,
		a.Action, a.Summary, nullableJSON(a.Changes), nullableJSON(a.Metadata),
		a.ActorID, a.ActorName, a.ActorRole,
		a.IPAddress, a.UserAgent, a.RequestID,
		a.OccurredAt, a.CreatedAt,
	)
	if err != nil {
		logger.Error("Failed to insert audit log",
			logger.String("reff_type", a.ReffType),
			logger.String("reff_id", a.ReffID),
			logger.String("action", a.Action),
			logger.Err(err),
		)
		return err
	}
	return nil
}

// ListFilter drives the flexible List query. Filters are optional.
// Zero-values are skipped.
type ListFilter struct {
	ReffType string
	ReffID   string
	ActorID  string
	Action   string
	DateFrom string // YYYY-MM-DD, matched against occurred_at
	DateTo   string // YYYY-MM-DD, inclusive (< next day)
	Page     int
	Limit    int
}

// List returns paginated audit rows matching the filter, newest first.
// Filters are additive (AND).
func (r *Repository) List(ctx context.Context, f ListFilter) ([]AuditLog, int64, error) {
	conditions := []string{"TRUE"}
	var args []any
	idx := 1

	if f.ReffType != "" {
		conditions = append(conditions, fmt.Sprintf("reff_type = $%d", idx))
		args = append(args, f.ReffType)
		idx++
	}
	if f.ReffID != "" {
		conditions = append(conditions, fmt.Sprintf("reff_id = $%d", idx))
		args = append(args, f.ReffID)
		idx++
	}
	if f.ActorID != "" {
		conditions = append(conditions, fmt.Sprintf("actor_id = $%d", idx))
		args = append(args, f.ActorID)
		idx++
	}
	if f.Action != "" {
		conditions = append(conditions, fmt.Sprintf("action = $%d", idx))
		args = append(args, f.Action)
		idx++
	}
	if f.DateFrom != "" {
		conditions = append(conditions, fmt.Sprintf("occurred_at >= $%d::date", idx))
		args = append(args, f.DateFrom)
		idx++
	}
	if f.DateTo != "" {
		conditions = append(conditions, fmt.Sprintf("occurred_at < ($%d::date + INTERVAL '1 day')", idx))
		args = append(args, f.DateTo)
		idx++
	}

	where := strings.Join(conditions, " AND ")

	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM core.audit_logs WHERE %s`, where)
	var total int64
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (f.Page - 1) * f.Limit
	dataQuery := fmt.Sprintf(`
		SELECT %s FROM core.audit_logs
		WHERE %s
		ORDER BY occurred_at DESC, created_at DESC
		LIMIT $%d OFFSET $%d
	`, auditColumns, where, idx, idx+1)
	args = append(args, f.Limit, offset)

	rows, err := r.db.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list []AuditLog
	for rows.Next() {
		var a AuditLog
		if err := scan(rows, &a); err != nil {
			return nil, 0, err
		}
		list = append(list, a)
	}
	return list, total, nil
}

// nullableJSON makes a nil/empty byte slice insert as SQL NULL so the column
// stays NULL instead of becoming "null"::jsonb.
func nullableJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}
