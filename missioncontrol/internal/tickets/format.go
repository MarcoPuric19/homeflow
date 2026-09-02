package tickets

import (
	"fmt"
	"strings"
	"time"
)

const commentHeading = "## Kommentar — "

// protokollHeading marks the audit trail. It is rendered before the comments so
// that splitting on the comment heading cannot swallow it, and so that a reader
// opening the file sees what happened before reading what was said.
const protokollHeading = "## Protokoll\n"

// render writes a ticket as Markdown with a small header. The format is meant
// to be edited by hand as well: an assistant without the cockpit can open the
// file, append a comment section and be understood.
func render(ticket Ticket) string {
	var b strings.Builder
	b.WriteString("---\n")
	for _, line := range [][2]string{
		{"id", ticket.Key},
		{"projekt", ticket.Project},
		{"typ", ticket.Type},
		{"titel", ticket.Title},
		{"status", ticket.Status},
		{"prioritaet", ticket.Priority},
		{"zustaendig", ticket.Owner},
		{"melder", ticket.Reporter},
		{"version", ticket.Version},
		{"geliefert", ticket.Delivered},
		{"geliefert_am", optionalTime(ticket.DeliveredAt)},
		{"epic", ticket.Epic},
		{"komponente", ticket.Component},
		{"scope", strings.Join(ticket.Scope, ", ")},
		{"tags", strings.Join(ticket.Tags, ", ")},
		{"beziehungen", renderRelations(ticket.Relations)},
		{"erstellt", ticket.CreatedAt.Format(time.RFC3339)},
		{"aktualisiert", ticket.UpdatedAt.Format(time.RFC3339)},
	} {
		fmt.Fprintf(&b, "%s: %s\n", line[0], line[1])
	}
	b.WriteString("---\n\n")
	if ticket.Body != "" {
		b.WriteString(ticket.Body)
		b.WriteString("\n")
	}
	if len(ticket.Events) > 0 {
		b.WriteString("\n" + protokollHeading + "\n")
		for _, e := range ticket.Events {
			from, to := e.From, e.To
			if from == "" {
				from = "(leer)"
			}
			if to == "" {
				to = "(leer)"
			}
			origin := e.Origin
			if origin == "" {
				origin = "(unbekannt)"
			}
			fmt.Fprintf(&b, "- %s · %s · %s · %s → %s · %s\n",
				e.At.Format(time.RFC3339), e.Author, e.Field, from, to, origin)
		}
	}
	for _, comment := range ticket.Comments {
		fmt.Fprintf(&b, "\n%s%s · %s\n\n%s\n", commentHeading, comment.Author, comment.At.Format(time.RFC3339), comment.Text)
	}
	return b.String()
}

// parse reads a ticket file. Unknown header keys are ignored rather than
// rejected, so a human can add a note without breaking the tool.
func parse(key, content string) (Ticket, error) {
	ticket := Ticket{
		Key: key, Status: StatusBacklog, Owner: "offen",
		Type: TypeTask, Priority: PriorityMedium,
	}
	// The key carries the project, so a file can never disagree with its name.
	if project, _, found := strings.Cut(key, "-"); found {
		ticket.Project = project
	}
	rest := content
	if strings.HasPrefix(content, "---\n") {
		end := strings.Index(content[4:], "\n---\n")
		if end < 0 {
			return Ticket{}, fmt.Errorf("%w: Kopf nicht abgeschlossen", ErrInvalid)
		}
		header := content[4 : 4+end]
		rest = content[4+end+5:]
		for _, line := range strings.Split(header, "\n") {
			name, value, found := strings.Cut(line, ":")
			if !found {
				continue
			}
			name, value = strings.TrimSpace(name), strings.TrimSpace(value)
			switch name {
			case "titel":
				ticket.Title = value
			case "typ":
				if ValidType(value) {
					ticket.Type = value
				}
			case "status":
				if ValidStatus(value) {
					ticket.Status = value
				} else if mapped, ok := legacyStatus(value); ok {
					ticket.Status = mapped
				}
			case "prioritaet":
				if ValidPriority(value) {
					ticket.Priority = value
				}
			case "zustaendig":
				ticket.Owner = value
			case "melder":
				ticket.Reporter = value
			case "version":
				ticket.Version = value
			case "geliefert_am":
				ticket.DeliveredAt, _ = time.Parse(time.RFC3339, value)
			case "geliefert":
				ticket.Delivered = value
			case "epic":
				if ValidKey(value) {
					ticket.Epic = value
				}
			case "komponente":
				ticket.Component = value
			case "scope":
				ticket.Scope = cleanScope(strings.Split(value, ","))
			case "tags":
				ticket.Tags = cleanTags(strings.Split(value, ","))
			case "beziehungen":
				ticket.Relations = parseRelations(value)
			case "erstellt":
				ticket.CreatedAt, _ = time.Parse(time.RFC3339, value)
			case "aktualisiert":
				ticket.UpdatedAt, _ = time.Parse(time.RFC3339, value)
			}
		}
	}
	body, comments := splitComments(rest)
	body, events := splitEvents(body)
	ticket.Body = strings.TrimSpace(body)
	ticket.Comments = comments
	ticket.Events = events
	if ticket.Title == "" {
		ticket.Title = key
	}
	if ticket.Reporter == "" {
		ticket.Reporter = ticket.Owner
	}
	// Never hand out nil lists: a consumer reading `null` where it expects a
	// list is a bug waiting in the browser.
	if ticket.Tags == nil {
		ticket.Tags = []string{}
	}
	if ticket.Scope == nil {
		ticket.Scope = []string{}
	}
	if ticket.Comments == nil {
		ticket.Comments = []Comment{}
	}
	if ticket.Events == nil {
		ticket.Events = []Event{}
	}
	if ticket.Relations == nil {
		ticket.Relations = []Relation{}
	}
	return ticket, nil
}

