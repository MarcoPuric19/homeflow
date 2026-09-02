package tickets

import (
	"testing"
	"time"
)

// Ein Claim, an dem niemand arbeitet, ist eine Sperre ohne Zweck. Am
// 2026-09-01 hingen zwei Tickets fünfzehn bzw. zwölf Stunden auf "In
// Bearbeitung", ohne dass es jemandem auffiel (MC-0021).
func TestClaimGoesStaleOnlyWhileInProgress(t *testing.T) {
	jetzt := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	alt := jetzt.Add(-ClaimStaleAfter - time.Minute)

	arbeitet := Ticket{Status: StatusActive, UpdatedAt: jetzt.Add(-time.Minute)}
	if arbeitet.ClaimStale(jetzt) {
		t.Fatal("ein frisch angefasster Claim ist nicht veraltet")
	}
	vergessen := Ticket{Status: StatusActive, UpdatedAt: alt}
	if !vergessen.ClaimStale(jetzt) {
		t.Fatal("ein Claim, der zu lange still steht, muss auffallen")
	}
	// Nur "In Bearbeitung" beansprucht Pfade. Ein altes Backlog-Ticket sperrt
	// nichts und darf deshalb auch nichts melden.
	for _, status := range []string{StatusBacklog, StatusReady, StatusTest, StatusDone} {
		if (Ticket{Status: status, UpdatedAt: alt}).ClaimStale(jetzt) {
			t.Fatalf("Status %q beansprucht nichts und kann nicht veralten", status)
		}
	}
}

// "Angelegt am … bis ausgeliefert am …" ist die Frage, die der Lebenslauf
// beantworten soll. Ohne Auslieferung zählt er bis heute.
func TestLifecycleCountsToDeliveryOrToNow(t *testing.T) {
	jetzt := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	erstellt := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	offen := Ticket{Status: StatusActive, CreatedAt: erstellt, UpdatedAt: jetzt}
	if got := offen.Life(jetzt).DaysOpen; got != 9 {
		t.Fatalf("offenes Ticket: tageOffen=%d, want 9", got)
	}
	if offen.Life(jetzt).DeliveredAt != nil {
		t.Fatal("ohne Auslieferung darf kein Datum dastehen, auch keine Nullzeit")
	}

	geliefert := offen
	geliefert.Delivered = "0.16.0"
	geliefert.DeliveredAt = time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	life := geliefert.Life(jetzt)
	if life.DaysOpen != 3 {
		t.Fatalf("ausgeliefertes Ticket: tageOffen=%d, want 3 (bis zur Auslieferung, nicht bis heute)", life.DaysOpen)
	}
	if life.DeliveredAt == nil || life.Delivered != "0.16.0" {
		t.Fatalf("Auslieferung fehlt: %#v", life)
	}
}
