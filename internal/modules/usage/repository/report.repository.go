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

const nearFull = `s.context_window > 0 AND s.peak_context >= 0.8 * s.context_window`

const overheadJSON = `CASE WHEN s.overhead_system_tokens IS NULL THEN NULL ELSE json_build_object(
	'system_tokens', s.overhead_system_tokens, 'memory_tokens', s.overhead_memory_tokens,
	'agents_tokens', s.overhead_agents_tokens, 'skills_tokens', s.overhead_skills_tokens) END`

var groupColumns = map[string]string{
	"user":  "user_id::text",
	"model": "model",
}

var sessionOrders = map[string]string{
	"last_request_at":  "s.last_request_at ASC, s.id",
	"-last_request_at": "s.last_request_at DESC, s.id",
	"new_context":      "g.new_context ASC, s.id",
	"-new_context":     "g.new_context DESC, s.id",
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

func sessionColumns(newContext, extra string) string {
	return `
	s.id, s.user_id::text, s.machine, s.project, s.title, s.model,
	s.context_window, s.start_context, s.latest_context, s.peak_context, ` + newContext + `, ` + extra + `,
	s.requests, s.compactions, s.first_request_at, s.last_request_at, s.last_reported_at,
	COALESCE(NULLIF(u.full_name, ''), u.username),
	COALESCE((
		SELECT array_agg(m.model ORDER BY m.new_context DESC, m.model)
		FROM (
			SELECT model, SUM(new_context) AS new_context
			FROM usage.session_hours WHERE session_id = s.id GROUP BY model
		) m
	), ARRAY[s.model]::text[])`
}

func scanSession(row pgx.Row) (dto.SessionRow, error) {
	var r dto.SessionRow
	s := &r.Session
	err := row.Scan(
		&s.ID, &s.UserID, &s.Machine, &s.Project, &s.Title, &s.Model,
		&s.ContextWindow, &s.StartContext, &s.LatestContext, &s.PeakContext, &s.NewContext,
		&r.OutputTokens, &r.SubagentRequests, &r.SubagentOutputTokens, &r.InputTokens, &r.CacheWriteTokens, &r.CacheReadTokens,
		&s.Requests, &s.Compactions, &s.FirstRequestAt, &s.LastRequestAt, &s.LastReportedAt,
		&r.UserName, &r.Models,
	)
	return r, err
}

func (r *Repository) Overview(ctx context.Context, q dto.ReportQuery) ([]dto.PersonSummary, error) {
	rows, err := r.db.Query(ctx, `
		WITH h AS (
			SELECT user_id,
			       SUM(new_context)::bigint AS new_context,
			       SUM(requests)::bigint AS requests,
			       COUNT(DISTINCT session_id) AS sessions,
			       SUM(output_tokens)::bigint AS output_tokens,
			       SUM(subagent_requests)::bigint AS subagent_requests,
			       SUM(subagent_output_tokens)::bigint AS subagent_output_tokens,
			       SUM(input_tokens)::bigint AS input_tokens,
			       SUM(cache_write_tokens)::bigint AS cache_write_tokens,
			       SUM(cache_read_tokens)::bigint AS cache_read_tokens
			FROM usage.session_hours
			WHERE hour >= $1 AND hour < $2 AND ($3::uuid IS NULL OR user_id = $3::uuid)
			GROUP BY user_id
		), st AS (
			SELECT user_id,
			       COUNT(*) FILTER (WHERE `+nearFull+`) AS near_full,
			       SUM(compactions)::bigint AS compactions
			FROM usage.sessions s
			WHERE ($3::uuid IS NULL OR user_id = $3::uuid) AND EXISTS (
				SELECT 1 FROM usage.session_hours h WHERE h.session_id = s.id AND h.hour >= $1 AND h.hour < $2)
			GROUP BY user_id
		)
		SELECT u.id::text, COALESCE(u.full_name, ''), u.email, COALESCE(ls.model, ''),
		       COALESCE(h.new_context, 0), COALESCE(h.requests, 0), COALESCE(h.sessions, 0),
		       COALESCE(h.output_tokens, 0), COALESCE(h.subagent_requests, 0), COALESCE(h.subagent_output_tokens, 0),
		       COALESCE(h.input_tokens, 0), COALESCE(h.cache_write_tokens, 0), COALESCE(h.cache_read_tokens, 0),
		       COALESCE(st.near_full, 0), COALESCE(st.compactions, 0),
		       ls.last_request_at, k.last_used_at, k.app_version, COALESCE(u.is_active, FALSE)
		FROM core.users u
		LEFT JOIN h ON h.user_id = u.id
		LEFT JOIN st ON st.user_id = u.id
		LEFT JOIN LATERAL (
			SELECT s.model, s.last_request_at FROM usage.sessions s
			WHERE s.user_id = u.id ORDER BY s.last_request_at DESC LIMIT 1
		) ls ON TRUE
		LEFT JOIN LATERAL (
			SELECT a.last_used_at, a.app_version FROM core.api_keys a
			WHERE a.user_id = u.id ORDER BY a.last_used_at DESC NULLS LAST LIMIT 1
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
			&p.NewContext, &p.Requests, &p.Sessions,
			&p.OutputTokens, &p.SubagentRequests, &p.SubagentOutputTokens,
			&p.InputTokens, &p.CacheWriteTokens, &p.CacheReadTokens, &p.SessionsNearFull, &p.Compactions,
			&p.LastRequestAt, &p.LastSyncedAt, &p.AppVersion, &p.IsActive)
		return p, err
	})
}

func (r *Repository) Timeseries(ctx context.Context, q dto.ReportQuery, interval, groupBy string) ([]dto.Point, error) {
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT date_trunc($1, hour, $2) AS bucket, %s AS key,
		       SUM(new_context)::bigint, SUM(requests)::bigint,
		       SUM(output_tokens)::bigint, SUM(subagent_requests)::bigint, SUM(subagent_output_tokens)::bigint
		FROM usage.session_hours
		JOIN core.users u ON u.id = user_id
		WHERE hour >= $3 AND hour < $4 AND ($5::uuid IS NULL OR user_id = $5::uuid) AND u.deleted_at IS NULL
		GROUP BY 1, 2
		ORDER BY 1, 2`, groupColumns[groupBy]),
		interval, q.TZ, q.From, q.To, userFilter(q.UserID))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (dto.Point, error) {
		var p dto.Point
		err := row.Scan(&p.Bucket, &p.Key, &p.NewContext, &p.Requests, &p.OutputTokens, &p.SubagentRequests, &p.SubagentOutputTokens)
		return p, err
	})
}

