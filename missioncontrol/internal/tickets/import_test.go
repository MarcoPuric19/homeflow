package tickets

import (
	"os"
	"strings"
	"testing"
)

// The importer is tested against the Austausch.md that was actually migrated on
// 2026-09-01, kept here as a fixture. A synthetic sample would only prove that
// the regexes match themselves - and this copy is at the same time the archive
// of a file that no longer exists in this form.
func importReal(t *testing.T) (*Store, ImportResult) {
	t.Helper()
	data, err := os.ReadFile("testdata/austausch-2026-09-01.md")
	if err != nil {
		t.Fatalf("Fixture fehlt: %v", err)
	}
	store, err := NewStore(t.TempDir(), map[string]string{ProjectHomeFlow: "0.15.0", ProjectMission: "0.2.0"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Import(string(data), "Claude Code")
	if err != nil {
		t.Fatal(err)
	}
	return store, result
}

func TestImportCoversEveryShapeInTheFile(t *testing.T) {
	_, result := importReal(t)
	counts := map[string]int{}
	for _, ticket := range result.Created {
		counts[ticket.Type]++
	}
	for kind, atLeast := range map[string]int{
		TypeEpic: 2, TypeTask: 11, TypeObservation: 12, TypeRecord: 28,
	} {
		if counts[kind] < atLeast {
			t.Fatalf("%s: %d übernommen, mindestens %d erwartet", kind, counts[kind], atLeast)
		}
	}
}

func TestImportedTicketsAreComplete(t *testing.T) {
	_, result := importReal(t)
	for _, ticket := range result.Created {
		if ticket.Title == "" || ticket.Body == "" {
			t.Fatalf("leeres Ticket: %#v", ticket)
		}
		if !ValidKey(ticket.Key) || ticket.Project != ProjectHomeFlow {
			t.Fatalf("Schlüssel/Projekt falsch: %#v", ticket)
		}
		if !ValidType(ticket.Type) || !ValidPriority(ticket.Priority) || !ValidStatus(ticket.Status) {
			t.Fatalf("ungültiges Feld: %#v", ticket)
		}
		if ticket.CreatedAt.IsZero() {
			t.Fatalf("ohne Datum: %s", ticket.Key)
		}
	}
}

func TestProtocolEntriesKeepTheirDateAndLandInDone(t *testing.T) {
	_, result := importReal(t)
	found := 0
	for _, ticket := range result.Created {
		if ticket.Type != TypeRecord {
			continue
		}
		found++
		if ticket.Status != StatusDone {
			t.Fatalf("Protokolleintrag nicht in Fertig: %#v", ticket)
		}
		if ticket.CreatedAt.Year() != 2026 {
			t.Fatalf("Datum nicht übernommen: %v", ticket.CreatedAt)
		}
		if !strings.HasPrefix(ticket.Title, ticket.CreatedAt.Format("2006-01-02")) {
			t.Fatalf("Titel ohne Datum: %q", ticket.Title)
		}
	}
	if found == 0 {
		t.Fatal("kein Protokolleintrag übernommen")
	}
}

func TestObservationsKeepTheirNumber(t *testing.T) {
	_, result := importReal(t)
	numbers := map[string]bool{}
	for _, ticket := range result.Created {
		if ticket.Type == TypeObservation {
			numbers[strings.SplitN(ticket.Title, " ", 2)[0]] = true
		}
	}
	// 7.2 (NetworkPolicy) und 7.7 (route-Label) sind die, auf die anderswo
	// verwiesen wird - wenn die Nummer verloren geht, brechen die Verweise.
	for _, want := range []string{"7.2", "7.7", "7.12"} {
		if !numbers[want] {
			t.Fatalf("Beobachtung %s fehlt; vorhanden: %v", want, numbers)
		}
	}
}

func TestImportIsIdempotent(t *testing.T) {
	store, first := importReal(t)
	data, err := os.ReadFile("testdata/austausch-2026-09-01.md")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Import(string(data), "Claude Code")
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Created) != 0 {
		t.Fatalf("zweiter Lauf legte %d Tickets an", len(second.Created))
	}
	if len(second.Skipped) != len(first.Created) {
		t.Fatalf("übersprungen=%d, angelegt beim ersten Lauf=%d", len(second.Skipped), len(first.Created))
	}
}

func TestImportedTicketsSurviveARoundTrip(t *testing.T) {
	store, result := importReal(t)
	for _, created := range result.Created {
		loaded, err := store.Get(created.Key)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.Title != created.Title || loaded.Type != created.Type || loaded.Status != created.Status {
			t.Fatalf("nach dem Lesen verändert:\n  vorher %#v\n  nachher %#v", created, loaded)
		}
		if !loaded.CreatedAt.Equal(created.CreatedAt) {
			t.Fatalf("Datum verändert: %v -> %v", created.CreatedAt, loaded.CreatedAt)
		}
	}
}
