package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"claude-usage-server/internal/modules/usage/domain"
	"claude-usage-server/internal/modules/usage/dto"
)

var ErrNoSession = errors.New("session not found")

var groupColumns = map[string]string{
	"user":  "user_id::text",
	"model": "model",
}

var sessionOrders = map[string]string{
	"last_request_at":  "s.last_request_at ASC, s.id",
	"-last_request_at": "s.last_request_at DESC, s.id",
	"new_context":      "s.new_context ASC, s.id",
	"-new_context":     "s.new_context DESC, s.id",
}

func ValidGroup(groupBy string) bool {
	_, ok := groupColumns[groupBy]
	return ok
}

func ValidSort(sort string) bool {
	_, ok := sessionOrders[sort]
	return ok
}

func userFilter(userID string) any {
	if userID == "" {
		return nil
	}
	return userID
}

const sessionColumns = `
	s.id, s.user_id::text, s.machine, s.project, s.title, s.model,
	s.context_window, s.start_context, s.latest_context, s.peak_context, s.new_context,
	s.requests, s.compactions, s.first_request_at, s.last_request_at, s.last_reported_at,
	COALESCE(NULLIF(u.full_name, ''), u.username)`

func scanSession(row pgx.Row) (dto.SessionRow, error) {
	var r dto.SessionRow
	s := &r.Session
	err := row.Scan(
		&s.ID, &s.UserID, &s.Machine, &s.Project, &s.Title, &s.Model,
		&s.ContextWindow, &s.StartContext, &s.LatestContext, &s.PeakContext, &s.NewContext,
		&s.Requests, &s.Compactions, &s.FirstRequestAt, &s.LastRequestAt, &s.LastReportedAt,
		&r.UserName,
	)
	return r, err
}

func (r *Repository) Overview(ctx context.Context, q dto.ReportQuery) ([]dto.PersonSummary, error) {
	rows, err := r.db.Query(ctx, `
		WITH h AS (
			SELECT user_id,
			       SUM(new_context)::bigint AS new_context,
			       SUM(requests)::bigint AS requests,
			       COUNT(DISTINCT session_id) AS sessions
			FROM usage.session_hours
			WHERE hour >= $1 AND hour < $2 AND ($3::uuid IS NULL OR user_id = $3::uuid)
			GROUP BY user_id
		)
		SELECT u.id::text, COALESCE(u.full_name, ''), u.email, COALESCE(ls.model, ''),
		       COALESCE(h.new_context, 0), COALESCE(h.requests, 0), COALESCE(h.sessions, 0),
		       ls.last_request_at, k.last_used_at
		FROM core.users u
		LEFT JOIN h ON h.user_id = u.id
		LEFT JOIN LATERAL (
			SELECT s.model, s.last_request_at FROM usage.sessions s
			WHERE s.user_id = u.id ORDER BY s.last_request_at DESC LIMIT 1
		) ls ON TRUE
		LEFT JOIN LATERAL (
			SELECT MAX(a.last_used_at) AS last_used_at FROM core.api_keys a WHERE a.user_id = u.id
		) k ON TRUE
		WHERE u.deleted_at IS NULL AND ($3::uuid IS NULL OR u.id = $3::uuid)
		ORDER BY COALESCE(h.new_context, 0) DESC, u.email`,
		q.From, q.To, userFilter(q.UserID))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (dto.PersonSummary, error) {
		var p dto.PersonSummary
		err := row.Scan(&p.UserID, &p.FullName, &p.Email, &p.CurrentModel,
			&p.NewContext, &p.Requests, &p.Sessions, &p.LastRequestAt, &p.LastSyncedAt)
		return p, err
	})
}

func (r *Repository) Timeseries(ctx context.Context, q dto.ReportQuery, interval, groupBy string) ([]dto.Point, error) {
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT date_trunc($1, hour, $2) AS bucket, %s AS key,
		       SUM(new_context)::bigint, SUM(requests)::bigint
		FROM usage.session_hours
		WHERE hour >= $3 AND hour < $4 AND ($5::uuid IS NULL OR user_id = $5::uuid)
		GROUP BY 1, 2
		ORDER BY 1, 2`, groupColumns[groupBy]),
		interval, q.TZ, q.From, q.To, userFilter(q.UserID))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (dto.Point, error) {
		var p dto.Point
		err := row.Scan(&p.Bucket, &p.Key, &p.NewContext, &p.Requests)
		return p, err
	})
}

func (r *Repository) Models(ctx context.Context, q dto.ReportQuery) ([]dto.ModelSummary, error) {
	rows, err := r.db.Query(ctx, `
		SELECT model, SUM(new_context)::bigint, SUM(requests)::bigint, COUNT(DISTINCT session_id)
		FROM usage.session_hours
		WHERE hour >= $1 AND hour < $2 AND ($3::uuid IS NULL OR user_id = $3::uuid)
		GROUP BY model
		ORDER BY 2 DESC, model`,
		q.From, q.To, userFilter(q.UserID))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (dto.ModelSummary, error) {
		var m dto.ModelSummary
		err := row.Scan(&m.Model, &m.NewContext, &m.Requests, &m.Sessions)
		return m, err
	})
}

func (r *Repository) Sessions(ctx context.Context, q dto.SessionQuery, limit, offset int) ([]dto.SessionRow, int64, error) {
	where := `
		FROM usage.sessions s
		JOIN core.users u ON u.id = s.user_id
		WHERE s.last_request_at >= $1 AND s.last_request_at < $2
		  AND ($3::uuid IS NULL OR s.user_id = $3::uuid)
		  AND ($4 = '' OR s.project = $4)
		  AND ($5 = '' OR s.model = $5)
		  AND ($6 = '' OR s.title ILIKE '%' || $6 || '%' OR s.project ILIKE '%' || $6 || '%')`
	args := []any{q.From, q.To, userFilter(q.UserID), q.Project, q.Model, q.Q}

	var total int64
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(ctx,
		`SELECT `+sessionColumns+where+` ORDER BY `+sessionOrders[q.Sort]+` LIMIT $7 OFFSET $8`,
		append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (dto.SessionRow, error) {
		return scanSession(row)
	})
	return list, total, err
}

func (r *Repository) Session(ctx context.Context, id string) (dto.SessionDetail, error) {
	found, err := scanSession(r.db.QueryRow(ctx, `
		SELECT `+sessionColumns+`
		FROM usage.sessions s
		JOIN core.users u ON u.id = s.user_id
		WHERE s.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return dto.SessionDetail{}, ErrNoSession
	}
	if err != nil {
		return dto.SessionDetail{}, err
	}
	rows, err := r.db.Query(ctx, `
		SELECT hour, model, new_context, requests
		FROM usage.session_hours
		WHERE session_id = $1
		ORDER BY hour, model`, id)
	if err != nil {
		return dto.SessionDetail{}, err
	}
	hours, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.HourUsage, error) {
		var h domain.HourUsage
		err := row.Scan(&h.Hour, &h.Model, &h.NewContext, &h.Requests)
		return h, err
	})
	return dto.SessionDetail{Session: found.Session, UserName: found.UserName, Hours: hours}, err
}

func (r *Repository) People(ctx context.Context) ([]dto.Person, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, COALESCE(full_name, ''), email, COALESCE(is_active, FALSE)
		FROM core.users
		WHERE deleted_at IS NULL
		ORDER BY COALESCE(NULLIF(full_name, ''), email)`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[dto.Person])
}
