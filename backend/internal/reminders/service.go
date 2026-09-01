package reminders

import (
	"context"
	"errors"
	"time"

	"homeflow/backend/internal/calendar"
	"homeflow/backend/internal/platform/id"
)

var ErrInvalid = errors.New("invalid reminder range")
var ErrNotFound = errors.New("reminder source not found")
var ErrDuplicate = errors.New("reminder appointment already exists")

const MaxRangeDays = 366
const MaxList = 100
const MaxRules = 10

type Repository interface {
	List(context.Context, string, time.Time, time.Time, int) ([]Reminder, error)
	GetRules(context.Context, string) (Rules, error)
	ReplaceRules(context.Context, string, []int) (Rules, error)
	CreateAppointment(context.Context, string, string, calendar.Event) (calendar.Event, error)
}

func (s *Service) CreateAppointment(ctx context.Context, userID, sourceID string, input AppointmentInput) (calendar.Event, error) {
	if !id.IsUUID(sourceID) || len(input.Title) < 1 || len(input.Title) > 300 || input.StartsAt.IsZero() || input.EndsAt.IsZero() || input.EndsAt.Before(input.StartsAt) || (input.Visibility != string(calendar.VisibilityShared) && input.Visibility != string(calendar.VisibilityPrivate)) {
		return calendar.Event{}, ErrInvalid
	}
	eventID, err := id.New()
	if err != nil {
		return calendar.Event{}, err
	}
	now := s.now().UTC()
	appointmentType := calendar.AppointmentVaccination
	return s.repository.CreateAppointment(ctx, userID, sourceID, calendar.Event{ID: eventID, Title: input.Title, StartsAt: input.StartsAt.UTC(), EndsAt: input.EndsAt.UTC(), OwnerID: userID, Visibility: calendar.Visibility(input.Visibility), Status: calendar.StatusPlanned, AppointmentType: &appointmentType, CreatedAt: now, UpdatedAt: now})
}

type Service struct {
	repository Repository
	now        func() time.Time
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository, now: func() time.Time { return time.Now().UTC() }}
}
func (s *Service) List(ctx context.Context, userID, until string, limit int) ([]Reminder, error) {
	now := s.now().In(time.Local)
	year, month, day := now.Date()
	today := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
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

func (s *Service) GetRules(ctx context.Context, userID string) (Rules, error) {
	return s.repository.GetRules(ctx, userID)
}

func (s *Service) ReplaceRules(ctx context.Context, userID string, rules Rules) (Rules, error) {
	if len(rules.LeadDays) < 1 || len(rules.LeadDays) > MaxRules {
		return Rules{}, ErrInvalid
	}
	seen := make(map[int]struct{}, len(rules.LeadDays))
	for _, value := range rules.LeadDays {
		if value < 0 || value > MaxRangeDays {
			return Rules{}, ErrInvalid
		}
		if _, exists := seen[value]; exists {
			return Rules{}, ErrInvalid
		}
		seen[value] = struct{}{}
	}
	return s.repository.ReplaceRules(ctx, userID, rules.LeadDays)
}
