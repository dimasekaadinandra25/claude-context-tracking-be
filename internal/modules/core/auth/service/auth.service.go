package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"claude-usage-server/internal/modules/core/auth/domain"
	"claude-usage-server/internal/modules/core/auth/dto"
	"claude-usage-server/internal/modules/core/auth/repository"
	userRepo "claude-usage-server/internal/modules/core/user/repository"
	"claude-usage-server/pkg/crypto"
	"claude-usage-server/pkg/jwt"
	"claude-usage-server/pkg/logger"
	"claude-usage-server/pkg/token"
	"claude-usage-server/pkg/validator"
)

var (
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrUserNotActive       = errors.New("user is not active")
	ErrUserLocked          = errors.New("user account is locked")
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	ErrRefreshTokenExpired = errors.New("refresh token expired")
	ErrRefreshTokenRevoked = errors.New("refresh token revoked")
	ErrInvalidPassword     = errors.New("current password is incorrect")
)

const (
	MaxFailedLoginAttempts = 5
	LockDuration           = 15 * time.Minute
	RefreshTokenExpiry     = 7 * 24 * time.Hour // 7 days
	AccessTokenExpiryHours = 24
)

// RoleRepository interface for role operations
type RoleRepository interface {
	GetUserPermissions(ctx context.Context, userID string) ([]string, error)
	GetUserRoleNames(ctx context.Context, userID string) ([]string, error)
}

// PermissionReader is the narrow seam GetMe needs from the authz
// package. Implemented by *authz.Service.
type PermissionReader interface {
	GetPermissions(ctx context.Context, userID string) ([]string, error)
}

type AuthService struct {
	userRepo         *userRepo.UserRepository
	refreshTokenRepo *repository.RefreshTokenRepository
	roleRepo         RoleRepository
	permissionReader PermissionReader
}

func NewAuthService(
	userRepo *userRepo.UserRepository,
	refreshTokenRepo *repository.RefreshTokenRepository,
	roleRepo RoleRepository,
	permissionReader PermissionReader,
) *AuthService {
	return &AuthService{
		userRepo:         userRepo,
		refreshTokenRepo: refreshTokenRepo,
		roleRepo:         roleRepo,
		permissionReader: permissionReader,
	}
}

// SignIn authenticates a user and returns tokens
func (s *AuthService) SignIn(ctx context.Context, req *dto.SignInRequest, deviceInfo domain.DeviceInfo, ipAddress string) (*dto.SignInResponse, error) {
	// Find user by email or username
	user, err := s.userRepo.FindByEmailOrUsername(ctx, req.Email)
	if err != nil {
		return nil, err
	}
	if user == nil {
		logger.Warn("User not found", logger.String("login", req.Email))
		return nil, ErrInvalidCredentials
	}

	// Check if user is active
	if !user.IsActive {
		logger.Warn("Inactive user login attempt", logger.String("user_id", user.ID))
		return nil, ErrUserNotActive
	}

	// Check if user is locked
	if user.IsLocked() {
		logger.Warn("Locked user login attempt", logger.String("user_id", user.ID))
		return nil, ErrUserLocked
	}

	// Verify password
	if !crypto.ComparePassword(user.PasswordHash, req.Password) {
		// Increment failed login count
		_ = s.userRepo.IncrementFailedLogin(ctx, user.ID)

		// Lock user if too many failed attempts
		if user.FailedLoginCount+1 >= MaxFailedLoginAttempts {
			lockUntil := time.Now().Add(LockDuration)
			_ = s.userRepo.LockUser(ctx, user.ID, lockUntil)
			logger.Warn("User locked due to failed login attempts", logger.String("user_id", user.ID))
		}

		logger.Warn("Invalid password", logger.String("user_id", user.ID))
		return nil, ErrInvalidCredentials
	}

	// Update last login
	_ = s.userRepo.UpdateLastLogin(ctx, user.ID)

	roles, _ := s.roleRepo.GetUserRoleNames(ctx, user.ID)
	permissions, _ := s.roleRepo.GetUserPermissions(ctx, user.ID)

	fullName := ""
	if user.FullName != nil {
		fullName = *user.FullName
	}
	isSuperAdmin := slices.Contains(roles, jwt.RoleSuperAdmin)
	accessToken, err := jwt.GenerateToken(user.ID, user.Email, user.Username, fullName, isSuperAdmin, roles)
	if err != nil {
		logger.Error("Failed to generate access token", logger.Err(err))
		return nil, err
	}

	// Generate refresh token
	refreshTokenStr, err := token.GenerateSecureToken(32)
	if err != nil {
		logger.Error("Failed to generate refresh token", logger.Err(err))
		return nil, err
	}

	// Store refresh token hash in database
	refreshToken := &domain.RefreshToken{
		ID:         uuid.New().String(),
		UserID:     user.ID,
		TokenHash:  hashToken(refreshTokenStr),
		DeviceInfo: deviceInfo,
		IPAddress:  &ipAddress,
		ExpiresAt:  time.Now().Add(RefreshTokenExpiry),
		CreatedAt:  time.Now(),
	}

	err = s.refreshTokenRepo.Create(ctx, refreshToken)
	if err != nil {
		logger.Error("Failed to store refresh token", logger.Err(err))
		return nil, err
	}

	logger.Info("User signed in",
		logger.String("user_id", user.ID),
		logger.String("email", user.Email))

	return &dto.SignInResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshTokenStr,
		TokenType:    "Bearer",
		ExpiresIn:    AccessTokenExpiryHours * 3600,
		User: dto.UserInfo{
			ID:       user.ID,
			Email:    user.Email,
			Username: user.Username,
			FullName: user.FullName,
		},
		Roles:       roles,
		Permissions: permissions,
	}, nil
}

