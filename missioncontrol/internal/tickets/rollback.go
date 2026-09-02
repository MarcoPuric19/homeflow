package tickets

import (
	"strings"
	"time"
)

// WithdrawDelivery is the inverse of accepting a ticket after a deploy: the
// delivered version is cleared and the ticket goes back to Test.
//
// It exists because "geliefert" is a claim about the world, not about the
// repository. If the deploy that shipped a ticket is rolled back, the claim is
// no longer true — and a ticket that says it shipped in a version that is no
// longer running is worse than one that says nothing, because nobody rechecks
// it (HF-0104).
//
// Only tickets that a deploy actually closed are touched. Something a person
// moved to Fertig by hand stays there: the rollback undoes a delivery, not a
// human decision.
func (s *Store) WithdrawDelivery(key, version string, by Actor) (Ticket, error) {
	by.Name = strings.TrimSpace(by.Name)
	version = strings.TrimSpace(version)
	if by.Name == "" || version == "" || !ValidKey(key) {
		return Ticket{}, ErrInvalid
	}
	ticket, err := s.Get(key)
	if err != nil {
		return Ticket{}, err
	}
	if ticket.Delivered != version {
		// Das Ticket haengt an einer anderen Auslieferung - oder an keiner.
		// Nicht anfassen.
		return ticket, nil
	}
	updated, err := s.update(key, func(t *Ticket) {
		t.Events = append(t.Events, Event{
			At: s.now().Truncate(1e9), Author: by.Name, Origin: by.Origin,
			Field: "Geliefert in", From: t.Delivered, To: "",
		})
		t.Delivered = ""
		t.DeliveredAt = time.Time{}
		if t.Status == StatusDone {
			t.Events = append(t.Events, Event{
				At: s.now().Truncate(1e9), Author: by.Name, Origin: by.Origin,
				Field: "Status", From: StatusDone, To: StatusTest,
			})
			t.Status = StatusTest
		}
	})
	return updated, err
}
