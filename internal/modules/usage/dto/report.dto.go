package dto

import (
	"time"

	"claude-usage-server/internal/modules/usage/domain"
)

const (
	DefaultPerPage = 25
	MaxPerPage     = 100
)

type ReportQuery struct {
	From   time.Time `form:"from"`
	To     time.Time `form:"to"`
	TZ     string    `form:"tz"`
	UserID string    `form:"user_id"`
}

type Caller struct {
	UserID  string
	IsAdmin bool
}

type PersonSummary struct {
	UserID               string       `json:"user_id"`
	FullName             string       `json:"full_name"`
	Email                string       `json:"email"`
	CurrentModel         string       `json:"current_model"`
	NewContext           int64        `json:"new_context"`
	Requests             int64        `json:"requests"`
	Sessions             int64        `json:"sessions"`
	OutputTokens         int64        `json:"output_tokens"`
	SubagentRequests     int64        `json:"subagent_requests"`
	SubagentOutputTokens int64        `json:"subagent_output_tokens"`
	InputTokens          int64        `json:"input_tokens"`
	CacheWriteTokens     int64        `json:"cache_write_tokens"`
	CacheReadTokens      int64        `json:"cache_read_tokens"`
	SessionsNearFull     int64        `json:"sessions_near_full"`
	Compactions          int64        `json:"compactions"`
	LinesAdded           int64        `json:"lines_added"`
	LinesRemoved         int64        `json:"lines_removed"`
	Commits              int64        `json:"commits"`
	FiveHourUsed         *float64     `json:"five_hour_used"`
	FiveHourResetsAt     *time.Time   `json:"five_hour_resets_at"`
	SevenDayUsed         *float64     `json:"seven_day_used"`
	SevenDayResetsAt     *time.Time   `json:"seven_day_resets_at"`
	LimitsCapturedAt     *time.Time   `json:"limits_captured_at"`
	ModelLimits          []ModelLimit `json:"model_limits"`
	ClaudeEmail          *string      `json:"claude_email"`
	ClaudePlan           *string      `json:"claude_plan"`
	ClaudeTier           *string      `json:"claude_tier"`
	LastRequestAt        *time.Time   `json:"last_request_at"`
	LastSyncedAt         *time.Time   `json:"last_synced_at"`
	AppVersion           *string      `json:"app_version"`
	IsActive             bool         `json:"is_active"`
}

type Overview struct {
	People                    []PersonSummary `json:"people"`
	TotalNewContext           int64           `json:"total_new_context"`
	TotalRequests             int64           `json:"total_requests"`
	TotalSessions             int64           `json:"total_sessions"`
	ActivePeople              int64           `json:"active_people"`
	TotalOutputTokens         int64           `json:"total_output_tokens"`
	TotalSubagentRequests     int64           `json:"total_subagent_requests"`
	TotalSubagentOutputTokens int64           `json:"total_subagent_output_tokens"`
	TotalInputTokens          int64           `json:"total_input_tokens"`
	TotalCacheWriteTokens     int64           `json:"total_cache_write_tokens"`
	TotalCacheReadTokens      int64           `json:"total_cache_read_tokens"`
	TotalSessionsNearFull     int64           `json:"total_sessions_near_full"`
	TotalCompactions          int64           `json:"total_compactions"`
	TotalLinesAdded           int64           `json:"total_lines_added"`
	TotalLinesRemoved         int64           `json:"total_lines_removed"`
	TotalCommits              int64           `json:"total_commits"`
}

type Point struct {
	Bucket               time.Time `json:"bucket"`
	Key                  string    `json:"key"`
	NewContext           int64     `json:"new_context"`
	Requests             int64     `json:"requests"`
	OutputTokens         int64     `json:"output_tokens"`
	SubagentRequests     int64     `json:"subagent_requests"`
	SubagentOutputTokens int64     `json:"subagent_output_tokens"`
}

