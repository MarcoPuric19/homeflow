package cluster

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureRunner replays recorded kubectl output from the real cluster, so the
// graph is tested against objects that actually exist rather than against
// invented YAML.
func fixtureRunner(t *testing.T, broken ...string) Runner {
	t.Helper()
	fails := map[string]bool{}
	for _, namespace := range broken {
		fails[namespace] = true
	}
	fixtures := map[string]string{
		"homeflow":            "homeflow-combined.json",
		"homeflow-monitoring": "monitoring-combined.json",
	}
	return func(_ context.Context, args ...string) ([]byte, error) {
		namespace := args[1]
		if fails[namespace] {
			return nil, errors.New("kubectl " + namespace + ": simulierter Fehler")
		}
		name, ok := fixtures[namespace]
		if !ok {
			t.Fatalf("keine Fixture für %s", namespace)
		}
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatalf("fixture fehlt: %v", err)
		}
		return data, nil
	}
}

func take(t *testing.T, broken ...string) Snapshot {
	t.Helper()
	return Take(context.Background(), fixtureRunner(t, broken...), []string{"homeflow"})
}

func findNode(t *testing.T, snapshot Snapshot, kind, name string) Node {
	t.Helper()
	for _, node := range snapshot.Nodes {
		if node.Kind == kind && node.Name == name {
			return node
		}
	}
	t.Fatalf("Knoten %s/%s fehlt; vorhanden: %v", kind, name, names(snapshot))
	return Node{}
}

func names(snapshot Snapshot) []string {
	out := make([]string, 0, len(snapshot.Nodes))
	for _, node := range snapshot.Nodes {
		out = append(out, node.Kind+"/"+node.Name)
	}
	return out
}

func hasEdge(snapshot Snapshot, fromName, toName, kind string) bool {
	byID := map[string]Node{}
	for _, node := range snapshot.Nodes {
		byID[node.ID] = node
	}
	for _, edge := range snapshot.Edges {
		if edge.Kind != kind {
			continue
		}
		if strings.Contains(byID[edge.From].Name, fromName) && strings.Contains(byID[edge.To].Name, toName) {
			return true
		}
	}
	return false
}

func TestSnapshotFindsPodsAndTheirWorkloads(t *testing.T) {
	snapshot := take(t)
	if len(snapshot.Warnings) != 0 {
		t.Fatalf("Warnungen: %v", snapshot.Warnings)
	}
	api := findNode(t, snapshot, KindPod, "homeflow-api-78d9c74898-8vxmg")
	if !api.Healthy || api.Status != "Running" {
		t.Fatalf("api=%#v", api)
	}
	if !strings.Contains(api.Detail["image"], "homeflow/api") {
		t.Fatalf("image=%q", api.Detail["image"])
	}
	if !hasEdge(snapshot, "homeflow-api-78", "homeflow-api-78d9c74898", EdgeOwnedBy) {
		t.Fatal("Pod ist keinem Workload zugeordnet")
	}
}

func TestServiceRoutesToItsPods(t *testing.T) {
	snapshot := take(t)
	if !hasEdge(snapshot, "homeflow-api", "homeflow-api-78d9c74898", EdgeRoutes) {
		t.Fatal("Service homeflow-api zeigt auf keinen Pod")
	}
	if !hasEdge(snapshot, "postgres", "homeflow-postgres-0", EdgeRoutes) {
		t.Fatal("Service postgres zeigt auf keinen Pod")
	}
	service := findNode(t, snapshot, KindService, "homeflow-api")
	if !service.Healthy {
		t.Fatalf("Service ohne Backend gemeldet, obwohl der Pod passt: %#v", service)
	}
}

func TestPodDependenciesOnStorageAndConfiguration(t *testing.T) {
	snapshot := take(t)
	if !hasEdge(snapshot, "homeflow-postgres-0", "postgres-data", EdgeMounts) &&
		!hasEdge(snapshot, "homeflow-postgres-0", "data", EdgeMounts) {
		t.Fatalf("PostgreSQL ohne Volume-Kante; Knoten: %v", names(snapshot))
	}
	if !hasEdge(snapshot, "homeflow-api", "homeflow-config", EdgeReads) {
		t.Fatalf("API liest laut Graph keine ConfigMap; Knoten: %v", names(snapshot))
	}
	if !hasEdge(snapshot, "homeflow-api", "homeflow-secrets", EdgeReads) {
		t.Fatal("API liest laut Graph kein Secret")
	}
}

