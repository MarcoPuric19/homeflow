package repo

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
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
	// Drei Hinweise: kein Remote, kein Commit, kein Spiegel. Der dritte kam mit
	// HF-0106 dazu und ist der einzige, der auf eine Gegenmaßnahme zeigt.
	if len(state.Notes) != 3 {
		t.Fatalf("hinweise=%v", state.Notes)
	}
	alle := strings.Join(state.Notes, " ")
	for _, erwartet := range []string{"Kein Remote", "Kein Spiegel", "mirror-setup"} {
		if !strings.Contains(alle, erwartet) {
			t.Fatalf("Hinweis auf %q fehlt: %v", erwartet, state.Notes)
		}
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
	// Ein Remote existiert, aber keines heißt `spiegel*` - das ist kein Spiegel,
	// und genau das soll dastehen.
	if len(state.Notes) != 1 || !strings.Contains(state.Notes[0], "Kein Spiegel") {
		t.Fatalf("hinweise=%v", state.Notes)
	}
	if len(state.Mirrors) != 0 {
		t.Fatalf("`origin` ist kein Spiegel: %#v", state.Mirrors)
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

// A mirror that nobody notices has gone stale is the failure mode this whole
// feature exists for (ADR 0008): there is no NAS and no second machine here, so
// the only real redundancy is a stick that spends most of its life unplugged.
func TestMirrorsReportBehindUnreachableAndMissing(t *testing.T) {
	head := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	run := func(_ context.Context, args ...string) ([]byte, error) {
		switch {
		case args[0] == "remote" && len(args) == 1:
			return []byte("origin\nspiegel-lokal\nspiegel-usb\n"), nil
		case args[0] == "remote" && args[1] == "get-url":
			return []byte("/pfad/" + args[2] + ".git\n"), nil
		case args[0] == "ls-remote" && args[2] == "spiegel-usb":
			return nil, errors.New("could not read from remote repository")
		case args[0] == "ls-remote":
			return []byte(head + "\trefs/heads/main\n"), nil
		case args[0] == "rev-list":
			return []byte("3\n"), nil
		case args[0] == "log":
			return []byte("2026-08-20T10:00:00+02:00\n"), nil
		}
		return nil, errors.New("unexpected: " + strings.Join(args, " "))
	}

	list := mirrors(context.Background(), run, "main")
	if len(list) != 2 {
		t.Fatalf("got %d mirrors, want 2 - `origin` is not a mirror", len(list))
	}
	lokal, usb := list[0], list[1]
	if !lokal.Reachable || lokal.Behind != 3 {
		t.Fatalf("spiegel-lokal: reachable=%v behind=%d, want true/3", lokal.Reachable, lokal.Behind)
	}
	if lokal.At.IsZero() {
		t.Fatal("spiegel-lokal has no timestamp for its newest mirrored commit")
	}
	if usb.Reachable {
		t.Fatal("an unmounted stick must read as unreachable, not as up to date")
	}

	notes := mirrorNotes(list)
	if len(notes) != 2 {
		t.Fatalf("notes = %#v, want one for behind and one for unreachable", notes)
	}

	if got := mirrorNotes(nil); len(got) != 1 || !strings.Contains(got[0], "Kein Spiegel") {
		t.Fatalf("no mirror at all must say so plainly, got %#v", got)
	}
}

// The near mirror shares a disk with the working tree. Saying "up to date" and
// stopping there would sell it as something it is not.
func TestSingleLocalMirrorIsNotSoldAsRedundancy(t *testing.T) {
	list := []Mirror{{Name: "spiegel-lokal", Reachable: true, Behind: 0, At: time.Now()}}
	notes := mirrorNotes(list)
	if len(notes) != 1 || !strings.Contains(notes[0], "derselben Platte") {
		t.Fatalf("notes = %#v, want the warning that one local mirror is not redundancy", notes)
	}
	zwei := append(list, Mirror{Name: "spiegel-usb", Reachable: true, Behind: 0, At: time.Now()})
	if got := mirrorNotes(zwei); len(got) != 0 {
		t.Fatalf("two healthy mirrors need no note, got %#v", got)
	}
}
