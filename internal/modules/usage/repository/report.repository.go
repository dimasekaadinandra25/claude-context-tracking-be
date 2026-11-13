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

const hourWeight = `(h.input_tokens + h.cache_write_tokens + h.cache_read_tokens + 5 * (h.output_tokens + h.subagent_output_tokens))`

const accounts = `
	SELECT u.id AS user_id, COALESCE(NULLIF(u.full_name, ''), u.username) AS name,
	       NULLIF(ca.email, '') AS email, NULLIF(ca.tier, '') AS tier,
	       COALESCE(NULLIF(LOWER(ca.email), ''), u.id::text) AS account
	FROM core.users u LEFT JOIN usage.claude_accounts ca ON ca.user_id = u.id
	WHERE u.deleted_at IS NULL`

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
		), t AS (
			SELECT user_id,
			       SUM(lines_added)::bigint AS lines_added,
			       SUM(lines_removed)::bigint AS lines_removed,
			       SUM(commits)::bigint AS commits
			FROM usage.session_tool_hours
			WHERE hour >= $1 AND hour < $2 AND ($3::uuid IS NULL OR user_id = $3::uuid)
			GROUP BY user_id
		)
		SELECT u.id::text, COALESCE(u.full_name, ''), u.email, COALESCE(ls.model, ''),
		       COALESCE(h.new_context, 0), COALESCE(h.requests, 0), COALESCE(h.sessions, 0),
		       COALESCE(h.output_tokens, 0), COALESCE(h.subagent_requests, 0), COALESCE(h.subagent_output_tokens, 0),
		       COALESCE(h.input_tokens, 0), COALESCE(h.cache_write_tokens, 0), COALESCE(h.cache_read_tokens, 0),
		       COALESCE(st.near_full, 0), COALESCE(st.compactions, 0),
		       COALESCE(t.lines_added, 0), COALESCE(t.lines_removed, 0), COALESCE(t.commits, 0),
		       CASE WHEN pl.five_hour_resets_at > NOW() THEN pl.five_hour_used END,
		       CASE WHEN pl.five_hour_resets_at > NOW() THEN pl.five_hour_resets_at END,
		       CASE WHEN pl.seven_day_resets_at > NOW() THEN pl.seven_day_used END,
		       CASE WHEN pl.seven_day_resets_at > NOW() THEN pl.seven_day_resets_at END,
		       pl.captured_at, COALESCE((
		           SELECT jsonb_agg(jsonb_build_object('name', m->>'name', 'used', (m->>'used_percentage')::float8,
		                  'resets_at', to_timestamp((m->>'resets_at')::bigint)) ORDER BY m->>'name')
		           FROM jsonb_array_elements(pl.model_limits) m
		           WHERE to_timestamp((m->>'resets_at')::bigint) > NOW()), '[]'::jsonb),
		       NULLIF(ca.email, ''), NULLIF(ca.plan, ''), NULLIF(ca.tier, ''),
		       ls.last_request_at, k.last_used_at, k.app_version, COALESCE(u.is_active, FALSE), fh.share
		FROM core.users u
		LEFT JOIN h ON h.user_id = u.id
		LEFT JOIN st ON st.user_id = u.id
		LEFT JOIN t ON t.user_id = u.id
		LEFT JOIN usage.claude_accounts ca ON ca.user_id = u.id
		LEFT JOIN LATERAL (
			SELECT p.* FROM usage.plan_limits p
			LEFT JOIN usage.claude_accounts pa ON pa.user_id = p.user_id
			WHERE p.user_id = u.id OR (ca.email <> '' AND LOWER(pa.email) = LOWER(ca.email))
			ORDER BY p.captured_at DESC
			LIMIT 1
		) pl ON TRUE
		LEFT JOIN LATERAL (
			SELECT (pl.five_hour_used * COALESCE(SUM(`+hourWeight+`) FILTER (WHERE h.user_id = u.id), 0)
			        / NULLIF(SUM(`+hourWeight+`), 0))::float8 AS share
			FROM usage.session_hours h
			LEFT JOIN usage.claude_accounts ha ON ha.user_id = h.user_id
			WHERE pl.five_hour_resets_at > NOW()
			  AND h.hour >= date_trunc('hour', pl.five_hour_resets_at - INTERVAL '5 hours')
			  AND (h.user_id = u.id OR (ca.email <> '' AND LOWER(ha.email) = LOWER(ca.email)))
		) fh ON TRUE
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
			&p.LinesAdded, &p.LinesRemoved, &p.Commits,
			&p.FiveHourUsed, &p.FiveHourResetsAt, &p.SevenDayUsed, &p.SevenDayResetsAt, &p.LimitsCapturedAt, &p.ModelLimits,
			&p.ClaudeEmail, &p.ClaudePlan, &p.ClaudeTier,
			&p.LastRequestAt, &p.LastSyncedAt, &p.AppVersion, &p.IsActive, &p.FiveHourShare)
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
		       SUM(output_tokens)::bigint, SUM(subagent_requests)::bigint, SUM(subagent_output_tokens)::bigint,
		       SUM(input_tokens)::bigint, SUM(cache_write_tokens)::bigint, SUM(cache_read_tokens)::bigint
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
		err := row.Scan(&m.Model, &m.NewContext, &m.Requests, &m.Sessions, &m.OutputTokens, &m.SubagentRequests, &m.SubagentOutputTokens,
			&m.InputTokens, &m.CacheWriteTokens, &m.CacheReadTokens)
		return m, err
	})
}

