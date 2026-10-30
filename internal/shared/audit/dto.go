package audit

import (
	"encoding/json"
	"time"
)

// ListQueryParams drives the global GET /core/v1/audit-logs endpoint.
// All filters are optional. When reff_type + reff_id are both provided,
// the result is the per-entity history typically consumed by a detail
// page. Without them, it.s a global audit feed useful for admin
// dashboards.
type ListQueryParams struct {
	ReffType string `form:"reff_type"`
	ReffID   string `form:"reff_id" binding:"omitempty,uuid"`
	ActorID  string `form:"actor_id" binding:"omitempty,uuid"`
	Action   string `form:"action"`
	DateFrom string `form:"date_from"` // YYYY-MM-DD, matched against occurred_at
	DateTo   string `form:"date_to"`   // YYYY-MM-DD, inclusive end-of-day
	Page     int    `form:"page,default=1" binding:"min=1"`
	Limit    int    `form:"limit,default=50" binding:"min=1,max=200"`
}

// AuditLogResponse is the API-facing view of an audit row. Raw JSONB bytes
// are decoded into Go maps so the FE can render them directly.
type AuditLogResponse struct {
	ID         string                 `json:"id"`
	ReffType   string                 `json:"reff_type"`
	ReffID     string                 `json:"reff_id"`
	ReffNo     *string                `json:"reff_no,omitempty"`
	Action     string                 `json:"action"`
	Summary    *string                `json:"summary,omitempty"`
	Changes    map[string]FieldChange `json:"changes,omitempty"`
	Metadata   map[string]any         `json:"metadata,omitempty"`
	ActorID    *string                `json:"actor_id,omitempty"`
	ActorName  *string                `json:"actor_name,omitempty"`
	ActorRole  *string                `json:"actor_role,omitempty"`
	IPAddress  *string                `json:"ip_address,omitempty"`
	UserAgent  *string                `json:"user_agent,omitempty"`
	RequestID  *string                `json:"request_id,omitempty"`
	OccurredAt time.Time              `json:"occurred_at"`
	CreatedAt  time.Time              `json:"created_at"`
}

// ToResponse converts a domain AuditLog into its API response form. Invalid
// JSON in the changes/metadata columns is swallowed so a single corrupt row
// doesn't break the whole history list.
func ToResponse(l *AuditLog) AuditLogResponse {
	resp := AuditLogResponse{
		ID:         l.ID,
		ReffType:   l.ReffType,
		ReffID:     l.ReffID,
		ReffNo:     l.ReffNo,
		Action:     l.Action,
		Summary:    l.Summary,
		ActorID:    l.ActorID,
		ActorName:  l.ActorName,
		ActorRole:  l.ActorRole,
		IPAddress:  l.IPAddress,
		UserAgent:  l.UserAgent,
		RequestID:  l.RequestID,
		OccurredAt: l.OccurredAt,
		CreatedAt:  l.CreatedAt,
	}
	if len(l.Changes) > 0 {
		var c map[string]FieldChange
		if err := json.Unmarshal(l.Changes, &c); err == nil {
			resp.Changes = c
		}
	}
	if len(l.Metadata) > 0 {
		var m map[string]any
		if err := json.Unmarshal(l.Metadata, &m); err == nil {
			resp.Metadata = m
		}
	}
	return resp
}
