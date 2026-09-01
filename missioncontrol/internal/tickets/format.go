package tickets

import (
	"fmt"
	"strings"
	"time"
)

const commentHeading = "## Kommentar — "

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
		{"epic", ticket.Epic},
		{"komponente", ticket.Component},
		{"scope", strings.Join(ticket.Scope, ", ")},
		{"tags", strings.Join(ticket.Tags, ", ")},
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
			case "erstellt":
				ticket.CreatedAt, _ = time.Parse(time.RFC3339, value)
			case "aktualisiert":
				ticket.UpdatedAt, _ = time.Parse(time.RFC3339, value)
			}
		}
	}
	body, comments := splitComments(rest)
	ticket.Body = strings.TrimSpace(body)
	ticket.Comments = comments
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
	return ticket, nil
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
