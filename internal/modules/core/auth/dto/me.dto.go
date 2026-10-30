package dto

// MeResponse is the shape returned by GET /core/v1/auth/me.
//
// It is intentionally shaped like SignInResponse minus the tokens.
// `permissions` reflects the caller's *current* effective set — not a
// snapshot of what the JWT was minted with.
type MeResponse struct {
	User        UserInfo `json:"user"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`

	// IsSuperAdmin mirrors the JWT claim of the same name. Exposed here
	// so FE can render admin-only UI without decoding the JWT. Super
	// admins effectively bypass `permissions` on the backend.
	IsSuperAdmin bool `json:"is_super_admin"`
}
