package reminders

import (
	"context"
	"testing"
	"time"

	"homeflow/backend/internal/calendar"
)

type fakeRepository struct{}

func (fakeRepository) List(_ context.Context, _ string, _, _ time.Time, _ int) ([]Reminder, error) {
	return []Reminder{}, nil
}
func (fakeRepository) GetRules(context.Context, string) (Rules, error) {
	return Rules{LeadDays: []int{30, 7, 1}}, nil
}
func (fakeRepository) ReplaceRules(_ context.Context, _ string, values []int) (Rules, error) {
	return Rules{LeadDays: values}, nil
}
func (fakeRepository) CreateAppointment(_ context.Context, _ string, _ string, event calendar.Event) (calendar.Event, error) {
	return event, nil
}
func TestListBoundsRange(t *testing.T) {
	service := NewService(fakeRepository{})
	service.now = func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) }
	if _, err := service.List(context.Background(), "user", "2026-09-30", 0); err != nil {
		t.Fatal(err)
	}
	for _, until := range []string{"", "2026-08-31", "2027-09-03"} {
		if _, err := service.List(context.Background(), "user", until, 0); err != ErrInvalid {
			t.Fatalf("until=%q err=%v", until, err)
		}
	}
}

func TestReminderRulesValidation(t *testing.T) {
	service := NewService(fakeRepository{})
	for _, rules := range []Rules{{LeadDays: nil}, {LeadDays: []int{7, 7}}, {LeadDays: []int{367}}} {
		if _, err := service.ReplaceRules(context.Background(), "user", rules); err != ErrInvalid {
			t.Fatalf("rules=%v err=%v", rules, err)
		}
	}
	got, err := service.ReplaceRules(context.Background(), "user", Rules{LeadDays: []int{30, 7, 1}})
	if err != nil || len(got.LeadDays) != 3 {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestListUsesLocalCalendarDateAtTimezoneBoundary(t *testing.T) {
	previous := time.Local
	time.Local = time.FixedZone("UTC+2", 2*60*60)
	t.Cleanup(func() { time.Local = previous })
	service := NewService(fakeRepository{})
	service.now = func() time.Time { return time.Date(2026, 8, 31, 22, 30, 0, 0, time.UTC) }
	if _, err := service.List(context.Background(), "user", "2026-08-31", 100); err != ErrInvalid {
		t.Fatalf("previous local day err=%v", err)
	}
	if _, err := service.List(context.Background(), "user", "2026-09-01", 100); err != nil {
		t.Fatalf("local today err=%v", err)
	}
}

func TestCreateAppointmentRequiresExplicitValidFields(t *testing.T) {
	service := NewService(fakeRepository{})
	sourceID := "00000000-0000-4000-8000-000000000010"
	start := time.Date(2026, 9, 10, 9, 0, 0, 0, time.FixedZone("CEST", 7200))
	end := start.Add(time.Hour)
	event, err := service.CreateAppointment(context.Background(), "user", sourceID, AppointmentInput{Title: "Impftermin", StartsAt: start, EndsAt: end, Visibility: "shared"})
	if err != nil || event.OwnerID != "user" || event.Visibility != calendar.VisibilityShared || event.AppointmentType == nil || *event.AppointmentType != calendar.AppointmentVaccination {
		t.Fatalf("event=%#v err=%v", event, err)
	}
	if _, err := service.CreateAppointment(context.Background(), "user", sourceID, AppointmentInput{Title: "Impftermin", StartsAt: end, EndsAt: start, Visibility: "shared"}); err != ErrInvalid {
		t.Fatalf("invalid err=%v", err)
	}
}
