package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"claude-usage-server/internal/modules/core/role/domain"
	"claude-usage-server/internal/modules/core/role/dto"
	"claude-usage-server/internal/modules/core/role/repository"
	"claude-usage-server/pkg/logger"
)

var (
	ErrRoleNotFound       = errors.New("role not found")
	ErrCodeAlreadyExists  = errors.New("role code already exists")
	ErrCannotModifySystem = errors.New("cannot modify system role")
	ErrCannotDeleteSystem = errors.New("cannot delete system role")
	ErrInvalidPermissions = errors.New("invalid permissions format")
)

type RoleService struct {
	roleRepo *repository.RoleRepository
}

func NewRoleService(roleRepo *repository.RoleRepository) *RoleService {
	return &RoleService{
		roleRepo: roleRepo,
	}
}

// GetAll returns paginated list of roles
func (s *RoleService) GetAll(ctx context.Context, params *dto.RoleQueryParams) (*dto.RoleListResponse, error) {
	if params.Page == 0 {
		params.Page = 1
	}
	if params.Limit == 0 {
		params.Limit = 10
	}

	roles, total, err := s.roleRepo.FindAll(ctx, params)
	if err != nil {
		return nil, err
	}

	roleResponses := make([]dto.RoleResponse, len(roles))
	for i, role := range roles {
		roleResponses[i] = s.toRoleResponse(&role)
	}

	return &dto.RoleListResponse{
		Roles: roleResponses,
		Total: total,
		Page:  params.Page,
		Limit: params.Limit,
	}, nil
}

// GetByID returns a role by ID
func (s *RoleService) GetByID(ctx context.Context, id string) (*dto.RoleResponse, error) {
	role, err := s.roleRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, ErrRoleNotFound
	}

	response := s.toRoleResponse(role)
	return &response, nil
}

// Create creates a new role
func (s *RoleService) Create(ctx context.Context, req *dto.CreateRoleRequest, createdBy string) (*dto.RoleResponse, error) {
	// Validate permissions
	if err := s.validatePermissions(req.Permissions); err != nil {
		return nil, err
	}

	// Check code uniqueness
	exists, err := s.roleRepo.CodeExists(ctx, req.Code, nil)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrCodeAlreadyExists
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	now := time.Now()
	role := &domain.Role{
		ID:          uuid.New().String(),
		Code:        req.Code,
		Name:        req.Name,
		Description: req.Description,
		Permissions: domain.Permissions(req.Permissions),
		IsSystem:    false,
		IsActive:    isActive,
		CreatedAt:   now,
		CreatedBy:   &createdBy,
		UpdatedAt:   now,
		UpdatedBy:   &createdBy,
	}

	err = s.roleRepo.Create(ctx, role)
	if err != nil {
		return nil, err
	}

	logger.Info("Role created",
		logger.String("role_id", role.ID),
		logger.String("code", role.Code))

	response := s.toRoleResponse(role)
	return &response, nil
}

// Update updates a role. Super admins may edit system roles.
func (s *RoleService) Update(ctx context.Context, id string, req *dto.UpdateRoleRequest, updatedBy string, isSuperAdmin bool) (*dto.RoleResponse, error) {
	role, err := s.roleRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, ErrRoleNotFound
	}

	// Cannot modify system roles unless caller is super admin
	if role.IsSystem && !isSuperAdmin {
		return nil, ErrCannotModifySystem
	}

	if req.Name != nil {
		role.Name = *req.Name
	}
	if req.Description != nil {
		role.Description = req.Description
	}
	if req.Permissions != nil {
		if err := s.validatePermissions(req.Permissions); err != nil {
			return nil, err
		}
		role.Permissions = domain.Permissions(req.Permissions)
	}
	if req.IsActive != nil {
		role.IsActive = *req.IsActive
	}

	role.UpdatedAt = time.Now()
	role.UpdatedBy = &updatedBy

	err = s.roleRepo.Update(ctx, role)
	if err != nil {
		return nil, err
	}

	response := s.toRoleResponse(role)
	return &response, nil
}

