package calendar

import (
	"context"
	"errors"
	"testing"
	"time"
)

const (
	alice = "00000000-0000-4000-8000-000000000001"
	bob   = "00000000-0000-4000-8000-000000000002"
)

func newService(t *testing.T) *Service {
	t.Helper()
	service := NewService(NewMemoryRepository())
	service.now = func() time.Time { return time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC) }
	return service
}

func input(overrides func(*EventInput)) EventInput {
	value := EventInput{
		Title:    "Zahnarzt",
		StartsAt: time.Date(2026, 9, 10, 9, 0, 0, 0, time.UTC),
		EndsAt:   time.Date(2026, 9, 10, 10, 0, 0, 0, time.UTC),
	}
	if overrides != nil {
		overrides(&value)
	}
	return value
}

func fullMonth() Range {
	return Range{From: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
}

func TestCreateAppliesDefaultsAndOwner(t *testing.T) {
	service := newService(t)
	event, err := service.Create(context.Background(), input(nil), alice)
	if err != nil {
		t.Fatal(err)
	}
	if event.OwnerID != alice {
		t.Fatalf("ownerId=%q, want the authenticated user", event.OwnerID)
	}
	if event.Visibility != VisibilityShared || event.Status != StatusPlanned {
		t.Fatalf("defaults not applied: %#v", event)
	}
	if event.CreatedAt.IsZero() || !event.UpdatedAt.Equal(event.CreatedAt) {
		t.Fatalf("timestamps=%v/%v", event.CreatedAt, event.UpdatedAt)
	}
}

func TestCreateRejectsInvalidInput(t *testing.T) {
	service := newService(t)
	long := make([]byte, maxTitleLength+1)
	for i := range long {
		long[i] = 'x'
	}
	tooLate := 40321
	negative := -1
	cases := map[string]EventInput{
		"empty title":        input(func(e *EventInput) { e.Title = "   " }),
		"overlong title":     input(func(e *EventInput) { e.Title = string(long) }),
		"end before start":   input(func(e *EventInput) { e.EndsAt = e.StartsAt.Add(-time.Second) }),
		"missing start":      input(func(e *EventInput) { e.StartsAt = time.Time{} }),
		"missing end":        input(func(e *EventInput) { e.EndsAt = time.Time{} }),
		"unknown status":     input(func(e *EventInput) { e.Status = "maybe" }),
		"unknown visibility": input(func(e *EventInput) { e.Visibility = "world" }),
		"reminder too far":   input(func(e *EventInput) { e.ReminderMinutes = &tooLate }),
		"negative reminder":  input(func(e *EventInput) { e.ReminderMinutes = &negative }),
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := service.Create(context.Background(), value, alice); !errors.Is(err, ErrInvalid) {
				t.Fatalf("err=%v, want ErrInvalid", err)
			}
		})
	}
}