type ModelSummary struct {
	Model                string `json:"model"`
	NewContext           int64  `json:"new_context"`
	Requests             int64  `json:"requests"`
	Sessions             int64  `json:"sessions"`
	OutputTokens         int64  `json:"output_tokens"`
	SubagentRequests     int64  `json:"subagent_requests"`
	SubagentOutputTokens int64  `json:"subagent_output_tokens"`
	InputTokens          int64  `json:"input_tokens"`
	CacheWriteTokens     int64  `json:"cache_write_tokens"`
	CacheReadTokens      int64  `json:"cache_read_tokens"`
}

type ModelLimit struct {
	Name     string    `json:"name"`
	Used     float64   `json:"used"`
	ResetsAt time.Time `json:"resets_at"`
}

type MemberModel struct {
	UserID           string `json:"user_id"`
	FullName         string `json:"full_name"`
	Email            string `json:"email"`
	Model            string `json:"model"`
	Requests         int64  `json:"requests"`
	Sessions         int64  `json:"sessions"`
	InputTokens      int64  `json:"input_tokens"`
	CacheWriteTokens int64  `json:"cache_write_tokens"`
	CacheReadTokens  int64  `json:"cache_read_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
}

type ToolSummary struct {
	Tool         string `json:"tool"`
	Calls        int64  `json:"calls"`
	LinesAdded   int64  `json:"lines_added"`
	LinesRemoved int64  `json:"lines_removed"`
	Commits      int64  `json:"commits"`
}

type DailyUsage struct {
	UserID           string   `json:"user_id"`
	Day              string   `json:"day"`
	InputTokens      int64    `json:"input_tokens"`
	CacheWriteTokens int64    `json:"cache_write_tokens"`
	CacheReadTokens  int64    `json:"cache_read_tokens"`
	OutputTokens     int64    `json:"output_tokens"`
	Requests         int64    `json:"requests"`
	SevenDayUsed     *float64 `json:"seven_day_used"`
	LimitUsed        *float64 `json:"limit_used"`
	AccountWeight    int64    `json:"account_weight"`
	AccountMembers   int32    `json:"account_members"`
}

type SessionQuery struct {
	ReportQuery
	Project string `form:"project"`
	Model   string `form:"model"`
	Q       string `form:"q"`
	Page    int    `form:"page"`
	PerPage int    `form:"per_page"`
	Sort    string `form:"sort"`
}

func (q SessionQuery) Paging() (page, perPage int) {
	page, perPage = q.Page, q.PerPage
	if page == 0 {
		page = 1
	}
	if perPage == 0 {
		perPage = DefaultPerPage
	}
	return page, perPage
}

type SessionRow struct {
	domain.Session
	UserName             string   `json:"user_name"`
	Models               []string `json:"models"`
	OutputTokens         int64    `json:"output_tokens"`
	SubagentRequests     int64    `json:"subagent_requests"`
	SubagentOutputTokens int64    `json:"subagent_output_tokens"`
	InputTokens          int64    `json:"input_tokens"`
	CacheWriteTokens     int64    `json:"cache_write_tokens"`
	CacheReadTokens      int64    `json:"cache_read_tokens"`
}

type SessionDetail struct {
	Session  domain.Session     `json:"session"`
	UserName string             `json:"user_name"`
	Models   []string           `json:"models"`
	Hours    []domain.HourUsage `json:"hours"`
	Overhead *domain.Overhead   `json:"overhead"`
	Tools    []ToolSummary      `json:"tools"`
}

type ProjectSummary struct {
	Project          string           `json:"project"`
	NewContext       int64            `json:"new_context"`
	Requests         int64            `json:"requests"`
	OutputTokens     int64            `json:"output_tokens"`
	InputTokens      int64            `json:"input_tokens"`
	CacheWriteTokens int64            `json:"cache_write_tokens"`
	CacheReadTokens  int64            `json:"cache_read_tokens"`
	Sessions         int64            `json:"sessions"`
	Members          int64            `json:"members"`
	Models           []string         `json:"models"`
	Compactions      int64            `json:"compactions"`
	SessionsNearFull int64            `json:"sessions_near_full"`
	Overhead         *domain.Overhead `json:"overhead"`
}

type Person struct {
	ID       string `json:"id"`
	FullName string `json:"full_name"`
	Email    string `json:"email"`
	IsActive bool   `json:"is_active"`
}
