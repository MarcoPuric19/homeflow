package httpd

import (
	"net/http"
	"sort"
	"time"

	"missioncontrol/internal/repo"
	"missioncontrol/internal/tickets"
)

// A ticket's view of the repository, and the repository's view of the tickets.
type ticketLink struct {
	Key      string        `json:"key"`
	Title    string        `json:"titel"`
	Status   string        `json:"status"`
	Owner    string        `json:"zustaendig"`
	Scope    []string      `json:"scope"`
	Changes  []repo.Change `json:"aenderungen"`
	Commits  []repo.Commit `json:"commits"`
	Conflict []conflict    `json:"kollisionen,omitempty"`
}

type conflict struct {
	With    string   `json:"mit"`
	Owner   string   `json:"zustaendig"`
	Reasons []string `json:"grund"`
}

type orphanChange struct {
	repo.Change
	Ticket string `json:"ticket"`
}

// repository joins the two halves: which ticket owns which files, and which
// commit belongs to which ticket.
func (s *Server) repository(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 15*time.Second)
	defer cancel()
	state := repo.Read(ctx, s.git, 50)

	all, err := s.store.List()
	if err != nil {
		s.logger.Error("tickets lesen", "error", err)
		writeError(w, http.StatusInternalServerError, "Tickets konnten nicht gelesen werden")
		return
	}

	byKey := map[string]*ticketLink{}
	links := make([]ticketLink, 0)
	for _, ticket := range all {
		if len(ticket.Scope) == 0 && ticket.Type == tickets.TypeRecord {
			continue
		}
		link := ticketLink{Key: ticket.Key, Title: ticket.Title, Status: ticket.Status,
			Owner: ticket.Owner, Scope: ticket.Scope,
			Changes: make([]repo.Change, 0), Commits: make([]repo.Commit, 0)}
		links = append(links, link)
		byKey[ticket.Key] = &links[len(links)-1]
	}

	// Commits find their ticket through the key in the message.
	for _, commit := range state.Commits {
		for _, key := range commit.Tickets {
			if link, ok := byKey[key]; ok {
				link.Commits = append(link.Commits, commit)
			}
		}
	}

	// Uncommitted changes find their ticket through the scope.
	changes := make([]orphanChange, 0, len(state.Changes))
	for _, change := range state.Changes {
		owner := ""
		for _, ticket := range all {
			for _, pattern := range ticket.Scope {
				if repo.Matches(pattern, change.Path) {
					if link, ok := byKey[ticket.Key]; ok {
						link.Changes = append(link.Changes, change)
					}
					if owner == "" {
						owner = ticket.Key
					}
				}
			}
		}
		changes = append(changes, orphanChange{Change: change, Ticket: owner})
	}

	// Two tickets in progress on overlapping paths is the collision the claim
	// table in Austausch.md catches by hand.
	active := make([]tickets.Ticket, 0)
	for _, ticket := range all {
		if ticket.Status == tickets.StatusActive && len(ticket.Scope) > 0 {
			active = append(active, ticket)
		}
	}
	for i, left := range active {
		for j, right := range active {
			if i >= j {
				continue
			}
			if reasons := repo.Overlap(left.Scope, right.Scope); len(reasons) > 0 {
				byKey[left.Key].Conflict = append(byKey[left.Key].Conflict,
					conflict{With: right.Key, Owner: right.Owner, Reasons: reasons})
				byKey[right.Key].Conflict = append(byKey[right.Key].Conflict,
					conflict{With: left.Key, Owner: left.Owner, Reasons: reasons})
			}
		}
	}

	// Tickets with something to show first; the rest is noise on this page.
	sort.SliceStable(links, func(i, j int) bool {
		return score(links[i]) > score(links[j])
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"branch": state.Branch, "remote": state.Remote, "hatCommits": state.HasCommit,
		"hinweise": state.Notes, "commits": state.Commits,
		"aenderungen": changes, "tickets": links,
	})
}

func score(link ticketLink) int {
	return len(link.Conflict)*100 + len(link.Changes)*10 + len(link.Commits)*5 + len(link.Scope)
}
