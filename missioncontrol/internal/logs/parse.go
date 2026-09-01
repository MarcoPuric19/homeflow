// Package logs turns pod output into structured entries.
//
// Five formats actually occur in this cluster, and a log view that treats them
// all as text is a log view nobody reads:
//
//	HomeFlow API        slog JSON       {"time":…,"level":"INFO","msg":…}
//	Prometheus          logfmt          time=… level=WARN msg="…"
//	Grafana             logfmt          logger=… t=… level=info msg="…"
//	kube-state-metrics  klog            I0831 22:34:48.305252  1 file.go:357] "…"
//	PostgreSQL          eigenes Format  2026-09-01 06:30:38.389 UTC [68] LOG:  …
//
// Everything else falls back to plain text with a level guessed from the words
// in it. The point of parsing is not tidiness: without a level there is no
// filtering, no colour and no histogram, and warnings stay invisible.
package logs

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Levels, normalised. Everything a source calls its levels ends up as one of
// these, so one filter works across five formats.
const (
	LevelError   = "ERROR"
	LevelWarn    = "WARN"
	LevelInfo    = "INFO"
	LevelDebug   = "DEBUG"
	LevelUnknown = "UNBEKANNT"
)

func Levels() []string { return []string{LevelError, LevelWarn, LevelInfo, LevelDebug, LevelUnknown} }

type Entry struct {
	Time    time.Time         `json:"zeit"`
	Level   string            `json:"level"`
	Message string            `json:"nachricht"`
	Fields  map[string]string `json:"felder,omitempty"`
	Raw     string            `json:"roh"`
	Format  string            `json:"format"`
}

var (
	// kubectl --timestamps prefixes every line with an RFC3339Nano instant.
	kubectlStamp = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z) (.*)$`)
	klogLine     = regexp.MustCompile(`^([IWEF])(\d{4} \d{2}:\d{2}:\d{2}\.\d+)\s+\d+ ([^\]]+)\] (.*)$`)
	postgresLine = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d+ \w+) \[(\d+)\] (\w+):\s+(.*)$`)
	logfmtPair   = regexp.MustCompile(`(\w[\w.\-]*)=("(?:[^"\\]|\\.)*"|\S*)`)
	wordLevel    = regexp.MustCompile(`(?i)\b(panic|fatal|error|err|warn(?:ing)?|debug|trace|info)\b`)
)

// Parse reads one line. It never fails: an unparsable line is still an entry,
// because a log view that silently drops what it does not understand is worse
// than one that shows it plainly.
func Parse(line string) Entry {
	raw := strings.TrimRight(line, "\r")
	entry := Entry{Raw: raw, Level: LevelUnknown, Format: "text", Message: raw}

	rest := raw
	if match := kubectlStamp.FindStringSubmatch(raw); match != nil {
		if stamp, err := time.Parse(time.RFC3339Nano, match[1]); err == nil {
			entry.Time = stamp
		}
		rest = match[2]
		entry.Message = rest
	}

	switch {
	case strings.HasPrefix(rest, "{"):
		if parseJSON(rest, &entry) {
			return entry
		}
	case klogLine.MatchString(rest):
		parseKlog(rest, &entry)
		return entry
	case postgresLine.MatchString(rest):
		parsePostgres(rest, &entry)
		return entry
	}
	if strings.Contains(rest, "level=") || strings.Contains(rest, "logger=") {
		parseLogfmt(rest, &entry)
		return entry
	}
	entry.Level = levelFromWords(rest)
	return entry
}

func parseJSON(line string, entry *Entry) bool {
	var raw map[string]any
	if json.Unmarshal([]byte(line), &raw) != nil {
		return false
	}
	entry.Format = "json"
	entry.Fields = map[string]string{}
	for key, value := range raw {
		text := stringify(value)
		switch strings.ToLower(key) {
		case "level", "severity", "lvl":
			entry.Level = normaliseLevel(text)
		case "msg", "message":
			entry.Message = text
		case "time", "ts", "timestamp", "t":
			if entry.Time.IsZero() {
				if stamp, err := time.Parse(time.RFC3339Nano, text); err == nil {
					entry.Time = stamp
				}
			}
		default:
			entry.Fields[key] = text
		}
	}
	if entry.Level == "" {
		entry.Level = LevelUnknown
	}
	return true
}

