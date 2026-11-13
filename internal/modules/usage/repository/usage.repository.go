package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"claude-usage-server/internal/modules/usage/domain"
)

type Repository struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

const upsertSession = `
	INSERT INTO usage.sessions (
		id, user_id, machine, project, title, model,
		context_window, start_context, latest_context, peak_context, new_context,
		requests, compactions, first_request_at, last_request_at,
		overhead_system_tokens, overhead_memory_tokens, overhead_agents_tokens, overhead_skills_tokens
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
	ON CONFLICT (id) DO UPDATE SET
		machine = EXCLUDED.machine,
		project = EXCLUDED.project,
		title = EXCLUDED.title,
		model = EXCLUDED.model,
		context_window = EXCLUDED.context_window,
		start_context = EXCLUDED.start_context,
		latest_context = EXCLUDED.latest_context,
		peak_context = EXCLUDED.peak_context,
		new_context = EXCLUDED.new_context,
		requests = EXCLUDED.requests,
		compactions = EXCLUDED.compactions,
		first_request_at = EXCLUDED.first_request_at,
		last_request_at = EXCLUDED.last_request_at,
		overhead_system_tokens = COALESCE(EXCLUDED.overhead_system_tokens, usage.sessions.overhead_system_tokens),
		overhead_memory_tokens = COALESCE(EXCLUDED.overhead_memory_tokens, usage.sessions.overhead_memory_tokens),
		overhead_agents_tokens = COALESCE(EXCLUDED.overhead_agents_tokens, usage.sessions.overhead_agents_tokens),
		overhead_skills_tokens = COALESCE(EXCLUDED.overhead_skills_tokens, usage.sessions.overhead_skills_tokens),
		last_reported_at = NOW(),
		updated_at = NOW()
	WHERE usage.sessions.user_id = EXCLUDED.user_id AND EXCLUDED.last_request_at >= usage.sessions.last_request_at
	RETURNING id`

const insertHours = `
	INSERT INTO usage.session_hours (
		session_id, hour, model, user_id, new_context, requests, output_tokens, subagent_requests, subagent_output_tokens,
		input_tokens, cache_write_tokens, cache_read_tokens
	)
	SELECT $1, h.hour, h.model, $6, h.new_context, h.requests, h.output_tokens, h.subagent_requests, h.subagent_output_tokens,
		h.input_tokens, h.cache_write_tokens, h.cache_read_tokens
	FROM unnest($2::timestamptz[], $3::text[], $4::bigint[], $5::int[], $7::bigint[], $8::int[], $9::bigint[], $10::bigint[], $11::bigint[], $12::bigint[])
		AS h(hour, model, new_context, requests, output_tokens, subagent_requests, subagent_output_tokens, input_tokens, cache_write_tokens, cache_read_tokens)
	ON CONFLICT (session_id, hour, model) DO NOTHING`

const insertTools = `
	INSERT INTO usage.session_tool_hours (session_id, hour, tool, user_id, calls, lines_added, lines_removed, commits)
	SELECT $1, t.hour, t.tool, $2, t.calls, t.lines_added, t.lines_removed, t.commits
	FROM unnest($3::timestamptz[], $4::text[], $5::int[], $6::bigint[], $7::bigint[], $8::int[])
		AS t(hour, tool, calls, lines_added, lines_removed, commits)
	ON CONFLICT (session_id, hour, tool) DO NOTHING`

const upsertPlanLimits = `
	INSERT INTO usage.plan_limits (user_id, five_hour_used, five_hour_resets_at, seven_day_used, seven_day_resets_at, captured_at, model_limits)
	VALUES ($1, $2, $3, $4, $5, $6, $7)
	ON CONFLICT (user_id) DO UPDATE SET
		model_limits = EXCLUDED.model_limits,
		five_hour_used = EXCLUDED.five_hour_used,
		five_hour_resets_at = EXCLUDED.five_hour_resets_at,
		seven_day_used = EXCLUDED.seven_day_used,
		seven_day_resets_at = EXCLUDED.seven_day_resets_at,
		captured_at = EXCLUDED.captured_at
	WHERE EXCLUDED.captured_at >= usage.plan_limits.captured_at`

const insertPlanReading = `
	INSERT INTO usage.plan_limit_readings (user_id, captured_at, five_hour_used, seven_day_used, seven_day_resets_at)
	VALUES ($1, $2, $3, $4, $5)
	ON CONFLICT DO NOTHING`

const upsertClaudeAccount = `
	INSERT INTO usage.claude_accounts (user_id, email, plan, tier, updated_at)
	VALUES ($1, $2, $3, $4, NOW())
	ON CONFLICT (user_id) DO UPDATE SET
		email = EXCLUDED.email, plan = EXCLUDED.plan, tier = EXCLUDED.tier, updated_at = NOW()`