// splitEvents peels the audit trail off the body. A value containing the exact
// separator " · " would confuse this; field values are capped at 60 characters
// and none has ever contained it, but a malformed line is skipped rather than
// guessed at.
// renderRelations writes "blockiert MC-0027, hängt ab von MC-0025". The kind
// comes first because it reads as a sentence, and because a key alone would not
// say what the link means.
func renderRelations(relations []Relation) string {
	parts := make([]string, 0, len(relations))
	for _, r := range relations {
		parts = append(parts, r.Kind+" "+r.Key)
	}
	return strings.Join(parts, ", ")
}

// parseRelations is strict about the kind and drops what it does not
// understand, so a typo cannot silently become a different relation. The kinds
// are tried longest first: "hängt ab von" would otherwise never be reached if a
// shorter kind were a prefix of it.
func parseRelations(value string) []Relation {
	relations := []Relation{}
	for _, entry := range strings.Split(value, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		for _, kind := range []string{RelDependsOn, RelDuplicates, RelBlocks, RelRelates} {
			if !strings.HasPrefix(entry, kind+" ") {
				continue
			}
			key := strings.TrimSpace(strings.TrimPrefix(entry, kind))
			if ValidKey(key) {
				relations = append(relations, Relation{Kind: kind, Key: key})
			}
			break
		}
	}
	return relations
}

func splitEvents(rest string) (string, []Event) {
	head, block, found := strings.Cut(rest, protokollHeading)
	if !found {
		return rest, nil
	}
	events := []Event{}
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		parts := strings.SplitN(strings.TrimPrefix(line, "- "), " · ", 5)
		if len(parts) < 5 {
			continue
		}
		at, err := time.Parse(time.RFC3339, strings.TrimSpace(parts[0]))
		if err != nil {
			continue
		}
		from, to, _ := strings.Cut(parts[3], " → ")
		events = append(events, Event{
			At: at, Author: strings.TrimSpace(parts[1]), Field: strings.TrimSpace(parts[2]),
			From: blankToEmpty(from), To: blankToEmpty(to), Origin: blankToEmpty(parts[4]),
		})
	}
	return head, events
}

// blankToEmpty turns the placeholders back into the empty strings they stand
// for, so a round trip through the file does not invent a value.
func blankToEmpty(value string) string {
	value = strings.TrimSpace(value)
	if value == "(leer)" || value == "(unbekannt)" {
		return ""
	}
	return value
}

func splitComments(rest string) (string, []Comment) {
	parts := strings.Split(rest, commentHeading)
	body := parts[0]
	comments := make([]Comment, 0, len(parts)-1)
	for _, part := range parts[1:] {
		line, text, _ := strings.Cut(part, "\n")
		author, stamp, _ := strings.Cut(line, " · ")
		at, _ := time.Parse(time.RFC3339, strings.TrimSpace(stamp))
		comments = append(comments, Comment{
			Author: strings.TrimSpace(author), At: at, Text: strings.TrimSpace(text),
		})
	}
	return body, comments
}

// optionalTime keeps an unset time an empty line rather than the zero date.
// "0001-01-01" in a header reads like a value and is none.
func optionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}