// RefreshToken refreshes the access token using a refresh token
func (s *AuthService) RefreshToken(ctx context.Context, refreshTokenStr string) (*dto.RefreshTokenResponse, error) {
	tokenHash := hashToken(refreshTokenStr)

	// Find refresh token
	refreshToken, err := s.refreshTokenRepo.FindByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, err
	}
	if refreshToken == nil {
		return nil, ErrInvalidRefreshToken
	}

	// Check if token is revoked
	if refreshToken.IsRevoked() {
		return nil, ErrRefreshTokenRevoked
	}

	// Check if token is expired
	if refreshToken.IsExpired() {
		return nil, ErrRefreshTokenExpired
	}

	// Get user
	user, err := s.userRepo.FindByID(ctx, refreshToken.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil || !user.IsActive {
		return nil, ErrUserNotActive
	}

	// Update last used
	_ = s.refreshTokenRepo.UpdateLastUsed(ctx, refreshToken.ID)

	roles, _ := s.roleRepo.GetUserRoleNames(ctx, user.ID)

	// Generate new access token
	fullName := ""
	if user.FullName != nil {
		fullName = *user.FullName
	}
	isSuperAdmin := slices.Contains(roles, jwt.RoleSuperAdmin)
	accessToken, err := jwt.GenerateToken(user.ID, user.Email, user.Username, fullName, isSuperAdmin, roles)
	if err != nil {
		logger.Error("Failed to generate access token", logger.Err(err))
		return nil, err
	}

	// Generate new refresh token (token rotation)
	newRefreshTokenStr, err := token.GenerateSecureToken(32)
	if err != nil {
		logger.Error("Failed to generate new refresh token", logger.Err(err))
		return nil, err
	}

	// Revoke old refresh token
	_ = s.refreshTokenRepo.Revoke(ctx, refreshToken.ID)

	// Store new refresh token
	newRefreshToken := &domain.RefreshToken{
		ID:         uuid.New().String(),
		UserID:     user.ID,
		TokenHash:  hashToken(newRefreshTokenStr),
		DeviceInfo: refreshToken.DeviceInfo,
		IPAddress:  refreshToken.IPAddress,
		ExpiresAt:  time.Now().Add(RefreshTokenExpiry),
		CreatedAt:  time.Now(),
	}

	err = s.refreshTokenRepo.Create(ctx, newRefreshToken)
	if err != nil {
		logger.Error("Failed to store new refresh token", logger.Err(err))
		return nil, err
	}

	return &dto.RefreshTokenResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshTokenStr,
		TokenType:    "Bearer",
		ExpiresIn:    AccessTokenExpiryHours * 3600,
	}, nil
}

