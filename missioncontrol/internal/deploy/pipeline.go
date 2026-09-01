// Package deploy runs the project's own make targets and streams what happens.
//
// This is the one part of Mission Control that changes something. Three rules
// follow from that and are enforced here, not in the user interface:
//
//  1. No arbitrary commands. Only the pipelines defined below can run; the API
//     takes a name, never a command line.
//  2. Only against the local kind cluster. Every pipeline that touches
//     Kubernetes checks the current context first and refuses otherwise.
//  3. One at a time. A second run while one is going would fight over the
//     image tag and the cluster.
//
// The steps are `make` targets on purpose: the Makefile already carries the
// guards (kind-load refuses a foreign context, deploy refuses a missing
// Secret), and duplicating them here would mean two truths.
package deploy

import "time"

type Step struct {
	Name string   `json:"name"`
	Args []string `json:"args"`
}

// Covers lists the repository paths a pipeline actually carries into a running
// system. It is what makes the question answerable: a ticket whose scope
// touches none of these cannot be accepted by any deployment, however green the
// run goes.
type Pipeline struct {
	Name        string `json:"name"`
	Title       string `json:"titel"`
	Description string `json:"beschreibung"`
	// NeedsCluster makes the run refuse unless kubectl points at kind-dev.
	NeedsCluster bool          `json:"brauchtCluster"`
	Steps        []Step        `json:"schritte"`
	Covers       []string      `json:"deckt_ab"`
	Timeout      time.Duration `json:"-"`
}

// Pipelines is the allowlist. Adding one is a code change and a review; that is
// the point.
func Pipelines() []Pipeline {
	return []Pipeline{
		{
			Name:  "homeflow",
			Title: "HomeFlow deployen",
			Description: "Baut das Image mit der VERSION aus der Makefile, lädt es in den kind-Node " +
				"und wendet kubernetes/base an. Ersetzt die laufende Anwendung.",
			NeedsCluster: true,
			Timeout:      20 * time.Minute,
			Covers:       []string{"backend/", "frontend/", "docker/", "kubernetes/base/"},
			Steps: []Step{
				{Name: "Prüfen", Args: []string{"ci"}},
				{Name: "Image bauen", Args: []string{"docker-build"}},
				{Name: "In den Cluster laden", Args: []string{"kind-load"}},
				{Name: "Anwenden", Args: []string{"deploy"}},
			},
		},
		{
			Name:         "monitoring",
			Title:        "Monitoring deployen",
			Description:  "Wendet monitoring/kubernetes an: Prometheus, Grafana, kube-state-metrics.",
			NeedsCluster: true,
			Timeout:      10 * time.Minute,
			Covers:       []string{"monitoring/"},
			Steps: []Step{
				{Name: "Manifeste rendern", Args: []string{"monitoring-manifests"}},
				{Name: "Anwenden", Args: []string{"monitoring-deploy"}},
			},
		},
		{
			Name:  "missioncontrol",
			Title: "Mission Control bauen",
			Description: "Prüft und baut das Cockpit selbst. Der laufende Dienst bleibt unberührt - " +
				"das neue Binary liegt danach bereit und wird mit einem ausdrücklichen Neustart übernommen.",
			NeedsCluster: false,
			Timeout:      10 * time.Minute,
			Covers:       []string{"missioncontrol/"},
			Steps:        []Step{{Name: "Prüfen und bauen", Args: []string{"mission-control-ci"}}},
		},
		{
			Name:         "pruefen",
			Title:        "Nur prüfen",
			Description:  "make ci: gofmt, vet, TypeScript, alle Unit-Tests, beide Manifest-Renderings und Mission Control. Ändert nichts.",
			NeedsCluster: false,
			Timeout:      15 * time.Minute,
			Steps:        []Step{{Name: "make ci", Args: []string{"ci"}}},
		},
	}
}

func Find(name string) (Pipeline, bool) {
	for _, pipeline := range Pipelines() {
		if pipeline.Name == name {
			return pipeline, true
		}
	}
	return Pipeline{}, false
}
