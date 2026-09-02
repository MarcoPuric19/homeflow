package tickets

import "testing"

// TestWithdrawDeliveryOnlyTouchesWhatTheDeployClosed guards the line between
// undoing a delivery and overruling a person. A rollback may take back what a
// deploy claimed; it may not move a ticket somebody finished by hand, and it
// may not touch a ticket that shipped in a different release.
func TestWithdrawDeliveryOnlyTouchesWhatTheDeployClosed(t *testing.T) {
	store := newTestStore(t)
	geliefert := mustCreate(t, store, "Vom Deploy geschlossen")
	fremd := mustCreate(t, store, "Andere Auslieferung")
	vonHand := mustCreate(t, store, "Von Hand auf Fertig")

	for _, c := range []struct {
		key, version string
	}{{geliefert.Key, "0.15.12"}, {fremd.Key, "0.15.11"}} {
		if _, err := store.SetDelivered(c.key, c.version, "Marco"); err != nil {
			t.Fatal(err)
		}
		if _, err := store.SetStatus(c.key, StatusDone, "Marco"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.SetStatus(vonHand.Key, StatusDone, "Marco"); err != nil {
		t.Fatal(err)
	}

	// Rollback von 0.15.12.
	for _, key := range []string{geliefert.Key, fremd.Key, vonHand.Key} {
		if _, err := store.WithdrawDelivery(key, "0.15.12", Actor{Name: "Marco", Origin: "test"}); err != nil {
			t.Fatal(err)
		}
	}

	zurueck, err := store.Get(geliefert.Key)
	if err != nil {
		t.Fatal(err)
	}
	if zurueck.Status != StatusTest || zurueck.Delivered != "" || !zurueck.DeliveredAt.IsZero() {
		t.Fatalf("nicht zurueckgenommen: status=%q geliefert=%q am=%v",
			zurueck.Status, zurueck.Delivered, zurueck.DeliveredAt)
	}
	if len(zurueck.Events) < 2 {
		t.Fatalf("Ruecknahme nicht protokolliert: %#v", zurueck.Events)
	}

	andere, _ := store.Get(fremd.Key)
	if andere.Status != StatusDone || andere.Delivered != "0.15.11" {
		t.Fatalf("fremde Auslieferung angetastet: %#v", andere)
	}
	handisch, _ := store.Get(vonHand.Key)
	if handisch.Status != StatusDone {
		t.Fatalf("menschliche Entscheidung ueberstimmt: status=%q", handisch.Status)
	}
}
