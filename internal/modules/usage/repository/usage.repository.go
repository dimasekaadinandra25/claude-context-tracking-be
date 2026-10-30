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
		requests, compactions, first_request_at, last_request_at
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
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
		last_reported_at = NOW(),
		updated_at = NOW()
	WHERE usage.sessions.user_id = EXCLUDED.user_id
	RETURNING id`

const insertHours = `
	INSERT INTO usage.session_hours (session_id, hour, model, user_id, new_context, requests)
	SELECT $1, h.hour, h.model, $6, h.new_context, h.requests
	FROM unnest($2::timestamptz[], $3::text[], $4::bigint[], $5::int[]) AS h(hour, model, new_context, requests)
	ON CONFLICT (session_id, hour, model) DO NOTHING`

func (r *Repository) SaveSession(ctx context.Context, s domain.Session, hours []domain.HourUsage) (bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id string
	err = tx.QueryRow(ctx, upsertSession,
		s.ID, s.UserID, s.Machine, s.Project, s.Title, s.Model,
		s.ContextWindow, s.StartContext, s.LatestContext, s.PeakContext, s.NewContext,
		s.Requests, s.Compactions, s.FirstRequestAt, s.LastRequestAt,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
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
	for i, h := range hours {
		at[i], models[i], newContext[i], requests[i] = h.Hour, h.Model, h.NewContext, h.Requests
	}
	if _, err := tx.Exec(ctx, insertHours, s.ID, at, models, newContext, requests, s.UserID); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}
