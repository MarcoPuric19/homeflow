package logs

import (
	"testing"
	"time"
)

// Every line below was copied out of this cluster on 2026-09-01. Testing
// against invented samples would only prove that the regexes match themselves.
const (
	apiLine        = `2026-09-01T07:52:07.415069963Z {"time":"2026-09-01T07:52:07.415069963Z","level":"INFO","msg":"http request","method":"GET","path":"/health","status":200,"duration_ms":0}`
	postgresLog    = `2026-09-01T06:30:38.389000000Z 2026-09-01 06:30:38.389 UTC [68] LOG:  checkpoint starting: time`
	postgresErr    = `2026-09-01T06:30:38.389000000Z 2026-09-01 06:30:38.389 UTC [81] ERROR:  new row for relation "users" violates check constraint "users_username_check"`
	prometheusWarn = `2026-09-01T07:51:51.390Z time=2026-09-01T07:51:51.390Z level=WARN source=scrape_append_v2.go:402 msg="Error on ingesting samples with different value but same timestamp" component="scrape manager" scrape_pool=kube-state-metrics`
	grafanaInfo    = `2026-09-01T07:45:35.763Z logger=plugins.update.checker t=2026-09-01T07:45:35.763282721Z level=info msg="Update check succeeded" duration=179.474472ms`
	kubeStateInfo  = `2026-08-31T22:34:48.305Z I0831 22:34:48.305252       1 tls_config.go:357] "TLS is disabled." http2=false address="[::]:8081"`
	kubeStateErr   = `2026-08-31T22:34:48.305Z E0831 22:34:48.305252       1 reflector.go:158] "Unhandled Error" err="pkg/mod/k8s.io/client-go: failed to list"`
	plainLine      = `2026-09-01T07:45:35.763Z irgendein Text ohne Struktur`
)

func TestParseRecognisesEveryFormatInThisCluster(t *testing.T) {
	cases := map[string]struct {
		line    string
		format  string
		level   string
		message string
	}{
		"HomeFlow API (slog JSON)": {apiLine, "json", LevelInfo, "http request"},
		"PostgreSQL LOG":           {postgresLog, "postgres", LevelInfo, "checkpoint starting: time"},
		"PostgreSQL ERROR":         {postgresErr, "postgres", LevelError, `new row for relation "users" violates check constraint "users_username_check"`},
		"Prometheus logfmt":        {prometheusWarn, "logfmt", LevelWarn, "Error on ingesting samples with different value but same timestamp"},
		"Grafana logfmt":           {grafanaInfo, "logfmt", LevelInfo, "Update check succeeded"},
		"kube-state-metrics klog":  {kubeStateInfo, "klog", LevelInfo, "TLS is disabled."},
		"klog Fehler":              {kubeStateErr, "klog", LevelError, "Unhandled Error"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			entry := Parse(testCase.line)
			if entry.Format != testCase.format {
				t.Fatalf("format=%q, erwartet %q", entry.Format, testCase.format)
			}
			if entry.Level != testCase.level {
				t.Fatalf("level=%q, erwartet %q", entry.Level, testCase.level)
			}
			if entry.Message != testCase.message {
				t.Fatalf("nachricht=%q, erwartet %q", entry.Message, testCase.message)
			}
			if entry.Time.IsZero() {
				t.Fatal("ohne Zeitstempel")
			}
		})
	}
}

func TestFieldsAreExtractedNotJustLevels(t *testing.T) {
	entry := Parse(apiLine)
	for name, want := range map[string]string{"method": "GET", "path": "/health", "status": "200", "duration_ms": "0"} {
		if entry.Fields[name] != want {
			t.Fatalf("%s=%q, erwartet %q (alle: %v)", name, entry.Fields[name], want, entry.Fields)
		}
	}
	// Zeit, Level und Nachricht gehören in die eigenen Felder, nicht doppelt
	// in die Feldliste - sonst steht in der Seitenleiste dreimal dasselbe.
	for _, unwanted := range []string{"level", "msg", "time"} {
		if _, exists := entry.Fields[unwanted]; exists {
			t.Fatalf("%q sollte kein freies Feld sein: %v", unwanted, entry.Fields)
		}
	}
	prometheus := Parse(prometheusWarn)
	if prometheus.Fields["scrape_pool"] != "kube-state-metrics" {
		t.Fatalf("logfmt-Felder=%v", prometheus.Fields)
	}
	klog := Parse(kubeStateInfo)
	if klog.Fields["source"] != "tls_config.go:357" || klog.Fields["http2"] != "false" {
		t.Fatalf("klog-Felder=%v", klog.Fields)
	}
}

