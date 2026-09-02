package tickets

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

// The vocabulary of the board, kept small on purpose. It is modelled on Jira -
// project, issue type, priority, epic, component, fix version - because that is
// the vocabulary both of us already know, not because HomeFlow needs a process.

// Projects. Mission Control versions itself independently of HomeFlow, so a
// ticket has to say which product it belongs to before a version means anything.
const (
	ProjectHomeFlow = "HF"
	ProjectMission  = "MC"
)

type Project struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func Projects() []Project {
	return []Project{
		{Key: ProjectHomeFlow, Name: "HomeFlow", Description: "Die Haushaltsplattform selbst: Backend, Frontend, Kubernetes, Monitoring."},
		{Key: ProjectMission, Name: "Mission Control", Description: "Das Cockpit. Eigener Lebenszyklus, eigene Versionen."},
	}
}

// Issue types.
const (
	TypeEpic        = "Epic"
	TypeTask        = "Task"
	TypeBug         = "Bug"
	TypeObservation = "Beobachtung"
	TypeRecord      = "Protokoll"
)

// TypeRecord is what the old Austausch.md protocol became: a finished piece of
// work that is kept for the record, not something anyone still acts on.
func Types() []string {
	return []string{TypeEpic, TypeTask, TypeBug, TypeObservation, TypeRecord}
}

const (
	PriorityHigh   = "Hoch"
	PriorityMedium = "Mittel"
	PriorityLow    = "Niedrig"
)

func Priorities() []string { return []string{PriorityHigh, PriorityMedium, PriorityLow} }

// The board columns, in order.
const (
	StatusBacklog = "Backlog"
	StatusReady   = "Ready"
	StatusActive  = "In Bearbeitung"
	StatusTest    = "Test"
	StatusDone    = "Fertig"
)

func Columns() []string {
	return []string{StatusBacklog, StatusReady, StatusActive, StatusTest, StatusDone}
}

func valid(value string, allowed []string) bool {
	for _, candidate := range allowed {
		if candidate == value {
			return true
		}
	}
	return false
}

func ValidStatus(status string) bool  { return valid(status, Columns()) }
func ValidType(value string) bool     { return valid(value, Types()) }
func ValidPriority(value string) bool { return valid(value, Priorities()) }
func ValidProject(value string) bool  { return value == ProjectHomeFlow || value == ProjectMission }

// legacyStatus maps the status names used before the board existed.
func legacyStatus(status string) (string, bool) {
	switch status {
	case "offen":
		return StatusBacklog, true
	case "in Arbeit":
		return StatusActive, true
	case "wartet":
		return StatusReady, true
	case "erledigt", "verworfen":
		return StatusDone, true
	}
	return "", false
}

// keyPattern doubles as the guard against path traversal: a key from a request
// only ever becomes a filename after it matched this.
var keyPattern = regexp.MustCompile(`^(HF|MC)-[0-9]{4}$`)

func ValidKey(key string) bool { return keyPattern.MatchString(key) }

// Actor is who claims to be acting, plus what the system could actually
// observe about the request. The two are deliberately separate fields: there is
// no login, so Name is a claim and nothing more. Origin is evidence — it does
// not prevent a forged name, it makes one visible afterwards.
type Actor struct {
	Name   string
	Origin string
}

// Event is one recorded field change. It is kept apart from Comment on purpose:
// a comment is what somebody said, an event is what happened. Mixing them makes
// both unreadable, and lets a hand-written comment imitate a change record.
type Event struct {
	At     time.Time `json:"at"`
	Author string    `json:"author"`
	Field  string    `json:"field"`
	From   string    `json:"from"`
	To     string    `json:"to"`
	Origin string    `json:"origin"`
}

// Relation kinds. Each has an inverse, and both halves are always written:
// a relation maintained on one side only looks exactly like a maintained one
// and is wrong precisely when it matters.
const (
	RelBlocks     = "blockiert"
	RelDependsOn  = "hängt ab von"
	RelRelates    = "betrifft"
	RelDuplicates = "doppelt"
)

// Inverse returns the counterpart a relation gets on the other ticket.
func Inverse(kind string) (string, bool) {
	switch kind {
	case RelBlocks:
		return RelDependsOn, true
	case RelDependsOn:
		return RelBlocks, true
	case RelRelates:
		return RelRelates, true
	case RelDuplicates:
		return RelDuplicates, true
	}
	return "", false
}

// Relation points at another ticket. Kind is one of the constants above.
type Relation struct {
	Kind string `json:"kind"`
	Key  string `json:"key"`
}

