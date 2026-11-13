package domain

import "time"

type Session struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id"`
	Machine        string    `json:"machine"`
	Project        string    `json:"project"`
	Title          string    `json:"title"`
	Model          string    `json:"model"`
	ContextWindow  int64     `json:"context_window"`
	StartContext   int64     `json:"start_context"`
	LatestContext  int64     `json:"latest_context"`
	PeakContext    int64     `json:"peak_context"`
	NewContext     int64     `json:"new_context"`
	Requests       int32     `json:"requests"`
	Compactions    int32     `json:"compactions"`
	FirstRequestAt time.Time `json:"first_request_at"`
	LastRequestAt  time.Time `json:"last_request_at"`
	LastReportedAt time.Time `json:"last_reported_at"`
}

type HourUsage struct {
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

type ToolUsage struct {
	Hour         time.Time `json:"hour"`
	Tool         string    `json:"tool"`
	Calls        int32     `json:"calls"`
	LinesAdded   int64     `json:"lines_added"`
	LinesRemoved int64     `json:"lines_removed"`
	Commits      int32     `json:"commits"`
}

type PlanWindow struct {
	UsedPercentage float64 `json:"used_percentage"`
	ResetsAt       int64   `json:"resets_at"`
}

type ClaudeAccount struct {
	Email string `json:"email"`
	Plan  string `json:"plan"`
	Tier  string `json:"tier"`
}

type ModelLimit struct {
	Name           string  `json:"name"`
	UsedPercentage float64 `json:"used_percentage"`
	ResetsAt       int64   `json:"resets_at"`
}

type PlanLimits struct {
	FiveHour   *PlanWindow  `json:"five_hour"`
	SevenDay   *PlanWindow  `json:"seven_day"`
	Models     []ModelLimit `json:"models"`
	CapturedAt int64        `json:"captured_at"`
}

type Overhead struct {
	SystemTokens int64 `json:"system_tokens"`
	MemoryTokens int64 `json:"memory_tokens"`
	AgentsTokens int64 `json:"agents_tokens"`
	SkillsTokens int64 `json:"skills_tokens"`
}
