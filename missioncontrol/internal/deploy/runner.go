package deploy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	// Enough output to see what happened, bounded so a runaway build cannot
	// grow the cockpit's memory without limit.
	maxLines = 4000
	// The only cluster this may touch.
	allowedContext = "kind-dev"
)

type LogLine struct {
	At   time.Time `json:"zeit"`
	Step string    `json:"schritt"`
	Text string    `json:"text"`
	Err  bool      `json:"fehler,omitempty"`
}

type Run struct {
	ID        int       `json:"id"`
	Pipeline  string    `json:"pipeline"`
	Title     string    `json:"titel"`
	StartedBy string    `json:"gestartetVon"`
	Tickets   []string  `json:"tickets,omitempty"`
	Started   time.Time `json:"start"`
	Ended     time.Time `json:"ende,omitempty"`
	Status    string    `json:"status"` // läuft | erfolgreich | fehlgeschlagen | abgebrochen
	Forced    bool      `json:"erzwungen,omitempty"`
	// Version is the release this run actually rolled out. It is the answer a
	// ticket needs for "shipped in", and only the run knows it.
	Version string    `json:"version,omitempty"`
	Step    string    `json:"schritt,omitempty"`
	Lines   []LogLine `json:"zeilen,omitempty"`
}

func (r Run) Running() bool { return r.Status == "läuft" }

// Runner owns the one slot in which a deployment may run.
type Runner struct {
	mu      sync.Mutex
	root    string
	make    string
	kubectl string
	next    int
	current *Run
	history []Run
	cancel  context.CancelFunc
	// OnFinish is called once per run, after it ended and after the lock is
	// released - a callback that reaches back into the runner would otherwise
	// deadlock.
	OnFinish func(Run)
	// ProcessStarted is when this cockpit process began. Comparing it with the
	// binary on disk answers the question the source timestamp cannot: is the
	// service actually running what was built?
	ProcessStarted time.Time
}

func NewRunner(root, makeBinary, kubectlBinary string) *Runner {
	return &Runner{root: root, make: makeBinary, kubectl: kubectlBinary, next: 1}
}

var (
	ErrBusy         = errors.New("es läuft bereits ein Durchlauf")
	ErrUnknown      = errors.New("unbekannte Pipeline")
	ErrWrongCluster = errors.New("falscher kubectl-Kontext")
	ErrNoChanges    = errors.New("nichts zu deployen")
)

// Start refuses more often than it accepts, which is the right ratio for the
// only writing feature in this tool.
// Start takes force because refusing outright would be wrong in one real case:
// source that changed without a version bump produces an identical tag and an
// identical manifest, and the check cannot see it. Forcing is possible, but it
// is a deliberate act and lands in the log.
func (r *Runner) Start(pipelineName, startedBy string, ticketKeys []string, force bool, version string) (Run, error) {
	pipeline, ok := Find(pipelineName)
	if !ok {
		return Run{}, ErrUnknown
	}
	if strings.TrimSpace(startedBy) == "" {
		return Run{}, errors.New("wer den Durchlauf startet, gehört ins Protokoll")
	}
	r.mu.Lock()
	if r.current != nil && r.current.Running() {
		r.mu.Unlock()
		return Run{}, ErrBusy
	}
	r.mu.Unlock()

	if pipeline.NeedsCluster {
		if err := r.checkContext(); err != nil {
			return Run{}, err
		}
		if !force {
			assessment := r.Inspect(context.Background(), pipeline.Name)
			if assessment.Error != "" {
				return Run{}, errors.New(assessment.Error)
			}
			if !assessment.HasChanges() {
				return Run{}, fmt.Errorf("%w: %s", ErrNoChanges, assessment.Note)
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), pipeline.Timeout)
	r.mu.Lock()
	run := &Run{ID: r.next, Pipeline: pipeline.Name, Title: pipeline.Title, StartedBy: startedBy,
		Tickets: ticketKeys, Started: time.Now(), Status: "läuft", Forced: force,
		Version: version, Lines: []LogLine{}}
	r.next++
	r.current = run
	r.cancel = cancel
	snapshot := *run
	r.mu.Unlock()

	go r.execute(ctx, cancel, pipeline, run)
	return snapshot, nil
}

func (r *Runner) checkContext() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, r.kubectl, "config", "current-context").Output()
	if err != nil {
		return fmt.Errorf("%w: kubectl-Kontext nicht lesbar", ErrWrongCluster)
	}
	if current := strings.TrimSpace(string(out)); current != allowedContext {
		return fmt.Errorf("%w: %q statt %q", ErrWrongCluster, current, allowedContext)
	}
	return nil
}

func (r *Runner) execute(ctx context.Context, cancel context.CancelFunc, pipeline Pipeline, run *Run) {
	defer cancel()
	for _, step := range pipeline.Steps {
		r.append(run, LogLine{At: time.Now(), Step: step.Name, Text: "→ make " + strings.Join(step.Args, " ")})
		r.setStep(run, step.Name)
		if err := r.runStep(ctx, run, step); err != nil {
			status := "fehlgeschlagen"
			if ctx.Err() != nil {
				status = "abgebrochen"
			}
			r.append(run, LogLine{At: time.Now(), Step: step.Name, Text: err.Error(), Err: true})
			r.finish(run, status)
			return
		}
	}
	r.finish(run, "erfolgreich")
}

func (r *Runner) runStep(ctx context.Context, run *Run, step Step) error {
	cmd := exec.CommandContext(ctx, r.make, step.Args...)
	cmd.Dir = r.root
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		r.append(run, LogLine{At: time.Now(), Step: step.Name, Text: line, Err: looksLikeError(line)})
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		r.append(run, LogLine{At: time.Now(), Step: step.Name, Text: err.Error(), Err: true})
	}
	return cmd.Wait()
}

func looksLikeError(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(lower, "error") || strings.Contains(lower, "fehler") ||
		strings.Contains(lower, "refusing") || strings.Contains(lower, "failed")
}

func (r *Runner) append(run *Run, line LogLine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run.Lines = append(run.Lines, line)
	if len(run.Lines) > maxLines {
		run.Lines = run.Lines[len(run.Lines)-maxLines:]
	}
}

func (r *Runner) setStep(run *Run, step string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run.Step = step
}

func (r *Runner) finish(run *Run, status string) {
	r.mu.Lock()
	run.Status = status
	run.Ended = time.Now()
	run.Step = ""
	// Keep the last runs, without their output: the history should stay small.
	short := *run
	short.Lines = nil
	r.history = append([]Run{short}, r.history...)
	if len(r.history) > 20 {
		r.history = r.history[:20]
	}
	callback := r.OnFinish
	r.mu.Unlock()

	if callback != nil {
		callback(short)
	}
}

// Cancel stops a running deployment. The step that is running receives the
// context cancellation; make and its children die with it.
func (r *Runner) Cancel() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current == nil || !r.current.Running() || r.cancel == nil {
		return false
	}
	r.cancel()
	return true
}

func (r *Runner) Current() *Run {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current == nil {
		return nil
	}
	copied := *r.current
	copied.Lines = append([]LogLine{}, r.current.Lines...)
	return &copied
}

func (r *Runner) History() []Run {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Run{}, r.history...)
}

func readFile(path string) ([]byte, error) { return os.ReadFile(path) }
