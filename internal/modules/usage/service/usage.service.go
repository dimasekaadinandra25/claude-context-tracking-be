package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"claude-usage-server/internal/modules/usage/domain"
	"claude-usage-server/internal/modules/usage/dto"
	"claude-usage-server/internal/modules/usage/repository"
)

const (
	maxHours        = 20000
	maxIDLength     = 100
	maxModelSize    = 100
	maxMachineSize  = 255
	maxToolName     = 100
	maxPercent      = 100
	maxClockSkew    = 24 * time.Hour
	maxAccountEmail = 320
	maxAccountName  = 100
	maxModelLimits  = 20
)

var (
	appVersionPattern = regexp.MustCompile(`^[0-9A-Za-z.+_-]{1,32}$`)
	versionNumber     = regexp.MustCompile(`\d+`)
)

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
		owned, err := s.repo.SaveSession(ctx, toSession(userID, req.Machine, p), p.Overhead, toHours(p.Hours), toTools(p.Tools))
		if err != nil {
			return dto.IngestResponse{}, err
		}
		if !owned {
			res.Rejected = append(res.Rejected, dto.Rejection{SessionID: p.SessionID, Reason: "session belongs to another user"})
			continue
		}
		res.Accepted++
	}
	if limits, ok := decodePlanLimits(req.PlanLimits, time.Now()); ok {
		if err := s.repo.SavePlanLimits(ctx, userID, limits); err != nil {
			return dto.IngestResponse{}, err
		}
	}
	if account, ok := decodeAccount(req.Account); ok {
		if err := s.repo.SaveClaudeAccount(ctx, userID, account); err != nil {
			return dto.IngestResponse{}, err
		}
	}
	return res, nil
}

func decodeAccount(raw json.RawMessage) (domain.ClaudeAccount, bool) {
	var a domain.ClaudeAccount
	if len(raw) == 0 || json.Unmarshal(raw, &a) != nil {
		return a, false
	}
	a.Email, a.Plan, a.Tier = strings.TrimSpace(a.Email), strings.TrimSpace(a.Plan), strings.TrimSpace(a.Tier)
	if a.Email == "" && a.Tier == "" {
		return a, false
	}
	return a, accountField(a.Email, maxAccountEmail) && accountField(a.Plan, maxAccountName) && accountField(a.Tier, maxAccountName)
}

func accountField(value string, max int) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= max && !strings.ContainsRune(value, 0)
}

func decodePlanLimits(raw json.RawMessage, now time.Time) (domain.PlanLimits, bool) {
	var limits domain.PlanLimits
	if len(raw) == 0 || json.Unmarshal(raw, &limits) != nil {
		return limits, false
	}
	if limits.FiveHour == nil && limits.SevenDay == nil {
		return limits, false
	}
	if limits.CapturedAt <= 0 || limits.CapturedAt > now.Add(maxClockSkew).Unix() {
		return limits, false
	}
	limits.Models = validModelLimits(limits.Models)
	return limits, validWindow(limits.FiveHour) && validWindow(limits.SevenDay)
}

func validModelLimits(models []domain.ModelLimit) []domain.ModelLimit {
	valid := []domain.ModelLimit{}
	for _, m := range models {
		m.Name = strings.TrimSpace(m.Name)
		if len(valid) < maxModelLimits && accountField(m.Name, maxModelSize) && m.Name != "" &&
			validWindow(&domain.PlanWindow{UsedPercentage: m.UsedPercentage, ResetsAt: m.ResetsAt}) {
			valid = append(valid, m)
		}
	}
	return valid
}

func validWindow(w *domain.PlanWindow) bool {
	return w == nil || (w.UsedPercentage >= 0 && w.UsedPercentage <= maxPercent && w.ResetsAt > 0)
}

func (s *Service) RecordAppVersion(ctx context.Context, keyID, version string) error {
	version = strings.TrimSpace(version)
	if !appVersionPattern.MatchString(version) {
		return nil
	}
	return s.repo.SetAppVersion(ctx, keyID, version)
}