// UpdatePermissions updates only the permissions of a role. Super admins may edit system roles.
func (s *RoleService) UpdatePermissions(ctx context.Context, id string, req *dto.UpdatePermissionsRequest, updatedBy string, isSuperAdmin bool) (*dto.RoleResponse, error) {
	role, err := s.roleRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, ErrRoleNotFound
	}

	// Cannot modify system roles unless caller is super admin
	if role.IsSystem && !isSuperAdmin {
		return nil, ErrCannotModifySystem
	}

	// Validate permissions
	if err := s.validatePermissions(req.Permissions); err != nil {
		return nil, err
	}

	err = s.roleRepo.UpdatePermissions(ctx, id, domain.Permissions(req.Permissions), updatedBy)
	if err != nil {
		return nil, err
	}

	// Refresh role data
	role, err = s.roleRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	response := s.toRoleResponse(role)
	return &response, nil
}

// Delete soft deletes a role. System roles cannot be deleted by anyone.
func (s *RoleService) Delete(ctx context.Context, id, deletedBy string) error {
	role, err := s.roleRepo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if role == nil {
		return ErrRoleNotFound
	}

	// Cannot delete system roles
	if role.IsSystem {
		return ErrCannotDeleteSystem
	}

	return s.roleRepo.SoftDelete(ctx, id, deletedBy)
}

// AssignRoleToUser assigns a role to a user
func (s *RoleService) AssignRoleToUser(ctx context.Context, req *dto.AssignRoleRequest, createdBy string) error {
	// Verify role exists
	role, err := s.roleRepo.FindByID(ctx, req.RoleID)
	if err != nil {
		return err
	}
	if role == nil {
		return ErrRoleNotFound
	}

	return s.roleRepo.AssignRoleToUser(ctx, req.UserID, req.RoleID, createdBy)
}

// RemoveRoleFromUser removes a role from a user
func (s *RoleService) RemoveRoleFromUser(ctx context.Context, req *dto.RemoveRoleRequest) error {
	return s.roleRepo.RemoveRoleFromUser(ctx, req.UserID, req.RoleID)
}

// GetUserRoles gets all roles for a user
func (s *RoleService) GetUserRoles(ctx context.Context, userID string) ([]dto.RoleResponse, error) {
	roles, err := s.roleRepo.GetUserRoles(ctx, userID)
	if err != nil {
		return nil, err
	}

	responses := make([]dto.RoleResponse, len(roles))
	for i, role := range roles {
		responses[i] = s.toRoleResponse(&role)
	}

	return responses, nil
}

// GetUserPermissions gets all permissions for a user (expanded to actions)
func (s *RoleService) GetUserPermissions(ctx context.Context, userID string) ([]string, error) {
	return s.roleRepo.GetUserPermissions(ctx, userID)
}

// validatePermissions validates the level-based permissions format
func (s *RoleService) validatePermissions(permissions map[string]string) error {
	if permissions == nil {
		return ErrInvalidPermissions
	}

	for resource, level := range permissions {
		if resource == "" {
			return ErrInvalidPermissions
		}
		if !domain.IsValidLevel(level) {
			return ErrInvalidPermissions
		}
	}

	return nil
}

// toRoleResponse converts domain.Role to dto.RoleResponse
func (s *RoleService) toRoleResponse(role *domain.Role) dto.RoleResponse {
	return dto.RoleResponse{
		ID:          role.ID,
		Code:        role.Code,
		Name:        role.Name,
		Description: role.Description,
		Permissions: map[string]string(role.Permissions),
		IsSystem:    role.IsSystem,
		IsActive:    role.IsActive,
		CreatedBy:   role.CreatedBy,
		CreatedAt:   role.CreatedAt,
		UpdatedAt:   role.UpdatedAt,
	}
}
