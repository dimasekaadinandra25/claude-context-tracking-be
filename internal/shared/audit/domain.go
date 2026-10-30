package audit

import "time"

// Action constants — kept as simple lowercase snake_case strings validated by
// the CHECK regex in core.audit_logs. New modules can introduce their own
// actions without a migration; prefer these shared ones where they fit.
const (
	ActionCreated           = "created"
	ActionUpdated           = "updated"
	ActionSubmitted         = "submitted"
	ActionApproved          = "approved"
	ActionRejected          = "rejected"
	ActionCancelled         = "cancelled"
	ActionPosted            = "posted"
	ActionVoided            = "voided"
	ActionDeleted           = "deleted"
	ActionRestored          = "restored"
	ActionAttachmentAdded   = "attachment_added"
	ActionAttachmentRemoved = "attachment_removed"
)

// AuditLog mirrors a row in core.audit_logs. Rows are append-only; there is
// intentionally no UpdatedAt / DeletedAt.
type AuditLog struct {
	ID       string  `json:"id"`
	ReffType string  `json:"reff_type"`
	ReffID   string  `json:"reff_id"`
	ReffNo   *string `json:"reff_no,omitempty"`

	Action  string  `json:"action"`
	Summary *string `json:"summary,omitempty"`

	Changes  []byte `json:"-"`
	Metadata []byte `json:"-"`

	ActorID   *string `json:"actor_id,omitempty"`
	ActorName *string `json:"actor_name,omitempty"`
	ActorRole *string `json:"actor_role,omitempty"`

	IPAddress *string `json:"ip_address,omitempty"`
	UserAgent *string `json:"user_agent,omitempty"`
	RequestID *string `json:"request_id,omitempty"`

	OccurredAt time.Time `json:"occurred_at"`
	CreatedAt  time.Time `json:"created_at"`
}

// FieldChange captures a single before/after pair for an updated field.
type FieldChange struct {
	Old any `json:"old"`
	New any `json:"new"`
}
