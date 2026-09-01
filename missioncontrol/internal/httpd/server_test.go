package httpd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"missioncontrol/internal/cluster"
	"missioncontrol/internal/deploy"
	"missioncontrol/internal/tickets"
)

func newTestServer(t *testing.T) (*Server, *tickets.Store) {
	t.Helper()
	// Speicher und Server bekommen dieselbe Version - in main.go ist das
	// dieselbe Konstante, und ein Auseinanderlaufen wäre genau der Fehler,
	// den dieser Test sonst verdecken würde.
	store, err := tickets.NewStore(t.TempDir(), map[string]string{tickets.ProjectMission: "test", tickets.ProjectHomeFlow: "9.9.9"})
	if err != nil {
		t.Fatal(err)
	}
	run := func(_ context.Context, _ ...string) ([]byte, error) {
		data, err := os.ReadFile(filepath.Join("..", "cluster", "testdata", "homeflow-combined.json"))
		if err != nil {
			t.Fatalf("fixture: %v", err)
		}
		return data, nil
	}
	server := New(slog.New(slog.NewTextHandler(io.Discard, nil)), store, run, noGit, nil,
		Config{Namespaces: []string{"homeflow"}, Interval: time.Hour, Version: "test", RepoRoot: filepath.Join("..", "..", "..")})
	return server, store
}

func call(t *testing.T, server *Server, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, target, reader)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	return recorder
}

func TestClusterEndpointServesTheWarmSnapshot(t *testing.T) {
	server, _ := newTestServer(t)
	// Vor dem ersten Snapshot muss die Antwort leer, aber gültig sein - die
	// Oberfläche darf nicht auf einen Fehler laufen, nur weil sie schnell war.
	empty := call(t, server, http.MethodGet, "/api/cluster", "")
	if empty.Code != http.StatusOK {
		t.Fatalf("status=%d", empty.Code)
	}
	server.takeSnapshot(context.Background())
	response := call(t, server, http.MethodGet, "/api/cluster", "")
	var snapshot cluster.Snapshot
	if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Nodes) == 0 || len(snapshot.Edges) == 0 {
		t.Fatalf("snapshot leer: %#v", snapshot)
	}
	if len(snapshot.Warnings) != 0 {
		t.Fatalf("warnungen=%v", snapshot.Warnings)
	}
}

func TestTicketLifecycleOverHTTP(t *testing.T) {
	server, _ := newTestServer(t)
	created := call(t, server, http.MethodPost, "/api/tickets",
		`{"title":"Erstes Ticket","body":"Text","owner":"Codex","tags":["cockpit"],"author":"Claude Code"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", created.Code, created.Body.String())
	}
	var ticket tickets.Ticket
	if err := json.Unmarshal(created.Body.Bytes(), &ticket); err != nil {
		t.Fatal(err)
	}
	if ticket.Key != "MC-0001" || ticket.Status != tickets.StatusBacklog {
		t.Fatalf("ticket=%#v", ticket)
	}

	if got := call(t, server, http.MethodPost, "/api/tickets/"+ticket.Key+"/comments",
		`{"author":"Codex","text":"Antwort"}`); got.Code != http.StatusOK {
		t.Fatalf("kommentar status=%d body=%s", got.Code, got.Body.String())
	}
	if got := call(t, server, http.MethodPost, "/api/tickets/"+ticket.Key+"/status",
		`{"wert":"In Bearbeitung","author":"Codex"}`); got.Code != http.StatusOK {
		t.Fatalf("status status=%d body=%s", got.Code, got.Body.String())
	}
	if got := call(t, server, http.MethodPost, "/api/tickets/"+ticket.Key+"/owner",
		`{"wert":"Claude Code","author":"Marco"}`); got.Code != http.StatusOK {
		t.Fatalf("owner status=%d body=%s", got.Code, got.Body.String())
	}

	listed := call(t, server, http.MethodGet, "/api/tickets", "")
	var list struct {
		Tickets []tickets.Ticket `json:"tickets"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Tickets) != 1 {
		t.Fatalf("liste=%#v", list.Tickets)
	}
	final := list.Tickets[0]
	if final.Status != tickets.StatusActive || final.Owner != "Claude Code" || len(final.Comments) != 3 {
		t.Fatalf("ticket=%#v", final)
	}
}

