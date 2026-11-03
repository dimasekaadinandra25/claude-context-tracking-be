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
	UserID               string     `json:"user_id"`
	FullName             string     `json:"full_name"`
	Email                string     `json:"email"`
	CurrentModel         string     `json:"current_model"`
	NewContext           int64      `json:"new_context"`
	Requests             int64      `json:"requests"`
	Sessions             int64      `json:"sessions"`
	OutputTokens         int64      `json:"output_tokens"`
	SubagentRequests     int64      `json:"subagent_requests"`
	SubagentOutputTokens int64      `json:"subagent_output_tokens"`
	InputTokens          int64      `json:"input_tokens"`
	CacheWriteTokens     int64      `json:"cache_write_tokens"`
	CacheReadTokens      int64      `json:"cache_read_tokens"`
	SessionsNearFull     int64      `json:"sessions_near_full"`
	Compactions          int64      `json:"compactions"`
	LastRequestAt        *time.Time `json:"last_request_at"`
	LastSyncedAt         *time.Time `json:"last_synced_at"`
	AppVersion           *string    `json:"app_version"`
	IsActive             bool       `json:"is_active"`
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
