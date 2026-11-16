package domain

import "time"

const (
	ActionUserCreate          = "user.create"
	ActionUserActivate        = "user.activate"
	ActionUserDeactivate      = "user.deactivate"
	ActionUserResetPassword   = "user.reset_password"
	ActionUserChangePassword  = "user.change_password"
	ActionUserChangeRole      = "user.change_role"
	ActionApiKeyIssue         = "api_key.issue"
	ActionApiKeyReveal        = "api_key.reveal"
	ActionApiKeyRevoke        = "api_key.revoke"
	ActionAlertSettingsUpdate = "alert_settings.update"

	TargetUser          = "user"
	TargetApiKey        = "api_key"
	TargetAlertSettings = "alert_settings"

	Retention = 365 * 24 * time.Hour
)

type AuditLog struct {
	ID          string    `json:"id"`
	ActorID     *string   `json:"actor_id"`
	ActorEmail  string    `json:"actor_email"`
	Action      string    `json:"action"`
	TargetType  string    `json:"target_type"`
	TargetID    string    `json:"target_id"`
	TargetLabel string    `json:"target_label"`
	Summary     string    `json:"summary"`
	IP          string    `json:"ip"`
	CreatedAt   time.Time `json:"created_at"`
}