type Comment struct {
	Author string    `json:"author"`
	At     time.Time `json:"at"`
	Text   string    `json:"text"`
}

type Ticket struct {
	Key      string `json:"key"`
	Project  string `json:"project"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
	// Owner is the assignee, Reporter whoever raised it.
	Owner    string `json:"owner"`
	Reporter string `json:"reporter"`
	// Version is where this was found: the release the project was on when the
	// ticket was written. Known at creation time.
	//
	// Delivered is which release actually shipped it, and stays empty until a
	// deployment says otherwise. Merging the two made every untouched ticket
	// claim membership in a release that had long since shipped.
	Version   string `json:"version"`
	Delivered string `json:"delivered,omitempty"`
	// DeliveredAt is when that release actually shipped. Without it a ticket can
	// say in which version it went out but not when - und "angelegt am … bis
	// ausgeliefert am …" ist genau die Frage, die ein Lebenslauf beantworten
	// soll (MC-0021).
	DeliveredAt time.Time `json:"-"`
	Epic        string    `json:"epic,omitempty"`
	Component   string    `json:"component,omitempty"`
	// Scope names the files a ticket works on. It is what Austausch.md's claim
	// table did by hand: a ticket in "In Bearbeitung" with a scope is a claim,
	// and two such tickets on overlapping paths are a collision.
	Scope     []string   `json:"scope"`
	Tags      []string   `json:"tags"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	Body      string     `json:"body"`
	Comments  []Comment  `json:"comments"`
	Events    []Event    `json:"events"`
	Relations []Relation `json:"relations"`
}

// Done reports whether a ticket still needs attention. Records are archive by
// definition and never count as open work.
func (t Ticket) Done() bool { return t.Status == StatusDone || t.Type == TypeRecord }

// cleanScope keeps path patterns usable: trimmed, deduplicated, no commas or
// line breaks, and no absolute paths - a scope is always relative to the repo.
func cleanScope(scope []string) []string {
	out := make([]string, 0, len(scope))
	seen := map[string]bool{}
	for _, entry := range scope {
		entry = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(entry), "./"))
		entry = strings.TrimPrefix(entry, "/")
		if entry == "" || strings.ContainsAny(entry, "\n,") || seen[entry] || len(entry) > 200 {
			continue
		}
		seen[entry] = true
		out = append(out, entry)
	}
	sort.Strings(out)
	return out
}

func cleanTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := map[string]bool{}
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || strings.ContainsAny(tag, "\n,") || seen[tag] {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}

// ClaimStaleAfter is when a claim stops being work and starts being a lock.
//
// Am 2026-09-01 hing HF-0089 fünfzehn Stunden auf "In Bearbeitung" und HF-0104
// zwölf, ohne dass jemand daran arbeitete. Beide sperrten Pfade, und niemandem
// fiel es auf. Vier Stunden sind großzügig für eine Sitzung und kurz genug, dass
// ein vergessener Claim über Nacht auffällt (MC-0021).
const ClaimStaleAfter = 4 * time.Hour

// ClaimStale answers whether this ticket holds a claim nobody is tending.
// Only "In Bearbeitung" claims paths, so only it can go stale.
func (t Ticket) ClaimStale(now time.Time) bool {
	if t.Status != StatusActive {
		return false
	}
	return now.Sub(t.UpdatedAt) > ClaimStaleAfter
}

// Lifecycle is what a ticket answers about itself: since when it exists, how
// long it has been sitting where it is, and whether it ever shipped.
type Lifecycle struct {
	Created     time.Time  `json:"erstellt"`
	Updated     time.Time  `json:"aktualisiert"`
	Delivered   string     `json:"geliefert,omitempty"`
	DeliveredAt *time.Time `json:"geliefertAm,omitempty"`
	// DaysOpen counts from creation to delivery, or to now while it is open.
	DaysOpen   int  `json:"tageOffen"`
	ClaimStale bool `json:"claimVeraltet,omitempty"`
}

func (t Ticket) Life(now time.Time) Lifecycle {
	ende := now
	if !t.DeliveredAt.IsZero() {
		ende = t.DeliveredAt
	}
	tage := int(ende.Sub(t.CreatedAt).Hours() / 24)
	if tage < 0 {
		tage = 0
	}
	life := Lifecycle{
		Created: t.CreatedAt, Updated: t.UpdatedAt,
		Delivered: t.Delivered,
		DaysOpen:  tage, ClaimStale: t.ClaimStale(now),
	}
	if !t.DeliveredAt.IsZero() {
		wann := t.DeliveredAt
		life.DeliveredAt = &wann
	}
	return life
}