const (
	monthsPerYear  = 12
	weeksPerYear   = 52
	million        = 1_000_000
	percentOfWhole = 100
)

var planPrices = []struct {
	tier  string
	price float64
}{
	{"max_20x", 200},
	{"max_5x", 100},
	{"pro", 20},
}

func planPrice(tier string) (float64, bool) {
	for _, p := range planPrices {
		if strings.Contains(tier, p.tier) {
			return p.price, true
		}
	}
	return 0, false
}

func costPerMillion(used float64, tier string, weight int64) float64 {
	price, ok := planPrice(tier)
	if !ok || weight <= 0 || used <= 0 {
		return 0
	}
	weekly := price * monthsPerYear / weeksPerYear
	return used / percentOfWhole * weekly / float64(weight) * million
}

func (s *Service) CostPerMillion(ctx context.Context, userID string) (float64, error) {
	used, tier, weight, ok, err := s.repo.AccountWeek(ctx, userID)
	if err != nil || !ok {
		return 0, err
	}
	return costPerMillion(used, tier, weight), nil
}

func (s *Service) LatestAppVersion(ctx context.Context) (string, error) {
	versions, err := s.repo.AppVersions(ctx)
	if err != nil {
		return "", err
	}
	return newestVersion(versions), nil
}

func newestVersion(versions []string) string {
	newest := ""
	for _, v := range versions {
		if newest == "" || slices.Compare(versionParts(v), versionParts(newest)) > 0 {
			newest = v
		}
	}
	return newest
}

func versionParts(version string) []int {
	parts := []int{}
	for _, number := range versionNumber.FindAllString(version, -1) {
		n, _ := strconv.Atoi(number)
		parts = append(parts, n)
	}
	return parts
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
	numbers := []int64{s.ContextWindow, s.StartContext, s.LatestContext, s.PeakContext, s.NewContext, int64(s.Requests), int64(s.Compactions)}
	if o := s.Overhead; o != nil {
		numbers = append(numbers, o.SystemTokens, o.MemoryTokens, o.AgentsTokens, o.SkillsTokens)
	}
	for _, n := range numbers {
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
	return validateTools(s.Tools)
}

func validateTools(tools []dto.ToolPayload) error {
	if len(tools) > maxHours {
		return fmt.Errorf("at most %d tool rows per session", maxHours)
	}
	type toolKey struct {
		hour int64
		tool string
	}
	seen := make(map[toolKey]bool, len(tools))
	for _, t := range tools {
		u := t.Hour.UTC()
		if t.Hour.IsZero() || u.Minute() != 0 || u.Second() != 0 || u.Nanosecond() != 0 {
			return errors.New("hour must be a whole UTC hour")
		}
		if n := utf8.RuneCountInString(t.Tool); n < 1 || n > maxToolName {
			return fmt.Errorf("tool must be 1-%d characters", maxToolName)
		}
		if strings.ContainsRune(t.Tool, 0) {
			return errors.New("tool must not contain NUL characters")
		}
		if t.Calls < 0 || t.LinesAdded < 0 || t.LinesRemoved < 0 || t.Commits < 0 {
			return errors.New("numbers must be >= 0")
		}
		k := toolKey{u.Unix(), t.Tool}
		if seen[k] {
			return errors.New("duplicate hour and tool")
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

func toTools(in []dto.ToolPayload) []domain.ToolUsage {
	if in == nil {
		return nil
	}
	out := make([]domain.ToolUsage, len(in))
	for i, t := range in {
		out[i] = domain.ToolUsage{
			Hour:         t.Hour.UTC(),
			Tool:         t.Tool,
			Calls:        t.Calls,
			LinesAdded:   t.LinesAdded,
			LinesRemoved: t.LinesRemoved,
			Commits:      t.Commits,
		}
	}
	return out
}
