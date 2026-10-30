package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"claude-usage-server/internal/modules/core/user/domain"
	"claude-usage-server/internal/modules/core/user/dto"
	"claude-usage-server/internal/modules/core/user/repository"
	"claude-usage-server/pkg/crypto"
	"claude-usage-server/pkg/logger"
)

var (
	ErrUserNotFound          = errors.New("user not found")
	ErrEmailAlreadyExists    = errors.New("email already exists")
	ErrUsernameAlreadyExists = errors.New("username already exists")
	ErrInvalidPassword       = errors.New("invalid password")
	ErrDeactivateSelf        = errors.New("cannot deactivate yourself")
)

const memberRoleID = "00000000-0000-0000-0000-000000000003"

type KeyRevoker interface {
	RevokeAllForUser(ctx context.Context, actorID, userID, reason string) error
}

type UserService struct {
	userRepo   *repository.UserRepository
	keyRevoker KeyRevoker
}

func (s *UserService) SetKeyRevoker(r KeyRevoker) {
	s.keyRevoker = r
}

func NewUserService(userRepo *repository.UserRepository) *UserService {
	return &UserService{
		userRepo: userRepo,
	}
}

// GetAll returns paginated list of users
func (s *UserService) GetAll(ctx context.Context, params *dto.UserQueryParams) (*dto.UserListResponse, error) {
	if params.Page == 0 {
		params.Page = 1
	}
	if params.Limit == 0 {
		params.Limit = 10
	}

	users, total, err := s.userRepo.FindAll(ctx, params)
	if err != nil {
		return nil, err
	}

	ids := make([]string, len(users))
	for i, user := range users {
		ids[i] = user.ID
	}
	roles, err := s.userRepo.RoleCodes(ctx, ids)
	if err != nil {
		return nil, err
	}

	userResponses := make([]dto.UserResponse, len(users))
	for i, user := range users {
		userResponses[i] = s.toUserResponse(&user)
		userResponses[i].Roles = roles[user.ID]
	}

	return &dto.UserListResponse{
		Users: userResponses,
		Total: total,
		Page:  params.Page,
		Limit: params.Limit,
	}, nil
}

// GetByID returns a user by ID
func (s *UserService) GetByID(ctx context.Context, id string) (*dto.UserResponse, error) {
	user, err := s.userRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	response := s.toUserResponse(user)
	return &response, nil
}

// Create creates a new user
func (s *UserService) Create(ctx context.Context, req *dto.CreateUserRequest, createdBy string) (*dto.UserResponse, error) {
	// Check email uniqueness
	exists, err := s.userRepo.EmailExists(ctx, req.Email, nil)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrEmailAlreadyExists
	}

	// Check username uniqueness
	exists, err = s.userRepo.UsernameExists(ctx, req.Username, nil)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrUsernameAlreadyExists
	}

	// Hash password
	passwordHash, err := crypto.HashPassword(req.Password)
	if err != nil {
		logger.Error("Failed to hash password", logger.Err(err))
		return nil, err
	}

	now := time.Now()
	user := &domain.User{
		ID:              uuid.New().String(),
		Email:           req.Email,
		Username:        req.Username,
		PasswordHash:    passwordHash,
		FullName:        req.FullName,
		Phone:           req.Phone,
		IsActive:        true,
		IsEmailVerified: true,
		CreatedAt:       now,
		CreatedBy:       &createdBy,
		UpdatedAt:       now,
		UpdatedBy:       &createdBy,
	}

	tx, err := s.userRepo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	// Rollback is a no-op after a successful Commit, so it's always safe to
	// defer it unconditionally.
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.userRepo.CreateTx(ctx, tx, user); err != nil {
		return nil, err
	}

	if req.RoleID != nil {
		if err := s.userRepo.AssignRoleTx(ctx, tx, user.ID, *req.RoleID, createdBy); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	response := s.toUserResponse(user)
	return &response, nil
}

func (s *UserService) CreateMember(ctx context.Context, req *dto.CreateUserRequest, createdBy string) (*dto.UserResponse, error) {
	roleID := memberRoleID
	req.RoleID = &roleID
	return s.Create(ctx, req, createdBy)
}

