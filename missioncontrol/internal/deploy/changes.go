package deploy

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// A deployment that changes nothing is not harmless: it replaces running pods,
// kills open port-forwards and costs minutes of build time for no result. So
// before a run may start, the runner asks what would actually change.
type Difference struct {
	Kind   string `json:"art"`
	Detail string `json:"detail"`
}

type Assessment struct {
	Pipeline string       `json:"pipeline"`
	Changes  []Difference `json:"aenderungen"`
	// Checked says what was compared, so "nothing to do" is traceable rather
	// than a shrug.
	Checked []string `json:"geprueft"`
	Note    string   `json:"hinweis,omitempty"`
	Error   string   `json:"fehler,omitempty"`
}

func (a Assessment) HasChanges() bool { return len(a.Changes) > 0 }

var (
	makefileVersion = regexp.MustCompile(`(?m)^VERSION \?= (.+)$`)
	diffHeader      = regexp.MustCompile(`(?m)^diff -u -N \S+ (\S+)$`)
)

// Inspect compares the repository against the cluster. It never changes
// anything: `kubectl diff` is a dry run, and everything else is a read.
func (r *Runner) Inspect(ctx context.Context, pipelineName string) Assessment {
	pipeline, ok := Find(pipelineName)
	if !ok {
		return Assessment{Pipeline: pipelineName, Error: "unbekannte Pipeline"}
	}
	assessment := Assessment{Pipeline: pipeline.Name, Changes: []Difference{}, Checked: []string{}}
	if pipeline.Name == "missioncontrol" {
		r.compareBinary(&assessment)
		if !assessment.HasChanges() && assessment.Error == "" {
			assessment.Note = "Das gebaute Binary ist aktuell."
		}
		return assessment
	}
	if !pipeline.NeedsCluster {
		assessment.Note = "Prüft nur und verändert nichts - darf deshalb immer laufen."
		assessment.Changes = append(assessment.Changes, Difference{Kind: "Prüflauf", Detail: "ändert nichts"})
		return assessment
	}

	switch pipeline.Name {
	case "homeflow":
		r.compareVersion(ctx, &assessment)
		r.compareManifests(ctx, &assessment, filepath.Join("kubernetes", "base"))
	case "monitoring":
		r.compareManifests(ctx, &assessment, filepath.Join("monitoring", "kubernetes"))
	}
	if !assessment.HasChanges() && assessment.Error == "" {
		assessment.Note = "Cluster und Repository stimmen überein."
	}
	return assessment
}

// compareVersion is the check that catches the case this project keeps hitting:
// the repository moves on and the cluster keeps running an old image.
func (r *Runner) compareVersion(ctx context.Context, assessment *Assessment) {
	assessment.Checked = append(assessment.Checked, "VERSION in der Makefile gegen das laufende Image")
	wanted := r.repoVersion()
	if wanted == "" {
		return
	}
	out, err := r.kubectlOutput(ctx, "-n", "homeflow", "get", "pods",
		"-o", "jsonpath={.items[*].spec.containers[*].image}")
	if err != nil {
		assessment.Error = "Cluster nicht lesbar: " + err.Error()
		return
	}
	running := ""
	for _, image := range strings.Fields(string(out)) {
		if strings.Contains(image, "homeflow/api:") {
			running = image[strings.LastIndex(image, ":")+1:]
		}
	}
	switch {
	case running == "":
		assessment.Changes = append(assessment.Changes, Difference{
			Kind: "Version", Detail: "Im Cluster läuft kein HomeFlow-API-Pod; " + wanted + " wäre neu."})
	case running != wanted:
		assessment.Changes = append(assessment.Changes, Difference{
			Kind: "Version", Detail: "Cluster " + running + " → Repository " + wanted})
		return
	}

	// Gleicher Tag heißt nicht gleicher Inhalt. Am 2026-09-02 liefen unter
	// `0.15.11` drei verschiedene Images - lokal, im Node und im Pod -, und
	// diese Prüfung meldete „nichts zu tun", während der Pod fünfzehn Stunden
	// alten Code fuhr (MC-0019, HF-0135).
	//
	// Die inhaltliche Antwort steht in scripts/deploy-verify.sh; sie hier zu
	// wiederholen hieße, zwei Wahrheiten über denselben Zustand zu pflegen.
	r.compareImageContent(ctx, assessment)
}

// compareImageContent fragt das Prüfskript, ob der laufende Pod dasselbe Image
// benutzt, das im Node liegt. Ein Fehlschlag des Skripts selbst - kein Docker,
// kein Node - ist kein Befund und bleibt still: die Änderungserkennung soll
// nichts behaupten, was sie nicht wissen kann.
func (r *Runner) compareImageContent(ctx context.Context, assessment *Assessment) {
	assessment.Checked = append(assessment.Checked, "Image-Inhalt im Pod gegen den Node")
	script := filepath.Join(r.root, "scripts", "deploy-verify.sh")
	if _, err := os.Stat(script); err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, script, "--pruefen")
	cmd.Dir = r.root
	out, err := cmd.CombinedOutput()
	if err == nil {
		return
	}
	if !strings.Contains(string(out), "Abweichung") {
		return
	}
	assessment.Changes = append(assessment.Changes, Difference{
		Kind: "Image",
		Detail: "Gleicher Tag, anderer Inhalt: der Pod läuft nicht auf dem Image, " +
			"das im Node liegt. Ein Deploy rollt es aus."})
}

