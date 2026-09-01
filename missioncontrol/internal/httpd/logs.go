package httpd

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"missioncontrol/internal/logs"
)

// Kubernetes object names are restricted to this shape. Checking it here means
// nothing from a query string is ever handed to kubectl as something that could
// be read as a flag or another argument.
var nameAllowed = regexp.MustCompile(`^[a-z0-9]([-a-z0-9.]{0,61}[a-z0-9])?$`)

const (
	maxLogLines     = 2000
	defaultLogLines = 200
	// Enough bars to see a spike, few enough to stay readable on a laptop.
	histogramBuckets = 40
)

// logs streams the tail of one pod's log. It is read-only in every sense:
// `kubectl logs` cannot change anything in the cluster, and the pod must be one
// of the namespaces this cockpit was started with.
func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	namespace, pod, container := query.Get("namespace"), query.Get("pod"), query.Get("container")
	if !nameAllowed.MatchString(namespace) || !nameAllowed.MatchString(pod) {
		writeError(w, http.StatusBadRequest, "Namespace oder Pod fehlt oder ist ungültig")
		return
	}
	if container != "" && !nameAllowed.MatchString(container) {
		writeError(w, http.StatusBadRequest, "Container-Name ungültig")
		return
	}
	allowed := false
	for _, candidate := range s.config.Namespaces {
		if candidate == namespace {
			allowed = true
		}
	}
	if !allowed {
		// Not a leak of anything secret, but the cockpit only speaks for the
		// namespaces it was told to watch.
		writeError(w, http.StatusForbidden, "Dieser Namespace gehört nicht zu Mission Control")
		return
	}

	lines := defaultLogLines
	if value := query.Get("lines"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > maxLogLines {
			writeError(w, http.StatusBadRequest, "lines muss zwischen 1 und 2000 liegen")
			return
		}
		lines = parsed
	}

	args := []string{"-n", namespace, "logs", pod, "--tail", strconv.Itoa(lines), "--timestamps"}
	if container != "" {
		args = append(args, "-c", container)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	out, err := s.run(ctx, args...)
	if err != nil {
		// A pod that is starting or has no logs yet is normal, not a failure of
		// the cockpit - say so instead of returning 500.
		writeJSON(w, http.StatusOK, map[string]any{
			"pod": pod, "namespace": namespace, "container": container,
			"eintraege": []any{}, "zaehler": map[string]int{}, "hinweis": err.Error(),
		})
		return
	}

	result := logs.Analyse(splitLines(string(out)), buildQuery(query), histogramBuckets)
	writeJSON(w, http.StatusOK, map[string]any{
		"pod": pod, "namespace": namespace, "container": container,
		"eintraege": result.Entries, "zaehler": result.Counts, "gesamt": result.Total,
		"gefiltert": result.Matched, "histogramm": result.Histogram, "bucketMs": result.BucketMS,
		"felder": result.Fields, "formate": result.Formats, "level": logs.Levels(),
	})
}

// buildQuery reads the Kibana-style controls: level chips, a free text box and
// any number of field:value filters added by clicking a value in the sidebar.
func buildQuery(query url.Values) logs.Query {
	built := logs.Query{Text: strings.TrimSpace(query.Get("q")), Fields: map[string]string{}}
	for _, level := range strings.Split(query.Get("level"), ",") {
		if level = strings.TrimSpace(level); level != "" {
			built.Levels = append(built.Levels, strings.ToUpper(level))
		}
	}
	// feld=name:wert, mehrfach erlaubt.
	for _, pair := range query["feld"] {
		name, value, found := strings.Cut(pair, ":")
		if found && name != "" {
			built.Fields[name] = value
		}
	}
	built.From = parseInstant(query.Get("von"))
	built.To = parseInstant(query.Get("bis"))
	return built
}

// parseInstant accepts RFC 3339 or milliseconds since epoch. The histogram
// hands back milliseconds because that is what a browser has; a person typing
// a range into the address bar has RFC 3339.
func parseInstant(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	if instant, err := time.Parse(time.RFC3339, value); err == nil {
		return instant
	}
	if millis, err := strconv.ParseInt(value, 10, 64); err == nil && millis > 0 {
		return time.UnixMilli(millis)
	}
	return time.Time{}
}

func splitLines(value string) []string {
	lines := make([]string, 0, 64)
	start := 0
	for i := 0; i < len(value); i++ {
		if value[i] == '\n' {
			if i > start {
				lines = append(lines, value[start:i])
			}
			start = i + 1
		}
	}
	if start < len(value) {
		lines = append(lines, value[start:])
	}
	return lines
}
