package reminders

import (
	"context"
	"errors"
	"time"
)

var ErrInvalid = errors.New("invalid reminder range")

const MaxRangeDays = 366
const MaxList = 100

type Repository interface {
	List(context.Context, string, time.Time, time.Time, int) ([]Reminder, error)
}
type Service struct {
	repository Repository
	now        func() time.Time
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository, now: func() time.Time { return time.Now().UTC() }}
}
func (s *Service) List(ctx context.Context, userID, until string, limit int) ([]Reminder, error) {
	today := s.now().UTC().Truncate(24 * time.Hour)
	end, err := time.Parse(time.DateOnly, until)
	if err != nil || end.Before(today) || end.After(today.AddDate(0, 0, MaxRangeDays)) {
		return nil, ErrInvalid
	}
	if limit == 0 {
		limit = MaxList
	}
	if limit < 1 || limit > MaxList {
		return nil, ErrInvalid
	}
	return s.repository.List(ctx, userID, today, end, limit)
}
