package service

import (
	"context"
	"errors"
	"fmt"
	"time"
	_ "time/tzdata"

	"github.com/google/uuid"

	"claude-usage-server/internal/modules/usage/dto"
	"claude-usage-server/internal/modules/usage/repository"
)

const (
	defaultTZ    = "Asia/Jakarta"
	defaultRange = 7 * 24 * time.Hour
	defaultSort  = "-last_request_at"
)

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrNotFound     = errors.New("session not found")
	ErrForbidden    = errors.New("admins only")
)

func (s *Service) scope(c dto.Caller, requested string) string {
	if c.IsAdmin {
		return requested
	}
	return c.UserID
}

func location(tz string) (*time.Location, error) {
	loc, err := time.LoadLocation(tz)
	if err != nil || tz == "Local" {
		return nil, fmt.Errorf("%w: unknown tz %q", ErrInvalidInput, tz)
	}
	return loc, nil
}

func (s *Service) prepare(c dto.Caller, q dto.ReportQuery) (dto.ReportQuery, *time.Location, error) {
	q.UserID = s.scope(c, q.UserID)
	if q.UserID != "" {
		if err := uuid.Validate(q.UserID); err != nil {
			return q, nil, fmt.Errorf("%w: user_id must be a UUID", ErrInvalidInput)
		}
	}
	if q.TZ == "" {
		q.TZ = defaultTZ
	}
	loc, err := location(q.TZ)
	if err != nil {
		return q, nil, err
	}
	if q.To.IsZero() {
		q.To = time.Now()
	}
	if q.From.IsZero() {
		q.From = q.To.Add(-defaultRange)
	}
	if !q.From.Before(q.To) {
		return q, nil, fmt.Errorf("%w: from must be before to", ErrInvalidInput)
	}
	return q, loc, nil
}

func (s *Service) Overview(ctx context.Context, c dto.Caller, q dto.ReportQuery) (dto.Overview, error) {
	q, _, err := s.prepare(c, q)
	if err != nil {
		return dto.Overview{}, err
	}
	people, err := s.repo.Overview(ctx, q)
	if err != nil {
		return dto.Overview{}, err
	}
	ov := dto.Overview{People: people}
	for _, p := range people {
		ov.TotalNewContext += p.NewContext
		ov.TotalRequests += p.Requests
		ov.TotalSessions += p.Sessions
		ov.TotalOutputTokens += p.OutputTokens
		ov.TotalSubagentRequests += p.SubagentRequests
		ov.TotalSubagentOutputTokens += p.SubagentOutputTokens
		ov.TotalInputTokens += p.InputTokens
		ov.TotalCacheWriteTokens += p.CacheWriteTokens
		ov.TotalCacheReadTokens += p.CacheReadTokens
		ov.TotalSessionsNearFull += p.SessionsNearFull
		ov.TotalCompactions += p.Compactions
		ov.TotalLinesAdded += p.LinesAdded
		ov.TotalLinesRemoved += p.LinesRemoved
		ov.TotalCommits += p.Commits
		if p.NewContext > 0 {
			ov.ActivePeople++
		}
	}
	return ov, nil
}

func (s *Service) Timeseries(ctx context.Context, c dto.Caller, q dto.ReportQuery, interval, groupBy string) ([]dto.Point, error) {
	if interval != "hour" && interval != "day" {
		return nil, fmt.Errorf("%w: interval must be hour or day", ErrInvalidInput)
	}
	if !repository.ValidGroup(groupBy) {
		return nil, fmt.Errorf("%w: group_by must be user or model", ErrInvalidInput)
	}
	q, loc, err := s.prepare(c, q)
	if err != nil {
		return nil, err
	}
	points, err := s.repo.Timeseries(ctx, q, interval, groupBy)
	for i := range points {
		points[i].Bucket = points[i].Bucket.In(loc)
	}
	return points, err
}

func (s *Service) Models(ctx context.Context, c dto.Caller, q dto.ReportQuery) ([]dto.ModelSummary, error) {
	q, _, err := s.prepare(c, q)
	if err != nil {
		return nil, err
	}
	return s.repo.Models(ctx, q)
}

