package domain

import "time"

// DeviceInfo represents device information stored as JSONB
type DeviceInfo struct {
	Name    string `json:"name,omitempty"`
	Type    string `json:"type,omitempty"`
	OS      string `json:"os,omitempty"`
	Browser string `json:"browser,omitempty"`
}

// RefreshToken represents a refresh token entity
type RefreshToken struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	TokenHash  string     `json:"-"`
	DeviceInfo DeviceInfo `json:"device_info"`
	IPAddress  *string    `json:"ip_address,omitempty"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// IsExpired checks if the token is expired
func (r *RefreshToken) IsExpired() bool {
	return time.Now().After(r.ExpiresAt)
}

// IsRevoked checks if the token is revoked
func (r *RefreshToken) IsRevoked() bool {
	return r.RevokedAt != nil
}

// IsValid checks if the token is valid (not expired and not revoked)
func (r *RefreshToken) IsValid() bool {
	return !r.IsExpired() && !r.IsRevoked()
}
