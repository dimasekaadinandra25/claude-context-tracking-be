package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"claude-usage-server/internal/modules/usage/dto"
)

func (r *Repository) Budgets(ctx context.Context, userID string, from, to time.Time) ([]dto.Budget, error) {
	rows, err := r.db.Query(ctx, `
		SELECT u.id::text, COALESCE(u.full_name, ''), u.email, b.monthly_new_context, COALESCE(h.used, 0)
		FROM core.users u
		LEFT JOIN usage.budgets b ON b.user_id = u.id
		LEFT JOIN LATERAL (
			SELECT SUM(new_context)::bigint AS used FROM usage.session_hours
			WHERE user_id = u.id AND hour >= $1 AND hour < $2
		) h ON TRUE
		WHERE u.deleted_at IS NULL AND COALESCE(u.is_active, FALSE) AND ($3::uuid IS NULL OR u.id = $3::uuid)
		ORDER BY COALESCE(NULLIF(u.full_name, ''), u.email)`,
		from, to, userFilter(userID))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (dto.Budget, error) {
		var b dto.Budget
		err := row.Scan(&b.UserID, &b.FullName, &b.Email, &b.MonthlyNewContext, &b.UsedNewContext)
		return b, err
	})
}

func (r *Repository) SetBudget(ctx context.Context, userID string, monthlyNewContext int64) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		INSERT INTO usage.budgets (user_id, monthly_new_context)
		SELECT id, $2 FROM core.users WHERE id = $1 AND deleted_at IS NULL
		ON CONFLICT (user_id) DO UPDATE SET
			monthly_new_context = EXCLUDED.monthly_new_context,
			updated_at = NOW()`,
		userID, monthlyNewContext)
	return tag.RowsAffected() > 0, err
}

func (r *Repository) DeleteBudget(ctx context.Context, userID string) (bool, error) {
	var found bool
	err := r.db.QueryRow(ctx, `
		WITH d AS (DELETE FROM usage.budgets WHERE user_id = $1)
		SELECT EXISTS (SELECT 1 FROM core.users WHERE id = $1 AND deleted_at IS NULL)`,
		userID).Scan(&found)
	return found, err
}