// compareManifests asks kubectl itself. Only the names of the affected objects
// are reported, never the diff body: it can contain configuration values, and
// the cockpit has no business printing those.
func (r *Runner) compareManifests(ctx context.Context, assessment *Assessment, dir string) {
	assessment.Checked = append(assessment.Checked, "kubectl diff -k "+dir)
	cmd := exec.CommandContext(ctx, r.kubectl, "diff", "-k", dir)
	cmd.Dir = r.root
	out, err := cmd.Output()
	if err != nil {
		// Exit code 1 is kubectl's way of saying "there are differences".
		var exit *exec.ExitError
		if !asExitError(err, &exit) || exit.ExitCode() != 1 {
			assessment.Error = "kubectl diff fehlgeschlagen: " + err.Error()
			return
		}
	}
	for _, match := range diffHeader.FindAllStringSubmatch(string(out), -1) {
		assessment.Changes = append(assessment.Changes, Difference{
			Kind: "Manifest", Detail: filepath.Base(match[1])})
	}
}

func asExitError(err error, target **exec.ExitError) bool {
	exit, ok := err.(*exec.ExitError)
	if ok {
		*target = exit
	}
	return ok
}

func (r *Runner) repoVersion() string {
	data, err := readFile(filepath.Join(r.root, "Makefile"))
	if err != nil {
		return ""
	}
	if match := makefileVersion.FindSubmatch(data); match != nil {
		return strings.TrimSpace(string(match[1]))
	}
	return ""
}

func (r *Runner) kubectlOutput(ctx context.Context, args ...string) ([]byte, error) {
	timeout, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(timeout, r.kubectl, args...)
	cmd.Dir = r.root
	return cmd.Output()
}

// compareBinary beantwortet für das Cockpit selbst, was kubectl diff für den
// Cluster beantwortet: gibt es etwas zu tun? Verglichen wird der Zeitstempel
// des gebauten Binaries mit dem neuesten Quelltext. Das ist eine Heuristik über
// Änderungszeiten - sie irrt höchstens zugunsten von "es gibt etwas zu tun",
// und das ist die richtige Richtung.
func (r *Runner) compareBinary(assessment *Assessment) {
	assessment.Checked = append(assessment.Checked, "Quelltext gegen missioncontrol/bin/missioncontrol")
	binary := filepath.Join(r.root, "missioncontrol", "bin", "missioncontrol")
	info, err := os.Stat(binary)
	if err != nil {
		assessment.Changes = append(assessment.Changes, Difference{
			Kind: "Binary", Detail: "noch nicht gebaut"})
		return
	}
	newer := newerThan(filepath.Join(r.root, "missioncontrol"), info.ModTime())
	for _, file := range newer {
		assessment.Changes = append(assessment.Changes, Difference{Kind: "Quelltext", Detail: file})
	}

	// Zweite Frage, und die eigentlich wichtige: ist das, was gebaut wurde, auch
	// ausgeliefert? Der Vergleich läuft gegen den letzten erfolgreichen Deploy
	// und ausdrücklich NICHT gegen den Start dieses Prozesses.
	//
	// Vorher tat er genau das, mit einer unangenehmen Folge: wer den Dienst neu
	// startet, um eine Änderung überhaupt verifizieren zu können - und die
	// Verifikationspflicht verlangt das -, hat damit den Deploy verbraucht. Das
	// Cockpit meldete "nichts zu tun", der Knopf war tot und der automatische
	// Übergang Test → Fertig fiel aus (MC-0018).
	assessment.Checked = append(assessment.Checked, "gebautes Binary gegen den letzten Deploy")
	if r.Journal == nil {
		return
	}
	last, ok := r.Journal.Last("missioncontrol")
	if !ok {
		assessment.Changes = append(assessment.Changes, Difference{
			Kind:   "Deploy",
			Detail: "Noch nie über die Pipeline ausgeliefert."})
		return
	}
	if info.ModTime().After(last.At) {
		assessment.Changes = append(assessment.Changes, Difference{
			Kind: "Deploy",
			Detail: "Das gebaute Binary ist neuer als der zuletzt ausgelieferte Stand (" +
				last.Version + " vom " + last.At.Format("02.01. 15:04") + ")."})
	}
}

// newerThan lists source files younger than the build, at most a handful: the
// point is to name examples, not to produce an inventory.
func newerThan(root string, since time.Time) []string {
	var out []string
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		switch filepath.Ext(path) {
		case ".go", ".js", ".css", ".html":
		default:
			return nil
		}
		// Das Binary selbst und alles darunter zählt nicht als Quelltext.
		if strings.Contains(path, string(filepath.Separator)+"bin"+string(filepath.Separator)) {
			return nil
		}
		info, statErr := entry.Info()
		if statErr == nil && info.ModTime().After(since) {
			if relative, relErr := filepath.Rel(root, path); relErr == nil {
				out = append(out, relative)
			}
		}
		if len(out) >= 5 {
			return fs.SkipAll
		}
		return nil
	})
	return out
}
