package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"claude-usage-server/internal/modules/usage/domain"
	"claude-usage-server/internal/modules/usage/dto"
	"claude-usage-server/internal/modules/usage/repository"
)

const (
	maxHours       = 20000
	maxIDLength    = 100
	maxModelSize   = 100
	maxMachineSize = 255
)

var appVersionPattern = regexp.MustCompile(`^[0-9A-Za-z.+_-]{1,32}$`)

type Service struct {
	repo *repository.Repository
}

func New(repo *repository.Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Ingest(ctx context.Context, userID string, req dto.IngestRequest) (dto.IngestResponse, error) {
	res := dto.IngestResponse{Rejected: []dto.Rejection{}}
	for _, raw := range req.Sessions {
		p, err := decode(raw)
		if err == nil {
			err = checkMachine(req.Machine)
		}
		if err != nil {
			res.Rejected = append(res.Rejected, dto.Rejection{SessionID: p.SessionID, Reason: err.Error()})
			continue
		}
		owned, err := s.repo.SaveSession(ctx, toSession(userID, req.Machine, p), toHours(p.Hours))
		if err != nil {
			return dto.IngestResponse{}, err
		}
		if !owned {
			res.Rejected = append(res.Rejected, dto.Rejection{SessionID: p.SessionID, Reason: "session belongs to another user"})
			continue
		}
		res.Accepted++
	}
	return res, nil
}

func (s *Service) RecordAppVersion(ctx context.Context, keyID, version string) error {
	version = strings.TrimSpace(version)
	if !appVersionPattern.MatchString(version) {
		return nil
	}
	return s.repo.SetAppVersion(ctx, keyID, version)
}

func decode(raw json.RawMessage) (dto.SessionPayload, error) {
	var p dto.SessionPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		var id struct {
			SessionID string `json:"session_id"`
		}
		_ = json.Unmarshal(raw, &id)
		return dto.SessionPayload{SessionID: id.SessionID}, err
	}
	return p, validate(p)
}

func checkMachine(machine string) error {
	if utf8.RuneCountInString(machine) > maxMachineSize {
		return fmt.Errorf("machine must be at most %d characters", maxMachineSize)
	}
	if strings.ContainsRune(machine, 0) {
		return errors.New("machine must not contain NUL characters")
	}
	return nil
}

func validate(s dto.SessionPayload) error {
	if n := utf8.RuneCountInString(s.SessionID); n < 1 || n > maxIDLength {
		return fmt.Errorf("session_id must be 1-%d characters", maxIDLength)
	}
	if err := checkModel(s.Model); err != nil {
		return err
	}
	for field, v := range map[string]string{"session_id": s.SessionID, "project": s.Project, "title": s.Title} {
		if strings.ContainsRune(v, 0) {
			return fmt.Errorf("%s must not contain NUL characters", field)
		}
	}
	for _, n := range []int64{s.ContextWindow, s.StartContext, s.LatestContext, s.PeakContext, s.NewContext, int64(s.Requests), int64(s.Compactions)} {
		if n < 0 {
			return errors.New("numbers must be >= 0")
		}
	}
	if s.FirstAt.IsZero() || s.LatestAt.IsZero() {
		return errors.New("first_at and latest_at are required")
	}
	if s.FirstAt.After(s.LatestAt) {
		return errors.New("first_at must not be after latest_at")
	}
	if len(s.Hours) > maxHours {
		return fmt.Errorf("at most %d hours per session", maxHours)
	}
	type hourKey struct {
		hour  int64
		model string
	}
	seen := make(map[hourKey]bool, len(s.Hours))
	for _, h := range s.Hours {
		u := h.Hour.UTC()
		if h.Hour.IsZero() || u.Minute() != 0 || u.Second() != 0 || u.Nanosecond() != 0 {
			return errors.New("hour must be a whole UTC hour")
		}
		if err := checkModel(h.Model); err != nil {
			return err
		}
		if h.NewContext < 0 || h.Requests < 0 || h.OutputTokens < 0 || h.SubagentRequests < 0 || h.SubagentOutputTokens < 0 ||
			h.InputTokens < 0 || h.CacheWriteTokens < 0 || h.CacheReadTokens < 0 {
			return errors.New("numbers must be >= 0")
		}
		k := hourKey{u.Unix(), h.Model}
		if seen[k] {
			return errors.New("duplicate hour and model")
		}
		seen[k] = true
	}
	return nil
}

func checkModel(model string) error {
	if n := utf8.RuneCountInString(model); n < 1 || n > maxModelSize {
		return fmt.Errorf("model must be 1-%d characters", maxModelSize)
	}
	if strings.ContainsRune(model, 0) {
		return errors.New("model must not contain NUL characters")
	}
	return nil
}

func toSession(userID, machine string, p dto.SessionPayload) domain.Session {
	return domain.Session{
		ID:             p.SessionID,
		UserID:         userID,
		Machine:        machine,
		Project:        p.Project,
		Title:          p.Title,
		Model:          p.Model,
		ContextWindow:  p.ContextWindow,
		StartContext:   p.StartContext,
		LatestContext:  p.LatestContext,
		PeakContext:    p.PeakContext,
		NewContext:     p.NewContext,
		Requests:       p.Requests,
		Compactions:    p.Compactions,
		FirstRequestAt: p.FirstAt,
		LastRequestAt:  p.LatestAt,
	}
}

func toHours(in []dto.HourPayload) []domain.HourUsage {
	out := make([]domain.HourUsage, len(in))
	for i, h := range in {
		out[i] = domain.HourUsage{
			Hour:                 h.Hour.UTC(),
			Model:                h.Model,
			NewContext:           h.NewContext,
			Requests:             h.Requests,
			OutputTokens:         h.OutputTokens,
			SubagentRequests:     h.SubagentRequests,
			SubagentOutputTokens: h.SubagentOutputTokens,
			InputTokens:          h.InputTokens,
			CacheWriteTokens:     h.CacheWriteTokens,
			CacheReadTokens:      h.CacheReadTokens,
		}
	}
	return out
}