// Logout revokes a refresh token
func (s *AuthService) Logout(ctx context.Context, refreshTokenStr string) error {
	tokenHash := hashToken(refreshTokenStr)

	err := s.refreshTokenRepo.RevokeByTokenHash(ctx, tokenHash)
	if err != nil {
		logger.Error("Failed to revoke refresh token", logger.Err(err))
		return err
	}

	return nil
}

// LogoutAll revokes all refresh tokens for a user
func (s *AuthService) LogoutAll(ctx context.Context, userID string) error {
	err := s.refreshTokenRepo.RevokeAllByUserID(ctx, userID)
	if err != nil {
		logger.Error("Failed to revoke all refresh tokens", logger.Err(err))
		return err
	}

	return nil
}

func (s *AuthService) ChangePassword(ctx context.Context, userID string, req *dto.ChangePasswordRequest) error {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil || !user.IsActive {
		return ErrUserNotActive
	}
	if !crypto.ComparePassword(user.PasswordHash, req.CurrentPassword) {
		return ErrInvalidPassword
	}
	if err := validator.ValidatePassword(req.NewPassword); err != nil {
		return err
	}
	hash, err := crypto.HashPassword(req.NewPassword)
	if err != nil {
		return err
	}
	return s.userRepo.UpdatePassword(ctx, userID, hash, userID)
}

// GetMe returns the signed-in user's current profile, roles, and
// *effective* permissions.
func (s *AuthService) GetMe(ctx context.Context, userID string, isSuperAdmin bool) (*dto.MeResponse, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotActive
	}

	roles, err := s.roleRepo.GetUserRoleNames(ctx, userID)
	if err != nil {
		return nil, err
	}
	permissions, err := s.permissionReader.GetPermissions(ctx, userID)
	if err != nil {
		return nil, err
	}
	if roles == nil {
		roles = []string{}
	}

	return &dto.MeResponse{
		User: dto.UserInfo{
			ID:       user.ID,
			Email:    user.Email,
			Username: user.Username,
			FullName: user.FullName,
		},
		Roles:        roles,
		Permissions:  permissions,
		IsSuperAdmin: isSuperAdmin,
	}, nil
}

// hashToken creates a SHA-256 hash of the token
func hashToken(tokenStr string) string {
	h := sha256.New()
	h.Write([]byte(tokenStr))
	return hex.EncodeToString(h.Sum(nil))
}

// GetDeviceInfoFromUserAgent extracts device info from user agent string
func GetDeviceInfoFromUserAgent(userAgent string) domain.DeviceInfo {
	info := domain.DeviceInfo{}

	ua := strings.ToLower(userAgent)

	// Detect browser
	switch {
	case strings.Contains(ua, "chrome"):
		info.Browser = "Chrome"
	case strings.Contains(ua, "firefox"):
		info.Browser = "Firefox"
	case strings.Contains(ua, "safari"):
		info.Browser = "Safari"
	case strings.Contains(ua, "edge"):
		info.Browser = "Edge"
	default:
		info.Browser = "Unknown"
	}

	// Detect OS
	switch {
	case strings.Contains(ua, "windows"):
		info.OS = "Windows"
	case strings.Contains(ua, "mac"):
		info.OS = "macOS"
	case strings.Contains(ua, "linux"):
		info.OS = "Linux"
	case strings.Contains(ua, "android"):
		info.OS = "Android"
	case strings.Contains(ua, "iphone"), strings.Contains(ua, "ipad"):
		info.OS = "iOS"
	default:
		info.OS = "Unknown"
	}

	// Detect type
	switch {
	case strings.Contains(ua, "mobile"), strings.Contains(ua, "android"), strings.Contains(ua, "iphone"):
		info.Type = "mobile"
	case strings.Contains(ua, "tablet"), strings.Contains(ua, "ipad"):
		info.Type = "tablet"
	default:
		info.Type = "desktop"
	}

	info.Name = info.Browser + " on " + info.OS

	return info
}
