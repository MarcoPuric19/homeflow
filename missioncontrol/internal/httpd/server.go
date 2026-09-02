// Package httpd serves the cockpit: a JSON API and the static UI next to it.
//
// The server binds loopback only and has no authentication, which is the same
// bargain the rest of this repository makes for local tools. It reads cluster
// state and writes ticket files, so it must never be exposed beyond 127.0.0.1
// without an authentication story first.
package httpd

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"missioncontrol/internal/cluster"
	"missioncontrol/internal/deploy"
	"missioncontrol/internal/repo"
	"missioncontrol/internal/tickets"
)

type Config struct {
	Namespaces []string
	Interval   time.Duration
	WebRoot    string
	Version    string
	// RepoRoot is where the version page reads Makefile, go.mod and the like.
	RepoRoot string
}

type Server struct {
	logger   *slog.Logger
	store    *tickets.Store
	run      cluster.Runner
	git      repo.Runner
	deploy   *deploy.Runner
	config   Config
	handler  http.Handler
	mu       sync.RWMutex
	snapshot cluster.Snapshot
}

func New(logger *slog.Logger, store *tickets.Store, run cluster.Runner, git repo.Runner, deployer *deploy.Runner, config Config) *Server {
	s := &Server{logger: logger, store: store, run: run, git: git, deploy: deployer, config: config}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/cluster", s.cluster)
	mux.HandleFunc("POST /api/cluster/refresh", s.refresh)
	mux.HandleFunc("GET /api/tickets", s.listTickets)
	mux.HandleFunc("POST /api/tickets", s.createTicket)
	mux.HandleFunc("GET /api/tickets/{id}", s.getTicket)
	mux.HandleFunc("POST /api/tickets/{id}/comments", s.commentTicket)
	mux.HandleFunc("POST /api/tickets/{id}/status", s.statusTicket)
	mux.HandleFunc("POST /api/tickets/{id}/owner", s.ownerTicket)
	mux.HandleFunc("POST /api/tickets/{id}/version", s.versionTicket)
	mux.HandleFunc("POST /api/tickets/{id}/priority", s.priorityTicket)
	mux.HandleFunc("POST /api/tickets/{id}/type", s.typeTicket)
	mux.HandleFunc("POST /api/tickets/{id}/epic", s.epicTicket)
	mux.HandleFunc("POST /api/tickets/{id}/scope", s.scopeTicket)
	mux.HandleFunc("POST /api/tickets/{id}/dod", s.dodTicket)
	mux.HandleFunc("POST /api/tickets/{id}/relations", s.addRelation)
	mux.HandleFunc("DELETE /api/tickets/{id}/relations", s.removeRelation)
	mux.HandleFunc("GET /api/board", s.board)
	mux.HandleFunc("GET /api/logs", s.logs)
	mux.HandleFunc("GET /api/versions", s.versions)
	mux.HandleFunc("GET /api/repo", s.repository)
	mux.HandleFunc("GET /api/deploy", s.deployState)
	mux.HandleFunc("POST /api/deploy", s.deployStart)
	mux.HandleFunc("GET /api/deploy/changes", s.deployChanges)
	mux.HandleFunc("POST /api/deploy/cancel", s.deployCancel)
	mux.HandleFunc("POST /api/deploy/restart", s.deployRestart)
	if config.WebRoot != "" {
		mux.Handle("/", noCache(http.FileServer(http.Dir(config.WebRoot))))
	}
	if deployer != nil {
		deployer.OnStart = s.beforeDeploy
		deployer.OnFinish = s.afterDeploy
	}
	s.handler = headers(sameOriginOnly(mux))
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.handler.ServeHTTP(w, r) }

// Poll keeps one snapshot warm so the UI never waits for kubectl, and so a
// cluster that is down shows the last known state plus a warning instead of an
// empty page.
func (s *Server) Poll(ctx context.Context) {
	s.takeSnapshot(ctx)
	ticker := time.NewTicker(s.config.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.takeSnapshot(ctx)
		}
	}
}

func (s *Server) takeSnapshot(ctx context.Context) {
	timeout, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	snapshot := cluster.Take(timeout, s.run, s.config.Namespaces)
	s.mu.Lock()
	s.snapshot = snapshot
	s.mu.Unlock()
	for _, warning := range snapshot.Warnings {
		s.logger.Warn("snapshot unvollständig", "detail", warning)
	}
}

func headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// contextWithTimeout keeps the git calls from hanging the page.
func contextWithTimeout(r *http.Request, limit time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), limit)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"fehler": message})
}

func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "Anfrage nicht lesbar")
		return false
	}
	return true
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	snapshot := s.snapshot
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok", "version": s.config.Version,
		"letzterSnapshot": snapshot.TakenAt, "namespaces": s.config.Namespaces,
	})
}

