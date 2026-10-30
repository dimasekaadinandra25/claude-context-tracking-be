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
	UserID        string     `json:"user_id"`
	FullName      string     `json:"full_name"`
	Email         string     `json:"email"`
	CurrentModel  string     `json:"current_model"`
	NewContext    int64      `json:"new_context"`
	Requests      int64      `json:"requests"`
	Sessions      int64      `json:"sessions"`
	LastRequestAt *time.Time `json:"last_request_at"`
	LastSyncedAt  *time.Time `json:"last_synced_at"`
}

type Overview struct {
	People          []PersonSummary `json:"people"`
	TotalNewContext int64           `json:"total_new_context"`
	TotalRequests   int64           `json:"total_requests"`
	TotalSessions   int64           `json:"total_sessions"`
	ActivePeople    int64           `json:"active_people"`
}

type Point struct {
	Bucket     time.Time `json:"bucket"`
	Key        string    `json:"key"`
	NewContext int64     `json:"new_context"`
	Requests   int64     `json:"requests"`
}

type ModelSummary struct {
	Model      string `json:"model"`
	NewContext int64  `json:"new_context"`
	Requests   int64  `json:"requests"`
	Sessions   int64  `json:"sessions"`
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
	UserName string   `json:"user_name"`
	Models   []string `json:"models"`
}

type SessionDetail struct {
	Session  domain.Session     `json:"session"`
	UserName string             `json:"user_name"`
	Models   []string           `json:"models"`
	Hours    []domain.HourUsage `json:"hours"`
}

type Person struct {
	ID       string `json:"id"`
	FullName string `json:"full_name"`
	Email    string `json:"email"`
	IsActive bool   `json:"is_active"`
}