func TestNetworkPolicyEdgesReflectTheRealRules(t *testing.T) {
	snapshot := take(t)
	// api-egress-postgres: die API darf ausschließlich zur Datenbank.
	if !hasEdge(snapshot, "homeflow-api", "homeflow-postgres-0", EdgeMayReach) {
		t.Fatal("Egress API -> PostgreSQL fehlt")
	}
	// postgres-ingress-api: die Datenbank akzeptiert nur die API.
	if !hasEdge(snapshot, "homeflow-api", "homeflow-postgres-0", EdgeAccepts) {
		t.Fatal("Ingress PostgreSQL <- API fehlt")
	}
	// default-deny nennt beide Richtungen ohne Regeln - das muss sichtbar sein.
	if !hasEdge(snapshot, "homeflow-postgres-0", "abgeriegelt", EdgeMayReach) {
		t.Fatal("default-deny wird nicht als Abriegelung dargestellt")
	}
	// api-ingress-monitoring verweist auf einen fremden Namespace.
	if !hasEdge(snapshot, "prometheus", "homeflow-api", EdgeAccepts) {
		t.Fatalf("Ingress aus dem Monitoring-Namespace fehlt; Knoten: %v", names(snapshot))
	}
}

func TestEveryEdgeNamesItsSource(t *testing.T) {
	snapshot := take(t)
	for _, edge := range snapshot.Edges {
		if edge.Source == "" {
			t.Fatalf("Kante ohne Quellenangabe: %#v", edge)
		}
	}
}

func TestEveryEdgeReferencesKnownNodes(t *testing.T) {
	snapshot := take(t)
	known := map[string]bool{}
	for _, node := range snapshot.Nodes {
		known[node.ID] = true
	}
	for _, edge := range snapshot.Edges {
		if !known[edge.From] || !known[edge.To] {
			t.Fatalf("Kante zeigt ins Leere: %#v", edge)
		}
	}
}

func TestABrokenNamespaceDoesNotBlankTheOthers(t *testing.T) {
	// Ein Namespace, den kubectl nicht liefert, wird zur Warnung - der andere
	// muss vollständig stehen bleiben.
	snapshot := Take(context.Background(), fixtureRunner(t, "homeflow"),
		[]string{"homeflow", "homeflow-monitoring"})
	if len(snapshot.Warnings) != 1 {
		t.Fatalf("Warnungen=%v", snapshot.Warnings)
	}
	for _, node := range snapshot.Nodes {
		if node.Namespace == "homeflow" {
			t.Fatalf("Knoten aus dem kaputten Namespace: %#v", node)
		}
	}
	found := false
	for _, node := range snapshot.Nodes {
		if node.Kind == KindPod && strings.HasPrefix(node.Name, "prometheus-") {
			found = true
		}
	}
	if !found {
		t.Fatal("Prometheus-Pod fehlt")
	}
	if !hasEdge(snapshot, "prometheus", "prometheus", EdgeRoutes) {
		t.Fatal("Monitoring-Namespace unvollständig")
	}
}

func TestBothNamespacesInOneSnapshot(t *testing.T) {
	snapshot := Take(context.Background(), fixtureRunner(t), []string{"homeflow", "homeflow-monitoring"})
	if len(snapshot.Warnings) != 0 {
		t.Fatalf("Warnungen=%v", snapshot.Warnings)
	}
	namespaces := map[string]int{}
	for _, node := range snapshot.Nodes {
		if node.Kind == KindPod {
			namespaces[node.Namespace]++
		}
	}
	if namespaces["homeflow"] != 2 || namespaces["homeflow-monitoring"] != 3 {
		t.Fatalf("Pods je Namespace=%v", namespaces)
	}
}

func TestSelectorMatching(t *testing.T) {
	labels := map[string]string{"app.kubernetes.io/name": "homeflow-api", "extra": "x"}
	if !matches(map[string]string{}, labels) {
		t.Fatal("leerer Selektor muss alles treffen")
	}
	if !matches(map[string]string{"app.kubernetes.io/name": "homeflow-api"}, labels) {
		t.Fatal("Teilmenge muss treffen")
	}
	if matches(map[string]string{"app.kubernetes.io/name": "andere"}, labels) {
		t.Fatal("falscher Wert darf nicht treffen")
	}
}

func TestRunnerPassesKubeconfigAndContextFirst(t *testing.T) {
	// Der Cluster, den Mission Control beobachtet, muss festlegbar sein - sonst
	// hängt das Cockpit an dem, was zufällig in der Umgebung steht.
	var captured []string
	binary := "/bin/true"
	run := KubectlRunner(binary, "/tmp/kubeconfig", "kind-dev")
	original := execCommand
	execCommand = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		captured = args
		return []byte("{}"), nil
	}
	defer func() { execCommand = original }()
	if _, err := run(context.Background(), "-n", "homeflow", "get", "pods", "-o", "json"); err != nil {
		t.Fatal(err)
	}
	want := "--kubeconfig /tmp/kubeconfig --context kind-dev -n homeflow get pods -o json"
	if strings.Join(captured, " ") != want {
		t.Fatalf("args=%v", captured)
	}
}
