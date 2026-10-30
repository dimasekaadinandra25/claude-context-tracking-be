package audit

import (
	"context"
	"encoding/json"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Event is the input to RecordTx. It intentionally uses plain Go types
// (maps, *string) so callers don't have to hand-roll SQL or JSON.
//
// Required: ReffType, ReffID, Action.
// Optional: everything else. Changes is typically populated only when
// Action == ActionUpdated.
type Event struct {
	ReffType string
	ReffID   string
	ReffNo   *string

	Action   string
	Summary  *string
	Changes  map[string]FieldChange
	Metadata map[string]any

	// OccurredAt defaults to time.Now() when zero.
	OccurredAt time.Time
}

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// Repo exposes the underlying repository for callers that want to run list
// queries bypassing the service (e.g. admin dashboards).
func (s *Service) Repo() *Repository { return s.repo }

// RecordTx writes an audit row inside the caller's transaction. The actor
// snapshot is pulled from the provided context (see audit.WithActor). An
// error from this method must abort the enclosing transaction — audit
// gaps defeat the purpose of the feature.
func (s *Service) RecordTx(ctx context.Context, tx pgx.Tx, e Event) error {
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now()
	}

	row := &AuditLog{
		ID:         uuid.New().String(),
		ReffType:   e.ReffType,
		ReffID:     e.ReffID,
		ReffNo:     e.ReffNo,
		Action:     e.Action,
		Summary:    e.Summary,
		OccurredAt: e.OccurredAt,
		CreatedAt:  time.Now(),
	}

	if len(e.Changes) > 0 {
		b, err := json.Marshal(e.Changes)
		if err != nil {
			return err
		}
		row.Changes = b
	}
	if len(e.Metadata) > 0 {
		b, err := json.Marshal(e.Metadata)
		if err != nil {
			return err
		}
		row.Metadata = b
	}

	actor := ActorFromContext(ctx)
	if actor.UserID != "" {
		row.ActorID = strPtr(actor.UserID)
	}
	if actor.Name != "" {
		row.ActorName = strPtr(actor.Name)
	}
	if actor.Role != "" {
		row.ActorRole = strPtr(actor.Role)
	}
	if actor.IP != "" {
		row.IPAddress = strPtr(actor.IP)
	}
	if actor.UserAgent != "" {
		row.UserAgent = strPtr(actor.UserAgent)
	}
	if actor.RequestID != "" {
		row.RequestID = strPtr(actor.RequestID)
	}

	return s.repo.InsertTx(ctx, tx, row)
}

// List fetches audit rows matching the filter, newest first. Filters are
// optional — zero values skipped.
func (s *Service) List(ctx context.Context, f ListFilter) ([]AuditLogResponse, int64, error) {
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.Limit <= 0 {
		f.Limit = 50
	}
	logs, total, err := s.repo.List(ctx, f)
	if err != nil {
		return nil, 0, err
	}
	out := make([]AuditLogResponse, 0, len(logs))
	for i := range logs {
		out = append(out, ToResponse(&logs[i]))
	}
	return out, total, nil
}

// ─── DIFF HELPERS ───────────────────────────────────────────────

// Diff returns the field-level differences between two same-keyed maps.
// Only fields whose values differ (by reflect.DeepEqual) are included.
// Fields present in only one side are recorded with the missing side set
// to nil.
func Diff(oldVals, newVals map[string]any) map[string]FieldChange {
	out := make(map[string]FieldChange)
	for k, ov := range oldVals {
		nv, ok := newVals[k]
		if !ok {
			out[k] = FieldChange{Old: ov, New: nil}
			continue
		}
		if !reflect.DeepEqual(ov, nv) {
			out[k] = FieldChange{Old: ov, New: nv}
		}
	}
	for k, nv := range newVals {
		if _, ok := oldVals[k]; !ok {
			out[k] = FieldChange{Old: nil, New: nv}
		}
	}
	return out
}

func strPtr(s string) *string { return &s }
