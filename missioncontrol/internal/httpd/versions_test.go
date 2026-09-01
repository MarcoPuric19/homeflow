package httpd

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"missioncontrol/internal/tickets"
)

// versionServer builds a server over a fake repository, so the test states the
// versions it expects instead of following whatever the real repo happens to be.
func versionServer(t *testing.T, makefileVersion, openapi, packageJSON string) *Server {
	t.Helper()
	root := t.TempDir()
	write := func(path, content string) {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("Makefile", "APP := homeflow\nVERSION ?= "+makefileVersion+"\n")
	write("docs/api/openapi.yaml", "openapi: 3.1.0\ninfo:\n  title: HomeFlow\n  version: "+openapi+"\n")
	write("frontend/package.json", `{"name":"homeflow-web","version":"`+packageJSON+`"}`)
	write("backend/go.mod", "module homeflow/backend\n\ngo 1.26\n")
	write("missioncontrol/go.mod", "module missioncontrol\n\ngo 1.26\n")

	store, err := tickets.NewStore(t.TempDir(), map[string]string{tickets.ProjectMission: "9.9.9"})
	if err != nil {
		t.Fatal(err)
	}
	run := func(_ context.Context, args ...string) ([]byte, error) {
		if args[0] == "version" {
			return []byte(`{"clientVersion":{"gitVersion":"v1.37.0"},"serverVersion":{"gitVersion":"v1.34.0"}}`), nil
		}
		data, err := os.ReadFile(filepath.Join("..", "cluster", "testdata", "homeflow-combined.json"))
		if err != nil {
			t.Fatal(err)
		}
		return data, nil
	}
	server := New(slog.New(slog.NewTextHandler(io.Discard, nil)), store, run, noGit, nil,
		Config{Namespaces: []string{"homeflow"}, Interval: time.Hour, Version: "9.9.9", RepoRoot: root})
	server.takeSnapshot(context.Background())
	return server
}

func readVersions(t *testing.T, server *Server) map[string]map[string]versionEntry {
	t.Helper()
	response := call(t, server, http.MethodGet, "/api/versions", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d", response.Code)
	}
	var payload struct {
		Gruppen []versionGroup `json:"gruppen"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	out := map[string]map[string]versionEntry{}
	for _, group := range payload.Gruppen {
		entries := map[string]versionEntry{}
		for _, entry := range group.Entries {
			entries[entry.Name] = entry
		}
		out[group.Title] = entries
	}
	return out
}

func TestVersionsReadEverySourceOfTruth(t *testing.T) {
	server := versionServer(t, "0.15.0", "0.15.0", "0.15.0")
	groups := readVersions(t, server)

	homeflow := groups["HomeFlow"]
	for name, want := range map[string]string{"Repository": "0.15.0", "OpenAPI": "0.15.0", "Web-UI": "0.15.0"} {
		if homeflow[name].Value != want {
			t.Fatalf("%s=%q, erwartet %q", name, homeflow[name].Value, want)
		}
		if homeflow[name].Drift {
			t.Fatalf("%s als Drift markiert, obwohl es passt", name)
		}
	}
	// Der Cluster in der Fixture läuft auf 0.5.0 - das ist der Drift-Fall.
	if !homeflow["Im Cluster"].Drift || homeflow["Im Cluster"].Value != "0.5.0" {
		t.Fatalf("Cluster-Eintrag=%#v", homeflow["Im Cluster"])
	}
	if groups["Mission Control"]["Laufende Version"].Value != "9.9.9" {
		t.Fatalf("Mission Control=%#v", groups["Mission Control"])
	}
	tools := groups["Werkzeuge und Laufzeit"]
	if tools["kubectl"].Value != "v1.37.0" || tools["Kubernetes (Server)"].Value != "v1.34.0" {
		t.Fatalf("Werkzeuge=%#v", tools)
	}
	if tools["Go (HomeFlow)"].Value != "1.26" {
		t.Fatalf("Go=%#v", tools["Go (HomeFlow)"])
	}
}

func TestVersionsMarkDriftBetweenRepositoryFiles(t *testing.T) {
	// Das Web-UI hinkt hinterher - genau der Fall aus Beobachtung 7.6.
	server := versionServer(t, "0.15.0", "0.15.0", "0.4.0")
	homeflow := readVersions(t, server)["HomeFlow"]
	if !homeflow["Web-UI"].Drift {
		t.Fatalf("abweichende package.json nicht markiert: %#v", homeflow["Web-UI"])
	}
	if homeflow["OpenAPI"].Drift {
		t.Fatalf("OpenAPI fälschlich markiert: %#v", homeflow["OpenAPI"])
	}
}

func TestVersionsSurviveAMissingFile(t *testing.T) {
	server := versionServer(t, "0.15.0", "0.15.0", "0.15.0")
	if err := os.Remove(filepath.Join(server.config.RepoRoot, "frontend", "package.json")); err != nil {
		t.Fatal(err)
	}
	homeflow := readVersions(t, server)["HomeFlow"]
	if homeflow["Web-UI"].Value != "unbekannt" {
		t.Fatalf("fehlende Datei=%#v", homeflow["Web-UI"])
	}
	// "unbekannt" darf nicht als Abweichung durchgehen - sonst ist die Seite
	// nach jedem Umbenennen rot.
	if homeflow["Web-UI"].Drift {
		t.Fatal("unbekannt wurde als Drift gemeldet")
	}
}

func TestVersionsListRunningImages(t *testing.T) {
	server := versionServer(t, "0.15.0", "0.15.0", "0.15.0")
	images := readVersions(t, server)["Im Cluster"]
	if images["homeflow/api"].Value != "0.5.0" {
		t.Fatalf("Images=%#v", images)
	}
	if images["homeflow/api"].Source != "homeflow" {
		t.Fatalf("Namespace fehlt: %#v", images["homeflow/api"])
	}
}