func (s *Server) cluster(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	snapshot := s.snapshot
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	s.takeSnapshot(r.Context())
	s.cluster(w, r)
}

func (s *Server) listTickets(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.List()
	if err != nil {
		s.logger.Error("tickets lesen", "error", err)
		writeError(w, http.StatusInternalServerError, "Tickets konnten nicht gelesen werden")
		return
	}
	// Jira-artige Filter, alle optional und kombinierbar.
	query := r.URL.Query()
	filters := map[string]func(tickets.Ticket) string{
		"status":     func(t tickets.Ticket) string { return t.Status },
		"projekt":    func(t tickets.Ticket) string { return t.Project },
		"typ":        func(t tickets.Ticket) string { return t.Type },
		"zustaendig": func(t tickets.Ticket) string { return t.Owner },
		"version":    func(t tickets.Ticket) string { return t.Version },
		"epic":       func(t tickets.Ticket) string { return t.Epic },
	}
	for name, field := range filters {
		want := query.Get(name)
		if want == "" {
			continue
		}
		filtered := make([]tickets.Ticket, 0, len(list))
		for _, ticket := range list {
			if field(ticket) == want {
				filtered = append(filtered, ticket)
			}
		}
		list = filtered
	}
	// Der Lebenslauf wird berechnet und nicht gespeichert: "seit wann offen" und
	// "Claim veraltet" haengen an der Uhr, nicht an der Datei (MC-0021).
	now := time.Now()
	// Offene Blocker werden aus der bereits geladenen Liste berechnet, nicht je
	// Ticket nachgeschlagen: sonst wird aus einer Ansicht ein Dateisturm.
	status := make(map[string]string, len(list))
	for _, ticket := range list {
		status[ticket.Key] = ticket.Status
	}
	angereichert := make([]map[string]any, 0, len(list))
	for _, ticket := range list {
		raw, err := json.Marshal(ticket)
		if err != nil {
			continue
		}
		var felder map[string]any
		if err := json.Unmarshal(raw, &felder); err != nil {
			continue
		}
		felder["lebenslauf"] = ticket.Life(now)
		blocker := []string{}
		for _, r := range ticket.Relations {
			if r.Kind != tickets.RelDependsOn {
				continue
			}
			if other, ok := status[r.Key]; ok && other != tickets.StatusDone {
				blocker = append(blocker, r.Key+" ("+other+")")
			}
		}
		felder["offene_blocker"] = blocker
		felder["dod_punkte"] = ticket.DefinitionOfDone()
		felder["offene_dod"] = ticket.OpenDoD()
		angereichert = append(angereichert, felder)
	}
	writeJSON(w, http.StatusOK, map[string]any{"tickets": angereichert})
}

// Beziehungen werden immer beidseitig geschrieben; der Store lehnt Selbstbezug,
// unbekannte Tickets und Zyklen in der Blockierrichtung ab.
func (s *Server) relation(w http.ResponseWriter, r *http.Request,
	apply func(key, kind, other string, by tickets.Actor) (tickets.Ticket, error)) {
	var input struct {
		Art    string `json:"art"`
		Ziel   string `json:"ziel"`
		Author string `json:"author"`
	}
	if !decode(w, r, &input) {
		return
	}
	ticket, err := apply(r.PathValue("id"), input.Art, input.Ziel, actorOf(r, input.Author))
	s.respond(w, ticket, err, http.StatusOK)
}

func (s *Server) addRelation(w http.ResponseWriter, r *http.Request) {
	s.relation(w, r, s.store.AddRelationBy)
}

func (s *Server) removeRelation(w http.ResponseWriter, r *http.Request) {
	s.relation(w, r, s.store.RemoveRelationBy)
}

// Die Punkte 1 und 6 kann das Cockpit selbst sehen und lehnt sie deshalb hier
// ab: ein Haken, den ein Mensch auch von Hand setzen koennte, waere kein Beleg
// mehr (MC-0027).
func (s *Server) dodTicket(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Nr       int    `json:"nr"`
		Erfuellt bool   `json:"erfuellt"`
		Author   string `json:"author"`
	}
	if !decode(w, r, &input) {
		return
	}
	ticket, err := s.store.SetDoDBy(r.PathValue("id"), input.Nr, input.Erfuellt, actorOf(r, input.Author))
	s.respond(w, ticket, err, http.StatusOK)
}

func (s *Server) createTicket(w http.ResponseWriter, r *http.Request) {
	var input tickets.NewTicket
	if !decode(w, r, &input) {
		return
	}
	ticket, err := s.store.Create(input)
	s.respond(w, ticket, err, http.StatusCreated)
}