func (s *Service) Tools(ctx context.Context, c dto.Caller, q dto.ReportQuery) ([]dto.ToolSummary, error) {
	q, _, err := s.prepare(c, q)
	if err != nil {
		return nil, err
	}
	return s.repo.Tools(ctx, q)
}

func (s *Service) Daily(ctx context.Context, c dto.Caller, q dto.ReportQuery) ([]dto.DailyUsage, error) {
	q, _, err := s.prepare(c, q)
	if err != nil {
		return nil, err
	}
	return s.repo.Daily(ctx, q)
}

func (s *Service) Weeks(ctx context.Context, c dto.Caller, q dto.ReportQuery) ([]dto.WeekUsage, error) {
	q, _, err := s.prepare(c, q)
	if err != nil {
		return nil, err
	}
	return s.repo.Weeks(ctx, q)
}

func (s *Service) MemberModels(ctx context.Context, c dto.Caller, q dto.ReportQuery) ([]dto.MemberModel, error) {
	q, _, err := s.prepare(c, q)
	if err != nil {
		return nil, err
	}
	return s.repo.MemberModels(ctx, q)
}

const (
	defaultHotShare = 5
	maxShare        = 100
)

func (s *Service) HotSessions(ctx context.Context, c dto.Caller, minShare float64) ([]dto.HotSession, error) {
	if minShare == 0 {
		minShare = defaultHotShare
	}
	if minShare < 0 || minShare > maxShare {
		return nil, fmt.Errorf("%w: min_share must be between 0 and 100", ErrInvalidInput)
	}
	return s.repo.HotSessions(ctx, minShare, s.scope(c, ""))
}

func (s *Service) Costs(ctx context.Context, c dto.Caller, q dto.ReportQuery) ([]dto.CostRow, error) {
	q, _, err := s.prepare(c, q)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.Costs(ctx, q)
	for i := range rows {
		rows[i].Cost = splitCost(rows[i])
	}
	return rows, err
}

func splitCost(row dto.CostRow) float64 {
	if row.ClaudeTier == nil || row.AccountWeight <= 0 {
		return 0
	}
	price, ok := planPrice(*row.ClaudeTier)
	if !ok {
		return 0
	}
	return price * float64(row.Weight) / float64(row.AccountWeight)
}

func (s *Service) Projects(ctx context.Context, c dto.Caller, q dto.ReportQuery) ([]dto.ProjectSummary, error) {
	q, _, err := s.prepare(c, q)
	if err != nil {
		return nil, err
	}
	return s.repo.Projects(ctx, q)
}

func (s *Service) Sessions(ctx context.Context, c dto.Caller, sq dto.SessionQuery) ([]dto.SessionRow, int64, error) {
	q, _, err := s.prepare(c, sq.ReportQuery)
	if err != nil {
		return nil, 0, err
	}
	sq.ReportQuery = q
	if sq.Sort == "" {
		sq.Sort = defaultSort
	}
	if !repository.ValidSort(sq.Sort) {
		return nil, 0, fmt.Errorf("%w: sort must be one of last_request_at, -last_request_at, new_context, -new_context", ErrInvalidInput)
	}
	page, perPage := sq.Paging()
	if page < 1 || perPage < 1 || perPage > dto.MaxPerPage {
		return nil, 0, fmt.Errorf("%w: page must be >= 1 and per_page 1-%d", ErrInvalidInput, dto.MaxPerPage)
	}
	return s.repo.Sessions(ctx, sq, perPage, (page-1)*perPage)
}

func (s *Service) Session(ctx context.Context, c dto.Caller, id string) (dto.SessionDetail, error) {
	detail, err := s.repo.Session(ctx, id)
	if errors.Is(err, repository.ErrNoSession) || (err == nil && !c.IsAdmin && detail.Session.UserID != c.UserID) {
		return dto.SessionDetail{}, ErrNotFound
	}
	return detail, err
}

func (s *Service) People(ctx context.Context, c dto.Caller) ([]dto.Person, error) {
	if !c.IsAdmin {
		return nil, ErrForbidden
	}
	return s.repo.People(ctx)
}
