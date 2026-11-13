package dto

import (
	"encoding/json"
	"time"

	"claude-usage-server/internal/modules/usage/domain"
)

type IngestRequest struct {
	Machine    string            `json:"machine" binding:"required"`
	AppVersion string            `json:"app_version"`
	Sessions   []json.RawMessage `json:"sessions" binding:"max=200"`
	PlanLimits json.RawMessage   `json:"plan_limits"`
	Account    json.RawMessage   `json:"account"`
}

type SessionPayload struct {
	SessionID     string           `json:"session_id"`
	Project       string           `json:"project"`
	Title         string           `json:"title"`
	Model         string           `json:"model"`
	ContextWindow int64            `json:"context_window"`
	StartContext  int64            `json:"start_context"`
	LatestContext int64            `json:"latest_context"`
	PeakContext   int64            `json:"peak_context"`
	NewContext    int64            `json:"new_context"`
	Requests      int32            `json:"requests"`
	Compactions   int32            `json:"compactions"`
	FirstAt       time.Time        `json:"first_at"`
	LatestAt      time.Time        `json:"latest_at"`
	Hours         []HourPayload    `json:"hours"`
	Overhead      *domain.Overhead `json:"overhead"`
	Tools         []ToolPayload    `json:"tools"`
}

type ToolPayload struct {
	Hour         time.Time `json:"hour"`
	Tool         string    `json:"tool"`
	Calls        int32     `json:"calls"`
	LinesAdded   int64     `json:"lines_added"`
	LinesRemoved int64     `json:"lines_removed"`
	Commits      int32     `json:"commits"`
}

type HourPayload struct {
	Hour                 time.Time `json:"hour"`
	Model                string    `json:"model"`
	NewContext           int64     `json:"new_context"`
	Requests             int32     `json:"requests"`
	OutputTokens         int64     `json:"output_tokens"`
	SubagentRequests     int32     `json:"subagent_requests"`
	SubagentOutputTokens int64     `json:"subagent_output_tokens"`
	InputTokens          int64     `json:"input_tokens"`
	CacheWriteTokens     int64     `json:"cache_write_tokens"`
	CacheReadTokens      int64     `json:"cache_read_tokens"`
}

type IngestResponse struct {
	Accepted          int         `json:"accepted"`
	Rejected          []Rejection `json:"rejected"`
	LatestAppVersion  string      `json:"latest_app_version,omitempty"`
	CostPerMillion    float64     `json:"cost_per_million_tokens,omitempty"`
	PercentPerMillion float64     `json:"percent_per_million_tokens,omitempty"`
}

type Rejection struct {
	SessionID string `json:"session_id"`
	Reason    string `json:"reason"`
}
