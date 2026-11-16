package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"claude-usage-server/internal/modules/core/audit/domain"
	"claude-usage-server/internal/modules/core/audit/dto"
)

type AuditRepository struct {
	db *pgxpool.Pool
}

func NewAuditRepository(db *pgxpool.Pool) *AuditRepository {
	return &AuditRepository{db: db}
}

func (r *AuditRepository) Create(ctx context.Context, l *domain.AuditLog) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO core.audit_logs (actor_id, actor_email, action, target_type, target_id, summary, ip)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7)`,
		l.ActorID, l.ActorEmail, l.Action, l.TargetType, l.TargetID, l.Summary, l.IP)
	return err
}

const listFilter = `($1 = '' OR a.action = $1) AND ($2 = '' OR a.actor_email ILIKE '%' || $2 || '%' OR a.actor_id::text = $2)`

func (r *AuditRepository) List(ctx context.Context, q *dto.AuditLogQuery) ([]domain.AuditLog, int64, error) {
	var total int64
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM core.audit_logs a WHERE `+listFilter, q.Action, q.Actor).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(ctx, `
		SELECT a.id::text, a.actor_id::text, a.actor_email, a.action, a.target_type, a.target_id,
		       COALESCE(tu.email, ku.email || ' · ' || k.name, ''), a.summary, a.ip, a.created_at
		FROM core.audit_logs a
		LEFT JOIN core.users tu ON a.target_type = 'user' AND tu.id::text = a.target_id
		LEFT JOIN core.api_keys k ON a.target_type = 'api_key' AND k.id::text = a.target_id
		LEFT JOIN core.users ku ON ku.id = k.user_id
		WHERE `+listFilter+`
		ORDER BY a.created_at DESC, a.id DESC
		LIMIT $3 OFFSET $4`,
		q.Action, q.Actor, q.Limit, (q.Page-1)*q.Limit)
	if err != nil {
		return nil, 0, err
	}
	logs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.AuditLog, error) {
		var l domain.AuditLog
		err := row.Scan(&l.ID, &l.ActorID, &l.ActorEmail, &l.Action, &l.TargetType, &l.TargetID,
			&l.TargetLabel, &l.Summary, &l.IP, &l.CreatedAt)
		return l, err
	})
	return logs, total, err
}

func (r *AuditRepository) Prune(ctx context.Context, before time.Time) (int64, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM core.audit_logs WHERE created_at < $1`, before)
	return tag.RowsAffected(), err
}
