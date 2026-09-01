package tickets

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := NewStore(t.TempDir(), map[string]string{ProjectMission: "0.2.0", ProjectHomeFlow: "9.9.9"})
	if err != nil {
		t.Fatal(err)
	}
	moment := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time {
		moment = moment.Add(time.Minute)
		return moment
	}
	return store
}

func create(t *testing.T, store *Store, title string) Ticket {
	t.Helper()
	ticket, err := store.Create(NewTicket{Title: title, Body: "Beschreibung", Owner: "Codex",
		Tags: []string{"cockpit", "ui"}, Author: "Claude Code"})
	if err != nil {
		t.Fatal(err)
	}
	return ticket
}

func TestCreateNumbersTicketsAndSetsDefaults(t *testing.T) {
	store := newTestStore(t)
	first := create(t, store, "Erstes Ticket")
	second := create(t, store, "Zweites Ticket")
	if first.Key != "MC-0001" || second.Key != "MC-0002" {
		t.Fatalf("ids=%s,%s", first.Key, second.Key)
	}
	if first.Status != StatusBacklog {
		t.Fatalf("status=%q", first.Status)
	}
	if got := strings.Join(first.Tags, ","); got != "cockpit,ui" {
		t.Fatalf("tags=%q", got)
	}
}

func TestNumberingSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	first, err := NewStore(dir, map[string]string{ProjectMission: "0.2.0"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Create(NewTicket{Title: "A", Author: "Claude Code"}); err != nil {
		t.Fatal(err)
	}
	// Ein zweiter Store auf demselben Verzeichnis darf keine Nummer doppelt
	// vergeben - sonst überschreibt ein Neustart ein Ticket.
	second, err := NewStore(dir, map[string]string{ProjectMission: "0.2.0"})
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := second.Create(NewTicket{Title: "B", Author: "Codex"})
	if err != nil {
		t.Fatal(err)
	}
	if ticket.Key != "MC-0002" {
		t.Fatalf("id=%s, erwartet MC-0002", ticket.Key)
	}
}

func TestRoundTripThroughTheFile(t *testing.T) {
	store := newTestStore(t)
	created := create(t, store, "Umlaute: Prüfung äöüß")
	if _, err := store.Comment(created.Key, "Codex", "Erste Antwort\nmit zwei Zeilen"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetStatus(created.Key, StatusActive, "Codex"); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Get(created.Key)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Title != "Umlaute: Prüfung äöüß" {
		t.Fatalf("titel=%q", loaded.Title)
	}
	if loaded.Status != StatusActive || loaded.Owner != "Codex" {
		t.Fatalf("ticket=%#v", loaded)
	}
	if len(loaded.Comments) != 2 {
		t.Fatalf("kommentare=%d: %#v", len(loaded.Comments), loaded.Comments)
	}
	if loaded.Comments[0].Author != "Codex" || !strings.Contains(loaded.Comments[0].Text, "zwei Zeilen") {
		t.Fatalf("kommentar=%#v", loaded.Comments[0])
	}
	if !strings.Contains(loaded.Comments[1].Text, "Backlog → In Bearbeitung") {
		t.Fatalf("statuswechsel nicht protokolliert: %#v", loaded.Comments[1])
	}
	if !loaded.UpdatedAt.After(loaded.CreatedAt) {
		t.Fatal("aktualisiert wurde nicht fortgeschrieben")
	}
}

func TestFileIsReadableAndEditableByHand(t *testing.T) {
	store := newTestStore(t)
	ticket := create(t, store, "Handarbeit")
	raw, err := os.ReadFile(filepath.Join(store.dir, ticket.Key+".md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"id: MC-0001", "titel: Handarbeit", "status: Backlog", "zustaendig: Codex"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("Kopfzeile %q fehlt in:\n%s", want, raw)
		}
	}
	// Von Hand angehängter Kommentar - genau so würde ein Assistent ohne
	// Cockpit antworten.
	appended := string(raw) + "\n## Kommentar — Codex · 2026-09-01T12:00:00Z\n\nVon Hand ergänzt\n"
	if err := os.WriteFile(filepath.Join(store.dir, ticket.Key+".md"), []byte(appended), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Get(ticket.Key)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Comments) != 1 || loaded.Comments[0].Text != "Von Hand ergänzt" {
		t.Fatalf("kommentare=%#v", loaded.Comments)
	}
}

func TestUnknownHeaderKeysAreIgnored(t *testing.T) {
	store := newTestStore(t)
	content := "---\nid: MC-0007\ntitel: Fremdes Feld\nstatus: Backlog\nprioritaet: hoch\n---\n\nText\n"
	if err := os.WriteFile(filepath.Join(store.dir, "MC-0007.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	ticket, err := store.Get("MC-0007")
	if err != nil {
		t.Fatal(err)
	}
	if ticket.Title != "Fremdes Feld" || ticket.Body != "Text" {
		t.Fatalf("ticket=%#v", ticket)
	}
}

func TestRejectsUnusableInput(t *testing.T) {
	store := newTestStore(t)
	ticket := create(t, store, "Basis")
	if _, err := store.Create(NewTicket{Title: "  ", Author: "Claude Code"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("leerer Titel: err=%v", err)
	}
	if _, err := store.Create(NewTicket{Title: "Ohne Autor"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("fehlender Autor: err=%v", err)
	}
	if _, err := store.SetStatus(ticket.Key, "vielleicht", "Codex"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unbekannter Status: err=%v", err)
	}
	if _, err := store.Comment(ticket.Key, "Codex", "   "); !errors.Is(err, ErrInvalid) {
		t.Fatalf("leerer Kommentar: err=%v", err)
	}
	if _, err := store.Get("MC-9999"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unbekanntes Ticket: err=%v", err)
	}
}

func TestIDsCannotEscapeTheTicketDirectory(t *testing.T) {
	store := newTestStore(t)
	for _, id := range []string{"../../etc/passwd", "MC-0001/../../x", "", "MC-1", "MC-00001", "mc-0001", "MC-000a"} {
		if _, err := store.Get(id); !errors.Is(err, ErrInvalid) && !errors.Is(err, ErrNotFound) {
			t.Fatalf("Get(%q) err=%v", id, err)
		}
		if _, err := store.Comment(id, "Codex", "Text"); err == nil {
			t.Fatalf("Comment(%q) wurde angenommen", id)
		}
	}
}

func TestListIsNewestFirstAndSkipsForeignFiles(t *testing.T) {
	store := newTestStore(t)
	create(t, store, "Alt")
	create(t, store, "Neu")
	if err := os.WriteFile(filepath.Join(store.dir, "README.md"), []byte("kein Ticket"), 0o600); err != nil {
		t.Fatal(err)
	}
	tickets, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(tickets) != 2 || tickets[0].Key != "MC-0002" {
		t.Fatalf("liste=%#v", tickets)
	}
}

func TestVersionDefaultsToTheRunningReleaseAndCanBeChanged(t *testing.T) {
	store := newTestStore(t)
	ticket := create(t, store, "Mit Version")
	if ticket.Version != "0.2.0" {
		t.Fatalf("version=%q", ticket.Version)
	}
	explicit, err := store.Create(NewTicket{Title: "Eigene Version", Version: "0.3.0", Author: "Marco"})
	if err != nil {
		t.Fatal(err)
	}
	if explicit.Version != "0.3.0" {
		t.Fatalf("version=%q", explicit.Version)
	}
	changed, err := store.SetVersion(ticket.Key, "0.4.0", "Marco")
	if err != nil {
		t.Fatal(err)
	}
	if changed.Version != "0.4.0" {
		t.Fatalf("version=%q", changed.Version)
	}
	if !strings.Contains(changed.Comments[len(changed.Comments)-1].Text, "0.2.0 → 0.4.0") {
		t.Fatalf("Versionswechsel nicht protokolliert: %#v", changed.Comments)
	}
	reloaded, err := store.Get(ticket.Key)
	if err != nil || reloaded.Version != "0.4.0" {
		t.Fatalf("version nach dem Neulesen=%q err=%v", reloaded.Version, err)
	}
	if _, err := store.SetVersion(ticket.Key, "", "Marco"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("leere Version akzeptiert: %v", err)
	}
}

func TestOlderStatusNamesStillLoad(t *testing.T) {
	store := newTestStore(t)
	// Tickets aus der Zeit vor dem Board dürfen nicht stillschweigend
	// zurückgesetzt werden.
	for name, want := range map[string]string{
		"offen": StatusBacklog, "in Arbeit": StatusActive, "wartet": StatusReady,
		"erledigt": StatusDone, "verworfen": StatusDone,
	} {
		id := "MC-01" + string(rune('0'+len(name)%10)) + "0"
		content := "---\nid: " + id + "\ntitel: Alt\nstatus: " + name + "\n---\n\nText\n"
		if err := os.WriteFile(filepath.Join(store.dir, id+".md"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		ticket, err := store.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if ticket.Status != want {
			t.Fatalf("status %q wurde zu %q, erwartet %q", name, ticket.Status, want)
		}
	}
}

func TestBoardColumnsAreOrdered(t *testing.T) {
	want := []string{"Backlog", "Ready", "In Bearbeitung", "Test", "Fertig"}
	got := Columns()
	if len(got) != len(want) {
		t.Fatalf("spalten=%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("spalten=%v, erwartet %v", got, want)
		}
	}
}

func TestTicketsWithoutAVersionAdoptTheRunningRelease(t *testing.T) {
	store := newTestStore(t)
	content := "---\nid: MC-0042\ntitel: Von frueher\nstatus: offen\n---\n\nText\n"
	if err := os.WriteFile(filepath.Join(store.dir, "MC-0042.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	ticket, err := store.Get("MC-0042")
	if err != nil {
		t.Fatal(err)
	}
	if ticket.Version != "0.2.0" {
		t.Fatalf("version=%q", ticket.Version)
	}
	// Und beim nächsten Schreiben steht sie auch in der Datei.
	if _, err := store.Comment("MC-0042", "Marco", "Erster Kommentar"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(store.dir, "MC-0042.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "version: 0.2.0") {
		t.Fatalf("Version nicht persistiert:\n%s", raw)
	}
}

func TestListsAreNeverNil(t *testing.T) {
	// Die Oberfläche liest scope, tags und comments direkt; null statt []
	// bricht sie an einer Stelle, die im Go-Test sonst nie auffällt.
	store := newTestStore(t)
	ticket, err := store.Create(NewTicket{Title: "Ohne alles", Author: "Marco"})
	if err != nil {
		t.Fatal(err)
	}
	if ticket.Scope == nil || ticket.Tags == nil || ticket.Comments == nil {
		t.Fatalf("nil-Liste beim Anlegen: %#v", ticket)
	}
	loaded, err := store.Get(ticket.Key)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Scope == nil || loaded.Tags == nil || loaded.Comments == nil {
		t.Fatalf("nil-Liste nach dem Lesen: %#v", loaded)
	}
}
