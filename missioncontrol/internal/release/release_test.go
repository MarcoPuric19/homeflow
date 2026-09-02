package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHistorySurvivesAndReplacesTheSameVersion(t *testing.T) {
	pfad := filepath.Join(t.TempDir(), "sub", "releases.json")

	// Die erste Auslieferung findet keine Historie vor. Das ist kein Fehler.
	datei, err := Load(pfad)
	if err != nil || len(datei.Releases) != 0 {
		t.Fatalf("leere Historie erwartet, got %#v err=%v", datei, err)
	}

	erste := Entry{Version: "0.16.0", At: time.Now().Add(-time.Hour), By: "Marco",
		Tickets: []Item{{Key: "HF-0116", Title: "Zuschnitt"}}}
	datei.Record(erste)
	if err := Save(pfad, datei); err != nil {
		t.Fatal(err)
	}

	// Zweiter Deploy derselben Version: berichtigt, nicht verdoppelt - sonst
	// stünde dieselbe Release zweimal da und niemand wüsste, welche gilt.
	wieder, err := Load(pfad)
	if err != nil {
		t.Fatal(err)
	}
	wieder.Record(Entry{Version: "0.16.0", At: time.Now(), By: "Marco",
		Tickets: []Item{{Key: "HF-0116", Title: "Zuschnitt"}, {Key: "HF-0117", Title: "Tierarzt"}}})
	if len(wieder.Releases) != 1 {
		t.Fatalf("%d Einträge, want 1 - dieselbe Version ist eine Berichtigung", len(wieder.Releases))
	}
	if len(wieder.Releases[0].Tickets) != 2 {
		t.Fatal("die Berichtigung muss den neueren Inhalt tragen")
	}

	// Neuere Release nach vorn, unabhängig von der Reihenfolge der Aufrufe.
	wieder.Record(Entry{Version: "0.17.0", At: time.Now().Add(time.Hour)})
	if wieder.Releases[0].Version != "0.17.0" {
		t.Fatalf("neueste zuerst erwartet, got %q", wieder.Releases[0].Version)
	}
}

func TestHistoryIsBounded(t *testing.T) {
	var datei File
	for i := 0; i < keep+7; i++ {
		datei.Record(Entry{Version: "0." + string(rune('a'+i)), At: time.Now().Add(time.Duration(i) * time.Hour)})
	}
	if len(datei.Releases) != keep {
		t.Fatalf("%d Einträge, want %d - die Datei wandert ins Image", len(datei.Releases), keep)
	}
}

func TestBrokenFileIsAnErrorAndNotAnEmptyHistory(t *testing.T) {
	pfad := filepath.Join(t.TempDir(), "releases.json")
	if err := os.WriteFile(pfad, []byte("{kein json"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Eine beschädigte Historie darf nicht als "keine Releases" durchgehen -
	// das würde eine leere Datei schreiben und die Geschichte löschen.
	if _, err := Load(pfad); err == nil {
		t.Fatal("beschädigte Datei muss einen Fehler liefern, nicht eine leere Historie")
	}
}

func TestMarkdownSaysWhenNothingWasAttached(t *testing.T) {
	leer := Markdown(Entry{Version: "0.16.0", At: time.Now()})
	if !strings.Contains(leer, "Kein Ticket") {
		t.Fatalf("ein Lauf ohne Ticket ist gültig und gehört benannt:\n%s", leer)
	}
	voll := Markdown(Entry{Version: "0.16.0", At: time.Now(), By: "Marco",
		Commit: "abcdef1234567890", Tickets: []Item{{Key: "HF-0116", Title: "Zuschnitt", Component: "daten"}}})
	for _, want := range []string{"HF-0116", "Zuschnitt", "daten", "abcdef12", "Marco"} {
		if !strings.Contains(voll, want) {
			t.Fatalf("%q fehlt in:\n%s", want, voll)
		}
	}
}