func planWindow(w *domain.PlanWindow) (*float64, *time.Time) {
	if w == nil {
		return nil, nil
	}
	resetsAt := time.Unix(w.ResetsAt, 0).UTC()
	return &w.UsedPercentage, &resetsAt
}

func (r *Repository) SavePlanLimits(ctx context.Context, userID string, limits domain.PlanLimits) error {
	fiveUsed, fiveResets := planWindow(limits.FiveHour)
	sevenUsed, sevenResets := planWindow(limits.SevenDay)
	capturedAt := time.Unix(limits.CapturedAt, 0).UTC()
	batch := &pgx.Batch{}
	models := limits.Models
	if models == nil {
		models = []domain.ModelLimit{}
	}
	batch.Queue(upsertPlanLimits, userID, fiveUsed, fiveResets, sevenUsed, sevenResets, capturedAt, models)
	batch.Queue(insertPlanReading, userID, capturedAt, fiveUsed, sevenUsed, sevenResets)
	return r.db.SendBatch(ctx, batch).Close()
}

func (r *Repository) SaveClaudeAccount(ctx context.Context, userID string, a domain.ClaudeAccount) error {
	_, err := r.db.Exec(ctx, upsertClaudeAccount, userID, a.Email, a.Plan, a.Tier)
	return err
}

func replaceTools(ctx context.Context, tx pgx.Tx, sessionID, userID string, tools []domain.ToolUsage) error {
	if _, err := tx.Exec(ctx, `DELETE FROM usage.session_tool_hours WHERE session_id = $1`, sessionID); err != nil {
		return err
	}
	at := make([]time.Time, len(tools))
	names := make([]string, len(tools))
	calls := make([]int32, len(tools))
	added := make([]int64, len(tools))
	removed := make([]int64, len(tools))
	commits := make([]int32, len(tools))
	for i, t := range tools {
		at[i], names[i], calls[i] = t.Hour, t.Tool, t.Calls
		added[i], removed[i], commits[i] = t.LinesAdded, t.LinesRemoved, t.Commits
	}
	_, err := tx.Exec(ctx, insertTools, sessionID, userID, at, names, calls, added, removed, commits)
	return err
}

func (r *Repository) SaveSession(ctx context.Context, s domain.Session, overhead *domain.Overhead, hours []domain.HourUsage, tools []domain.ToolUsage) (bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var system, memory, agents, skills *int64
	if overhead != nil {
		system, memory, agents, skills = &overhead.SystemTokens, &overhead.MemoryTokens, &overhead.AgentsTokens, &overhead.SkillsTokens
	}
	var id string
	err = tx.QueryRow(ctx, upsertSession,
		s.ID, s.UserID, s.Machine, s.Project, s.Title, s.Model,
		s.ContextWindow, s.StartContext, s.LatestContext, s.PeakContext, s.NewContext,
		s.Requests, s.Compactions, s.FirstRequestAt, s.LastRequestAt,
		system, memory, agents, skills,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		var owned bool
		err = tx.QueryRow(ctx, `SELECT user_id = $2 FROM usage.sessions WHERE id = $1`, s.ID, s.UserID).Scan(&owned)
		return owned, err
	}
	if err != nil {
		return false, err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM usage.session_hours WHERE session_id = $1`, s.ID); err != nil {
		return false, err
	}
	at := make([]time.Time, len(hours))
	models := make([]string, len(hours))
	newContext := make([]int64, len(hours))
	requests := make([]int32, len(hours))
	output := make([]int64, len(hours))
	subRequests := make([]int32, len(hours))
	subOutput := make([]int64, len(hours))
	input := make([]int64, len(hours))
	cacheWrite := make([]int64, len(hours))
	cacheRead := make([]int64, len(hours))
	for i, h := range hours {
		at[i], models[i], newContext[i], requests[i] = h.Hour, h.Model, h.NewContext, h.Requests
		output[i], subRequests[i], subOutput[i] = h.OutputTokens, h.SubagentRequests, h.SubagentOutputTokens
		input[i], cacheWrite[i], cacheRead[i] = h.InputTokens, h.CacheWriteTokens, h.CacheReadTokens
	}
	if _, err := tx.Exec(ctx, insertHours, s.ID, at, models, newContext, requests, s.UserID, output, subRequests, subOutput, input, cacheWrite, cacheRead); err != nil {
		return false, err
	}
	if tools != nil {
		if err := replaceTools(ctx, tx, s.ID, s.UserID, tools); err != nil {
			return false, err
		}
	}
	return true, tx.Commit(ctx)
}

func (r *Repository) AppVersions(ctx context.Context) ([]string, error) {
	rows, err := r.db.Query(ctx, `SELECT DISTINCT app_version FROM core.api_keys WHERE app_version IS NOT NULL AND deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func (r *Repository) SetAppVersion(ctx context.Context, keyID, version string) error {
	_, err := r.db.Exec(ctx, `UPDATE core.api_keys SET app_version = $2 WHERE key_id = $1 AND deleted_at IS NULL`, keyID, version)
	return err
}
