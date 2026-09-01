package pets

import (
	"context"
	"errors"
	"homeflow/backend/internal/platform/id"
	"time"
)

type DashboardRepository interface {
	Dashboard(context.Context, string, string, string, time.Time) (Dashboard, error)
}

func (s *Service) Dashboard(ctx context.Context, userID, petID, month string) (Dashboard, error) {
	if !id.IsUUID(petID) {
		return Dashboard{}, ErrInvalidID
	}
	if _, err := time.Parse("2006-01", month); err != nil {
		return Dashboard{}, ErrInvalid
	}
	repo, ok := s.repository.(DashboardRepository)
	if !ok {
		return Dashboard{}, errors.New("dashboard repository unavailable")
	}
	return repo.Dashboard(ctx, userID, petID, month, s.now())
}
