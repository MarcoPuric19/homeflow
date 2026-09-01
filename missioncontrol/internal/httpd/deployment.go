package httpd

import (
	"errors"
	"net/http"

	"missioncontrol/internal/deploy"
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
	})
}

const deployTarget = "kind-dev"

func (s *Server) deployStart(w http.ResponseWriter, r *http.Request) {
	if s.deploy == nil {
		writeError(w, http.StatusForbidden, "Der Deploy-Dienst ist abgeschaltet.")
		return
	}
	var input struct {
		Pipeline string `json:"pipeline"`
		Author   string `json:"author"`
		Ticket   string `json:"ticket"`
		// Trotzdem forces a run the change check would refuse.
		Trotzdem bool `json:"trotzdem"`
	}
	if !decode(w, r, &input) {
		return
	}
	if input.Ticket != "" && !tickets.ValidKey(input.Ticket) {
		writeError(w, http.StatusUnprocessableEntity, "Ticketschlüssel ungültig")
		return
	}
	run, err := s.deploy.Start(input.Pipeline, input.Author, input.Ticket, input.Trotzdem)
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
		s.logger.Info("deploy gestartet", "pipeline", run.Pipeline, "von", run.StartedBy, "ticket", run.Ticket)
		// A deployment is worth a line in the ticket it belongs to.
		if run.Ticket != "" {
			note := "Deploy gestartet: " + run.Title + " (Durchlauf #" + itoa(run.ID) + ")"
			if run.Forced {
				note += " - erzwungen, obwohl die Prüfung keine Änderung fand"
			}
			if _, err := s.store.Comment(run.Ticket, run.StartedBy, note); err != nil {
				s.logger.Warn("Ticketkommentar zum Deploy fehlgeschlagen", "error", err)
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

func (s *Server) deployCancel(w http.ResponseWriter, _ *http.Request) {
	if s.deploy == nil || !s.deploy.Cancel() {
		writeError(w, http.StatusConflict, "Es läuft nichts, was abgebrochen werden könnte.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "abgebrochen"})
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
