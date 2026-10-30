package dto

import "time"

// CreateApiKeyRequest represents the request body for creating a new API key
type CreateApiKeyRequest struct {
	Name              string   `json:"name" binding:"required,min=1,max=100"`
	Description       *string  `json:"description" binding:"omitempty,max=500"`
	Environment       string   `json:"environment" binding:"omitempty,oneof=live test"`
	ScopedPermissions []string `json:"scoped_permissions" binding:"omitempty,dive,min=1"`
	IPWhitelist       []string `json:"ip_whitelist" binding:"omitempty"`
	RateLimit         *int     `json:"rate_limit" binding:"omitempty,min=1,max=100000"`
	RateLimitWindow   *int     `json:"rate_limit_window" binding:"omitempty,min=60,max=86400"`
	ExpiresAt         *string  `json:"expires_at" binding:"omitempty"` // RFC3339 format
}

// CreateApiKeyResponse represents the response after creating an API key
// The full key is returned ONLY once during creation
type CreateApiKeyResponse struct {
	ID        string `json:"id"`
	Key       string `json:"key"`        // Full key: wiz_<env>_<key_id>_<secret> - shown ONCE
	KeyPrefix string `json:"key_prefix"` // For future reference
	Name      string `json:"name"`
	Message   string `json:"message"`
}

type CreateForUserRequest struct {
	Name string `json:"name" binding:"required,min=1,max=100"`
}

// UpdateApiKeyRequest represents the request body for updating an API key
type UpdateApiKeyRequest struct {
	Name              *string  `json:"name" binding:"omitempty,min=1,max=100"`
	Description       *string  `json:"description" binding:"omitempty,max=500"`
	ScopedPermissions []string `json:"scoped_permissions" binding:"omitempty"`
	IPWhitelist       []string `json:"ip_whitelist" binding:"omitempty"`
	RateLimit         *int     `json:"rate_limit" binding:"omitempty,min=1,max=100000"`
	RateLimitWindow   *int     `json:"rate_limit_window" binding:"omitempty,min=60,max=86400"`
}

// RevokeApiKeyRequest represents the request body for revoking an API key
type RevokeApiKeyRequest struct {
	Reason *string `json:"reason" binding:"omitempty,max=500"`
}

// ApiKeyResponse represents an API key in responses (masked)
type ApiKeyResponse struct {
	ID                string     `json:"id"`
	KeyPrefix         string     `json:"key_prefix"`
	Name              string     `json:"name"`
	Description       *string    `json:"description,omitempty"`
	Environment       string     `json:"environment"`
	ScopedPermissions []string   `json:"scoped_permissions"`
	IPWhitelist       []string   `json:"ip_whitelist,omitempty"`
	RateLimit         int        `json:"rate_limit"`
	RateLimitWindow   int        `json:"rate_limit_window"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	RevokedAt         *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt        *time.Time `json:"last_used_at,omitempty"`
	LastUsedIP        *string    `json:"last_used_ip,omitempty"`
	TotalRequests     int64      `json:"total_requests"`
	IsActive          bool       `json:"is_active"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// ApiKeyListResponse represents a paginated list of API keys (used internally)
type ApiKeyListResponse struct {
	ApiKeys []ApiKeyResponse `json:"api_keys"`
	Total   int64            `json:"total"`
	Page    int              `json:"page"`
	Limit   int              `json:"limit"`
}

// ApiKeyQueryParams represents query parameters for listing API keys
type ApiKeyQueryParams struct {
	Page        int     `form:"page,default=1" binding:"omitempty,min=1"`
	Limit       int     `form:"limit,default=20" binding:"omitempty,min=1,max=100"`
	Environment *string `form:"environment" binding:"omitempty,oneof=live test"`
	IsActive    *bool   `form:"is_active" binding:"omitempty"`
}
