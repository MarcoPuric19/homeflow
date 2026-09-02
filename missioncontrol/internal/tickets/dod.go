package tickets

import (
	"sort"
	"strconv"
	"strings"
)

// The seven points of the Ticket-Regel from AGENTS.md. They are listed here so
// the cockpit and the document cannot drift apart silently; if the rule
// changes, this list changes with it in the same pass.
var dodTitles = [7]string{
	"Claim vor der Arbeit: Status, Scope, Version",
	"Nur kostenlos — kein Trial, kein Konto, keine Karte",
	"Verträge im selben Durchgang angepasst",
	"Verifiziert nach der Tabelle in AGENTS.md",
	"Kommentar mit Ergebnis und dem, was nicht geprüft werden konnte",
	"Kopf release-notiztauglich: Titel, Typ, Komponente",
	"Spalte weitergeschoben",
}

// DoDPoint is one line of the checklist. Automatic tells the reader whether a
// tick was set by a person or derived by the tool — a tick whose origin is
// unknown is worth nothing.
type DoDPoint struct {
	Nr          int    `json:"nr"`
	Titel       string `json:"titel"`
	Erfuellt    bool   `json:"erfuellt"`
	Automatisch bool   `json:"automatisch"`
	Hinweis     string `json:"hinweis"`
}

// DefinitionOfDone derives the checklist. Points 1 and 6 are facts the cockpit
// can see for itself, so it checks them and says so. The rest are statements a
// person makes; no tool can tell whether a contract was really updated.
func (t Ticket) DefinitionOfDone() []DoDPoint {
	manual := map[int]bool{}
	for _, nr := range t.DoD {
		manual[nr] = true
	}
	points := make([]DoDPoint, 0, len(dodTitles))
	for i, titel := range dodTitles {
		nr := i + 1
		point := DoDPoint{Nr: nr, Titel: titel, Erfuellt: manual[nr]}
		switch nr {
		case 1:
			point.Automatisch = true
			point.Erfuellt = len(t.Scope) > 0 && strings.TrimSpace(t.Version) != ""
			if !point.Erfuellt {
				point.Hinweis = "Scope oder Version fehlt"
			}
		case 6:
			point.Automatisch = true
			generisch := strings.TrimSpace(t.Title) == "" || t.Title == t.Key
			point.Erfuellt = !generisch && strings.TrimSpace(t.Type) != "" && strings.TrimSpace(t.Component) != ""
			if !point.Erfuellt {
				point.Hinweis = "Titel, Typ oder Komponente unbrauchbar"
			}
		}
		points = append(points, point)
	}
	return points
}

// OpenDoD returns the numbers still missing, for the warning before Fertig.
func (t Ticket) OpenDoD() []int {
	open := []int{}
	for _, p := range t.DefinitionOfDone() {
		if !p.Erfuellt {
			open = append(open, p.Nr)
		}
	}
	return open
}

// SetDoDBy ticks or unticks one of the manual points. The automatic ones are
// refused: a tick a person could set by hand would stop being evidence.
func (s *Store) SetDoDBy(key string, nr int, done bool, by Actor) (Ticket, error) {
	by.Name = strings.TrimSpace(by.Name)
	if by.Name == "" || nr < 1 || nr > len(dodTitles) || nr == 1 || nr == 6 {
		return Ticket{}, ErrInvalid
	}
	return s.update(key, func(ticket *Ticket) {
		has := false
		kept := make([]int, 0, len(ticket.DoD))
		for _, existing := range ticket.DoD {
			if existing == nr {
				has = true
				continue
			}
			kept = append(kept, existing)
		}
		if has == done {
			return
		}
		if done {
			kept = append(kept, nr)
		}
		sort.Ints(kept)
		ticket.DoD = kept
		from, to := "offen", "erledigt"
		if !done {
			from, to = "erledigt", "offen"
		}
		ticket.Events = append(ticket.Events, Event{
			At: s.now().Truncate(1e9), Author: by.Name, Origin: by.Origin,
			Field: "DoD " + strconv.Itoa(nr), From: from, To: to,
		})
	})
}