func scanToolSummaries(rows pgx.Rows) ([]dto.ToolSummary, error) {
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (dto.ToolSummary, error) {
		var t dto.ToolSummary
		err := row.Scan(&t.Tool, &t.Calls, &t.LinesAdded, &t.LinesRemoved, &t.Commits)
		return t, err
	})
}

func (r *Repository) MemberModels(ctx context.Context, q dto.ReportQuery) ([]dto.MemberModel, error) {
	rows, err := r.db.Query(ctx, `
		SELECT u.id::text, COALESCE(u.full_name, ''), u.email, h.model,
		       SUM(h.requests + h.subagent_requests)::bigint, COUNT(DISTINCT h.session_id),
		       SUM(h.input_tokens)::bigint, SUM(h.cache_write_tokens)::bigint,
		       SUM(h.cache_read_tokens)::bigint, SUM(h.output_tokens + h.subagent_output_tokens)::bigint
		FROM usage.session_hours h
		JOIN core.users u ON u.id = h.user_id
		WHERE h.hour >= $1 AND h.hour < $2 AND ($3::uuid IS NULL OR h.user_id = $3::uuid) AND u.deleted_at IS NULL
		GROUP BY 1, 2, 3, 4
		ORDER BY 2, 3, 4`,
		q.From, q.To, userFilter(q.UserID))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (dto.MemberModel, error) {
		var m dto.MemberModel
		err := row.Scan(&m.UserID, &m.FullName, &m.Email, &m.Model, &m.Requests, &m.Sessions,
			&m.InputTokens, &m.CacheWriteTokens, &m.CacheReadTokens, &m.OutputTokens)
		return m, err
	})
}

func (r *Repository) Tools(ctx context.Context, q dto.ReportQuery) ([]dto.ToolSummary, error) {
	rows, err := r.db.Query(ctx, `
		SELECT tool, SUM(calls)::bigint, SUM(lines_added)::bigint, SUM(lines_removed)::bigint, SUM(commits)::bigint
		FROM usage.session_tool_hours
		JOIN core.users u ON u.id = user_id
		WHERE hour >= $1 AND hour < $2 AND ($3::uuid IS NULL OR user_id = $3::uuid) AND u.deleted_at IS NULL
		GROUP BY tool
		ORDER BY 2 DESC, tool`,
		q.From, q.To, userFilter(q.UserID))
	if err != nil {
		return nil, err
	}
	return scanToolSummaries(rows)
}

