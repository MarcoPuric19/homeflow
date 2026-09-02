// Package release turns "was ausgeliefert wurde" into etwas, das die Anwendung
// selbst beantworten kann.
//
// Bis HF-0105 wusste HomeFlow seine eigene Version nicht: das Binary wurde ohne
// Versionsangabe gebaut, und es gab keinen Endpunkt, der sie ausgäbe. Wer
// wissen wollte, was läuft, musste im Cluster nachsehen - und am 2026-09-02 hat
// genau das nicht gestimmt.
package release

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Entry is one release: which version, when, and was drin steckt.
//
// Die Tickets stehen mit Schlüssel und Titel drin und nicht als Verweis: die
// Zusammenfassung wird ins Image gebacken und muss ohne Mission Control lesbar
// bleiben.
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

// File is the whole history, newest first.
type File struct {
	Releases []Entry `json:"releases"`
}

// keep bounds the embedded history. Zwanzig Releases sind mehr, als jemand in
// einer Anzeige liest, und wenig genug, dass die Datei im Image nicht wächst.
const keep = 20

// Load reads the history, tolerating a missing file: die erste Auslieferung
// findet keine vor, und das ist kein Fehler.
func Load(path string) (File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return File{Releases: []Entry{}}, nil
		}
		return File{}, err
	}
	var file File
	if err := json.Unmarshal(raw, &file); err != nil {
		return File{}, fmt.Errorf("%s ist beschädigt: %w", path, err)
	}
	if file.Releases == nil {
		file.Releases = []Entry{}
	}
	return file, nil
}

// Record adds an entry, replacing an existing one for the same version.
//
// Ein zweiter Deploy derselben Version ist kein neuer Eintrag, sondern ein
// berichtigter: sonst stünde dieselbe Release mehrfach in der Anzeige, und
// niemand wüsste, welcher Eintrag gilt.
func (f *File) Record(entry Entry) {
	if entry.Tickets == nil {
		entry.Tickets = []Item{}
	}
	out := make([]Entry, 0, len(f.Releases)+1)
	out = append(out, entry)
	for _, vorhanden := range f.Releases {
		if vorhanden.Version == entry.Version {
			continue
		}
		out = append(out, vorhanden)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	if len(out) > keep {
		out = out[:keep]
	}
	f.Releases = out
}

// Save writes through a temporary file and renames. Eine halb geschriebene
// Historie würde behaupten, eine Release habe nichts enthalten.
func Save(path string, file File) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Markdown renders one entry for people. Die Datei liegt neben der JSON, weil
// eine Release-Notiz auch lesbar sein soll, wenn niemand ein Werkzeug startet.
func Markdown(entry Entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", entry.Version)
	// Gespeichert wird UTC, angezeigt Ortszeit. Andernfalls liest sich 08:56
	// wie 10:56, und der Versatz faellt niemandem auf.
	fmt.Fprintf(&b, "Ausgeliefert am %s", entry.At.Local().Format("02.01.2006 um 15:04"))
	if entry.By != "" {
		fmt.Fprintf(&b, " von %s", entry.By)
	}
	if entry.Commit != "" {
		fmt.Fprintf(&b, ", Stand `%s`", short(entry.Commit))
	}
	b.WriteString(".\n\n")
	if len(entry.Tickets) == 0 {
		// Ehrlicher als eine leere Liste: ein Deploy ohne Ticketauswahl ist ein
		// gültiger Vorgang und soll nicht so aussehen, als sei etwas verloren.
		b.WriteString("Kein Ticket war diesem Lauf zugeordnet.\n")
		return b.String()
	}
	for _, ticket := range entry.Tickets {
		fmt.Fprintf(&b, "- **%s** %s", ticket.Key, ticket.Title)
		if ticket.Component != "" {
			fmt.Fprintf(&b, " _(%s)_", ticket.Component)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func short(commit string) string {
	if len(commit) > 8 {
		return commit[:8]
	}
	return commit
}
