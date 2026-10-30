package jwt

import (
	"github.com/golang-jwt/jwt/v5"
)

// RoleSuperAdmin is the constant for the super admin role code
const RoleSuperAdmin = "super_admin"

// Claims represents custom JWT claims.
//
// Permissions are intentionally NOT embedded here — they are looked up
// on the request path (see internal/shared/authz).
type Claims struct {
	UserID       string   `json:"user_id"`
	Email        string   `json:"email"`
	Username     string   `json:"username"`
	FullName     string   `json:"full_name"`
	IsSuperAdmin bool     `json:"is_super_admin"` // Bypasses all permission checks
	Roles        []string `json:"roles"`

	// ScopedPermissions is a runtime-only permission set used when a
	// request is authenticated via API key. Never serialised into the JWT.
	ScopedPermissions []string `json:"-"`

	jwt.RegisteredClaims
}

// HasScopedPermission is a helper for the API-key flow: true when the
// request-scoped set includes the given permission.
func (c *Claims) HasScopedPermission(permission string) bool {
	for _, p := range c.ScopedPermissions {
		if p == permission {
			return true
		}
	}
	return false
}

// HasRole checks if user has a specific role
func (c *Claims) HasRole(role string) bool {
	for _, r := range c.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// HasAnyRole checks if user has at least one of the specified roles
func (c *Claims) HasAnyRole(roles ...string) bool {
	for _, role := range roles {
		if c.HasRole(role) {
			return true
		}
	}
	return false
}

// HasAllRoles checks if user has all specified roles
func (c *Claims) HasAllRoles(roles ...string) bool {
	for _, role := range roles {
		if !c.HasRole(role) {
			return false
		}
	}
	return true
}
