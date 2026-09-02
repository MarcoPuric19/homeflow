// Package release macht HomeFlow auskunftsfähig über sich selbst.
//
// Bis HF-0105 wusste die Anwendung ihre eigene Version nicht: das Binary wurde
// ohne Versionsangabe gebaut, und es gab keinen Endpunkt, der sie ausgäbe. Wer
// wissen wollte, was läuft, musste im Cluster nachsehen - und am 2026-09-02 hat
// genau das nicht gestimmt, weil unter demselben Tag drei verschiedene Images
// unterwegs waren.
package release

import (
	_ "embed"
	"encoding/json"
	"time"
)

// notes wird von Mission Control geschrieben, bevor das Image gebaut wird, und
// wandert dadurch mit hinein. Fehlt die Datei, steht hier eine leere Historie -
// das Paket muss auch ohne sie bauen, sonst blockiert eine fehlende Notiz den
// gesamten Build.
//
//go:embed releases.json
var notes []byte

type Entry struct {
	Version string    `json:"version"`
	At      time.Time `json:"zeit"`
	Commit  string    `json:"commit,omitempty"`
	By      string    `json:"von,omitempty"`
	Tickets []Item    `json:"tickets"`
}

type Item struct {
	Key       string `json:"key"`
	Title     string `json:"titel"`
	Type      string `json:"typ,omitempty"`
	Component string `json:"komponente,omitempty"`
}

// Info is what the application answers about itself.
type Info struct {
	// Version kommt über -ldflags aus der Makefile. Ohne sie steht hier
	// "unbekannt" statt einer erfundenen Nummer.
	Version  string  `json:"version"`
	Releases []Entry `json:"releases"`
}

// History parses the embedded file. Eine beschädigte Datei liefert eine leere
// Historie statt eines Fehlers: die Anwendung soll starten, auch wenn ihre
// Release-Notiz kaputt ist.
func History() []Entry {
	var file struct {
		Releases []Entry `json:"releases"`
	}
	if err := json.Unmarshal(notes, &file); err != nil || file.Releases == nil {
		return []Entry{}
	}
	return file.Releases
}

// Current is the entry matching the running version, if the history knows it.
// Ein Binary, dessen Version in der Historie fehlt, ist ein Hinweis wert - es
// heißt, dass gebaut und nicht über die Pipeline ausgeliefert wurde.
func Current(version string) (Entry, bool) {
	for _, entry := range History() {
		if entry.Version == version {
			return entry, true
		}
	}
	return Entry{}, false
}