func (r *Repository) Models(ctx context.Context, q dto.ReportQuery) ([]dto.ModelSummary, error) {
	rows, err := r.db.Query(ctx, `
		SELECT model, SUM(new_context)::bigint, SUM(requests)::bigint, COUNT(DISTINCT session_id),
		       SUM(output_tokens)::bigint, SUM(subagent_requests)::bigint, SUM(subagent_output_tokens)::bigint
		FROM usage.session_hours
		JOIN core.users u ON u.id = user_id
		WHERE hour >= $1 AND hour < $2 AND ($3::uuid IS NULL OR user_id = $3::uuid) AND u.deleted_at IS NULL
		GROUP BY model
		ORDER BY 2 DESC, model`,
		q.From, q.To, userFilter(q.UserID))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (dto.ModelSummary, error) {
		var m dto.ModelSummary
		err := row.Scan(&m.Model, &m.NewContext, &m.Requests, &m.Sessions, &m.OutputTokens, &m.SubagentRequests, &m.SubagentOutputTokens)
		return m, err
	})
}

func (r *Repository) Sessions(ctx context.Context, q dto.SessionQuery, limit, offset int) ([]dto.SessionRow, int64, error) {
	where := `
		FROM usage.sessions s
		JOIN core.users u ON u.id = s.user_id
		JOIN LATERAL (
			SELECT SUM(h.new_context) AS new_context,
			       SUM(h.output_tokens)::bigint AS output_tokens,
			       SUM(h.subagent_requests)::bigint AS subagent_requests,
			       SUM(h.subagent_output_tokens)::bigint AS subagent_output_tokens,
			       SUM(h.input_tokens)::bigint AS input_tokens,
			       SUM(h.cache_write_tokens)::bigint AS cache_write_tokens,
			       SUM(h.cache_read_tokens)::bigint AS cache_read_tokens
			FROM usage.session_hours h
			WHERE h.session_id = s.id AND h.hour >= $1 AND h.hour < $2
		) g ON g.new_context > 0 OR g.output_tokens > 0 OR g.subagent_requests > 0
		WHERE u.deleted_at IS NULL
		  AND ($3::uuid IS NULL OR s.user_id = $3::uuid)
		  AND ($4 = '' OR s.project = $4)
		  AND ($5 = '' OR s.model = $5 OR EXISTS (
			SELECT 1 FROM usage.session_hours h WHERE h.session_id = s.id AND h.model = $5))
		  AND ($6 = '' OR s.title ILIKE '%' || $6 || '%' OR s.project ILIKE '%' || $6 || '%')`
	args := []any{q.From, q.To, userFilter(q.UserID), q.Project, q.Model, q.Q}

	var total int64
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(ctx,
		`SELECT `+sessionColumns("g.new_context", "g.output_tokens, g.subagent_requests, g.subagent_output_tokens, g.input_tokens, g.cache_write_tokens, g.cache_read_tokens")+where+` ORDER BY `+sessionOrders[q.Sort]+` LIMIT $7 OFFSET $8`,
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
		SELECT `+sessionColumns("s.new_context", "0::bigint, 0::bigint, 0::bigint, 0::bigint, 0::bigint, 0::bigint")+`
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
		SELECT hour, model, new_context, requests, output_tokens, subagent_requests, subagent_output_tokens,
		       input_tokens, cache_write_tokens, cache_read_tokens
		FROM usage.session_hours
		WHERE session_id = $1
		ORDER BY hour, model`, id)
	if err != nil {
		return dto.SessionDetail{}, err
	}
	hours, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.HourUsage, error) {
		var h domain.HourUsage
		err := row.Scan(&h.Hour, &h.Model, &h.NewContext, &h.Requests, &h.OutputTokens, &h.SubagentRequests, &h.SubagentOutputTokens,
			&h.InputTokens, &h.CacheWriteTokens, &h.CacheReadTokens)
		return h, err
	})
	if err != nil {
		return dto.SessionDetail{}, err
	}
	detail := dto.SessionDetail{Session: found.Session, UserName: found.UserName, Models: found.Models, Hours: hours}
	err = r.db.QueryRow(ctx, `SELECT `+overheadJSON+` FROM usage.sessions s WHERE s.id = $1`, id).Scan(&detail.Overhead)
	return detail, err
}

func (r *Repository) Projects(ctx context.Context, q dto.ReportQuery) ([]dto.ProjectSummary, error) {
	rows, err := r.db.Query(ctx, `
		WITH h AS (
			SELECT s.project, h.*
			FROM usage.session_hours h
			JOIN usage.sessions s ON s.id = h.session_id
			JOIN core.users u ON u.id = h.user_id
			WHERE h.hour >= $1 AND h.hour < $2 AND ($3::uuid IS NULL OR h.user_id = $3::uuid) AND u.deleted_at IS NULL
		), p AS (
			SELECT project, SUM(new_context)::bigint AS new_context, SUM(requests)::bigint AS requests,
			       SUM(output_tokens)::bigint AS output_tokens, SUM(input_tokens)::bigint AS input_tokens,
			       SUM(cache_write_tokens)::bigint AS cache_write_tokens, SUM(cache_read_tokens)::bigint AS cache_read_tokens,
			       COUNT(DISTINCT session_id) AS sessions, COUNT(DISTINCT user_id) AS members
			FROM h GROUP BY project
		), m AS (
			SELECT project, array_agg(model ORDER BY new_context DESC, model) AS models
			FROM (SELECT project, model, SUM(new_context) AS new_context FROM h GROUP BY project, model) x
			GROUP BY project
		), st AS (
			SELECT project, SUM(compactions)::bigint AS compactions, COUNT(*) FILTER (WHERE `+nearFull+`) AS near_full
			FROM usage.sessions s
			WHERE id IN (SELECT session_id FROM h)
			GROUP BY project
		)
		SELECT p.project, p.new_context, p.requests, p.output_tokens, p.input_tokens, p.cache_write_tokens, p.cache_read_tokens,
		       p.sessions, p.members, m.models, st.compactions, st.near_full,
		       (SELECT `+overheadJSON+` FROM usage.sessions s
		        WHERE s.project = p.project AND s.overhead_system_tokens IS NOT NULL AND s.id IN (SELECT session_id FROM h)
		        ORDER BY s.last_request_at DESC, s.id LIMIT 1)
		FROM p
		JOIN m USING (project)
		JOIN st USING (project)
		ORDER BY p.new_context DESC, p.project`,
		q.From, q.To, userFilter(q.UserID))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (dto.ProjectSummary, error) {
		var p dto.ProjectSummary
		err := row.Scan(&p.Project, &p.NewContext, &p.Requests, &p.OutputTokens, &p.InputTokens, &p.CacheWriteTokens, &p.CacheReadTokens,
			&p.Sessions, &p.Members, &p.Models, &p.Compactions, &p.SessionsNearFull, &p.Overhead)
		return p, err
	})
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