func TestTicketFilterByStatus(t *testing.T) {
	server, _ := newTestServer(t)
	for _, title := range []string{"A", "B"} {
		call(t, server, http.MethodPost, "/api/tickets", `{"title":"`+title+`","author":"Marco"}`)
	}
	call(t, server, http.MethodPost, "/api/tickets/MC-0002/status", `{"wert":"Fertig","author":"Marco"}`)
	response := call(t, server, http.MethodGet, "/api/tickets?status=Fertig", "")
	var list struct {
		Tickets []tickets.Ticket `json:"tickets"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Tickets) != 1 || list.Tickets[0].Key != "MC-0002" {
		t.Fatalf("gefiltert=%#v", list.Tickets)
	}
}

func TestBadRequestsAreAnsweredPrecisely(t *testing.T) {
	server, _ := newTestServer(t)
	call(t, server, http.MethodPost, "/api/tickets", `{"title":"Basis","author":"Marco"}`)
	cases := map[string]struct {
		method, target, body string
		want                 int
	}{
		"leerer Titel":       {http.MethodPost, "/api/tickets", `{"title":"  ","author":"Marco"}`, http.StatusUnprocessableEntity},
		"fehlender Autor":    {http.MethodPost, "/api/tickets", `{"title":"X"}`, http.StatusUnprocessableEntity},
		"unbekanntes Feld":   {http.MethodPost, "/api/tickets", `{"title":"X","author":"Marco","prio":"hoch"}`, http.StatusBadRequest},
		"kaputtes JSON":      {http.MethodPost, "/api/tickets", `{`, http.StatusBadRequest},
		"unbekanntes Ticket": {http.MethodGet, "/api/tickets/MC-9999", "", http.StatusNotFound},
		"kaputte ID":         {http.MethodGet, "/api/tickets/unsinn", "", http.StatusUnprocessableEntity},
		"Pfad-Traversal":     {http.MethodGet, "/api/tickets/..%2F..%2Fetc", "", http.StatusUnprocessableEntity},
		"unbekannter Status": {http.MethodPost, "/api/tickets/MC-0001/status", `{"wert":"vielleicht","author":"Marco"}`, http.StatusUnprocessableEntity},
		"leerer Kommentar":   {http.MethodPost, "/api/tickets/MC-0001/comments", `{"author":"Marco","text":""}`, http.StatusUnprocessableEntity},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			response := call(t, server, testCase.method, testCase.target, testCase.body)
			if response.Code != testCase.want {
				t.Fatalf("status=%d, erwartet %d, body=%s", response.Code, testCase.want, response.Body.String())
			}
		})
	}
}

func TestSecurityHeadersAreSet(t *testing.T) {
	server, _ := newTestServer(t)
	response := call(t, server, http.MethodGet, "/api/health", "")
	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	} {
		if got := response.Header().Get(header); got != want {
			t.Fatalf("%s=%q, erwartet %q", header, got, want)
		}
	}
	if csp := response.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
		t.Fatalf("CSP=%q", csp)
	}
}

func TestStatusesMatchTheStore(t *testing.T) {
	for _, status := range Statuses() {
		if !tickets.ValidStatus(status) {
			t.Fatalf("Oberfläche kennt Status %q, den der Speicher ablehnt", status)
		}
	}
}

func TestBoardExposesColumnsAndVersion(t *testing.T) {
	server, _ := newTestServer(t)
	response := call(t, server, http.MethodGet, "/api/board", "")
	var board struct {
		Spalten []string `json:"spalten"`
		Version string   `json:"version"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &board); err != nil {
		t.Fatal(err)
	}
	if board.Version != "test" {
		t.Fatalf("version=%q", board.Version)
	}
	want := []string{"Backlog", "Ready", "In Bearbeitung", "Test", "Fertig"}
	for i := range want {
		if board.Spalten[i] != want[i] {
			t.Fatalf("spalten=%v", board.Spalten)
		}
	}
}

func TestTicketVersionOverHTTP(t *testing.T) {
	server, _ := newTestServer(t)
	created := call(t, server, http.MethodPost, "/api/tickets", `{"title":"X","author":"Marco"}`)
	var ticket tickets.Ticket
	if err := json.Unmarshal(created.Body.Bytes(), &ticket); err != nil {
		t.Fatal(err)
	}
	if ticket.Version != "test" {
		t.Fatalf("version=%q, erwartet die laufende Mission-Control-Version", ticket.Version)
	}
	changed := call(t, server, http.MethodPost, "/api/tickets/"+ticket.Key+"/version",
		`{"wert":"0.3.0","author":"Marco"}`)
	if changed.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", changed.Code, changed.Body.String())
	}
	if empty := call(t, server, http.MethodPost, "/api/tickets/"+ticket.Key+"/version",
		`{"wert":"","author":"Marco"}`); empty.Code != http.StatusUnprocessableEntity {
		t.Fatalf("leere Version status=%d", empty.Code)
	}
}