func TestZeroLengthEventIsAllowed(t *testing.T) {
	service := newService(t)
	// The schema allows ends_at = starts_at, and a reminder-style entry at a
	// single instant is a legitimate use.
	if _, err := service.Create(context.Background(), input(func(e *EventInput) { e.EndsAt = e.StartsAt }), alice); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateEventsStayWithTheirOwner(t *testing.T) {
	service := newService(t)
	ctx := context.Background()
	private, err := service.Create(ctx, input(func(e *EventInput) {
		e.Title = "Überraschung"
		e.Visibility = VisibilityPrivate
	}), alice)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := service.Create(ctx, input(func(e *EventInput) { e.Title = "Müllabfuhr" }), alice)
	if err != nil {
		t.Fatal(err)
	}

	mine, err := service.List(ctx, alice, fullMonth())
	if err != nil || len(mine) != 2 {
		t.Fatalf("owner sees %d events, want 2 (err=%v)", len(mine), err)
	}
	theirs, err := service.List(ctx, bob, fullMonth())
	if err != nil || len(theirs) != 1 || theirs[0].ID != shared.ID {
		t.Fatalf("other member sees %#v (err=%v)", theirs, err)
	}

	// Not 403 and not an empty event: the other member must not be able to
	// tell a private event from a missing one.
	if _, err := service.Get(ctx, bob, private.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get err=%v, want ErrNotFound", err)
	}
	if _, err := service.Update(ctx, bob, private.ID, input(nil)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update err=%v, want ErrNotFound", err)
	}
	if err := service.Delete(ctx, bob, private.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete err=%v, want ErrNotFound", err)
	}
	if _, err := service.Get(ctx, alice, private.ID); err != nil {
		t.Fatalf("owner lost access to their own event: %v", err)
	}
}

func TestSharedEventsAreEditableByBoth(t *testing.T) {
	service := newService(t)
	ctx := context.Background()
	event, err := service.Create(ctx, input(nil), alice)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := service.Update(ctx, bob, event.ID, input(func(e *EventInput) {
		e.Title = "Zahnarzt verschoben"
		e.Status = StatusCompleted
	}))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "Zahnarzt verschoben" || updated.Status != StatusCompleted {
		t.Fatalf("update did not apply: %#v", updated)
	}
	if updated.OwnerID != alice {
		t.Fatalf("ownerId=%q, an update must not transfer ownership", updated.OwnerID)
	}
	if err := service.Delete(ctx, bob, event.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(ctx, alice, event.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("event survived deletion: %v", err)
	}
}

func TestListReturnsOverlappingEventsOnly(t *testing.T) {
	service := newService(t)
	ctx := context.Background()
	september := fullMonth()
	create := func(title string, start, end time.Time) {
		if _, err := service.Create(ctx, input(func(e *EventInput) {
			e.Title, e.StartsAt, e.EndsAt = title, start, end
		}), alice); err != nil {
			t.Fatal(err)
		}
	}
	day := func(d, h int) time.Time { return time.Date(2026, 9, d, h, 0, 0, 0, time.UTC) }
	create("inside", day(10, 9), day(10, 10))
	create("crosses the start", time.Date(2026, 8, 30, 9, 0, 0, 0, time.UTC), day(2, 10))
	create("crosses the end", day(30, 9), time.Date(2026, 10, 2, 10, 0, 0, 0, time.UTC))
	create("entirely before", time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC), time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC))
	create("entirely after", time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC), time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC))

	events, err := service.List(ctx, alice, september)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(events))
	for _, event := range events {
		got = append(got, event.Title)
	}
	want := []string{"crosses the start", "inside", "crosses the end"}
	if len(got) != len(want) {
		t.Fatalf("titles=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("titles=%v, want %v (sorted by start)", got, want)
		}
	}
}

func TestListRejectsUnusableRanges(t *testing.T) {
	service := newService(t)
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	cases := map[string]Range{
		"missing from": {To: from},
		"missing to":   {From: from},
		"inverted":     {From: from, To: from.Add(-time.Hour)},
		"empty":        {From: from, To: from},
		"too wide":     {From: from, To: from.Add((MaxRangeDays + 1) * 24 * time.Hour)},
	}
	for name, window := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := service.List(context.Background(), alice, window); !errors.Is(err, ErrRange) {
				t.Fatalf("err=%v, want ErrRange", err)
			}
		})
	}
}

func TestMalformedIDsAreRejectedBeforeTheRepository(t *testing.T) {
	service := newService(t)
	ctx := context.Background()
	if _, err := service.Get(ctx, alice, "nonsense"); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("get err=%v", err)
	}
	if _, err := service.Update(ctx, alice, "../../etc", input(nil)); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("update err=%v", err)
	}
	if err := service.Delete(ctx, alice, ""); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("delete err=%v", err)
	}
}

func TestRecurrenceAndReminderAreStoredVerbatim(t *testing.T) {
	service := newService(t)
	minutes := 30
	event, err := service.Create(context.Background(), input(func(e *EventInput) {
		e.RecurrenceRule = "FREQ=WEEKLY;BYDAY=MO"
		e.ReminderMinutes = &minutes
	}), alice)
	if err != nil {
		t.Fatal(err)
	}
	if event.RecurrenceRule != "FREQ=WEEKLY;BYDAY=MO" || event.ReminderMinutes == nil || *event.ReminderMinutes != 30 {
		t.Fatalf("event=%#v", event)
	}
	// The rule is stored, not expanded: a weekly series must still be exactly
	// one row in the range.
	events, err := service.List(context.Background(), alice, fullMonth())
	if err != nil || len(events) != 1 {
		t.Fatalf("events=%d, want 1 (err=%v)", len(events), err)
	}
}
