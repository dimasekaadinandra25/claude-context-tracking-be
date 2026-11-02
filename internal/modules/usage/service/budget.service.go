package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"claude-usage-server/internal/modules/usage/dto"
)

var ErrUserNotFound = errors.New("user not found")

func (s *Service) Budgets(ctx context.Context, c dto.Caller, tz string) ([]dto.Budget, error) {
	return s.budgets(ctx, c, tz, time.Now())
}

func (s *Service) budgets(ctx context.Context, c dto.Caller, tz string, now time.Time) ([]dto.Budget, error) {
	if tz == "" {
		tz = "UTC"
	}
	loc, err := location(tz)
	if err != nil {
		return nil, err
	}
	y, m, _ := now.In(loc).Date()
	start := time.Date(y, m, 1, 0, 0, 0, 0, loc)
	list, err := s.repo.Budgets(ctx, s.scope(c, ""), start, now)
	for i := range list {
		list[i].MonthStart = start
	}
	return list, err
}

func (s *Service) SetBudget(ctx context.Context, c dto.Caller, userID string, monthlyNewContext int64) error {
	return s.changeBudget(c, userID, func() (bool, error) {
		return s.repo.SetBudget(ctx, userID, monthlyNewContext)
	})
}

func (s *Service) DeleteBudget(ctx context.Context, c dto.Caller, userID string) error {
	return s.changeBudget(c, userID, func() (bool, error) {
		return s.repo.DeleteBudget(ctx, userID)
	})
}

func (s *Service) changeBudget(c dto.Caller, userID string, change func() (bool, error)) error {
	if !c.IsAdmin {
		return ErrForbidden
	}
	if uuid.Validate(userID) != nil {
		return ErrUserNotFound
	}
	found, err := change()
	if err == nil && !found {
		return ErrUserNotFound
	}
	return err
}
