// Package tickets stores the exchange between the assistants as plain files.
//
// One ticket is one Markdown file with a small header. That is deliberate:
// both assistants can read and write tickets with nothing but their normal
// file tools, the history stays diffable and committable, and the cockpit
// being down never blocks the exchange. It is the property that made
// Austausch.md work - only structured, so it can be queried.
package tickets

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotFound = errors.New("ticket nicht gefunden")
	ErrInvalid  = errors.New("ticket ungültig")
)

type Store struct {
	mu       sync.Mutex
	dir      string
	now      func() time.Time
	versions map[string]string // project -> default version
}

// NewStore takes the current version per project, so a new ticket is stamped
// with the release its project is actually on.
func NewStore(dir string, versions map[string]string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("ticket-Verzeichnis fehlt")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("ticket-Verzeichnis anlegen: %w", err)
	}
	copied := map[string]string{}
	for key, value := range versions {
		copied[key] = value
	}
	return &Store{dir: dir, now: func() time.Time { return time.Now() }, versions: copied}, nil
}

func (s *Store) path(key string) (string, error) {
	if !ValidKey(key) {
		return "", ErrInvalid
	}
	return filepath.Join(s.dir, key+".md"), nil
}

func (s *Store) List() ([]Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listLocked()
}

func (s *Store) listLocked() ([]Ticket, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	tickets := make([]Ticket, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".md") {
			continue
		}
		key := strings.TrimSuffix(name, ".md")
		if !ValidKey(key) {
			continue
		}
		ticket, err := s.readLocked(key)
		if err != nil {
			return nil, err
		}
		tickets = append(tickets, ticket)
	}
	// Newest first within a project, projects side by side.
	sort.Slice(tickets, func(i, j int) bool {
		if tickets[i].Project != tickets[j].Project {
			return tickets[i].Project < tickets[j].Project
		}
		return tickets[i].Key > tickets[j].Key
	})
	return tickets, nil
}

func (s *Store) Get(key string) (Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readLocked(key)
}

