package service

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"claude-usage-server/internal/modules/usage/dto"
)

var alertWords = []string{"pace", "summary", "stale", "runaway", "project", "budget", "version"}

func (s *Service) AlertSettings(ctx context.Context, c dto.Caller) (*dto.AlertSettings, error) {
	if !c.ViewAll {
		return nil, ErrForbidden
	}
	return s.repo.AlertSettings(ctx)
}

func (s *Service) SetAlertSettings(ctx context.Context, c dto.Caller, req dto.AlertSettings) (dto.AlertSettings, error) {
	if !c.IsAdmin {
		return req, ErrForbidden
	}
	alerts, err := normalizeAlerts(*req.Alerts)
	if err != nil {
		return req, err
	}
	req.Alerts = &alerts
	return req, s.repo.SetAlertSettings(ctx, c.UserID, req)
}

func normalizeAlerts(text string) (string, error) {
	parts := []string{}
	split := func(r rune) bool { return r == ',' || unicode.IsSpace(r) }
	for _, part := range strings.FieldsFunc(strings.ToLower(text), split) {
		if !slices.Contains(alertWords, part) {
			n, err := strconv.Atoi(part)
			if err != nil || n < 1 || n > maxPercent {
				return "", fmt.Errorf("%w: %q is not an alert word or a percent from 1 to 100", ErrInvalidInput, part)
			}
			part = strconv.Itoa(n)
		}
		if !slices.Contains(parts, part) {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, ","), nil
}
