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

	price := int64(499)
	item, err := service.Create(context.Background(), CreateItem{
		Name: "Milk", Quantity: "2", Unit: "litres", Category: "groceries", Priority: PriorityHigh, PreferredBrand: "Local", EstimatedPriceCents: &price, EstimatedPriceCurrency: "eur", Recurring: true,
	}, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if item.ListID != "shared" || item.AddedBy != "user-1" || item.Completed || item.PreferredBrand != "Local" || item.EstimatedPriceCents == nil || *item.EstimatedPriceCents != 499 || item.EstimatedPriceCurrency != "EUR" || !item.Recurring {
		t.Fatalf("unexpected item: %#v", item)
	}
	negative := int64(-1)
	if _, err := service.Create(context.Background(), CreateItem{Name: "Bad", Quantity: "1", EstimatedPriceCents: &negative}, "user"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("negative price err=%v", err)
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
