package tickets

import (
	"errors"
	"fmt"
	"strings"
)

// AddRelationBy links two tickets and always writes both halves. A relation
// kept on one side only is worse than none: it looks maintained and is wrong
// exactly when someone relies on it.
func (s *Store) AddRelationBy(key, kind, other string, by Actor) (Ticket, error) {
	kind, other, by.Name = strings.TrimSpace(kind), strings.TrimSpace(other), strings.TrimSpace(by.Name)
	inverse, ok := Inverse(kind)
	if !ok || by.Name == "" || !ValidKey(key) || !ValidKey(other) {
		return Ticket{}, ErrInvalid
	}
	if key == other {
		return Ticket{}, fmt.Errorf("%w: ein Ticket kann sich nicht auf sich selbst beziehen", ErrInvalid)
	}
	if _, err := s.Get(other); err != nil {
		return Ticket{}, fmt.Errorf("%w: %s", err, other)
	}
	// Only the blocking direction can form a cycle that means anything. "A
	// blocks B blocks A" is not a statement about order, it is a mistake.
	if kind == RelBlocks || kind == RelDependsOn {
		from, to := key, other
		if kind == RelDependsOn {
			from, to = other, key
		}
		cyclic, err := s.blocksReaches(to, from)
		if err != nil {
			return Ticket{}, err
		}
		if cyclic {
			return Ticket{}, fmt.Errorf("%w: %s und %s wuerden sich gegenseitig blockieren", ErrInvalid, key, other)
		}
	}
	if _, err := s.linkOneSide(other, inverse, key, by); err != nil {
		return Ticket{}, err
	}
	return s.linkOneSide(key, kind, other, by)
}

// RemoveRelationBy drops both halves, for the same reason AddRelationBy writes
// both.
func (s *Store) RemoveRelationBy(key, kind, other string, by Actor) (Ticket, error) {
	kind, other, by.Name = strings.TrimSpace(kind), strings.TrimSpace(other), strings.TrimSpace(by.Name)
	inverse, ok := Inverse(kind)
	if !ok || by.Name == "" || !ValidKey(key) || !ValidKey(other) {
		return Ticket{}, ErrInvalid
	}
	if _, err := s.unlinkOneSide(other, inverse, key, by); err != nil && !errors.Is(err, ErrNotFound) {
		return Ticket{}, err
	}
	return s.unlinkOneSide(key, kind, other, by)
}

func (s *Store) linkOneSide(key, kind, other string, by Actor) (Ticket, error) {
	return s.update(key, func(ticket *Ticket) {
		for _, r := range ticket.Relations {
			if r.Kind == kind && r.Key == other {
				return
			}
		}
		ticket.Relations = append(ticket.Relations, Relation{Kind: kind, Key: other})
		ticket.Events = append(ticket.Events, Event{
			At: s.now().Truncate(1e9), Author: by.Name, Origin: by.Origin,
			Field: "Beziehung", From: "", To: kind + " " + other,
		})
	})
}

func (s *Store) unlinkOneSide(key, kind, other string, by Actor) (Ticket, error) {
	return s.update(key, func(ticket *Ticket) {
		kept := make([]Relation, 0, len(ticket.Relations))
		removed := false
		for _, r := range ticket.Relations {
			if r.Kind == kind && r.Key == other {
				removed = true
				continue
			}
			kept = append(kept, r)
		}
		if !removed {
			return
		}
		ticket.Relations = kept
		ticket.Events = append(ticket.Events, Event{
			At: s.now().Truncate(1e9), Author: by.Name, Origin: by.Origin,
			Field: "Beziehung", From: kind + " " + other, To: "",
		})
	})
}

// blocksReaches answers whether `from` already blocks `to`, directly or through
// other tickets. It walks the blocking edges only.
func (s *Store) blocksReaches(from, to string) (bool, error) {
	all, err := s.List()
	if err != nil {
		return false, err
	}
	edges := map[string][]string{}
	for _, t := range all {
		for _, r := range t.Relations {
			switch r.Kind {
			case RelBlocks:
				edges[t.Key] = append(edges[t.Key], r.Key)
			case RelDependsOn:
				edges[r.Key] = append(edges[r.Key], t.Key)
			}
		}
	}
	seen := map[string]bool{}
	var walk func(string) bool
	walk = func(node string) bool {
		if node == to {
			return true
		}
		if seen[node] {
			return false
		}
		seen[node] = true
		for _, next := range edges[node] {
			if walk(next) {
				return true
			}
		}
		return false
	}
	return walk(from), nil
}

// OpenBlockers lists the tickets that block this one and are not finished yet.
// It is used to warn on a claim, never to prevent one: sometimes the person
// knows better than the graph.
func (s *Store) OpenBlockers(key string) ([]string, error) {
	ticket, err := s.Get(key)
	if err != nil {
		return nil, err
	}
	open := []string{}
	for _, r := range ticket.Relations {
		if r.Kind != RelDependsOn {
			continue
		}
		other, err := s.Get(r.Key)
		if err != nil {
			continue
		}
		if other.Status != StatusDone {
			open = append(open, other.Key+" ("+other.Status+")")
		}
	}
	return open, nil
}