func (s *UserService) SetActive(ctx context.Context, actorID, id string, active bool) error {
	if !active && id == actorID {
		return ErrDeactivateSelf
	}
	if _, err := s.Update(ctx, id, &dto.UpdateUserRequest{IsActive: &active}, actorID); err != nil {
		return err
	}
	if active {
		return nil
	}
	return s.keyRevoker.RevokeAllForUser(ctx, actorID, id, "user deactivated")
}

func (s *UserService) ResetPassword(ctx context.Context, actorID, id, password string) error {
	user, err := s.userRepo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}
	hash, err := crypto.HashPassword(password)
	if err != nil {
		return err
	}
	return s.userRepo.ResetPassword(ctx, id, hash, actorID)
}

// Update updates a user
func (s *UserService) Update(ctx context.Context, id string, req *dto.UpdateUserRequest, updatedBy string) (*dto.UserResponse, error) {
	user, err := s.userRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	// Check email uniqueness if changed
	if req.Email != nil && *req.Email != user.Email {
		exists, err := s.userRepo.EmailExists(ctx, *req.Email, &id)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, ErrEmailAlreadyExists
		}
		user.Email = *req.Email
	}

	// Check username uniqueness if changed
	if req.Username != nil && *req.Username != user.Username {
		exists, err := s.userRepo.UsernameExists(ctx, *req.Username, &id)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, ErrUsernameAlreadyExists
		}
		user.Username = *req.Username
	}

	if req.FullName != nil {
		user.FullName = req.FullName
	}
	if req.Phone != nil {
		user.Phone = req.Phone
	}
	if req.AvatarURL != nil {
		user.AvatarURL = req.AvatarURL
	}
	if req.IsActive != nil {
		user.IsActive = *req.IsActive
	}

	user.UpdatedAt = time.Now()
	user.UpdatedBy = &updatedBy

	err = s.userRepo.Update(ctx, user)
	if err != nil {
		return nil, err
	}

	response := s.toUserResponse(user)
	return &response, nil
}

// GetMe returns current user profile
func (s *UserService) GetMe(ctx context.Context, userID string) (*dto.UserResponse, error) {
	return s.GetByID(ctx, userID)
}

// UpdateMe updates current user profile
func (s *UserService) UpdateMe(ctx context.Context, userID string, req *dto.UpdateMeRequest) (*dto.UserResponse, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	if req.FullName != nil {
		user.FullName = req.FullName
	}
	if req.Phone != nil {
		user.Phone = req.Phone
	}
	if req.AvatarURL != nil {
		user.AvatarURL = req.AvatarURL
	}

	user.UpdatedAt = time.Now()
	user.UpdatedBy = &userID

	err = s.userRepo.Update(ctx, user)
	if err != nil {
		return nil, err
	}

	response := s.toUserResponse(user)
	return &response, nil
}

// ChangePassword changes user password
func (s *UserService) ChangePassword(ctx context.Context, userID string, req *dto.ChangePasswordRequest) error {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}

	// Verify current password
	if !crypto.ComparePassword(user.PasswordHash, req.CurrentPassword) {
		return ErrInvalidPassword
	}

	// Hash new password
	newPasswordHash, err := crypto.HashPassword(req.NewPassword)
	if err != nil {
		logger.Error("Failed to hash new password", logger.Err(err))
		return err
	}

	return s.userRepo.UpdatePassword(ctx, userID, newPasswordHash, userID)
}

// toUserResponse converts domain.User to dto.UserResponse
func (s *UserService) toUserResponse(user *domain.User) dto.UserResponse {
	return dto.UserResponse{
		ID:              user.ID,
		Email:           user.Email,
		Username:        user.Username,
		FullName:        user.FullName,
		Phone:           user.Phone,
		AvatarURL:       user.AvatarURL,
		IsActive:        user.IsActive,
		IsEmailVerified: user.IsEmailVerified,
		LastLoginAt:     user.LastLoginAt,
		CreatedAt:       user.CreatedAt,
		UpdatedAt:       user.UpdatedAt,
	}
}
