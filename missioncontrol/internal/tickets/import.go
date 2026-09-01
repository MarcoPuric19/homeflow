package tickets

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Import turns Austausch.md into tickets. The file grew four shapes over time
// and each one becomes a different issue type:
//
//	Abschnitt 6, Tabellen P1/P2/P3  -> Task, Backlog
//	Abschnitt 6, F1 und F2          -> Epic, Backlog
//	Abschnitt 7, ### 7.x            -> Beobachtung, Backlog
//	Abschnitt 8, Protokolltabelle   -> Protokoll, Fertig
//
// Nothing is interpreted beyond that: titles, bodies and dates are carried over
// verbatim, because this file is the project's memory and a clever importer
// that "improves" entries would quietly lose it.
type ImportResult struct {
	Created []Ticket
	Skipped []string
}

var (
	backlogRow  = regexp.MustCompile(`(?m)^\| (P[123]-[0-9]+) \| ([^|]+) \| ([^|]*) \| ([^|]*) \| ([^|]*) \|\s*$`)
	epicHeading = regexp.MustCompile(`(?m)^### (F[12]) — (.+)$`)
	observation = regexp.MustCompile(`(?m)^### (7\.[0-9]+) (.+)$`)
	protocolRow = regexp.MustCompile(`(?m)^\| (20[0-9]{2}-[0-9]{2}-[0-9]{2}) \| ([^|]+) \| (.+) \|\s*$`)
	boldTitle   = regexp.MustCompile(`^\*\*(.+?)\.?\*\*`)
)

// Import is idempotent by title: a ticket whose title already exists is skipped,
// so running it twice does not double the board.
func (s *Store) Import(source, author string) (ImportResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, err := s.listLocked()
	if err != nil {
		return ImportResult{}, err
	}
	known := map[string]bool{}
	for _, ticket := range existing {
		known[strings.ToLower(ticket.Title)] = true
	}

	var result ImportResult
	add := func(input NewTicket, at time.Time, status string) error {
		if known[strings.ToLower(input.Title)] {
			result.Skipped = append(result.Skipped, input.Title)
			return nil
		}
		input.Author = author
		ticket, err := s.createLocked(input, at, status)
		if err != nil {
			return fmt.Errorf("%s: %w", input.Title, err)
		}
		known[strings.ToLower(ticket.Title)] = true
		result.Created = append(result.Created, ticket)
		return nil
	}

	// Epics first, so the tasks below can be linked to them.
	epics := map[string]string{}
	for _, match := range epicHeading.FindAllStringSubmatch(source, -1) {
		marker, heading := match[1], strings.TrimSpace(match[2])
		title := marker + " — " + stripEmphasis(heading)
		body := sectionBody(source, match[0])
		if err := add(NewTicket{
			Project: ProjectHomeFlow, Type: TypeEpic, Title: title, Body: body,
			Priority: PriorityMedium, Owner: "offen", Component: "produkt",
			Tags: []string{"vorhaben", strings.ToLower(marker)},
		}, time.Time{}, StatusBacklog); err != nil {
			return result, err
		}
		for _, ticket := range result.Created {
			if ticket.Title == title {
				epics[marker] = ticket.Key
			}
		}
	}

	for _, match := range backlogRow.FindAllStringSubmatch(source, -1) {
		number, title, why, scope := match[1], strings.TrimSpace(match[2]), strings.TrimSpace(match[3]), strings.TrimSpace(match[4])
		body := why
		if scope != "" {
			body += "\n\nScope: " + scope
		}
		body += "\n\nÜbernommen aus Austausch.md, Abschnitt 6 (" + number + ")."
		priority := PriorityMedium
		switch {
		case strings.HasPrefix(number, "P1"):
			priority = PriorityHigh
		case strings.HasPrefix(number, "P3"):
			priority = PriorityLow
		}
		if err := add(NewTicket{
			Project: ProjectHomeFlow, Type: TypeTask, Title: number + " " + title, Body: body,
			Priority: priority, Owner: "offen", Component: componentFor(scope),
			Tags: []string{"backlog", strings.ToLower(strings.SplitN(number, "-", 2)[0])},
		}, time.Time{}, StatusBacklog); err != nil {
			return result, err
		}
	}

	for _, match := range observation.FindAllStringSubmatch(source, -1) {
		number, heading := match[1], strings.TrimSpace(match[2])
		body := sectionBody(source, match[0]) + "\n\nÜbernommen aus Austausch.md, Beobachtung " + number + "."
		if err := add(NewTicket{
			Project: ProjectHomeFlow, Type: TypeObservation, Title: number + " " + stripEmphasis(heading),
			Body: body, Priority: PriorityLow, Owner: "offen", Component: "beobachtung",
			Tags: []string{"beobachtung"},
		}, time.Time{}, StatusBacklog); err != nil {
			return result, err
		}
	}

	for _, match := range protocolRow.FindAllStringSubmatch(source, -1) {
		day, owner, text := match[1], strings.TrimSpace(match[2]), strings.TrimSpace(match[3])
		at, err := time.Parse("2006-01-02", day)
		if err != nil {
			result.Skipped = append(result.Skipped, day)
			continue
		}
		title := day + " " + firstSentence(text)
		if err := add(NewTicket{
			Project: ProjectHomeFlow, Type: TypeRecord, Title: title, Body: text,
			Priority: PriorityLow, Owner: owner, Component: "protokoll",
			Tags: []string{"protokoll"}, Version: versionIn(text),
		}, at, StatusDone); err != nil {
			return result, err
		}
	}
	return result, nil
}

// sectionBody returns the text under a heading, up to the next heading of the
// same or a higher level.
func sectionBody(source, heading string) string {
	start := strings.Index(source, heading)
	if start < 0 {
		return ""
	}
	rest := source[start+len(heading):]
	if end := regexp.MustCompile(`(?m)^#{1,3} `).FindStringIndex(rest); end != nil {
		rest = rest[:end[0]]
	}
	return strings.TrimSpace(rest)
}

// firstSentence keeps protocol titles short. The entries start with a bold
// summary; that is the best title available.
func firstSentence(text string) string {
	if match := boldTitle.FindStringSubmatch(text); match != nil {
		return strings.TrimSpace(match[1])
	}
	plain := stripEmphasis(text)
	if index := strings.Index(plain, ". "); index > 0 && index < 120 {
		return plain[:index]
	}
	if len(plain) > 110 {
		return strings.TrimSpace(plain[:110]) + "…"
	}
	return plain
}

var versionMention = regexp.MustCompile(`\b(?:Version|VERSION)\s+([0-9]+\.[0-9]+\.[0-9]+)`)

func versionIn(text string) string {
	if match := versionMention.FindStringSubmatch(text); match != nil {
		return match[1]
	}
	return ""
}

func componentFor(scope string) string {
	lower := strings.ToLower(scope)
	for _, candidate := range []struct{ needle, name string }{
		{"frontend", "frontend"}, {"backend", "backend"}, {"migration", "backend"},
		{"kubernetes", "kubernetes"}, {"monitoring", "monitoring"}, {"docs", "docs"},
		{"scripts", "scripts"}, {"makefile", "build"}, {"openapi", "api"},
	} {
		if strings.Contains(lower, candidate.needle) {
			return candidate.name
		}
	}
	return ""
}

func stripEmphasis(value string) string {
	value = strings.ReplaceAll(value, "**", "")
	value = strings.ReplaceAll(value, "`", "")
	return strings.TrimSpace(value)
}