func parseKlog(line string, entry *Entry) {
	match := klogLine.FindStringSubmatch(line)
	entry.Format = "klog"
	switch match[1] {
	case "E", "F":
		entry.Level = LevelError
	case "W":
		entry.Level = LevelWarn
	default:
		entry.Level = LevelInfo
	}
	entry.Fields = map[string]string{"source": match[3]}
	body := match[4]
	// klog messages are usually a quoted string followed by key=value pairs.
	if strings.HasPrefix(body, `"`) {
		if end := strings.Index(body[1:], `"`); end >= 0 {
			entry.Message = body[1 : end+1]
			collectPairs(body[end+2:], entry.Fields)
			return
		}
	}
	entry.Message = body
}

func parsePostgres(line string, entry *Entry) {
	match := postgresLine.FindStringSubmatch(line)
	entry.Format = "postgres"
	entry.Fields = map[string]string{"pid": match[2], "postgres_level": match[3]}
	entry.Message = match[4]
	switch match[3] {
	case "ERROR", "FATAL", "PANIC":
		entry.Level = LevelError
	case "WARNING":
		entry.Level = LevelWarn
	case "DEBUG", "DEBUG1", "DEBUG2", "DEBUG3", "DEBUG4", "DEBUG5":
		entry.Level = LevelDebug
	default: // LOG, INFO, NOTICE, STATEMENT, DETAIL, HINT
		entry.Level = LevelInfo
	}
	if entry.Time.IsZero() {
		if stamp, err := time.Parse("2006-01-02 15:04:05.000 MST", match[1]); err == nil {
			entry.Time = stamp
		}
	}
}

func parseLogfmt(line string, entry *Entry) {
	entry.Format = "logfmt"
	entry.Fields = map[string]string{}
	collectPairs(line, entry.Fields)
	if level, ok := entry.Fields["level"]; ok {
		entry.Level = normaliseLevel(level)
		delete(entry.Fields, "level")
	} else {
		entry.Level = levelFromWords(line)
	}
	for _, key := range []string{"msg", "message"} {
		if value, ok := entry.Fields[key]; ok {
			entry.Message = value
			delete(entry.Fields, key)
			break
		}
	}
	for _, key := range []string{"time", "t", "ts"} {
		if value, ok := entry.Fields[key]; ok {
			if entry.Time.IsZero() {
				if stamp, err := time.Parse(time.RFC3339Nano, value); err == nil {
					entry.Time = stamp
				}
			}
			delete(entry.Fields, key)
			break
		}
	}
}

func collectPairs(text string, into map[string]string) {
	for _, pair := range logfmtPair.FindAllStringSubmatch(text, -1) {
		value := pair[2]
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
		into[pair[1]] = value
	}
}

func normaliseLevel(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "ERROR", "ERR", "FATAL", "PANIC", "CRITICAL", "CRIT", "SEVERE":
		return LevelError
	case "WARN", "WARNING":
		return LevelWarn
	case "DEBUG", "TRACE", "DBG":
		return LevelDebug
	case "INFO", "INFORMATION", "NOTICE", "LOG":
		return LevelInfo
	case "":
		return LevelUnknown
	default:
		return LevelUnknown
	}
}

// levelFromWords is the last resort for lines with no level field. It is
// deliberately conservative: a line that merely mentions "error" in prose gets
// the level, which is better than a real error staying grey.
func levelFromWords(line string) string {
	if match := wordLevel.FindString(line); match != "" {
		return normaliseLevel(match)
	}
	return LevelInfo
}

func stringify(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	case nil:
		return ""
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		return string(encoded)
	}
}
