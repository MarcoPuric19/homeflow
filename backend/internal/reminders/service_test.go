package reminders

import (
	"context"
	"testing"
	"time"
)

type fakeRepository struct{}

func (fakeRepository) List(_ context.Context, _ string, _, _ time.Time, _ int) ([]Reminder, error) {
	return []Reminder{}, nil
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