func TestLogsAreParsedCountedAndFiltered(t *testing.T) {
	server, _ := newTestServer(t)
	var captured []string
	server.run = func(_ context.Context, args ...string) ([]byte, error) {
		captured = args
		return []byte(`2026-09-01T07:52:07.415Z {"time":"2026-09-01T07:52:07.415Z","level":"INFO","msg":"http request","method":"GET","path":"/health","status":200}
2026-09-01T07:52:08.415Z {"time":"2026-09-01T07:52:08.415Z","level":"ERROR","msg":"db down","path":"/api/v1/expenses","status":500}
2026-09-01T07:52:09.415Z time=2026-09-01T07:52:09.415Z level=WARN msg="scrape langsam" scrape_pool=api
`), nil
	}
	response := call(t, server, http.MethodGet, "/api/logs?namespace=homeflow&pod=homeflow-api-1&lines=50", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Eintraege []struct {
			Level     string            `json:"level"`
			Nachricht string            `json:"nachricht"`
			Felder    map[string]string `json:"felder"`
		} `json:"eintraege"`
		Zaehler    map[string]int `json:"zaehler"`
		Gesamt     int            `json:"gesamt"`
		Gefiltert  int            `json:"gefiltert"`
		Histogramm []struct {
			Gesamt int `json:"gesamt"`
		} `json:"histogramm"`
		Felder []struct {
			Name string `json:"name"`
		} `json:"felder"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Gesamt != 3 || payload.Gefiltert != 3 {
		t.Fatalf("gesamt=%d gefiltert=%d", payload.Gesamt, payload.Gefiltert)
	}
	if payload.Zaehler["ERROR"] != 1 || payload.Zaehler["WARN"] != 1 || payload.Zaehler["INFO"] != 1 {
		t.Fatalf("zaehler=%v", payload.Zaehler)
	}
	if payload.Eintraege[1].Level != "ERROR" || payload.Eintraege[1].Felder["status"] != "500" {
		t.Fatalf("eintrag=%#v", payload.Eintraege[1])
	}
	if len(payload.Histogramm) == 0 || len(payload.Felder) == 0 {
		t.Fatalf("histogramm=%d felder=%d", len(payload.Histogramm), len(payload.Felder))
	}
	want := []string{"-n", "homeflow", "logs", "homeflow-api-1", "--tail", "50", "--timestamps"}
	if strings.Join(captured, " ") != strings.Join(want, " ") {
		t.Fatalf("kubectl-Aufruf=%v", captured)
	}
}

func TestLogFiltersNarrowTheResultButNotTheCounters(t *testing.T) {
	server, _ := newTestServer(t)
	server.run = func(_ context.Context, _ ...string) ([]byte, error) {
		return []byte(`2026-09-01T07:52:07.415Z {"time":"2026-09-01T07:52:07.415Z","level":"INFO","msg":"ok","path":"/health"}
2026-09-01T07:52:08.415Z {"time":"2026-09-01T07:52:08.415Z","level":"ERROR","msg":"kaputt","path":"/api"}
`), nil
	}
	read := func(target string) (int, map[string]int) {
		response := call(t, server, http.MethodGet, target, "")
		var payload struct {
			Gefiltert int            `json:"gefiltert"`
			Zaehler   map[string]int `json:"zaehler"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		return payload.Gefiltert, payload.Zaehler
	}
	base := "/api/logs?namespace=homeflow&pod=api"
	if matched, counts := read(base + "&level=ERROR"); matched != 1 || counts["INFO"] != 1 {
		t.Fatalf("Levelfilter: gefiltert=%d zaehler=%v", matched, counts)
	}
	if matched, _ := read(base + "&q=kaputt"); matched != 1 {
		t.Fatalf("Textfilter=%d", matched)
	}
	if matched, _ := read(base + "&feld=path:/health"); matched != 1 {
		t.Fatalf("Feldfilter=%d", matched)
	}
	if matched, _ := read(base + "&level=ERROR,WARN&feld=path:/health"); matched != 0 {
		t.Fatalf("kombinierte Filter=%d", matched)
	}
}

func TestLogsRejectAnythingUnsafe(t *testing.T) {
	server, _ := newTestServer(t)
	server.run = func(_ context.Context, _ ...string) ([]byte, error) {
		t.Fatal("kubectl darf für diese Anfragen gar nicht erst aufgerufen werden")
		return nil, nil
	}
	cases := map[string]struct {
		target string
		want   int
	}{
		"ohne Pod":             {"/api/logs?namespace=homeflow", http.StatusBadRequest},
		"ohne Namespace":       {"/api/logs?pod=api", http.StatusBadRequest},
		"fremder Namespace":    {"/api/logs?namespace=kube-system&pod=etcd", http.StatusForbidden},
		"Flag im Podnamen":     {"/api/logs?namespace=homeflow&pod=--all-containers", http.StatusBadRequest},
		"Leerzeichen im Namen": {"/api/logs?namespace=homeflow&pod=api%20-n%20kube-system", http.StatusBadRequest},
		"Pfad im Namen":        {"/api/logs?namespace=homeflow&pod=../../etc/passwd", http.StatusBadRequest},
		"Grossbuchstaben":      {"/api/logs?namespace=homeflow&pod=Api", http.StatusBadRequest},
		"zu viele Zeilen":      {"/api/logs?namespace=homeflow&pod=api&lines=99999", http.StatusBadRequest},
		"lines keine Zahl":     {"/api/logs?namespace=homeflow&pod=api&lines=viele", http.StatusBadRequest},
		"Container ungueltig":  {"/api/logs?namespace=homeflow&pod=api&container=--kubeconfig", http.StatusBadRequest},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			if response := call(t, server, http.MethodGet, testCase.target, ""); response.Code != testCase.want {
				t.Fatalf("status=%d, erwartet %d", response.Code, testCase.want)
			}
		})
	}
}

func TestLogsOfAStartingPodAreNotAnError(t *testing.T) {
	server, _ := newTestServer(t)
	server.run = func(_ context.Context, _ ...string) ([]byte, error) {
		return nil, errors.New("container is creating")
	}
	response := call(t, server, http.MethodGet, "/api/logs?namespace=homeflow&pod=neu", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d", response.Code)
	}
	if !strings.Contains(response.Body.String(), "hinweis") {
		t.Fatalf("body=%s", response.Body.String())
	}
}

// noGit stands in for a repository that answers nothing, so tests that do not
// care about git do not have to script it.
func noGit(_ context.Context, args ...string) ([]byte, error) {
	return nil, errors.New("kein git im Test")
}

