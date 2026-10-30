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
	Hour       time.Time `json:"hour"`
	Model      string    `json:"model"`
	NewContext int64     `json:"new_context"`
	Requests   int32     `json:"requests"`
}
