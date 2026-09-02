package tickets

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAutomaticPointsCannotBeFaked is the property that makes the checklist
// worth having. Points 1 and 6 are evidence the cockpit gathered itself; if a
// person could set them — through the API or by editing the file — they would
// say nothing that the other five do not already say.
func TestAutomaticPointsCannotBeFaked(t *testing.T) {
	store := newTestStore(t)
	ticket := mustCreate(t, store, "Prüfbarer Titel")

	if _, err := store.SetDoDBy(ticket.Key, 1, true, Actor{Name: "Marco"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Punkt 1 liess sich von Hand setzen: %v", err)
	}
	if _, err := store.SetDoDBy(ticket.Key, 6, true, Actor{Name: "Marco"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Punkt 6 liess sich von Hand setzen: %v", err)
	}

	// Und auch nicht am Werkzeug vorbei, direkt in der Datei.
	path := filepath.Join(store.dir, ticket.Key+".md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(raw), "dod: ", "dod: 1, 6, 3", 1)
	if edited == string(raw) {
		t.Fatal("dod-Zeile nicht gefunden - Testaufbau stimmt nicht")
	}
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Get(ticket.Key)
	if err != nil {
		t.Fatal(err)
	}
	for _, nr := range loaded.DoD {
		if nr == 1 || nr == 6 {
			t.Fatalf("automatischer Punkt aus der Datei uebernommen: %v", loaded.DoD)
		}
	}
	if len(loaded.DoD) != 1 || loaded.DoD[0] != 3 {
		t.Fatalf("dod=%v - Punkt 3 haette bleiben muessen", loaded.DoD)
	}
}

// TestAutomaticPointsFollowTheTicket: die beiden abgeleiteten Punkte muessen
// sich aendern, wenn sich das Ticket aendert - sonst sind sie Dekoration.
func TestAutomaticPointsFollowTheTicket(t *testing.T) {
	store := newTestStore(t)
	ticket := mustCreate(t, store, "Ein brauchbarer Titel")

	punkt := func(tk Ticket, nr int) DoDPoint {
		t.Helper()
		for _, p := range tk.DefinitionOfDone() {
			if p.Nr == nr {
				return p
			}
		}
		t.Fatalf("Punkt %d fehlt", nr)
		return DoDPoint{}
	}
	if p := punkt(ticket, 1); p.Erfuellt || !p.Automatisch {
		t.Fatalf("ohne Scope darf Punkt 1 nicht erfuellt sein: %#v", p)
	}
	if _, err := store.SetScope(ticket.Key, "missioncontrol/internal/tickets/", "Marco"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetVersion(ticket.Key, "0.15.12", "Marco"); err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.Get(ticket.Key)
	if err != nil {
		t.Fatal(err)
	}
	if p := punkt(reloaded, 1); !p.Erfuellt {
		t.Fatalf("mit Scope und Version muss Punkt 1 erfuellt sein: %#v", p)
	}
	// Punkt 6 braucht zusaetzlich eine Komponente.
	if p := punkt(reloaded, 6); p.Erfuellt {
		t.Fatalf("ohne Komponente darf Punkt 6 nicht erfuellt sein: %#v", p)
	}
	if open := reloaded.OpenDoD(); len(open) == 0 {
		t.Fatal("offene Punkte muessten gemeldet werden")
	}
}