func TestRepositoryLinksTicketsToFilesAndCommits(t *testing.T) {
	server, store := newTestServer(t)
	claimed, err := store.Create(tickets.NewTicket{
		Title: "Am Backend", Author: "Codex", Owner: "Codex",
		Scope: []string{"backend/internal/pets/"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetStatus(claimed.Key, tickets.StatusActive, "Codex"); err != nil {
		t.Fatal(err)
	}
	other, err := store.Create(tickets.NewTicket{
		Title: "Auch am Backend", Author: "Claude Code", Owner: "Claude Code",
		Scope: []string{"backend/internal/pets/service.go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetStatus(other.Key, tickets.StatusActive, "Claude Code"); err != nil {
		t.Fatal(err)
	}

	// Aufbau wie echtes `git log`: Trennzeichen vorn, Dateiliste nach dem Rumpf.
	log := "\x1e" + strings.Join([]string{"abc123def", "abc123d", "Marco", "2026-09-02T10:00:00+02:00",
		claimed.Key + " Pets aufgeräumt", "Mit Absatz.\n\nUnd noch einem.\n"}, "\x1f") +
		"\x1f\nbackend/internal/pets/service.go\n"
	server.git = func(_ context.Context, args ...string) ([]byte, error) {
		switch args[0] {
		case "rev-parse":
			return []byte("main\n"), nil
		case "remote":
			return []byte(""), nil
		case "log":
			return []byte(log), nil
		case "status":
			return []byte(" M backend/internal/pets/service.go\n?? notizen.txt\n"), nil
		}
		return nil, errors.New("unerwartet: " + args[0])
	}

	response := call(t, server, http.MethodGet, "/api/repo", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Branch      string   `json:"branch"`
		HatCommits  bool     `json:"hatCommits"`
		Hinweise    []string `json:"hinweise"`
		Aenderungen []struct {
			Pfad   string `json:"pfad"`
			Ticket string `json:"ticket"`
		} `json:"aenderungen"`
		Tickets []struct {
			Key         string `json:"key"`
			Aenderungen []struct {
				Pfad string `json:"pfad"`
			} `json:"aenderungen"`
			Commits []struct {
				Kurz string `json:"kurz"`
			} `json:"commits"`
			Kollisionen []struct {
				Mit   string   `json:"mit"`
				Grund []string `json:"grund"`
			} `json:"kollisionen"`
		} `json:"tickets"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Branch != "main" || !payload.HatCommits {
		t.Fatalf("payload=%#v", payload)
	}
	// Ohne Remote muss der Hinweis stehen - das ist P3-4. Seit HF-0106 kommt der
	// fehlende Spiegel dazu: beides beschreibt dieselbe Lage, aber nur der
	// zweite nennt die Gegenmaßnahme.
	alle := strings.Join(payload.Hinweise, " ")
	if len(payload.Hinweise) != 2 || !strings.Contains(alle, "Remote") || !strings.Contains(alle, "Spiegel") {
		t.Fatalf("hinweise=%v", payload.Hinweise)
	}

	byKey := map[string]int{}
	for index, link := range payload.Tickets {
		byKey[link.Key] = index
	}
	first := payload.Tickets[byKey[claimed.Key]]
	if len(first.Commits) != 1 || first.Commits[0].Kurz != "abc123d" {
		t.Fatalf("Commit nicht zugeordnet: %#v", first)
	}
	if len(first.Aenderungen) != 1 || first.Aenderungen[0].Pfad != "backend/internal/pets/service.go" {
		t.Fatalf("Änderung nicht zugeordnet: %#v", first)
	}
	if len(first.Kollisionen) != 1 || first.Kollisionen[0].Mit != other.Key {
		t.Fatalf("Kollision fehlt: %#v", first)
	}

	// Eine Datei ohne Ticket bleibt sichtbar, aber ohne Zuordnung.
	orphan := false
	for _, change := range payload.Aenderungen {
		if change.Pfad == "notizen.txt" {
			orphan = change.Ticket == ""
		}
	}
	if !orphan {
		t.Fatalf("verwaiste Änderung falsch zugeordnet: %#v", payload.Aenderungen)
	}
}

func TestRepositorySurvivesAMissingGit(t *testing.T) {
	server, _ := newTestServer(t)
	response := call(t, server, http.MethodGet, "/api/repo", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d", response.Code)
	}
	if !strings.Contains(response.Body.String(), "hinweise") {
		t.Fatalf("body=%s", response.Body.String())
	}
}

func TestScopeIsSetAndCleared(t *testing.T) {
	server, _ := newTestServer(t)
	created := call(t, server, http.MethodPost, "/api/tickets", `{"title":"Mit Scope","author":"Marco"}`)
	var ticket tickets.Ticket
	if err := json.Unmarshal(created.Body.Bytes(), &ticket); err != nil {
		t.Fatal(err)
	}
	set := call(t, server, http.MethodPost, "/api/tickets/"+ticket.Key+"/scope",
		`{"wert":"Makefile, missioncontrol/**","author":"Marco"}`)
	if set.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", set.Code, set.Body.String())
	}
	var updated tickets.Ticket
	if err := json.Unmarshal(set.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if len(updated.Scope) != 2 || updated.Scope[0] != "Makefile" {
		t.Fatalf("scope=%v", updated.Scope)
	}
	// Leeren muss erlaubt sein - ein Claim wird auch wieder abgegeben.
	cleared := call(t, server, http.MethodPost, "/api/tickets/"+ticket.Key+"/scope",
		`{"wert":"","author":"Marco"}`)
	if cleared.Code != http.StatusOK {
		t.Fatalf("status=%d", cleared.Code)
	}
	if err := json.Unmarshal(cleared.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if len(updated.Scope) != 0 {
		t.Fatalf("scope nach dem Leeren=%v", updated.Scope)
	}
}

func TestChangesFromAForeignOriginAreRefused(t *testing.T) {
	server, _ := newTestServer(t)
	body := `{"title":"Von einer fremden Seite","author":"Angreifer"}`

	// Eine beliebige Webseite kann den Browser dazu bringen, hierher zu posten.
	// Genau das muss scheitern, sonst legt ein Besuch irgendwo Tickets an - und
	// später deployt er den Cluster.
	request := httptest.NewRequest(http.MethodPost, "/api/tickets", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://beliebige-seite.example")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("fremder Origin: status=%d", recorder.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/tickets", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Sec-Fetch-Site", "cross-site")
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("cross-site: status=%d", recorder.Code)
	}
}

func TestOwnPageAndLocalToolsStillWork(t *testing.T) {
	server, _ := newTestServer(t)
	body := `{"title":"Aus der eigenen Oberfläche","author":"Marco"}`

	// Die eigene Seite schickt ihren Origin mit.
	request := httptest.NewRequest(http.MethodPost, "/api/tickets", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://"+request.Host)
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("eigene Oberfläche: status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	// curl und Skripte schicken keins von beidem - Codex muss weiterarbeiten
	// können.
	if response := call(t, server, http.MethodPost, "/api/tickets",
		`{"title":"Aus einem Skript","author":"Codex"}`); response.Code != http.StatusCreated {
		t.Fatalf("Skript: status=%d body=%s", response.Code, response.Body.String())
	}

	// Lesen bleibt in jedem Fall erlaubt.
	request = httptest.NewRequest(http.MethodGet, "/api/tickets", nil)
	request.Header.Set("Origin", "https://beliebige-seite.example")
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("Lesen: status=%d", recorder.Code)
	}
}

func deployServer(t *testing.T, contextName string) (*Server, *tickets.Store) {
	t.Helper()
	server, store := newTestServer(t)
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	server.deploy = deploy.NewRunner(dir,
		write("make", "#!/bin/sh\necho \"ok $*\"\n"),
		write("kubectl", "#!/bin/sh\necho "+contextName+"\n"))
	// New() verdrahtet den Rückruf, wenn der Runner beim Bauen schon da ist.
	// Hier wird er nachträglich gesetzt, also muss das hier auch passieren -
	// sonst testet der Helfer eine Verdrahtung, die es nicht gibt.
	server.deploy.OnFinish = server.afterDeploy
	return server, store
}

func TestDeployIsOffUnlessWiredIn(t *testing.T) {
	server, _ := newTestServer(t)
	state := call(t, server, http.MethodGet, "/api/deploy", "")
	if !strings.Contains(state.Body.String(), "abgeschaltet") {
		t.Fatalf("body=%s", state.Body.String())
	}
	if start := call(t, server, http.MethodPost, "/api/deploy",
		`{"pipeline":"pruefen","author":"Marco"}`); start.Code != http.StatusForbidden {
		t.Fatalf("status=%d", start.Code)
	}
}

func TestDeployStartsAndCommentsOnTheTicket(t *testing.T) {
	server, store := deployServer(t, "kind-dev")
	ticket, err := store.Create(tickets.NewTicket{Title: "Deploy-Ticket", Author: "Marco"})
	if err != nil {
		t.Fatal(err)
	}
	response := call(t, server, http.MethodPost, "/api/deploy",
		`{"pipeline":"pruefen","author":"Marco","tickets":["`+ticket.Key+`"]}`)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	// Ein Deploy gehört ins Ticket, sonst ist er nach zwei Tagen unauffindbar.
	updated, err := store.Get(ticket.Key)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Comments) != 1 || !strings.Contains(updated.Comments[0].Text, "Deploy gestartet") {
		t.Fatalf("kommentare=%#v", updated.Comments)
	}
}

func TestDeployRefusesForeignClusterAndUnknownPipeline(t *testing.T) {
	server, _ := deployServer(t, "produktion-eu-west")
	refused := call(t, server, http.MethodPost, "/api/deploy", `{"pipeline":"homeflow","author":"Marco"}`)
	if refused.Code != http.StatusPreconditionFailed {
		t.Fatalf("fremder Cluster: status=%d body=%s", refused.Code, refused.Body.String())
	}
	if !strings.Contains(refused.Body.String(), "produktion-eu-west") {
		t.Fatalf("body=%s", refused.Body.String())
	}
	if unknown := call(t, server, http.MethodPost, "/api/deploy",
		`{"pipeline":"beliebig","author":"Marco"}`); unknown.Code != http.StatusNotFound {
		t.Fatalf("unbekannte Pipeline: status=%d", unknown.Code)
	}
	if bad := call(t, server, http.MethodPost, "/api/deploy",
		`{"pipeline":"pruefen","author":"Marco","ticket":"kein-schluessel"}`); bad.Code != http.StatusUnprocessableEntity {
		t.Fatalf("kaputter Ticketschlüssel: status=%d", bad.Code)
	}
}

func TestDeployCannotBeStartedFromAForeignPage(t *testing.T) {
	server, _ := deployServer(t, "kind-dev")
	request := httptest.NewRequest(http.MethodPost, "/api/deploy",
		strings.NewReader(`{"pipeline":"homeflow","author":"Angreifer"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://beliebige-seite.example")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status=%d - eine fremde Seite darf niemals deployen", recorder.Code)
	}
}

func TestLogTimeRangeOverHTTP(t *testing.T) {
	server, _ := newTestServer(t)
	server.run = func(_ context.Context, _ ...string) ([]byte, error) {
		return []byte(`2026-09-01T10:00:00Z {"level":"INFO","msg":"erste"}
2026-09-01T10:01:00Z {"level":"ERROR","msg":"zweite"}
2026-09-01T10:02:00Z {"level":"INFO","msg":"dritte"}
`), nil
	}
	read := func(target string) (int, int) {
		response := call(t, server, http.MethodGet, target, "")
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		var payload struct {
			Gefiltert int `json:"gefiltert"`
			Gesamt    int `json:"gesamt"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		return payload.Gefiltert, payload.Gesamt
	}
	base := "/api/logs?namespace=homeflow&pod=api"

	// RFC 3339, wie es jemand von Hand in die Adresszeile schreibt.
	if matched, total := read(base + "&von=2026-09-01T10:00:30Z&bis=2026-09-01T10:01:30Z"); matched != 1 || total != 3 {
		t.Fatalf("RFC3339: gefiltert=%d gesamt=%d", matched, total)
	}
	// Millisekunden, wie das Histogramm sie liefert.
	from := time.Date(2026, 9, 1, 10, 0, 30, 0, time.UTC).UnixMilli()
	to := time.Date(2026, 9, 1, 10, 1, 30, 0, time.UTC).UnixMilli()
	if matched, _ := read(fmt.Sprintf("%s&von=%d&bis=%d", base, from, to)); matched != 1 {
		t.Fatalf("Millisekunden: gefiltert=%d", matched)
	}
	// Unsinn in den Parametern darf nicht zum Fehler führen, sondern zu
	// "kein Zeitraum" - sonst bricht ein alter Lesezeichen-Link die Seite.
	if matched, _ := read(base + "&von=gestern&bis="); matched != 3 {
		t.Fatalf("unlesbarer Zeitraum: gefiltert=%d", matched)
	}
}

// waitForRun wartet auf das Ende des Durchlaufs. Danach steht fest, ob der
// Rückruf gelaufen ist - auf einen Status zu warten, der nicht kommen soll,
// würde jeden Negativtest um Sekunden verlängern.
func waitForRun(t *testing.T, server *Server) deploy.Run {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if current := server.deploy.Current(); current != nil && !current.Running() {
			// Der Rückruf läuft im selben Goroutine unmittelbar danach.
			time.Sleep(20 * time.Millisecond)
			return *current
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("Durchlauf endete nicht")
	return deploy.Run{}
}

func ticketStatus(t *testing.T, store *tickets.Store, key string) tickets.Ticket {
	t.Helper()
	ticket, err := store.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	return ticket
}

func TestSuccessfulDeployPullsItsTicketFromTestToDone(t *testing.T) {
	server, store := deployServer(t, "kind-dev")
	ticket, err := store.Create(tickets.NewTicket{Title: "Wartet auf Abnahme", Author: "Claude Code"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetStatus(ticket.Key, tickets.StatusTest, "Claude Code"); err != nil {
		t.Fatal(err)
	}
	response := call(t, server, http.MethodPost, "/api/deploy",
		`{"pipeline":"monitoring","author":"Marco","tickets":["`+ticket.Key+`"],"trotzdem":true}`)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	waitForRun(t, server)
	done := ticketStatus(t, store, ticket.Key)
	if done.Status != tickets.StatusDone {
		t.Fatalf("status=%q, erwartet %q", done.Status, tickets.StatusDone)
	}
	// Ein Status, der sich unerklärt ändert, wäre schlimmer als einer von Hand.
	// Der Wechsel selbst wird von SetStatus protokolliert, die Begründung steht
	// in dem Kommentar davor - geprüft wird deshalb über alle Kommentare.
	var explanation string
	for _, comment := range done.Comments {
		if strings.Contains(comment.Text, "Test auf Fertig") {
			explanation = comment.Text
		}
	}
	if explanation == "" {
		t.Fatalf("kein Kommentar erklärt den Wechsel: %#v", done.Comments)
	}
	if !strings.Contains(explanation, "Durchlauf #") || !strings.Contains(explanation, "erzwungen") {
		t.Fatalf("Begründung unvollständig: %q", explanation)
	}
}

func TestDeployOnlyClosesTicketsThatWaitForAcceptance(t *testing.T) {
	// Wer nicht auf Test steht, wartet nicht auf Abnahme - und wird nicht
	// abgenommen. Sonst schließt ein Deploy Arbeit, die noch läuft.
	for _, start := range []string{tickets.StatusBacklog, tickets.StatusReady, tickets.StatusActive} {
		t.Run(start, func(t *testing.T) {
			server, store := deployServer(t, "kind-dev")
			ticket, err := store.Create(tickets.NewTicket{Title: "Noch nicht so weit", Author: "Claude Code"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.SetStatus(ticket.Key, start, "Claude Code"); err != nil {
				t.Fatal(err)
			}
			if response := call(t, server, http.MethodPost, "/api/deploy",
				`{"pipeline":"monitoring","author":"Marco","tickets":["`+ticket.Key+`"],"trotzdem":true}`); response.Code != http.StatusAccepted {
				t.Fatalf("status=%d", response.Code)
			}
			waitForRun(t, server)
			after := ticketStatus(t, store, ticket.Key)
			if after.Status != start {
				t.Fatalf("status=%q, erwartet unverändert %q", after.Status, start)
			}
		})
	}
}

func TestCheckOnlyPipelineAcceptsNothing(t *testing.T) {
	// "Nur prüfen" ändert nichts am Cluster und darf deshalb auch nichts
	// abnehmen.
	server, store := deployServer(t, "kind-dev")
	ticket, err := store.Create(tickets.NewTicket{Title: "Nur geprüft", Author: "Claude Code"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetStatus(ticket.Key, tickets.StatusTest, "Claude Code"); err != nil {
		t.Fatal(err)
	}
	if response := call(t, server, http.MethodPost, "/api/deploy",
		`{"pipeline":"pruefen","author":"Marco","tickets":["`+ticket.Key+`"]}`); response.Code != http.StatusAccepted {
		t.Fatalf("status=%d", response.Code)
	}
	waitForRun(t, server)
	after := ticketStatus(t, store, ticket.Key)
	if after.Status != tickets.StatusTest {
		t.Fatalf("status=%q, erwartet %q", after.Status, tickets.StatusTest)
	}
}

func TestFailedDeployLeavesTheTicketAlone(t *testing.T) {
	server, store := newTestServer(t)
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	server.deploy = deploy.NewRunner(dir,
		write("make", "#!/bin/sh\necho 'Error: kaputt' >&2\nexit 2\n"),
		write("kubectl", "#!/bin/sh\necho kind-dev\n"))
	server.deploy.OnFinish = server.afterDeploy

	ticket, err := store.Create(tickets.NewTicket{Title: "Deploy schlägt fehl", Author: "Claude Code"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetStatus(ticket.Key, tickets.StatusTest, "Claude Code"); err != nil {
		t.Fatal(err)
	}
	if response := call(t, server, http.MethodPost, "/api/deploy",
		`{"pipeline":"monitoring","author":"Marco","tickets":["`+ticket.Key+`"],"trotzdem":true}`); response.Code != http.StatusAccepted {
		t.Fatalf("status=%d", response.Code)
	}
	waitForRun(t, server)
	after := ticketStatus(t, store, ticket.Key)
	if after.Status != tickets.StatusTest {
		t.Fatalf("status=%q - ein fehlgeschlagener Deploy darf nichts abnehmen", after.Status)
	}
}

func TestRestartRefusesWithoutDeployService(t *testing.T) {
	server, _ := newTestServer(t)
	if response := call(t, server, http.MethodPost, "/api/deploy/restart", ""); response.Code != http.StatusForbidden {
		t.Fatalf("status=%d", response.Code)
	}
}

func TestRestartCannotBeTriggeredFromAForeignPage(t *testing.T) {
	// Ein Neustart aus einer fremden Seite heraus wäre ein billiger
	// Dienstausfall auf Zuruf.
	server, _ := deployServer(t, "kind-dev")
	request := httptest.NewRequest(http.MethodPost, "/api/deploy/restart", nil)
	request.Header.Set("Origin", "https://beliebige-seite.example")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status=%d", recorder.Code)
	}
}

func TestAcceptanceListSaysWhichTicketsADeployCanClose(t *testing.T) {
	server, store := deployServer(t, "kind-dev")
	create := func(title string, scope []string) tickets.Ticket {
		t.Helper()
		ticket, err := store.Create(tickets.NewTicket{Title: title, Author: "Claude Code", Scope: scope})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.SetStatus(ticket.Key, tickets.StatusTest, "Claude Code"); err != nil {
			t.Fatal(err)
		}
		return ticket
	}
	backend := create("Am Backend", []string{"backend/internal/ocr/"})
	cockpit := create("Am Cockpit", []string{"missioncontrol/internal/deploy/"})
	document := create("Ein Dokument", []string{"docs/security/threat-model.md"})
	noScope := create("Ohne Scope", nil)
	// Ein Ticket, das nicht auf Test steht, gehört nicht in die Liste.
	if _, err := store.Create(tickets.NewTicket{Title: "Noch im Backlog", Author: "Claude Code"}); err != nil {
		t.Fatal(err)
	}

	response := call(t, server, http.MethodGet, "/api/deploy", "")
	var payload struct {
		Abnahme []struct {
			Key      string `json:"key"`
			Pipeline string `json:"pipeline"`
			Grund    string `json:"grund"`
		} `json:"abnahme"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	byKey := map[string]struct {
		Key      string `json:"key"`
		Pipeline string `json:"pipeline"`
		Grund    string `json:"grund"`
	}{}
	for _, entry := range payload.Abnahme {
		byKey[entry.Key] = entry
	}
	if len(payload.Abnahme) != 4 {
		t.Fatalf("nur Tickets auf Test gehören hinein: %#v", payload.Abnahme)
	}
	if byKey[backend.Key].Pipeline != "homeflow" {
		t.Fatalf("backend=%#v", byKey[backend.Key])
	}
	if byKey[cockpit.Key].Pipeline != "missioncontrol" {
		t.Fatalf("cockpit=%#v", byKey[cockpit.Key])
	}
	// Der eigentliche Zweck: ein Dokument kann kein Deploy abnehmen, und das
	// muss dranstehen statt stillschweigend zu fehlen.
	if byKey[document.Key].Pipeline != "" || byKey[document.Key].Grund == "" {
		t.Fatalf("dokument=%#v", byKey[document.Key])
	}
	if byKey[noScope.Key].Grund == "" {
		t.Fatalf("ohne Scope=%#v", byKey[noScope.Key])
	}
}

func TestBuildOnlyPipelineStillAcceptsItsTicket(t *testing.T) {
	// Der Ablauf, der das Cockpit baut, braucht keinen Cluster - trotzdem
	// trägt er etwas in ein laufendes System und muss abnehmen können. Der
	// erste echte Durchlauf ist genau daran gescheitert.
	server, store := deployServer(t, "kind-dev")
	ticket, err := store.Create(tickets.NewTicket{Title: "Am Cockpit", Author: "Claude Code",
		Scope: []string{"missioncontrol/internal/deploy/"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetStatus(ticket.Key, tickets.StatusTest, "Claude Code"); err != nil {
		t.Fatal(err)
	}
	if response := call(t, server, http.MethodPost, "/api/deploy",
		`{"pipeline":"missioncontrol","author":"Marco","tickets":["`+ticket.Key+`"]}`); response.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	waitForRun(t, server)
	if after := ticketStatus(t, store, ticket.Key); after.Status != tickets.StatusDone {
		t.Fatalf("status=%q, erwartet %q", after.Status, tickets.StatusDone)
	}
}

func TestOneRunAcceptsSeveralTickets(t *testing.T) {
	// Der Punkt der Änderung: drei Tickets in einem Durchlauf statt dreimal
	// starten und dreimal einen Schlüssel abtippen.
	server, store := deployServer(t, "kind-dev")
	keys := []string{}
	for _, title := range []string{"Erstes", "Zweites", "Drittes"} {
		ticket, err := store.Create(tickets.NewTicket{Title: title, Author: "Claude Code",
			Scope: []string{"missioncontrol/internal/deploy/"}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.SetStatus(ticket.Key, tickets.StatusTest, "Claude Code"); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, ticket.Key)
	}
	// Ein viertes bleibt ungenannt und muss unberührt bleiben.
	untouched, err := store.Create(tickets.NewTicket{Title: "Nicht angekreuzt", Author: "Claude Code",
		Scope: []string{"missioncontrol/internal/deploy/"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetStatus(untouched.Key, tickets.StatusTest, "Claude Code"); err != nil {
		t.Fatal(err)
	}

	body, err := json.Marshal(map[string]any{
		"pipeline": "missioncontrol", "author": "Marco", "tickets": keys,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response := call(t, server, http.MethodPost, "/api/deploy", string(body)); response.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	waitForRun(t, server)
	for _, key := range keys {
		if after := ticketStatus(t, store, key); after.Status != tickets.StatusDone {
			t.Fatalf("%s=%q, erwartet Fertig", key, after.Status)
		}
	}
	if after := ticketStatus(t, store, untouched.Key); after.Status != tickets.StatusTest {
		t.Fatalf("ungenanntes Ticket wurde mitgeschlossen: %q", after.Status)
	}
}

func TestOneBadKeyRefusesTheWholeRun(t *testing.T) {
	server, _ := deployServer(t, "kind-dev")
	response := call(t, server, http.MethodPost, "/api/deploy",
		`{"pipeline":"missioncontrol","author":"Marco","tickets":["MC-0001","unsinn"]}`)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d", response.Code)
	}
	if !strings.Contains(response.Body.String(), "unsinn") {
		t.Fatalf("Fehlermeldung nennt den falschen Schlüssel nicht: %s", response.Body.String())
	}
}

func TestDeployRecordsWhichReleaseShippedTheTicket(t *testing.T) {
	// Der Kern der Korrektur: "gefunden in" wird beim Anlegen gestempelt,
	// "geliefert in" erst durch einen Deploy - sonst behauptet jedes offene
	// Ticket, zu einer längst fertigen Release zu gehören.
	server, store := deployServer(t, "kind-dev")
	ticket, err := store.Create(tickets.NewTicket{Title: "Wird ausgeliefert", Author: "Claude Code",
		Scope: []string{"missioncontrol/internal/deploy/"}})
	if err != nil {
		t.Fatal(err)
	}
	if ticket.Delivered != "" {
		t.Fatalf("frisches Ticket behauptet eine Auslieferung: %q", ticket.Delivered)
	}
	if _, err := store.SetStatus(ticket.Key, tickets.StatusTest, "Claude Code"); err != nil {
		t.Fatal(err)
	}
	if response := call(t, server, http.MethodPost, "/api/deploy",
		`{"pipeline":"missioncontrol","author":"Marco","tickets":["`+ticket.Key+`"]}`); response.Code != http.StatusAccepted {
		t.Fatalf("status=%d", response.Code)
	}
	waitForRun(t, server)
	after := ticketStatus(t, store, ticket.Key)
	if after.Status != tickets.StatusDone {
		t.Fatalf("status=%q", after.Status)
	}
	// Die Version des Cockpits, nicht die von HomeFlow.
	if after.Delivered != "test" {
		t.Fatalf("geliefert=%q, erwartet die Version des Ablaufs", after.Delivered)
	}
	var explained bool
	for _, comment := range after.Comments {
		if strings.Contains(comment.Text, "Ausgeliefert in test") {
			explained = true
		}
	}
	if !explained {
		t.Fatalf("Auslieferung nicht im Kommentar erklärt: %#v", after.Comments)
	}
}

func TestOpenTicketsNeverClaimADelivery(t *testing.T) {
	_, store := deployServer(t, "kind-dev")
	ticket, err := store.Create(tickets.NewTicket{Title: "Bleibt offen", Author: "Claude Code"})
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{tickets.StatusReady, tickets.StatusActive, tickets.StatusTest} {
		if _, err := store.SetStatus(ticket.Key, status, "Claude Code"); err != nil {
			t.Fatal(err)
		}
		if after := ticketStatus(t, store, ticket.Key); after.Delivered != "" {
			t.Fatalf("Status %q setzte eine Auslieferung: %q", status, after.Delivered)
		}
	}
}
