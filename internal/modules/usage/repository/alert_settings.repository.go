package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"claude-usage-server/internal/modules/usage/dto"
)

func (r *Repository) AlertSettings(ctx context.Context) (*dto.AlertSettings, error) {
	var s dto.AlertSettings
	err := r.db.QueryRow(ctx, `SELECT alerts, runaway_share, project_share FROM usage.alert_settings`).
		Scan(&s.Alerts, &s.RunawayShare, &s.ProjectShare)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *Repository) SetAlertSettings(ctx context.Context, userID string, s dto.AlertSettings) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO usage.alert_settings (alerts, runaway_share, project_share, updated_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET
			alerts = EXCLUDED.alerts,
			runaway_share = EXCLUDED.runaway_share,
			project_share = EXCLUDED.project_share,
			updated_by = EXCLUDED.updated_by,
			updated_at = NOW()`,
		s.Alerts, s.RunawayShare, s.ProjectShare, userID)
	return err
}
