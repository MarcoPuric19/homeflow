package calendar

import (
	"context"
	"sort"
	"sync"
)

// MemoryRepository mirrors the visibility rule of the PostgreSQL repository so
// tests that use it exercise the same authorisation behaviour.
type MemoryRepository struct {
	mu     sync.RWMutex
	events map[string]Event
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{events: make(map[string]Event)}
}

func visible(event Event, userID string) bool {
	return event.Visibility == VisibilityShared || event.OwnerID == userID
}

func (r *MemoryRepository) Create(_ context.Context, event Event) (Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events[event.ID] = event
	return event, nil
}

func (r *MemoryRepository) ListVisible(_ context.Context, userID string, window Range) ([]Event, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	events := make([]Event, 0)
	for _, event := range r.events {
		if visible(event, userID) && event.StartsAt.Before(window.To) && !event.EndsAt.Before(window.From) && (window.PetID == "" || event.PetID == window.PetID) {
			events = append(events, event)
		}
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].StartsAt.Equal(events[j].StartsAt) {
			return events[i].ID < events[j].ID
		}
		return events[i].StartsAt.Before(events[j].StartsAt)
	})
	return events, nil
}

func (r *MemoryRepository) GetVisible(_ context.Context, userID, eventID string) (Event, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	event, ok := r.events[eventID]
	if !ok || !visible(event, userID) {
		return Event{}, ErrNotFound
	}
	return event, nil
}

func (r *MemoryRepository) Update(_ context.Context, userID string, event Event) (Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.events[event.ID]
	if !ok || !visible(existing, userID) {
		return Event{}, ErrNotFound
	}
	existing.Title = event.Title
	existing.Description = event.Description
	existing.StartsAt = event.StartsAt
	existing.EndsAt = event.EndsAt
	existing.AllDay = event.AllDay
	existing.Category = event.Category
	existing.Visibility = event.Visibility
	existing.RecurrenceRule = event.RecurrenceRule
	existing.ReminderMinutes = event.ReminderMinutes
	existing.Status = event.Status
	existing.PetID = event.PetID
	existing.AppointmentType = event.AppointmentType
	existing.UpdatedAt = event.UpdatedAt
	r.events[event.ID] = existing
	return existing, nil
}

func (r *MemoryRepository) Delete(_ context.Context, userID, eventID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	event, ok := r.events[eventID]
	if !ok || !visible(event, userID) {
		return ErrNotFound
	}
	delete(r.events, eventID)
	return nil
}
