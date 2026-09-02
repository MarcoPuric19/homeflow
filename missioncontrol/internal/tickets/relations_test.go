package tickets

import (
	"errors"
	"strings"
	"testing"
)

func mustCreate(t *testing.T, store *Store, title string) Ticket {
	t.Helper()
	ticket, err := store.Create(NewTicket{Project: "MC", Title: title, Author: "Claude Code"})
	if err != nil {
		t.Fatal(err)
	}
	return ticket
}

func hasRelation(ticket Ticket, kind, key string) bool {
	for _, r := range ticket.Relations {
		if r.Kind == kind && r.Key == key {
			return true
		}
	}
	return false
}

// TestRelationWritesBothSides is the property the whole feature stands on. A
// relation kept on one side only looks exactly like a maintained one, and is
// wrong precisely when somebody relies on it.
func TestRelationWritesBothSides(t *testing.T) {
	store := newTestStore(t)
	a, b := mustCreate(t, store, "A"), mustCreate(t, store, "B")

	if _, err := store.AddRelationBy(a.Key, RelBlocks, b.Key, Actor{Name: "Marco"}); err != nil {
		t.Fatal(err)
	}
	// Neu laden: die Gegenseite muss in der DATEI stehen, nicht nur im Speicher.
	loadedA, err := store.Get(a.Key)
	if err != nil {
		t.Fatal(err)
	}
	loadedB, err := store.Get(b.Key)
	if err != nil {
		t.Fatal(err)
	}
	if !hasRelation(loadedA, RelBlocks, b.Key) {
		t.Fatalf("A ohne Beziehung: %#v", loadedA.Relations)
	}
	if !hasRelation(loadedB, RelDependsOn, a.Key) {
		t.Fatalf("Gegenseite fehlt: %#v", loadedB.Relations)
	}

	if _, err := store.RemoveRelationBy(a.Key, RelBlocks, b.Key, Actor{Name: "Marco"}); err != nil {
		t.Fatal(err)
	}
	loadedB, _ = store.Get(b.Key)
	if len(loadedB.Relations) != 0 {
		t.Fatalf("Gegenseite nicht entfernt: %#v", loadedB.Relations)
	}
}

func TestRelationRejectsNonsense(t *testing.T) {
	store := newTestStore(t)
	a, b := mustCreate(t, store, "A"), mustCreate(t, store, "B")
	c := mustCreate(t, store, "C")

	if _, err := store.AddRelationBy(a.Key, RelBlocks, a.Key, Actor{Name: "Marco"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Selbstbezug wurde angenommen: %v", err)
	}
	if _, err := store.AddRelationBy(a.Key, RelBlocks, "MC-9999", Actor{Name: "Marco"}); err == nil {
		t.Fatal("unbekanntes Ticket wurde angenommen")
	}
	if _, err := store.AddRelationBy(a.Key, "verwandt mit", b.Key, Actor{Name: "Marco"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("erfundene Art wurde angenommen: %v", err)
	}
	if _, err := store.AddRelationBy(a.Key, RelBlocks, b.Key, Actor{Name: ""}); !errors.Is(err, ErrInvalid) {
		t.Fatal("Beziehung ohne Autor wurde angenommen")
	}

	// A blockiert B blockiert C - dann darf C nicht A blockieren.
	for _, pair := range [][2]string{{a.Key, b.Key}, {b.Key, c.Key}} {
		if _, err := store.AddRelationBy(pair[0], RelBlocks, pair[1], Actor{Name: "Marco"}); err != nil {
			t.Fatal(err)
		}
	}
	_, err := store.AddRelationBy(c.Key, RelBlocks, a.Key, Actor{Name: "Marco"})
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "blockieren") {
		t.Fatalf("Zyklus wurde angenommen: %v", err)
	}
}

// TestOpenBlockersIgnoresFinishedTickets: die Warnung beim Claim ist nur so viel
// wert, wie sie schweigt, wenn nichts mehr offen ist.
func TestOpenBlockersIgnoresFinishedTickets(t *testing.T) {
	store := newTestStore(t)
	a, b := mustCreate(t, store, "A"), mustCreate(t, store, "B")
	if _, err := store.AddRelationBy(a.Key, RelDependsOn, b.Key, Actor{Name: "Marco"}); err != nil {
		t.Fatal(err)
	}
	open, err := store.OpenBlockers(a.Key)
	if err != nil || len(open) != 1 {
		t.Fatalf("offene Blocker=%v err=%v", open, err)
	}
	if _, err := store.SetStatus(b.Key, StatusDone, "Marco"); err != nil {
		t.Fatal(err)
	}
	open, _ = store.OpenBlockers(a.Key)
	if len(open) != 0 {
		t.Fatalf("erledigter Blocker zaehlt noch: %v", open)
	}
}
