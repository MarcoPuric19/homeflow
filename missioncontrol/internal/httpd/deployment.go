package httpd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"missioncontrol/internal/deploy"
	"missioncontrol/internal/repo"
	"missioncontrol/internal/tickets"
)

// deployState answers what can be started, what is running and what happened
// before.
func (s *Server) deployState(w http.ResponseWriter, _ *http.Request) {
	if s.deploy == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"aktiv": false, "pipelines": []any{}, "verlauf": []any{},
			"hinweis": "Der Deploy-Dienst ist abgeschaltet (--deploy=false).",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"aktiv":     true,
		"pipelines": deploy.Pipelines(),
		"laufend":   s.deploy.Current(),
		"verlauf":   s.deploy.History(),
		"cluster":   deployTarget,
		"abnahme":   s.awaitingAcceptance(),
		"stand":     s.deploymentState(),
	})
}

const deployTarget = "kind-dev"

// waiting is one ticket in the Test column and the answer to the question this
// view kept failing to answer: can a deployment accept it at all?
type waiting struct {
	Key      string `json:"key"`
	Title    string `json:"titel"`
	Owner    string `json:"zustaendig"`
	Pipeline string `json:"pipeline,omitempty"`
	Reason   string `json:"grund,omitempty"`
}

// awaitingAcceptance matches every ticket in "Test" against what the pipelines
// actually carry into a running system.
//
// Half the work in this project never reaches a cluster - documents, scripts,
// measurements. Leaving those in a column that promises acceptance by
// deployment is how somebody ends up waiting for a button that will never do
// anything.
func (s *Server) awaitingAcceptance() []waiting {
	all, err := s.store.List()
	if err != nil {
		s.logger.Warn("Tickets für die Abnahmeliste nicht lesbar", "error", err)
		return []waiting{}
	}
	out := make([]waiting, 0)
	for _, ticket := range all {
		if ticket.Status != tickets.StatusTest {
			continue
		}
		entry := waiting{Key: ticket.Key, Title: ticket.Title, Owner: ticket.Owner}
		switch {
		case len(ticket.Scope) == 0:
			entry.Reason = "Kein Scope eingetragen - unklar, was ein Deploy überhaupt ausrollen würde."
		default:
			if pipeline := coveringPipeline(ticket.Scope); pipeline != "" {
				entry.Pipeline = pipeline
			} else {
				entry.Reason = "Kein Ablauf deckt diese Pfade ab. Abnahme heißt hier lesen, nicht deployen."
			}
		}
		out = append(out, entry)
	}
	return out
}

// coveringPipeline returns the first pipeline whose covered paths overlap the
// ticket's scope.
func coveringPipeline(scope []string) string {
	for _, pipeline := range deploy.Pipelines() {
		for _, covered := range pipeline.Covers {
			for _, path := range scope {
				if repo.Matches(covered, path) || repo.Matches(path, covered) {
					return pipeline.Name
				}
			}
		}
	}
	return ""
}

func (s *Server) deployStart(w http.ResponseWriter, r *http.Request) {
	if s.deploy == nil {
		writeError(w, http.StatusForbidden, "Der Deploy-Dienst ist abgeschaltet.")
		return
	}
	var input struct {
		Pipeline string `json:"pipeline"`
		Author   string `json:"author"`
		// Tickets are the ones this run accepts. Ticket stays as a single-value
		// alias so a script that predates the list keeps working.
		Tickets []string `json:"tickets"`
		Ticket  string   `json:"ticket"`
		// Trotzdem forces a run the change check would refuse.
		Trotzdem bool `json:"trotzdem"`
	}
	if !decode(w, r, &input) {
		return
	}
	keys := input.Tickets
	if input.Ticket != "" {
		keys = append(keys, input.Ticket)
	}
	for _, key := range keys {
		if !tickets.ValidKey(key) {
			writeError(w, http.StatusUnprocessableEntity, "Ticketschlüssel ungültig: "+key)
			return
		}
	}
	run, err := s.deploy.Start(input.Pipeline, input.Author, keys, input.Trotzdem, s.deployedVersion(input.Pipeline))
	switch {
	case errors.Is(err, deploy.ErrUnknown):
		writeError(w, http.StatusNotFound, "Unbekannte Pipeline")
	case errors.Is(err, deploy.ErrBusy):
		writeError(w, http.StatusConflict, "Es läuft bereits ein Durchlauf.")
	case errors.Is(err, deploy.ErrNoChanges):
		// Nicht 500 und nicht 422: die Anfrage war in Ordnung, die Bedingung
		// dafür ist es nicht.
		writeError(w, http.StatusPreconditionFailed, err.Error())
	case errors.Is(err, deploy.ErrWrongCluster):
		// The most important refusal: never deploy into something that is not
		// the local kind cluster.
		writeError(w, http.StatusPreconditionFailed, err.Error())
	case err != nil:
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		s.logger.Info("deploy gestartet", "pipeline", run.Pipeline, "von", run.StartedBy, "tickets", run.Tickets)
		// A deployment is worth a line in every ticket it belongs to.
		note := "Deploy gestartet: " + run.Title + " (Durchlauf #" + itoa(run.ID) + ")"
		if run.Forced {
			note += " - erzwungen, obwohl die Prüfung keine Änderung fand"
		}
		for _, key := range run.Tickets {
			if _, err := s.store.Comment(key, run.StartedBy, note); err != nil {
				s.logger.Warn("Ticketkommentar zum Deploy fehlgeschlagen", "ticket", key, "error", err)
			}
		}
		writeJSON(w, http.StatusAccepted, run)
	}
}