func TestUnparsableLinesStayVisible(t *testing.T) {
	entry := Parse(plainLine)
	if entry.Format != "text" || entry.Message == "" {
		t.Fatalf("entry=%#v", entry)
	}
	broken := Parse(`2026-09-01T07:45:35.763Z {"level":"INFO", kaputt`)
	if broken.Message == "" {
		t.Fatal("kaputtes JSON darf nicht verschluckt werden")
	}
	// Ein Wort im Text reicht als letzte Rettung, damit ein Fehler nicht grau
	// bleibt, nur weil das Format unbekannt ist.
	if guessed := Parse("2026-09-01T07:45:35.763Z panic: runtime error"); guessed.Level != LevelError {
		t.Fatalf("level=%q", guessed.Level)
	}
}

func lines() []string {
	return []string{apiLine, postgresLog, postgresErr, prometheusWarn, grafanaInfo, kubeStateInfo, kubeStateErr, plainLine, ""}
}

func TestAnalyseCountsAcrossAllLinesEvenWhenFiltered(t *testing.T) {
	result := Analyse(lines(), Query{Levels: []string{LevelError}}, 20)
	if result.Total != 8 {
		t.Fatalf("gesamt=%d", result.Total)
	}
	if result.Matched != 2 || len(result.Entries) != 2 {
		t.Fatalf("gefiltert=%d", result.Matched)
	}
	// Der Zähler muss weiterhin alle Level kennen - sonst versteckt ein Filter,
	// dass es überhaupt Warnungen gibt.
	if result.Counts[LevelWarn] != 1 || result.Counts[LevelInfo] < 4 {
		t.Fatalf("zaehler=%v", result.Counts)
	}
	if result.Formats["json"] != 1 || result.Formats["postgres"] != 2 {
		t.Fatalf("formate=%v", result.Formats)
	}
}

func TestTextAndFieldFiltersNarrowTheView(t *testing.T) {
	if result := Analyse(lines(), Query{Text: "checkpoint"}, 10); result.Matched != 1 {
		t.Fatalf("Textfilter=%d", result.Matched)
	}
	if result := Analyse(lines(), Query{Fields: map[string]string{"path": "/health"}}, 10); result.Matched != 1 {
		t.Fatalf("Feldfilter=%d", result.Matched)
	}
	if result := Analyse(lines(), Query{Fields: map[string]string{"path": "/gibtsnicht"}}, 10); result.Matched != 0 {
		t.Fatalf("Feldfilter ohne Treffer=%d", result.Matched)
	}
}

func TestHistogramIsOrderedAndCountsByLevel(t *testing.T) {
	result := Analyse(lines(), Query{}, 20)
	if len(result.Histogram) == 0 {
		t.Fatal("kein Histogramm")
	}
	sum := 0
	for i, bucket := range result.Histogram {
		sum += bucket.Total
		if i > 0 && !result.Histogram[i-1].Time.Before(bucket.Time) {
			t.Fatalf("Histogramm nicht sortiert: %v", result.Histogram)
		}
	}
	if sum != result.Matched {
		t.Fatalf("Histogramm zählt %d, gefiltert sind %d", sum, result.Matched)
	}
	if result.BucketMS <= 0 {
		t.Fatalf("bucketMs=%d", result.BucketMS)
	}
}

func TestFieldSidebarRanksByCoverage(t *testing.T) {
	many := []string{}
	for i := 0; i < 6; i++ {
		many = append(many, apiLine)
	}
	many = append(many, prometheusWarn)
	result := Analyse(many, Query{}, 10)
	if len(result.Fields) == 0 {
		t.Fatal("keine Felder")
	}
	found := false
	for _, field := range result.Fields {
		if field.Name != "status" {
			continue
		}
		found = true
		if len(field.Values) != 1 || field.Values[0].Value != "200" || field.Values[0].Count != 6 {
			t.Fatalf("status-Feld=%#v", field)
		}
	}
	if !found {
		t.Fatalf("Feld status fehlt: %#v", result.Fields)
	}
	for _, field := range result.Fields {
		if len(field.Values) > 5 {
			t.Fatalf("mehr als fünf Werte je Feld: %#v", field)
		}
	}
}

func TestEmptyInputIsHarmless(t *testing.T) {
	result := Analyse(nil, Query{}, 20)
	if result.Total != 0 || len(result.Histogram) != 0 || result.Entries == nil {
		t.Fatalf("result=%#v", result)
	}
}

func TestPostgresTimestampIsParsedWhenKubectlPrefixIsMissing(t *testing.T) {
	entry := Parse(`2026-09-01 06:30:38.389 UTC [68] LOG:  checkpoint starting: time`)
	if entry.Time.IsZero() || entry.Time.Year() != 2026 {
		t.Fatalf("zeit=%v", entry.Time)
	}
	if entry.Time.Sub(time.Date(2026, 9, 1, 6, 30, 38, 389000000, time.UTC)) != 0 {
		t.Fatalf("zeit=%v", entry.Time)
	}
}
