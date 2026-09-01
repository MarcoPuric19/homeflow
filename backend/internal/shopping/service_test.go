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
	item, err := service.Create(context.Background(), CreateItem{ForAllPets: true,
		Name: "Milk", Quantity: "2", Unit: "litres", Category: "groceries", Priority: PriorityHigh, PreferredBrand: "Local", EstimatedPriceCents: &price, EstimatedPriceCurrency: "eur", Recurring: true,
	}, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if item.ListID != "shared" || item.AddedBy != "user-1" || item.Completed || item.PreferredBrand != "Local" || item.EstimatedPriceCents == nil || *item.EstimatedPriceCents != 499 || item.EstimatedPriceCurrency != "EUR" || !item.Recurring {
		t.Fatalf("unexpected item: %#v", item)
	}
	negative := int64(-1)
	if _, err := service.Create(context.Background(), CreateItem{ForAllPets: true, Name: "Bad", Quantity: "1", EstimatedPriceCents: &negative}, "user"); !errors.Is(err, ErrInvalid) {
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
	if _, err := service.Create(context.Background(), CreateItem{ForAllPets: true, Name: "", Quantity: "1"}, "user-1"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err=%v, want ErrInvalid", err)
	}
}

// HF-0116: ein Artikel ohne jeden Tierbezug ist ungültig. Das Schema kann es
// nicht prüfen - die Bedingung hängt an einer zweiten Tabelle -, also ist
// dieser Test die einzige Absicherung.
func TestItemWithoutAnyPetScopeIsRejected(t *testing.T) {
	service := NewService(NewMemoryRepository())
	ohne := CreateItem{Name: "Katzenstreu", Quantity: "1", EstimatedPriceCurrency: "EUR"}
	if _, err := service.Create(context.Background(), ohne, "user-1"); err != ErrInvalid {
		t.Fatalf("Artikel ohne Tier und ohne ForAllPets muss ungültig sein, err=%v", err)
	}

	mitTier := ohne
	mitTier.PetIDs = []string{"9f1d2c3b-4a5e-4f60-8123-abcdefabcdef"}
	if _, err := service.Create(context.Background(), mitTier, "user-1"); err != nil {
		t.Fatalf("Artikel mit Tier muss gültig sein: %v", err)
	}

	fuerAlle := ohne
	fuerAlle.ForAllPets = true
	if _, err := service.Create(context.Background(), fuerAlle, "user-1"); err != nil {
		t.Fatalf("Artikel für alle Tiere muss gültig sein: %v", err)
	}

	// Ein Tierbezug, der keine UUID ist, ist kein Tierbezug.
	kaputt := ohne
	kaputt.PetIDs = []string{"nicht-uuid"}
	if _, err := service.Create(context.Background(), kaputt, "user-1"); err != ErrInvalid {
		t.Fatalf("ungültige Tier-ID muss abgelehnt werden, err=%v", err)
	}
}

// Codex' Gegencheck zu HF-0116, Punkt 1: forAllPets wurde geprüft und dann
// weggeworfen. Der alte Test sah nur, dass Create keinen Fehler liefert - das
// ist genau der Test, der nichts beweist. Dieser hier liest den Wert zurück.
func TestForAllPetsSurvivesTheRoundTrip(t *testing.T) {
	service := NewService(NewMemoryRepository())
	created, err := service.Create(context.Background(), CreateItem{
		Name: "Katzenstreu", Quantity: "1", EstimatedPriceCurrency: "EUR", ForAllPets: true,
	}, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if !created.ForAllPets {
		t.Fatal("forAllPets ging beim Anlegen verloren")
	}

	items, err := service.List(context.Background(), "user-1", "", false, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || !items[0].ForAllPets {
		t.Fatalf("forAllPets ist nach erneutem Lesen weg: %#v", items)
	}
}

// Punkt 3 desselben Gegenchecks: "entweder oder" stand in Dienst und OpenAPI,
// abgelehnt wurde die Kombination nicht. Sie lässt offen, was gemeint war.
func TestForAllPetsTogetherWithPetIDsIsRejected(t *testing.T) {
	service := NewService(NewMemoryRepository())
	_, err := service.Create(context.Background(), CreateItem{
		Name: "Katzenstreu", Quantity: "1", EstimatedPriceCurrency: "EUR",
		ForAllPets: true, PetIDs: []string{"9f1d2c3b-4a5e-4f60-8123-abcdefabcdef"},
	}, "user-1")
	if err != ErrInvalid {
		t.Fatalf("Widerspruch muss abgelehnt werden, err=%v", err)
	}
}
