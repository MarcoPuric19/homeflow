package shopping

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCreateAndCompleteItem(t *testing.T) {
	service := NewService(NewMemoryRepository())
	fixedTime := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return fixedTime }

	item, err := service.Create(context.Background(), CreateItem{
		Name: "Milk", Quantity: "2", Unit: "litres", Category: "groceries", Priority: PriorityHigh,
	}, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if item.ListID != "shared" || item.AddedBy != "user-1" || item.Completed {
		t.Fatalf("unexpected item: %#v", item)
	}

	completed, err := service.Complete(context.Background(), item.ID, "user-2")
	if err != nil {
		t.Fatal(err)
	}
	if !completed.Completed || completed.PurchasedBy != "user-2" || completed.CompletedAt == nil {
		t.Fatalf("unexpected completed item: %#v", completed)
	}
	if _, err := service.Complete(context.Background(), item.ID, "user-2"); !errors.Is(err, ErrAlreadyDone) {
		t.Fatalf("second completion err=%v, want ErrAlreadyDone", err)
	}
}

func TestCreateRejectsInvalidItem(t *testing.T) {
	service := NewService(NewMemoryRepository())
	if _, err := service.Create(context.Background(), CreateItem{Name: "", Quantity: "1"}, "user-1"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err=%v, want ErrInvalid", err)
	}
}