func (s *Store) readLocked(key string) (Ticket, error) {
	path, err := s.path(key)
	if err != nil {
		return Ticket{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Ticket{}, ErrNotFound
	}
	if err != nil {
		return Ticket{}, err
	}
	ticket, err := parse(key, string(data))
	if err != nil {
		return Ticket{}, err
	}
	// Tickets written before a field existed adopt the current default rather
	// than showing a gap; the next write persists it.
	if ticket.Version == "" {
		ticket.Version = s.versions[ticket.Project]
	}
	return ticket, nil
}

type NewTicket struct {
	Project   string   `json:"project"`
	Type      string   `json:"type"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	Owner     string   `json:"owner"`
	Priority  string   `json:"priority"`
	Version   string   `json:"version"`
	Epic      string   `json:"epic"`
	Component string   `json:"component"`
	Scope     []string `json:"scope"`
	Tags      []string `json:"tags"`
	Author    string   `json:"author"`
}

func (s *Store) Create(input NewTicket) (Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createLocked(input, time.Time{}, "")
}

// createLocked also serves the importer, which needs to keep the original date
// and status of work that happened before this board existed.
func (s *Store) createLocked(input NewTicket, at time.Time, status string) (Ticket, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Author = strings.TrimSpace(input.Author)
	if input.Title == "" || len(input.Title) > 300 || input.Author == "" {
		return Ticket{}, ErrInvalid
	}
	if input.Project == "" {
		input.Project = ProjectMission
	}
	if !ValidProject(input.Project) {
		return Ticket{}, ErrInvalid
	}
	if input.Type == "" {
		input.Type = TypeTask
	}
	if !ValidType(input.Type) {
		return Ticket{}, ErrInvalid
	}
	if input.Priority == "" {
		input.Priority = PriorityMedium
	}
	if !ValidPriority(input.Priority) {
		return Ticket{}, ErrInvalid
	}
	if input.Epic != "" && !ValidKey(input.Epic) {
		return Ticket{}, ErrInvalid
	}
	if status == "" {
		status = StatusBacklog
	}
	if !ValidStatus(status) {
		return Ticket{}, ErrInvalid
	}

	key, err := s.nextKeyLocked(input.Project)
	if err != nil {
		return Ticket{}, err
	}
	moment := at
	if moment.IsZero() {
		moment = s.now()
	}
	// The file format stores RFC3339 without fractions, so truncate here: an
	// in-memory ticket and the same ticket read back must be identical.
	moment = moment.Truncate(time.Second)
	ticket := Ticket{
		Key: key, Project: input.Project, Type: input.Type, Title: input.Title,
		Status: status, Priority: input.Priority, Owner: strings.TrimSpace(input.Owner),
		Reporter: input.Author, Version: strings.TrimSpace(input.Version),
		Epic: input.Epic, Component: strings.TrimSpace(input.Component),
		Scope: cleanScope(input.Scope),
		Tags:  cleanTags(input.Tags), CreatedAt: moment, UpdatedAt: moment,
		Body: strings.TrimSpace(input.Body), Comments: []Comment{},
	}
	if ticket.Owner == "" {
		ticket.Owner = "offen"
	}
	if ticket.Version == "" {
		ticket.Version = s.versions[ticket.Project]
	}
	return ticket, s.writeLocked(ticket)
}

func (s *Store) nextKeyLocked(project string) (string, error) {
	existing, err := s.listLocked()
	if err != nil {
		return "", err
	}
	next := 1
	for _, ticket := range existing {
		if ticket.Project != project {
			continue
		}
		if number, err := strconv.Atoi(strings.TrimPrefix(ticket.Key, project+"-")); err == nil && number >= next {
			next = number + 1
		}
	}
	return fmt.Sprintf("%s-%04d", project, next), nil
}

func (s *Store) Comment(key, author, text string) (Ticket, error) {
	author, text = strings.TrimSpace(author), strings.TrimSpace(text)
	if author == "" || text == "" {
		return Ticket{}, ErrInvalid
	}
	return s.update(key, func(ticket *Ticket) {
		ticket.Comments = append(ticket.Comments, Comment{Author: author, At: s.now().Truncate(time.Second), Text: text})
	})
}

// Field changes all share one shape: validate, note the change as a comment so
// the history stays in the file, and write.
func (s *Store) setField(key, author, label, value string, check func(string) bool, get func(*Ticket) string, set func(*Ticket, string)) (Ticket, error) {
	return s.setFieldBy(key, Actor{Name: author}, label, value, check, get, set)
}

// setFieldBy records the change as an Event rather than a Comment. Until
// MC-0025 both lived in the comment stream, which meant the history of a ticket
// could not be read apart from the conversation about it — and a hand-written
// comment reading "Status: Test → Fertig" was indistinguishable from a real
// change.
func (s *Store) setFieldBy(key string, by Actor, label, value string, check func(string) bool, get func(*Ticket) string, set func(*Ticket, string)) (Ticket, error) {
	value, by.Name = strings.TrimSpace(value), strings.TrimSpace(by.Name)
	if value == "" || by.Name == "" || len(value) > 60 || (check != nil && !check(value)) {
		return Ticket{}, ErrInvalid
	}
	return s.update(key, func(ticket *Ticket) {
		previous := get(ticket)
		if previous == value {
			return
		}
		set(ticket, value)
		ticket.Events = append(ticket.Events, Event{
			At: s.now().Truncate(time.Second), Author: by.Name, Origin: by.Origin,
			Field: label, From: previous, To: value,
		})
	})
}

// SetFieldBy is the entry point the HTTP layer uses, so that the observable
// origin of a request reaches the record. The field name comes from the route,
// never from the request body.
func (s *Store) SetFieldBy(key, field, value string, by Actor) (Ticket, error) {
	switch field {
	case "status":
		return s.setFieldBy(key, by, "Status", value, ValidStatus,
			func(t *Ticket) string { return t.Status }, func(t *Ticket, v string) { t.Status = v })
	case "owner":
		return s.setFieldBy(key, by, "Zuständig", value, nil,
			func(t *Ticket) string { return t.Owner }, func(t *Ticket, v string) { t.Owner = v })
	case "version":
		return s.setFieldBy(key, by, "Version", value, nil,
			func(t *Ticket) string { return t.Version }, func(t *Ticket, v string) { t.Version = v })
	case "priority":
		return s.setFieldBy(key, by, "Priorität", value, ValidPriority,
			func(t *Ticket) string { return t.Priority }, func(t *Ticket, v string) { t.Priority = v })
	case "type":
		return s.setFieldBy(key, by, "Typ", value, ValidType,
			func(t *Ticket) string { return t.Type }, func(t *Ticket, v string) { t.Type = v })
	case "epic":
		return s.setFieldBy(key, by, "Epic", value, ValidKey,
			func(t *Ticket) string { return t.Epic }, func(t *Ticket, v string) { t.Epic = v })
	default:
		return Ticket{}, ErrInvalid
	}
}

func (s *Store) SetStatus(key, status, author string) (Ticket, error) {
	return s.setField(key, author, "Status", status, ValidStatus,
		func(t *Ticket) string { return t.Status }, func(t *Ticket, v string) { t.Status = v })
}

func (s *Store) SetOwner(key, owner, author string) (Ticket, error) {
	return s.setField(key, author, "Zuständig", owner, nil,
		func(t *Ticket) string { return t.Owner }, func(t *Ticket, v string) { t.Owner = v })
}

func (s *Store) SetVersion(key, version, author string) (Ticket, error) {
	return s.setField(key, author, "Version", version, nil,
		func(t *Ticket) string { return t.Version }, func(t *Ticket, v string) { t.Version = v })
}

// SetDelivered records which release actually shipped a ticket. Only a
// deployment has that answer, which is why nothing sets it at creation time.
// SetDelivered records both halves: which release shipped it and when. The date
// comes from the store rather than the caller so that two tickets accepted by
// the same run cannot disagree about it.
func (s *Store) SetDelivered(key, version, author string) (Ticket, error) {
	return s.setField(key, author, "Geliefert in", version, nil,
		func(t *Ticket) string { return t.Delivered },
		func(t *Ticket, v string) {
			t.Delivered = v
			if v == "" {
				t.DeliveredAt = time.Time{}
				return
			}
			t.DeliveredAt = time.Now().UTC()
		})
}

func (s *Store) SetPriority(key, priority, author string) (Ticket, error) {
	return s.setField(key, author, "Priorität", priority, ValidPriority,
		func(t *Ticket) string { return t.Priority }, func(t *Ticket, v string) { t.Priority = v })
}

func (s *Store) SetType(key, value, author string) (Ticket, error) {
	return s.setField(key, author, "Typ", value, ValidType,
		func(t *Ticket) string { return t.Type }, func(t *Ticket, v string) { t.Type = v })
}

func (s *Store) SetEpic(key, epic, author string) (Ticket, error) {
	return s.setField(key, author, "Epic", epic, ValidKey,
		func(t *Ticket) string { return t.Epic }, func(t *Ticket, v string) { t.Epic = v })
}

// SetScope takes a comma separated list, because that is how it is typed and
// how it is stored in the file.
func (s *Store) SetScope(key, scope, author string) (Ticket, error) {
	return s.SetScopeBy(key, scope, Actor{Name: author})
}

// SetScopeBy has its own implementation because the scope is a list, not a
// value: it is normalised before the comparison, so that reordering or
// respacing the same paths is not recorded as a change.
func (s *Store) SetScopeBy(key, scope string, by Actor) (Ticket, error) {
	by.Name = strings.TrimSpace(by.Name)
	if by.Name == "" {
		return Ticket{}, ErrInvalid
	}
	cleaned := cleanScope(strings.Split(scope, ","))
	return s.update(key, func(ticket *Ticket) {
		previous := strings.Join(ticket.Scope, ", ")
		next := strings.Join(cleaned, ", ")
		if previous == next {
			return
		}
		ticket.Scope = cleaned
		ticket.Events = append(ticket.Events, Event{
			At: s.now().Truncate(time.Second), Author: by.Name, Origin: by.Origin,
			Field: "Scope", From: previous, To: next,
		})
	})
}

func (s *Store) update(key string, change func(*Ticket)) (Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ticket, err := s.readLocked(key)
	if err != nil {
		return Ticket{}, err
	}
	change(&ticket)
	ticket.UpdatedAt = s.now().Truncate(time.Second)
	return ticket, s.writeLocked(ticket)
}

// writeLocked replaces the file atomically, so a crash mid-write never leaves a
// half-written ticket behind.
func (s *Store) writeLocked(ticket Ticket) error {
	path, err := s.path(ticket.Key)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(s.dir, ".ticket-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer func() {
		_ = temp.Close()
		_ = os.Remove(name)
	}()
	if _, err := temp.WriteString(render(ticket)); err != nil {
		return err
	}
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
