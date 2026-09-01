package calendar

import (
	"context"
	"errors"
	"strings"
	"time"

	"homeflow/backend/internal/platform/id"
)

var (
	ErrNotFound = errors.New("calendar event not found")
	ErrInvalid  = errors.New("invalid calendar event")
	// ErrInvalidID is kept apart from ErrInvalid so a malformed path id can
	// answer 400 while a rejected body answers 422.
	ErrInvalidID = errors.New("invalid calendar event id")
	ErrRange     = errors.New("invalid time range")
)

const (
	maxTitleLength       = 300
	maxDescriptionLength = 2000
	maxCategoryLength    = 100
	maxRecurrenceLength  = 500
	// A reminder further out than four weeks is a typo, not a plan.
	maxReminderMinutes = 40320
	// A single query must not be able to pull the whole table.
	MaxRangeDays = 366
)

type Repository interface {
	Create(ctx context.Context, event Event) (Event, error)
	ListVisible(ctx context.Context, userID string, window Range) ([]Event, error)
	GetVisible(ctx context.Context, userID, eventID string) (Event, error)
	Update(ctx context.Context, userID string, event Event) (Event, error)
	Delete(ctx context.Context, userID, eventID string) error
}

type Service struct {
	repository Repository
	now        func() time.Time
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) Create(ctx context.Context, input EventInput, userID string) (Event, error) {
	normalised, err := normalise(input)
	if err != nil {
		return Event{}, err
	}
	eventID, err := id.New()
	if err != nil {
		return Event{}, err
	}
	now := s.now()
	return s.repository.Create(ctx, Event{
		ID: eventID, Title: normalised.Title, Description: normalised.Description,
		StartsAt: normalised.StartsAt, EndsAt: normalised.EndsAt, AllDay: normalised.AllDay,
		Category: normalised.Category, OwnerID: userID, Visibility: normalised.Visibility,
		RecurrenceRule: normalised.RecurrenceRule, ReminderMinutes: normalised.ReminderMinutes,
		Status: normalised.Status, CreatedAt: now, UpdatedAt: now,
		PetID: normalised.PetID, AppointmentType: normalised.AppointmentType,
	})
}

// List returns the events visible to userID that overlap the window: shared
// events plus the caller's own private ones.
func (s *Service) List(ctx context.Context, userID string, window Range) ([]Event, error) {
	if window.From.IsZero() || window.To.IsZero() || !window.To.After(window.From) {
		return nil, ErrRange
	}
	if window.To.Sub(window.From) > MaxRangeDays*24*time.Hour {
		return nil, ErrRange
	}
	if window.PetID != "" && !id.IsUUID(window.PetID) {
		return nil, ErrRange
	}
	return s.repository.ListVisible(ctx, userID, window)
}

func (s *Service) Get(ctx context.Context, userID, eventID string) (Event, error) {
	if !id.IsUUID(eventID) {
		return Event{}, ErrInvalidID
	}
	return s.repository.GetVisible(ctx, userID, eventID)
}

// Update replaces the mutable fields. The owner and the creation timestamp are
// never taken from the request; a private event stays editable by its owner
// only, which the repository enforces in SQL.
func (s *Service) Update(ctx context.Context, userID, eventID string, input EventInput) (Event, error) {
	if !id.IsUUID(eventID) {
		return Event{}, ErrInvalidID
	}
	normalised, err := normalise(input)
	if err != nil {
		return Event{}, err
	}
	return s.repository.Update(ctx, userID, Event{
		ID: eventID, Title: normalised.Title, Description: normalised.Description,
		StartsAt: normalised.StartsAt, EndsAt: normalised.EndsAt, AllDay: normalised.AllDay,
		Category: normalised.Category, Visibility: normalised.Visibility,
		RecurrenceRule: normalised.RecurrenceRule, ReminderMinutes: normalised.ReminderMinutes,
		Status: normalised.Status, UpdatedAt: s.now(),
		PetID: normalised.PetID, AppointmentType: normalised.AppointmentType,
	})
}

func (s *Service) Delete(ctx context.Context, userID, eventID string) error {
	if !id.IsUUID(eventID) {
		return ErrInvalidID
	}
	return s.repository.Delete(ctx, userID, eventID)
}

// normalise trims, defaults and validates an input so the database CHECK
// constraints are never the first line of defence.
func normalise(input EventInput) (EventInput, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	input.Category = strings.TrimSpace(input.Category)
	input.RecurrenceRule = strings.TrimSpace(input.RecurrenceRule)

	if input.Visibility == "" {
		input.Visibility = VisibilityShared
	}
	if input.Status == "" {
		input.Status = StatusPlanned
	}
	switch {
	case input.Title == "" || len(input.Title) > maxTitleLength:
		return EventInput{}, ErrInvalid
	case len(input.Description) > maxDescriptionLength:
		return EventInput{}, ErrInvalid
	case len(input.Category) > maxCategoryLength:
		return EventInput{}, ErrInvalid
	case len(input.RecurrenceRule) > maxRecurrenceLength:
		return EventInput{}, ErrInvalid
	case input.Visibility != VisibilityShared && input.Visibility != VisibilityPrivate:
		return EventInput{}, ErrInvalid
	case input.Status != StatusPlanned && input.Status != StatusCompleted && input.Status != StatusCancelled:
		return EventInput{}, ErrInvalid
	case input.StartsAt.IsZero() || input.EndsAt.IsZero():
		return EventInput{}, ErrInvalid
	case input.EndsAt.Before(input.StartsAt):
		return EventInput{}, ErrInvalid
	}
	if input.PetID != nil {
		value := strings.TrimSpace(*input.PetID)
		if !id.IsUUID(value) {
			return EventInput{}, ErrInvalid
		}
		input.PetID = &value
		if input.AppointmentType == nil || !validAppointmentType(*input.AppointmentType) {
			return EventInput{}, ErrInvalid
		}
	} else if input.AppointmentType != nil {
		return EventInput{}, ErrInvalid
	}
	if input.ReminderMinutes != nil && (*input.ReminderMinutes < 0 || *input.ReminderMinutes > maxReminderMinutes) {
		return EventInput{}, ErrInvalid
	}
	input.StartsAt = input.StartsAt.UTC()
	input.EndsAt = input.EndsAt.UTC()
	return input, nil
}

func validAppointmentType(value AppointmentType) bool {
	switch value {
	case AppointmentVeterinarian, AppointmentVaccination, AppointmentMedication, AppointmentGrooming, AppointmentOther:
		return true
	}
	return false
}