// deployChanges is a separate endpoint because it costs two kubectl calls: the
// deploy view asks once when it opens, not on every poll.
func (s *Server) deployChanges(w http.ResponseWriter, r *http.Request) {
	if s.deploy == nil {
		writeError(w, http.StatusForbidden, "Der Deploy-Dienst ist abgeschaltet.")
		return
	}
	assessments := make([]deploy.Assessment, 0)
	for _, pipeline := range deploy.Pipelines() {
		assessments = append(assessments, s.deploy.Inspect(r.Context(), pipeline.Name))
	}
	writeJSON(w, http.StatusOK, map[string]any{"pipelines": assessments})
}

// deployRestart is the one action that ends the process answering it. The order
// matters: write and flush the response first, then ask systemd to restart the
// unit without waiting. Doing it the other way round means the browser sees a
// dropped connection and cannot tell success from crash.
// deployedVersion answers what a run actually ships. HomeFlow's release lives
// in the Makefile; Mission Control carries its own.
func (s *Server) deployedVersion(pipeline string) string {
	if pipeline == "missioncontrol" {
		return s.config.Version
	}
	return readFileMatch(filepath.Join(s.config.RepoRoot, "Makefile"), makefileVersion)
}

func (s *Server) deployRestart(w http.ResponseWriter, _ *http.Request) {
	if s.deploy == nil {
		writeError(w, http.StatusForbidden, "Der Deploy-Dienst ist abgeschaltet.")
		return
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		writeError(w, http.StatusPreconditionFailed,
			"Kein systemd erreichbar. Neustart von Hand: make -C missioncontrol install")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":  "neustart angefordert",
		"hinweis": "Die Seite verliert kurz die Verbindung und ist nach wenigen Sekunden wieder da.",
	})
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	go func() {
		// Kurz warten, damit die Antwort das Netz verlassen hat.
		time.Sleep(300 * time.Millisecond)
		s.logger.Info("neustart angefordert")
		cmd := exec.Command("systemctl", "--user", "restart", "--no-block", "mission-control.service")
		if err := cmd.Run(); err != nil {
			s.logger.Error("neustart fehlgeschlagen", "error", err)
		}
	}()
}

