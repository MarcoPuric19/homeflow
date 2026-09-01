package httpd

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"missioncontrol/internal/cluster"
)

// A version entry. Source says where the number was read, because a version
// nobody can trace back is a rumour.
type versionEntry struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Source string `json:"source"`
	Note   string `json:"note,omitempty"`
	Drift  bool   `json:"drift,omitempty"`
}

type versionGroup struct {
	Title   string         `json:"title"`
	Hint    string         `json:"hint,omitempty"`
	Entries []versionEntry `json:"entries"`
}

var (
	makefileVersion = regexp.MustCompile(`(?m)^VERSION \?= (.+)$`)
	openapiVersion  = regexp.MustCompile(`(?m)^  version: (.+)$`)
	goDirective     = regexp.MustCompile(`(?m)^go ([0-9.]+)$`)
	imageTag        = regexp.MustCompile(`:([^:/]+)$`)
)

func readFileMatch(path string, pattern *regexp.Regexp) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if match := pattern.FindSubmatch(data); match != nil {
		return strings.TrimSpace(string(match[1]))
	}
	return ""
}

func fallback(value string) string {
	if value == "" {
		return "unbekannt"
	}
	return value
}

// versions answers "which version of what is where" for every platform in this
// repository - and marks where the repository and the cluster disagree, because
// that mismatch has bitten this project twice.
func (s *Server) versions(w http.ResponseWriter, r *http.Request) {
	root := s.config.RepoRoot
	repoVersion := readFileMatch(filepath.Join(root, "Makefile"), makefileVersion)

	var packageVersion string
	if data, err := os.ReadFile(filepath.Join(root, "frontend", "package.json")); err == nil {
		var parsed struct {
			Version string `json:"version"`
		}
		if json.Unmarshal(data, &parsed) == nil {
			packageVersion = parsed.Version
		}
	}

	s.mu.RLock()
	snapshot := s.snapshot
	s.mu.RUnlock()

	deployed := map[string]string{}
	for _, node := range snapshot.Nodes {
		if node.Kind != cluster.KindPod {
			continue
		}
		for _, image := range strings.Split(node.Detail["image"], ", ") {
			if image = strings.TrimSpace(image); image != "" {
				deployed[image] = node.Namespace
			}
		}
	}
	apiInCluster := ""
	for image := range deployed {
		if strings.Contains(image, "homeflow/api") {
			if match := imageTag.FindStringSubmatch(image); match != nil {
				apiInCluster = match[1]
			}
		}
	}

	homeflow := []versionEntry{
		{Name: "Repository", Value: fallback(repoVersion), Source: "Makefile: VERSION"},
		{Name: "OpenAPI", Value: fallback(readFileMatch(filepath.Join(root, "docs", "api", "openapi.yaml"), openapiVersion)), Source: "docs/api/openapi.yaml: info.version"},
		{Name: "Web-UI", Value: fallback(packageVersion), Source: "frontend/package.json"},
		{Name: "Im Cluster", Value: fallback(apiInCluster), Source: "laufendes Image des API-Pods"},
	}
	// Any of the four disagreeing with the repository is worth seeing at a
	// glance; the cluster falling behind is the one that keeps happening.
	for i := range homeflow {
		if repoVersion != "" && homeflow[i].Value != "unbekannt" && homeflow[i].Value != repoVersion {
			homeflow[i].Drift = true
			homeflow[i].Note = "weicht vom Repository ab"
		}
	}

	groups := []versionGroup{
		{
			Title:   "HomeFlow",
			Hint:    "Vier Stellen führen dieselbe Zahl. Wenn sie auseinanderlaufen, steht es hier.",
			Entries: homeflow,
		},
		{
			Title: "Mission Control",
			Hint:  "Versioniert sich selbst und folgt HomeFlow bewusst nicht.",
			Entries: []versionEntry{
				{Name: "Laufende Version", Value: s.config.Version, Source: "cmd/missioncontrol: const version"},
				{Name: "Ticket-Vorgabe", Value: s.config.Version, Source: "neue MC-Tickets werden damit gestempelt"},
			},
		},
	}

	images := make([]versionEntry, 0, len(deployed))
	for image, namespace := range deployed {
		name, tag := image, "latest"
		if match := imageTag.FindStringSubmatch(image); match != nil {
			tag = match[1]
			name = strings.TrimSuffix(image, ":"+tag)
		}
		images = append(images, versionEntry{Name: shortImage(name), Value: tag, Source: namespace})
	}
	sort.Slice(images, func(i, j int) bool { return images[i].Name < images[j].Name })
	groups = append(groups, versionGroup{
		Title: "Im Cluster", Hint: "Aus den laufenden Pods gelesen, nicht aus den Manifesten.", Entries: images,
	})

	tools := []versionEntry{
		{Name: "Go (HomeFlow)", Value: fallback(readFileMatch(filepath.Join(root, "backend", "go.mod"), goDirective)), Source: "backend/go.mod"},
		{Name: "Go (Mission Control)", Value: fallback(readFileMatch(filepath.Join(root, "missioncontrol", "go.mod"), goDirective)), Source: "missioncontrol/go.mod"},
	}
	tools = append(tools, s.kubectlVersions(r.Context())...)
	groups = append(groups, versionGroup{Title: "Werkzeuge und Laufzeit", Entries: tools})

	writeJSON(w, http.StatusOK, map[string]any{"gruppen": groups, "gelesen": time.Now()})
}

func shortImage(name string) string {
	parts := strings.Split(name, "/")
	if len(parts) > 1 {
		return strings.Join(parts[len(parts)-2:], "/")
	}
	return name
}

func (s *Server) kubectlVersions(ctx context.Context) []versionEntry {
	timeout, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := s.run(timeout, "version", "-o", "json")
	if err != nil {
		return []versionEntry{{Name: "Kubernetes", Value: "nicht erreichbar", Source: "kubectl version"}}
	}
	var parsed struct {
		ClientVersion struct{ GitVersion string } `json:"clientVersion"`
		ServerVersion struct{ GitVersion string } `json:"serverVersion"`
	}
	if json.Unmarshal(out, &parsed) != nil {
		return nil
	}
	return []versionEntry{
		{Name: "kubectl", Value: fallback(parsed.ClientVersion.GitVersion), Source: "kubectl version"},
		{Name: "Kubernetes (Server)", Value: fallback(parsed.ServerVersion.GitVersion), Source: "kubectl version"},
	}
}
