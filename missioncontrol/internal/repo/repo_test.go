package repo

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestScopeMatchesTheThreeFormsPeopleWrite(t *testing.T) {
	cases := []struct {
		pattern, file string
		want          bool
	}{
		// Verzeichnis deckt alles darunter ab.
		{"backend/internal/pets/", "backend/internal/pets/service.go", true},
		{"backend/internal/pets", "backend/internal/pets/postgres.go", true},
		{"backend/internal/pets", "backend/internal/petsold/service.go", false},
		{"backend/internal/pets", "backend/internal/expenses/service.go", false},
		// Genauer Pfad.
		{"Makefile", "Makefile", true},
		{"Makefile", "missioncontrol/Makefile", false},
		// Muster.
		{"backend/**/*_test.go", "backend/internal/pets/service_test.go", true},
		{"backend/**/*_test.go", "backend/internal/pets/service.go", false},
		{"backend/*/pets", "backend/internal/pets", true},
		{"backend/*/pets", "backend/internal/deep/pets", false},
		{"**/openapi.yaml", "docs/api/openapi.yaml", true},
		{"missioncontrol/**", "missioncontrol/internal/repo/git.go", true},
		// Führendes ./ und Leerraum stören nicht.
		{"./Makefile", "Makefile", true},
		{" Makefile ", "./Makefile", true},
		{"", "Makefile", false},
	}
	for _, testCase := range cases {
		if got := Matches(testCase.pattern, testCase.file); got != testCase.want {
			t.Fatalf("Matches(%q, %q)=%v, erwartet %v", testCase.pattern, testCase.file, got, testCase.want)
		}
	}
}

func TestOverlapFindsCollidingClaims(t *testing.T) {
	// Der Fall, der am 2026-09-01 wirklich auftrat: Codex hielt backend/internal
	// und die Makefile war frei.
	codex := []string{"backend/internal/expenses/", "backend/internal/pets/"}
	mine := []string{"Makefile", "missioncontrol/**"}
	if hits := Overlap(codex, mine); len(hits) != 0 {
		t.Fatalf("falscher Alarm: %v", hits)
	}
	colliding := []string{"backend/internal/pets/service.go"}
	if hits := Overlap(codex, colliding); len(hits) == 0 {
		t.Fatal("Kollision nicht erkannt")
	}
	// Zwei Muster, von denen keines das andere trifft, die sich aber
	// überschneiden können.
	if hits := Overlap([]string{"backend/**"}, []string{"**/pets"}); len(hits) == 0 {
		t.Fatal("Musterüberschneidung nicht erkannt")
	}
}

// fakeGit replays recorded git output.
func fakeGit(responses map[string]string, failing ...string) Runner {
	fails := map[string]bool{}
	for _, key := range failing {
		fails[key] = true
	}
	return func(_ context.Context, args ...string) ([]byte, error) {
		key := args[0]
		if fails[key] {
			return nil, errors.New("git " + key + ": simulierter Fehler")
		}
		value, ok := responses[key]
		if !ok {
			return nil, errors.New("git " + key + ": kein Testwert")
		}
		return []byte(value), nil
	}
}

func TestReadReportsARepositoryWithoutCommits(t *testing.T) {
	// Genau der Zustand dieses Repos am 2026-09-01.
	state := Read(context.Background(), fakeGit(map[string]string{
		"rev-parse": "", "branch": "main\n", "remote": "",
		"status": "?? AGENTS.md\n?? Austausch.md\n",
	}, "log"), 10)

	if state.Branch != "main" || state.HasCommit {
		t.Fatalf("state=%#v", state)
	}
	if len(state.Notes) != 2 {
		t.Fatalf("hinweise=%v", state.Notes)
	}
	if !strings.Contains(strings.Join(state.Notes, " "), "Kein Remote") {
		t.Fatalf("Remote-Hinweis fehlt: %v", state.Notes)
	}
	if len(state.Changes) != 2 || state.Changes[0].Status != "ungetrackt" {
		t.Fatalf("aenderungen=%#v", state.Changes)
	}
}