func (s *Server) deployCancel(w http.ResponseWriter, _ *http.Request) {
	if s.deploy == nil || !s.deploy.Cancel() {
		writeError(w, http.StatusConflict, "Es läuft nichts, was abgebrochen werden könnte.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "abgebrochen"})
}

// afterDeploy is the last step of the chain: a deployment that went through is
// the acceptance the "Test" column is waiting for.
//
// Deliberately narrow. Only the ticket that was named when the run started,
// only out of "Test", only after a successful run, and never after a
// check-only pipeline - a run that changes nothing accepts nothing. Anything
// wider would close tickets nobody looked at, and a board that closes itself
// is worth less than one nobody updates.
func (s *Server) afterDeploy(run deploy.Run) {
	if run.Status != "erfolgreich" {
		return
	}
	// Erst festhalten, was ausgeliefert wurde, dann Tickets bewegen. Der Eintrag
	// ist auch dann richtig, wenn kein Ticket am Lauf hing - er beantwortet
	// "was läuft gerade", nicht "was wurde abgenommen" (MC-0018, HF-0104).
	if s.deploy != nil && s.deploy.Journal != nil {
		record := deploy.Record{Pipeline: run.Pipeline, Version: run.Version,
			At: run.Ended, RunID: run.ID, StartedBy: run.StartedBy, Tickets: run.Tickets}
		if record.At.IsZero() {
			record.At = time.Now()
		}
		record.Commit = s.headCommit()
		if err := s.deploy.Journal.Put(record); err != nil {
			s.logger.Error("Deploy-Journal schreiben", "error", err)
		}
	}
	if len(run.Tickets) == 0 {
		return
	}
	pipeline, ok := deploy.Find(run.Pipeline)
	// Das Kriterium ist nicht "braucht einen Cluster", sondern "trägt etwas in
	// ein laufendes System". Sonst nimmt der Ablauf, der das Cockpit selbst
	// baut, nie etwas ab - genau das ist beim ersten echten Durchlauf passiert.
	if !ok || len(pipeline.Covers) == 0 {
		return
	}
	for _, key := range run.Tickets {
		s.acceptTicket(run, key)
	}
}

// acceptTicket zieht ein einzelnes Ticket nach, wenn es auf Abnahme wartet.
func (s *Server) acceptTicket(run deploy.Run, key string) {
	ticket, err := s.store.Get(key)
	if err != nil {
		s.logger.Warn("Ticket nach dem Deploy nicht lesbar", "ticket", key, "error", err)
		return
	}
	if ticket.Status != tickets.StatusTest {
		// Kein Fehler: wer nicht auf Abnahme wartet, wird auch nicht
		// abgenommen. Der Kommentar aus dem Start steht bereits im Ticket.
		return
	}
	note := fmt.Sprintf("Deploy %s (Durchlauf #%d) erfolgreich - Ticket von Test auf Fertig gezogen.", run.Title, run.ID)
	if run.Version != "" {
		note += " Ausgeliefert in " + run.Version + "."
	}
	if run.Forced {
		note += " Der Durchlauf war erzwungen, die Änderungsprüfung fand nichts."
	}
	if _, err := s.store.Comment(key, run.StartedBy, note); err != nil {
		s.logger.Warn("Kommentar nach dem Deploy fehlgeschlagen", "ticket", key, "error", err)
	}
	// Erst die ausgelieferte Version, dann der Status: so steht im Ticket, WAS
	// es abgenommen hat, und nicht nur dass es abgenommen wurde.
	if run.Version != "" {
		if _, err := s.store.SetDelivered(key, run.Version, run.StartedBy); err != nil {
			s.logger.Warn("Auslieferungsversion nicht gesetzt", "ticket", key, "error", err)
		}
	}
	if _, err := s.store.SetStatus(key, tickets.StatusDone, run.StartedBy); err != nil {
		s.logger.Warn("Statuswechsel nach dem Deploy fehlgeschlagen", "ticket", key, "error", err)
		return
	}
	s.logger.Info("Ticket nach Deploy geschlossen", "ticket", key, "durchlauf", run.ID)
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	return digits
}

// headCommit is best effort: a journal entry without a commit is still useful,
// one that blocks the deploy because git hiccuped is not.
func (s *Server) headCommit() string {
	if s.git == nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := s.git(ctx, "rev-parse", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// deploymentState answers two questions per pipeline that nothing could answer
// before the journal existed: what is running, and what would a rollback go
// back to. Both survive a restart of the cockpit (MC-0018, HF-0104).
func (s *Server) deploymentState() []map[string]any {
	out := make([]map[string]any, 0)
	if s.deploy == nil || s.deploy.Journal == nil {
		return out
	}
	for _, pipeline := range deploy.Pipelines() {
		entry := map[string]any{"pipeline": pipeline.Name}
		if last, ok := s.deploy.Journal.Last(pipeline.Name); ok {
			entry["aktuell"] = last
		}
		// Ohne zweiten Eintrag gibt es kein Ziel. Das zu sagen ist ehrlicher als
		// eine Schaltfläche anzubieten, die nirgendwohin führt.
		if previous, ok := s.deploy.Journal.Previous(pipeline.Name); ok {
			entry["rueckfall"] = previous
		}
		out = append(out, entry)
	}
	return out
}
