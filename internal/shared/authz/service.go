package authz

import (
	"context"
	"fmt"
	"slices"
)

type PermissionFetcher interface {
	GetUserPermissions(ctx context.Context, userID string) ([]string, error)
}

type Service struct {
	fetcher PermissionFetcher
}

func NewService(fetcher PermissionFetcher) *Service {
	return &Service{fetcher: fetcher}
}

func (s *Service) GetPermissions(ctx context.Context, userID string) ([]string, error) {
	if userID == "" {
		return nil, fmt.Errorf("authz: empty userID")
	}
	perms, err := s.fetcher.GetUserPermissions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("authz: fetch user permissions: %w", err)
	}
	if perms == nil {
		perms = []string{}
	}
	return perms, nil
}

func (s *Service) Has(ctx context.Context, userID, permission string) (bool, error) {
	perms, err := s.GetPermissions(ctx, userID)
	if err != nil {
		return false, err
	}
	return slices.Contains(perms, permission), nil
}

func (s *Service) HasAny(ctx context.Context, userID string, permissions ...string) (bool, error) {
	perms, err := s.GetPermissions(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, need := range permissions {
		if slices.Contains(perms, need) {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) HasAll(ctx context.Context, userID string, permissions ...string) (bool, error) {
	perms, err := s.GetPermissions(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, need := range permissions {
		if !slices.Contains(perms, need) {
			return false, nil
		}
	}
	return true, nil
}