func (s *Server) getTicket(w http.ResponseWriter, r *http.Request) {
	ticket, err := s.store.Get(r.PathValue("id"))
	s.respond(w, ticket, err, http.StatusOK)
}

func (s *Server) commentTicket(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Author string `json:"author"`
		Text   string `json:"text"`
	}
	if !decode(w, r, &input) {
		return
	}
	ticket, err := s.store.Comment(r.PathValue("id"), input.Author, input.Text)
	s.respond(w, ticket, err, http.StatusOK)
}

func (s *Server) statusTicket(w http.ResponseWriter, r *http.Request) {
	s.setField(w, r, "status")
}

func (s *Server) ownerTicket(w http.ResponseWriter, r *http.Request) {
	s.setField(w, r, "owner")
}

// board hands the UI the column order and the current version, so neither is
// duplicated in JavaScript.
func (s *Server) board(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"spalten":      tickets.Columns(),
		"version":      s.config.Version,
		"projekte":     tickets.Projects(),
		"typen":        tickets.Types(),
		"prioritaeten": tickets.Priorities(),
	})
}

func (s *Server) priorityTicket(w http.ResponseWriter, r *http.Request) {
	s.setField(w, r, "priority")
}

func (s *Server) typeTicket(w http.ResponseWriter, r *http.Request) {
	s.setField(w, r, "type")
}

func (s *Server) epicTicket(w http.ResponseWriter, r *http.Request) {
	s.setField(w, r, "epic")
}

// scopeTicket differs from the others: an empty value is legitimate, it clears
// the scope.
func (s *Server) scopeTicket(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Wert   string `json:"wert"`
		Author string `json:"author"`
	}
	if !decode(w, r, &input) {
		return
	}
	ticket, err := s.store.SetScopeBy(r.PathValue("id"), input.Wert, actorOf(r, input.Author))
	s.respond(w, ticket, err, http.StatusOK)
}

// setField serves every "change one field" endpoint: they all take the same
// body and map errors the same way.
// actorOf records what the request actually shows, next to the name it claims.
// There is no login here on purpose, so the name is a claim and cannot be
// verified. The origin is not proof either — it is evidence, and it is the
// piece that was missing when a test call carrying "author":"Marco" put an
// acceptance under his name that he never gave (HF-0073).
func actorOf(r *http.Request, name string) tickets.Actor {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	agent := r.UserAgent()
	if len(agent) > 60 {
		agent = agent[:60] + "…"
	}
	if agent == "" {
		agent = "ohne User-Agent"
	}
	return tickets.Actor{Name: name, Origin: host + ", " + agent}
}

// setField takes the field name from the route rather than from the body, so a
// request cannot choose which field it writes.
func (s *Server) setField(w http.ResponseWriter, r *http.Request, field string) {
	var input struct {
		Wert   string `json:"wert"`
		Author string `json:"author"`
	}
	if !decode(w, r, &input) {
		return
	}
	ticket, err := s.store.SetFieldBy(r.PathValue("id"), field, input.Wert, actorOf(r, input.Author))
	s.respond(w, ticket, err, http.StatusOK)
}

func (s *Server) versionTicket(w http.ResponseWriter, r *http.Request) {
	s.setField(w, r, "version")
}

func (s *Server) respond(w http.ResponseWriter, ticket tickets.Ticket, err error, success int) {
	switch {
	case errors.Is(err, tickets.ErrInvalid):
		writeError(w, http.StatusUnprocessableEntity, "Eingabe unvollständig oder ungültig")
	case errors.Is(err, tickets.ErrNotFound):
		writeError(w, http.StatusNotFound, "Ticket nicht gefunden")
	case err != nil:
		s.logger.Error("ticket schreiben", "error", err)
		writeError(w, http.StatusInternalServerError, "Ticket konnte nicht gespeichert werden")
	default:
		writeJSON(w, success, ticket)
	}
}

// Statuses exposes the allowed values so the UI never invents its own.
func Statuses() []string { return tickets.Columns() }

// noCache lässt den Browser jede Datei nachfragen, statt sie zu raten.
//
// Der FileServer schickt nur Last-Modified und kein Cache-Control. Damit cachet
// ein Browser heuristisch - er darf eine Datei ohne Rückfrage weiterverwenden -,
// und genau das ist zweimal passiert: die Oberfläche war längst umgebaut, und
// im Browser stand die alte Fassung. Zweimal hieß der Befund "ist immer noch
// ein Board", und zweimal habe ich am falschen Ende gesucht.
//
// no-cache heißt nicht "nicht speichern", sondern "vor jeder Benutzung
// nachfragen". Auf einem Cockpit, das auf dem Loopback für einen Menschen läuft,
// kostet das nichts und spart die Frage, ob man gerade den aktuellen Stand sieht.
func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		next.ServeHTTP(w, r)
	})
}
