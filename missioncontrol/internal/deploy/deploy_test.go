package deploy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeProject builds a throwaway directory with a fake `make` and `kubectl`, so
// the runner can be exercised without building images or touching a cluster.
func fakeProject(t *testing.T, contextName string, makeScript string) *Runner {
	t.Helper()
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	makeBin := write("make", makeScript)
	kubectlBin := write("kubectl", "#!/bin/sh\necho "+contextName+"\n")
	return NewRunner(dir, makeBin, kubectlBin)
}

const okMake = "#!/bin/sh\necho \"läuft: $*\"\nexit 0\n"

func waitFor(t *testing.T, runner *Runner, want string) Run {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		current := runner.Current()
		if current != nil && current.Status == want {
			return *current
		}
		time.Sleep(10 * time.Millisecond)
	}
	current := runner.Current()
	t.Fatalf("Status wurde nicht %q; aktuell: %#v", want, current)
	return Run{}
}

func TestPipelinesAreAFixedAllowlist(t *testing.T) {
	// Die API nimmt einen Namen, nie eine Kommandozeile. Wer eine Pipeline
	// hinzufügen will, ändert Code - genau das ist beabsichtigt.
	if _, ok := Find("homeflow"); !ok {
		t.Fatal("homeflow fehlt")
	}
	if _, ok := Find("rm -rf /"); ok {
		t.Fatal("beliebiger Name wurde angenommen")
	}
	for _, pipeline := range Pipelines() {
		if len(pipeline.Steps) == 0 || pipeline.Timeout == 0 {
			t.Fatalf("Pipeline unvollständig: %#v", pipeline)
		}
		for _, step := range pipeline.Steps {
			for _, arg := range step.Args {
				if strings.ContainsAny(arg, ";|&$`") {
					t.Fatalf("verdächtiges Argument in %s: %q", pipeline.Name, arg)
				}
			}
		}
	}
}

func TestRunThroughAllSteps(t *testing.T) {
	runner := fakeProject(t, "kind-dev", okMake)
	if _, err := runner.Start("monitoring", "Marco", "MC-0001", true); err != nil {
		t.Fatal(err)
	}
	run := waitFor(t, runner, "erfolgreich")
	if run.Ticket != "MC-0001" || run.StartedBy != "Marco" {
		t.Fatalf("run=%#v", run)
	}
	joined := ""
	for _, line := range run.Lines {
		joined += line.Text + "\n"
	}
	for _, want := range []string{"make monitoring-manifests", "make monitoring-deploy", "läuft: monitoring-deploy"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("Ausgabe ohne %q:\n%s", want, joined)
		}
	}
	if history := runner.History(); len(history) != 1 || history[0].Status != "erfolgreich" {
		t.Fatalf("verlauf=%#v", history)
	}
	// Der Verlauf trägt die Ausgabe nicht mit - sonst wächst er unbegrenzt.
	if runner.History()[0].Lines != nil {
		t.Fatal("Verlauf enthält die Ausgabe")
	}
}

func TestAFailingStepStopsTheRun(t *testing.T) {
	// Der zweite Schritt scheitert; der dritte darf nicht mehr laufen.
	script := "#!/bin/sh\ncase \"$1\" in\n  monitoring-manifests) echo ok; exit 0;;\n  *) echo 'Error: kaputt' >&2; exit 2;;\nesac\n"
	runner := fakeProject(t, "kind-dev", script)
	if _, err := runner.Start("monitoring", "Marco", "", true); err != nil {
		t.Fatal(err)
	}
	run := waitFor(t, runner, "fehlgeschlagen")
	found := false
	for _, line := range run.Lines {
		if strings.Contains(line.Text, "kaputt") && line.Err {
			found = true
		}
	}
	if !found {
		t.Fatalf("Fehlerzeile nicht als Fehler markiert: %#v", run.Lines)
	}
}