func TestReadParsesCommitsAndTheirTicketKeys(t *testing.T) {
	// Die Vorlage stammt aus echtem `git log` dieses Repos, nicht aus einer
	// Annahme darüber, wie git ausgibt. Die erste Fassung dieses Tests war
	// selbst gebastelt und hat den Fehler durchgewinkt, den der erste Commit
	// dann zeigte: 15 statt 300 Dateien.
	raw, err := os.ReadFile("testdata/gitlog-echt.txt")
	if err != nil {
		t.Skipf("Vorlage fehlt: %v", err)
	}
	state := Read(context.Background(), fakeGit(map[string]string{
		"rev-parse": "main\n", "remote": "", "log": string(raw), "status": "",
	}), 10)

	if !state.HasCommit || len(state.Commits) != 1 {
		t.Fatalf("commits=%d", len(state.Commits))
	}
	commit := state.Commits[0]
	if commit.Short == "" || commit.Author == "" || commit.At.IsZero() {
		t.Fatalf("commit=%#v", commit)
	}
	// Eine mehrzeilige Nachricht mit Absätzen darf die Dateiliste nicht kürzen.
	if len(commit.Files) < 200 {
		t.Fatalf("nur %d Dateien erkannt - die Nachricht hat die Liste verschluckt", len(commit.Files))
	}
	for _, file := range commit.Files {
		if strings.ContainsAny(file, "\x1e\x1f") || strings.HasPrefix(file, " ") {
			t.Fatalf("kaputter Dateiname: %q", file)
		}
	}
	// Der Ticketschlüssel steht im Rumpf, nicht im Betreff.
	if len(commit.Tickets) != 1 || commit.Tickets[0] != "HF-0010" {
		t.Fatalf("tickets=%v", commit.Tickets)
	}
}

func TestCommitKeysAreReadFromSubjectAndBody(t *testing.T) {
	log := "\x1eabc123def456\x1fabc123d\x1fMarco\x1f2026-09-02T10:00:00+02:00\x1f" +
		"HF-0042 Rollen wirksam machen\x1fSiehe auch MC-0003.\n\nZweiter Absatz.\n\x1f" +
		"\nbackend/internal/auth/auth.go\nbackend/internal/httpapi/server.go\n"
	state := Read(context.Background(), fakeGit(map[string]string{
		"rev-parse": "main\n", "remote": "origin\n", "log": log, "status": "",
	}), 10)
	commit := state.Commits[0]
	if len(commit.Tickets) != 2 || commit.Tickets[0] != "HF-0042" || commit.Tickets[1] != "MC-0003" {
		t.Fatalf("tickets=%v", commit.Tickets)
	}
	if len(commit.Files) != 2 || commit.Files[0] != "backend/internal/auth/auth.go" {
		t.Fatalf("dateien=%v", commit.Files)
	}
	if len(state.Notes) != 0 {
		t.Fatalf("unerwartete Hinweise: %v", state.Notes)
	}
}

func TestStatusCodesAreTranslated(t *testing.T) {
	state := Read(context.Background(), fakeGit(map[string]string{
		"rev-parse": "main\n", "remote": "origin\n", "log": "", "status": strings.Join([]string{
			"?? neu.txt", " M geaendert.go", "A  hinzu.go", " D weg.go", `R  alt.go -> neu.go`,
		}, "\n"),
	}), 10)
	want := map[string]string{
		"neu.txt": "ungetrackt", "geaendert.go": "geändert", "hinzu.go": "hinzugefügt",
		"weg.go": "gelöscht", "neu.go": "umbenannt",
	}
	if len(state.Changes) != len(want) {
		t.Fatalf("aenderungen=%#v", state.Changes)
	}
	for _, change := range state.Changes {
		if want[change.Path] != change.Status {
			t.Fatalf("%s=%q, erwartet %q", change.Path, change.Status, want[change.Path])
		}
	}
}

func TestTicketKeyPatternIsStrict(t *testing.T) {
	found := TicketKey.FindAllString("HF-0042 und MC-0003, aber nicht XX-0001 oder HF-42 oder HF-00423", -1)
	if len(found) != 2 {
		t.Fatalf("gefunden=%v", found)
	}
}