func (r *Repository) Daily(ctx context.Context, q dto.ReportQuery) ([]dto.DailyUsage, error) {
	rows, err := r.db.Query(ctx, `
		WITH accounts AS (
			SELECT u.id AS user_id, COALESCE(NULLIF(LOWER(ca.email), ''), u.id::text) AS account
			FROM core.users u
			LEFT JOIN usage.claude_accounts ca ON ca.user_id = u.id
			WHERE u.deleted_at IS NULL
		), scope AS (
			SELECT DISTINCT account FROM accounts WHERE $4::uuid IS NULL OR user_id = $4::uuid
		), tokens AS (
			SELECT h.user_id, a.account, (h.hour AT TIME ZONE $3)::date AS day,
			       SUM(h.input_tokens)::bigint AS input_tokens,
			       SUM(h.cache_write_tokens)::bigint AS cache_write_tokens,
			       SUM(h.cache_read_tokens)::bigint AS cache_read_tokens,
			       SUM(h.output_tokens + h.subagent_output_tokens)::bigint AS output_tokens,
			       SUM(h.requests + h.subagent_requests)::bigint AS requests,
			       SUM(h.input_tokens + h.cache_write_tokens + h.cache_read_tokens + 5 * (h.output_tokens + h.subagent_output_tokens))::bigint AS weight
			FROM usage.session_hours h
			JOIN accounts a ON a.user_id = h.user_id
			WHERE h.hour >= $1 AND h.hour < $2 AND a.account IN (SELECT account FROM scope)
			GROUP BY 1, 2, 3
		), readings AS (
			SELECT a.account, r.captured_at, r.seven_day_used, r.seven_day_resets_at,
			       LAG(r.seven_day_used) OVER w AS previous_used,
			       LAG(r.seven_day_resets_at) OVER w AS previous_resets_at
			FROM usage.plan_limit_readings r
			JOIN accounts a ON a.user_id = r.user_id
			WHERE r.captured_at >= $1::timestamptz - INTERVAL '8 days' AND r.captured_at < $2
			  AND r.seven_day_used IS NOT NULL AND a.account IN (SELECT account FROM scope)
			WINDOW w AS (PARTITION BY a.account ORDER BY r.captured_at)
		), limits AS (
			SELECT account, (captured_at AT TIME ZONE $3)::date AS day,
			       (ARRAY_AGG(seven_day_used ORDER BY captured_at DESC))[1] AS seven_day_used,
			       SUM(CASE
			           WHEN ABS(EXTRACT(EPOCH FROM seven_day_resets_at - previous_resets_at)) < 3600
			           THEN GREATEST(0, seven_day_used - previous_used)
			           WHEN previous_resets_at IS NOT NULL OR captured_at < seven_day_resets_at - INTERVAL '6 days'
			           THEN seven_day_used END) AS limit_used
			FROM readings
			GROUP BY 1, 2
		), account_days AS (
			SELECT account, day, SUM(weight)::bigint AS weight FROM tokens GROUP BY 1, 2
		), members AS (
			SELECT account, COUNT(*)::int AS members FROM accounts GROUP BY account
		), member_days AS (
			SELECT a.user_id, a.account, d.day
			FROM accounts a
			JOIN (SELECT account, day FROM limits UNION SELECT account, day FROM account_days) d ON d.account = a.account
			WHERE $4::uuid IS NULL OR a.user_id = $4::uuid
		)
		SELECT md.user_id::text, to_char(md.day, 'YYYY-MM-DD'),
		       COALESCE(t.input_tokens, 0), COALESCE(t.cache_write_tokens, 0), COALESCE(t.cache_read_tokens, 0),
		       COALESCE(t.output_tokens, 0), COALESCE(t.requests, 0),
		       l.seven_day_used, l.limit_used, COALESCE(ad.weight, 0), m.members
		FROM member_days md
		JOIN members m ON m.account = md.account
		LEFT JOIN tokens t ON t.user_id = md.user_id AND t.day = md.day
		LEFT JOIN limits l ON l.account = md.account AND l.day = md.day
		LEFT JOIN account_days ad ON ad.account = md.account AND ad.day = md.day
		WHERE md.day >= ($1::timestamptz AT TIME ZONE $3)::date
		ORDER BY 1, 2`,
		q.From, q.To, q.TZ, userFilter(q.UserID))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (dto.DailyUsage, error) {
		var d dto.DailyUsage
		err := row.Scan(&d.UserID, &d.Day, &d.InputTokens, &d.CacheWriteTokens, &d.CacheReadTokens,
			&d.OutputTokens, &d.Requests, &d.SevenDayUsed, &d.LimitUsed, &d.AccountWeight, &d.AccountMembers)
		return d, err
	})
}