func TestRefusesAForeignCluster(t *testing.T) {
	// Die wichtigste Weigerung: niemals in etwas deployen, das nicht der lokale
	// kind-Cluster ist.
	runner := fakeProject(t, "produktion-eu-west", okMake)
	_, err := runner.Start("homeflow", "Marco", "", true)
	if !errors.Is(err, ErrWrongCluster) {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(err.Error(), "produktion-eu-west") {
		t.Fatalf("Fehlermeldung nennt den Kontext nicht: %v", err)
	}
	if runner.Current() != nil {
		t.Fatal("Durchlauf wurde trotzdem angelegt")
	}
}

func TestPipelineWithoutClusterRunsAnywhere(t *testing.T) {
	// "Nur prüfen" ändert nichts und darf deshalb auch bei fremdem Kontext laufen.
	runner := fakeProject(t, "irgendein-anderer", okMake)
	if _, err := runner.Start("pruefen", "Codex", "", false); err != nil {
		t.Fatalf("err=%v", err)
	}
	waitFor(t, runner, "erfolgreich")
}

func TestOnlyOneRunAtATime(t *testing.T) {
	runner := fakeProject(t, "kind-dev", "#!/bin/sh\nsleep 2\n")
	if _, err := runner.Start("monitoring", "Marco", "", true); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Start("monitoring", "Codex", "", true); !errors.Is(err, ErrBusy) {
		t.Fatalf("zweiter Start: err=%v", err)
	}
	runner.Cancel()
	waitFor(t, runner, "abgebrochen")
	// Nach dem Abbruch ist der Platz wieder frei.
	if _, err := runner.Start("monitoring", "Marco", "", true); err != nil {
		t.Fatalf("nach Abbruch: err=%v", err)
	}
}

func TestUnknownPipelineAndMissingAuthorAreRefused(t *testing.T) {
	runner := fakeProject(t, "kind-dev", okMake)
	if _, err := runner.Start("gibtsnicht", "Marco", "", false); !errors.Is(err, ErrUnknown) {
		t.Fatalf("err=%v", err)
	}
	if _, err := runner.Start("pruefen", "  ", "", false); err == nil {
		t.Fatal("Durchlauf ohne Urheber wurde angenommen")
	}
}

func TestCancelWithoutARunIsHarmless(t *testing.T) {
	runner := fakeProject(t, "kind-dev", okMake)
	if runner.Cancel() {
		t.Fatal("Abbruch meldete Erfolg, obwohl nichts lief")
	}
}

// changeProject builds a project whose kubectl answers a scripted diff, so the
// change check can be tested without a cluster.
func changeProject(t *testing.T, version, runningImage, diffOut string, diffExit int) *Runner {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Makefile"), []byte("APP := homeflow\nVERSION ?= "+version+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Das echte kubectl wird als "kubectl -n homeflow get pods …" aufgerufen -
	// die Attrappe darf sich deshalb nicht auf $1 verlassen.
	script := `#!/bin/sh
case " $* " in
  *" config "*) echo kind-dev ;;
  *" get "*)    printf '%s' "` + runningImage + `" ;;
  *" diff "*)   cat <<'DIFF'
` + diffOut + `
DIFF
                exit ` + itoa(diffExit) + ` ;;
esac
`
	kubectl := filepath.Join(dir, "kubectl")
	if err := os.WriteFile(kubectl, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	makeBin := filepath.Join(dir, "make")
	if err := os.WriteFile(makeBin, []byte(okMake), 0o755); err != nil {
		t.Fatal(err)
	}
	return NewRunner(dir, makeBin, kubectl)
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	return string(rune('0' + v))
}

const sampleDiff = `diff -u -N /tmp/LIVE-1/apps.v1.Deployment.homeflow.homeflow-api /tmp/MERGED-1/apps.v1.Deployment.homeflow.homeflow-api
--- a
+++ b
@@ -6,7 +6,7 @@
-        image: localhost:5000/homeflow/api:0.5.0
+        image: localhost:5000/homeflow/api:0.15.0`

func TestInspectFindsVersionDriftAndManifestChanges(t *testing.T) {
	runner := changeProject(t, "0.15.0", "localhost:5000/homeflow/api:0.5.0", sampleDiff, 1)
	assessment := runner.Inspect(context.Background(), "homeflow")
	if !assessment.HasChanges() {
		t.Fatalf("keine Änderung erkannt: %#v", assessment)
	}
	kinds := map[string]string{}
	for _, change := range assessment.Changes {
		kinds[change.Kind] = change.Detail
	}
	if !strings.Contains(kinds["Version"], "0.5.0") || !strings.Contains(kinds["Version"], "0.15.0") {
		t.Fatalf("Versionsunterschied=%q", kinds["Version"])
	}
	if kinds["Manifest"] != "apps.v1.Deployment.homeflow.homeflow-api" {
		t.Fatalf("Manifestunterschied=%q", kinds["Manifest"])
	}
	// Der Diff-Inhalt darf nicht mitgeliefert werden - er kann
	// Konfigurationswerte enthalten.
	for _, change := range assessment.Changes {
		if strings.Contains(change.Detail, "image:") || strings.Contains(change.Detail, "@@") {
			t.Fatalf("Diff-Inhalt ausgeliefert: %q", change.Detail)
		}
	}
	if len(assessment.Checked) != 2 {
		t.Fatalf("geprüft=%v", assessment.Checked)
	}
}

func TestInspectReportsNothingToDo(t *testing.T) {
	runner := changeProject(t, "0.15.0", "localhost:5000/homeflow/api:0.15.0", "", 0)
	assessment := runner.Inspect(context.Background(), "homeflow")
	if assessment.HasChanges() {
		t.Fatalf("Änderung gemeldet, obwohl alles übereinstimmt: %#v", assessment)
	}
	if assessment.Note == "" {
		t.Fatal("ohne Begründung")
	}
}

func TestStartRefusesWhenNothingWouldChange(t *testing.T) {
	runner := changeProject(t, "0.15.0", "localhost:5000/homeflow/api:0.15.0", "", 0)
	_, err := runner.Start("homeflow", "Marco", "", false)
	if !errors.Is(err, ErrNoChanges) {
		t.Fatalf("err=%v", err)
	}
	if runner.Current() != nil {
		t.Fatal("Durchlauf trotzdem angelegt")
	}
	// Erzwingen bleibt möglich - Quelltextänderungen ohne Versionssprung sieht
	// die Prüfung nicht.
	run, err := runner.Start("homeflow", "Marco", "", true)
	if err != nil {
		t.Fatalf("erzwungen: err=%v", err)
	}
	if !run.Forced {
		t.Fatal("erzwungener Lauf ist nicht als solcher markiert")
	}
	waitFor(t, runner, "erfolgreich")
}

func TestStartProceedsWhenSomethingWouldChange(t *testing.T) {
	runner := changeProject(t, "0.15.0", "localhost:5000/homeflow/api:0.5.0", sampleDiff, 1)
	if _, err := runner.Start("homeflow", "Marco", "", false); err != nil {
		t.Fatalf("err=%v", err)
	}
	waitFor(t, runner, "erfolgreich")
}

func TestCheckOnlyPipelineNeverNeedsChanges(t *testing.T) {
	runner := changeProject(t, "0.15.0", "localhost:5000/homeflow/api:0.15.0", "", 0)
	assessment := runner.Inspect(context.Background(), "pruefen")
	if !assessment.HasChanges() {
		t.Fatalf("Prüflauf wurde gesperrt: %#v", assessment)
	}
	if _, err := runner.Start("pruefen", "Marco", "", false); err != nil {
		t.Fatalf("err=%v", err)
	}
	waitFor(t, runner, "erfolgreich")
}
