package repo

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Runner exists so tests can replay recorded git output.
type Runner func(ctx context.Context, args ...string) ([]byte, error)

// GitRunner only ever runs read-only commands. Mission Control observes the
// repository; it never commits, stages or checks anything out.
func GitRunner(binary, root string) Runner {
	return func(ctx context.Context, args ...string) ([]byte, error) {
		full := append([]string{"-C", root}, args...)
		cmd := exec.CommandContext(ctx, binary, full...)
		var stderr strings.Builder
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
		}
		return out, nil
	}
}

// TicketKey is the link: a commit message mentioning HF-0042 belongs to that
// ticket. Same convention as Jira's smart commits, minus the magic words.
var TicketKey = regexp.MustCompile(`\b(HF|MC)-[0-9]{4}\b`)

type Commit struct {
	Hash    string    `json:"hash"`
	Short   string    `json:"kurz"`
	Author  string    `json:"autor"`
	At      time.Time `json:"zeit"`
	Subject string    `json:"betreff"`
	Tickets []string  `json:"tickets"`
	Files   []string  `json:"dateien,omitempty"`
}

type Change struct {
	Path   string `json:"pfad"`
	Status string `json:"status"` // untracked | modified | added | deleted | renamed
}

type State struct {
	Branch    string   `json:"branch"`
	Remote    string   `json:"remote"`
	HasCommit bool     `json:"hatCommits"`
	Commits   []Commit `json:"commits"`
	Changes   []Change `json:"aenderungen"`
	Notes     []string `json:"hinweise"`
}

// newState keeps every list empty rather than nil: a consumer that reads
// `null` where it expects a list is a bug waiting in the browser.
func newState() State {
	return State{Commits: make([]Commit, 0), Changes: make([]Change, 0), Notes: make([]string, 0)}
}

const commitSeparator = "\x1e"
const fieldSeparator = "\x1f"

// Read collects everything the cockpit shows about the repository. A repository
// without commits is a normal state here, not an error: this one has none, and
// that fact is worth showing rather than hiding behind an empty list.
func Read(ctx context.Context, run Runner, limit int) State {
	state := newState()

	if out, err := run(ctx, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		state.Branch = strings.TrimSpace(string(out))
	}
	if state.Branch == "" || state.Branch == "HEAD" {
		if out, err := run(ctx, "branch", "--show-current"); err == nil {
			state.Branch = strings.TrimSpace(string(out))
		}
	}
	if out, err := run(ctx, "remote"); err == nil {
		state.Remote = strings.TrimSpace(string(out))
	}
	if state.Remote == "" {
		state.Notes = append(state.Notes, "Kein Remote: die Historie existiert nur auf diesem Gerät.")
	}

	if out, err := run(ctx, "log", "-1", "--format=%H"); err == nil && strings.TrimSpace(string(out)) != "" {
		state.HasCommit = true
	}
	if !state.HasCommit {
		state.Notes = append(state.Notes,
			"Noch kein Commit. Sobald einer existiert, ordnet Mission Control ihn über den Ticketschlüssel im Betreff zu (z. B. \"HF-0042 …\").")
	} else {
		state.Commits = commits(ctx, run, limit)
	}
	state.Changes = changes(ctx, run)
	return state
}

func commits(ctx context.Context, run Runner, limit int) []Commit {
	// The separator goes at the *front*: git prints the format first and the
	// --name-only file list after it, so a separator at the end would put every
	// commit's files into the next block. And the body needs a closing field
	// separator, because a message with blank lines would otherwise swallow the
	// list. The first commit of this repository had exactly that shape and was
	// reported with 15 of 300 files.
	format := commitSeparator + strings.Join([]string{"%H", "%h", "%an", "%aI", "%s", "%b"}, fieldSeparator) +
		fieldSeparator
	out, err := run(ctx, "log", "--max-count="+strconv.Itoa(limit), "--name-only", "--format="+format)
	if err != nil {
		return nil
	}
	list := make([]Commit, 0)
	for _, block := range strings.Split(string(out), commitSeparator) {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		parts := strings.SplitN(block, fieldSeparator, 7)
		if len(parts) < 7 {
			continue
		}
		body, files := parts[5], parts[6]
		commit := Commit{Hash: parts[0], Short: parts[1], Author: parts[2], Subject: parts[4]}
		commit.At, _ = time.Parse(time.RFC3339, parts[3])
		for _, match := range TicketKey.FindAllString(parts[4]+" "+body, -1) {
			if !contains(commit.Tickets, match) {
				commit.Tickets = append(commit.Tickets, match)
			}
		}
		for _, file := range strings.Split(files, "\n") {
			if file = strings.TrimSpace(file); file != "" {
				commit.Files = append(commit.Files, file)
			}
		}
		list = append(list, commit)
	}
	return list
}

func changes(ctx context.Context, run Runner) []Change {
	// -uall lists untracked files individually. Without it git collapses a whole
	// new directory into one line, and a scope like missioncontrol/internal/logs/
	// would never match anything.
	out, err := run(ctx, "status", "--porcelain", "-uall")
	if err != nil {
		return nil
	}
	list := make([]Change, 0)
	for _, line := range strings.Split(string(out), "\n") {
		if len(line) < 4 {
			continue
		}
		code, file := strings.TrimSpace(line[:2]), strings.TrimSpace(line[3:])
		// Renames read as "old -> new"; the new path is the interesting one.
		if _, after, found := strings.Cut(file, " -> "); found {
			file = after
		}
		file = strings.Trim(file, `"`)
		list = append(list, Change{Path: file, Status: describe(code)})
	}
	return list
}

func describe(code string) string {
	switch {
	case code == "??":
		return "ungetrackt"
	case strings.Contains(code, "D"):
		return "gelöscht"
	case strings.Contains(code, "R"):
		return "umbenannt"
	case strings.Contains(code, "A"):
		return "hinzugefügt"
	case strings.Contains(code, "M"):
		return "geändert"
	default:
		return code
	}
}

func contains(list []string, value string) bool {
	for _, entry := range list {
		if entry == value {
			return true
		}
	}
	return false
}