func (r *Repository) Weeks(ctx context.Context, q dto.ReportQuery) ([]dto.WeekUsage, error) {
	rows, err := r.db.Query(ctx, `
		WITH accounts AS (
			SELECT u.id AS user_id, COALESCE(u.full_name, '') AS full_name,
			       COALESCE(NULLIF(LOWER(ca.email), ''), u.id::text) AS account, NULLIF(ca.tier, '') AS tier
			FROM core.users u
			LEFT JOIN usage.claude_accounts ca ON ca.user_id = u.id
			WHERE u.deleted_at IS NULL
		), scope AS (
			SELECT DISTINCT account FROM accounts WHERE $3::uuid IS NULL OR user_id = $3::uuid
		), windows AS (
			SELECT a.account, date_trunc('hour', r.seven_day_resets_at + INTERVAL '30 minutes') AS resets_at,
			       (ARRAY_AGG(r.seven_day_used ORDER BY r.captured_at DESC))[1] AS used
			FROM usage.plan_limit_readings r
			JOIN accounts a ON a.user_id = r.user_id
			WHERE r.seven_day_used IS NOT NULL AND r.seven_day_resets_at IS NOT NULL
			  AND a.account IN (SELECT account FROM scope)
			  AND r.seven_day_resets_at > $1 AND r.seven_day_resets_at - INTERVAL '7 days' < $2
			GROUP BY 1, 2
		), hours AS (
			SELECT w.account, w.resets_at, h.user_id, h.model,
			       h.input_tokens + h.cache_write_tokens + h.cache_read_tokens AS input_tokens,
			       h.output_tokens + h.subagent_output_tokens AS output_tokens,
			       h.requests + h.subagent_requests AS requests
			FROM windows w
			JOIN accounts a ON a.account = w.account
			JOIN usage.session_hours h ON h.user_id = a.user_id
			 AND h.hour >= w.resets_at - INTERVAL '7 days' AND h.hour < w.resets_at
		), members AS (
			SELECT account, resets_at, user_id,
			       SUM(input_tokens)::bigint AS input_tokens, SUM(output_tokens)::bigint AS output_tokens,
			       SUM(requests)::bigint AS requests, SUM(input_tokens + 5 * output_tokens)::bigint AS weight
			FROM hours GROUP BY 1, 2, 3
		), top_models AS (
			SELECT DISTINCT ON (account, resets_at) account, resets_at, model
			FROM hours
			GROUP BY 1, 2, 3
			ORDER BY account, resets_at, SUM(input_tokens + 5 * output_tokens) DESC
		)
		SELECT w.account, CASE WHEN w.account LIKE '%@%' THEN w.account END,
		       (SELECT MAX(tier) FROM accounts a WHERE a.account = w.account),
		       w.resets_at, w.used, tm.model, m.user_id::text, COALESCE(a.full_name, ''),
		       COALESCE(m.input_tokens, 0), COALESCE(m.output_tokens, 0), COALESCE(m.requests, 0), COALESCE(m.weight, 0)
		FROM windows w
		LEFT JOIN top_models tm ON tm.account = w.account AND tm.resets_at = w.resets_at
		LEFT JOIN members m ON m.account = w.account AND m.resets_at = w.resets_at
		     AND ($3::uuid IS NULL OR m.user_id = $3::uuid)
		LEFT JOIN accounts a ON a.user_id = m.user_id
		ORDER BY w.resets_at DESC, w.account, m.weight DESC NULLS LAST`,
		q.From, q.To, userFilter(q.UserID))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (dto.WeekUsage, error) {
		var w dto.WeekUsage
		err := row.Scan(&w.Account, &w.ClaudeEmail, &w.ClaudeTier, &w.ResetsAt, &w.Used, &w.TopModel, &w.UserID,
			&w.FullName, &w.InputTokens, &w.OutputTokens, &w.Requests, &w.Weight)
		return w, err
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
	if err != nil {
		return dto.SessionDetail{}, err
	}
	rows, err = r.db.Query(ctx, `
		SELECT tool, SUM(calls)::bigint, SUM(lines_added)::bigint, SUM(lines_removed)::bigint, SUM(commits)::bigint
		FROM usage.session_tool_hours
		WHERE session_id = $1
		GROUP BY tool
		ORDER BY 2 DESC, tool`, id)
	if err != nil {
		return dto.SessionDetail{}, err
	}
	detail.Tools, err = scanToolSummaries(rows)
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

func (r *Repository) HotSessions(ctx context.Context, minShare float64, userID string) ([]dto.HotSession, error) {
	rows, err := r.db.Query(ctx, `
		WITH a AS (`+accounts+`
		), lim AS (
			SELECT DISTINCT ON (a.account) a.account, p.seven_day_used AS used, p.seven_day_resets_at AS resets_at
			FROM usage.plan_limits p JOIN a ON a.user_id = p.user_id
			WHERE p.seven_day_resets_at > NOW() AND p.seven_day_used IS NOT NULL
			ORDER BY a.account, p.captured_at DESC
		), wk AS (
			SELECT a.account, SUM(`+hourWeight+`)::bigint AS weight
			FROM usage.session_hours h JOIN a ON a.user_id = h.user_id JOIN lim ON lim.account = a.account
			WHERE h.hour >= lim.resets_at - INTERVAL '7 days'
			GROUP BY a.account
		), hot AS (
			SELECT h.session_id, h.user_id, h.hour, SUM(`+hourWeight+`)::bigint AS weight
			FROM usage.session_hours h
			WHERE h.hour >= date_trunc('hour', NOW()) - INTERVAL '1 hour' AND ($2::uuid IS NULL OR h.user_id = $2::uuid)
			GROUP BY h.session_id, h.user_id, h.hour
		)
		SELECT s.id, a.user_id::text, a.name, s.project, s.title, hot.hour, hot.weight,
		       (lim.used * hot.weight / wk.weight)::float8 AS share
		FROM hot
		JOIN usage.sessions s ON s.id = hot.session_id
		JOIN a ON a.user_id = hot.user_id
		JOIN lim ON lim.account = a.account
		JOIN wk ON wk.account = a.account
		WHERE wk.weight > 0 AND lim.used * hot.weight / wk.weight >= $1
		ORDER BY share DESC, s.id`, minShare, userFilter(userID))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (dto.HotSession, error) {
		var h dto.HotSession
		err := row.Scan(&h.SessionID, &h.UserID, &h.FullName, &h.Project, &h.Title, &h.Hour, &h.Weight, &h.Share)
		return h, err
	})
}

func (r *Repository) Costs(ctx context.Context, q dto.ReportQuery) ([]dto.CostRow, error) {
	rows, err := r.db.Query(ctx, `
		WITH a AS (`+accounts+`
		), used AS (
			SELECT h.user_id, s.project,
			       SUM(h.input_tokens)::bigint AS input_tokens, SUM(h.cache_write_tokens)::bigint AS cache_write_tokens,
			       SUM(h.cache_read_tokens)::bigint AS cache_read_tokens,
			       SUM(h.output_tokens + h.subagent_output_tokens)::bigint AS output_tokens,
			       SUM(h.requests)::bigint AS requests, SUM(`+hourWeight+`)::bigint AS weight
			FROM usage.session_hours h JOIN usage.sessions s ON s.id = h.session_id
			WHERE h.hour >= $1 AND h.hour < $2
			GROUP BY h.user_id, s.project
		), priced AS (
			SELECT a.user_id, a.name, a.email, used.project, used.input_tokens, used.cache_write_tokens,
			       used.cache_read_tokens, used.output_tokens, used.requests, used.weight,
			       MAX(a.tier) OVER (PARTITION BY a.account) AS tier,
			       SUM(used.weight) OVER (PARTITION BY a.account)::bigint AS account_weight
			FROM used JOIN a ON a.user_id = used.user_id
		)
		SELECT user_id::text, name, email, tier, project, input_tokens, cache_write_tokens, cache_read_tokens,
		       output_tokens, requests, weight, account_weight
		FROM priced
		WHERE $3::uuid IS NULL OR user_id = $3::uuid
		ORDER BY name, weight DESC, project`, q.From, q.To, userFilter(q.UserID))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (dto.CostRow, error) {
		var c dto.CostRow
		err := row.Scan(&c.UserID, &c.FullName, &c.ClaudeEmail, &c.ClaudeTier, &c.Project, &c.InputTokens, &c.CacheWriteTokens,
			&c.CacheReadTokens, &c.OutputTokens, &c.Requests, &c.Weight, &c.AccountWeight)
		return c, err
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
