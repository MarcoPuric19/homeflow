package logs

import (
	"sort"
	"strings"
	"time"
)

// Everything the log view needs in one answer. The aggregation happens here and
// not in the browser: 2000 lines are cheap in Go and expensive in JavaScript,
// and the histogram has to agree with the counters.
type Result struct {
	Entries   []Entry        `json:"eintraege"`
	Counts    map[string]int `json:"zaehler"`
	Total     int            `json:"gesamt"`
	Matched   int            `json:"gefiltert"`
	Histogram []Bucket       `json:"histogramm"`
	BucketMS  int64          `json:"bucketMs"`
	Fields    []Field        `json:"felder"`
	Formats   map[string]int `json:"formate"`
}

type Bucket struct {
	Time   time.Time      `json:"zeit"`
	Counts map[string]int `json:"zaehler"`
	Total  int            `json:"gesamt"`
}

type FieldValue struct {
	Value string `json:"wert"`
	Count int    `json:"anzahl"`
}

type Field struct {
	Name   string       `json:"name"`
	Values []FieldValue `json:"werte"`
	Total  int          `json:"gesamt"`
}

// Query is what the user typed and clicked.
type Query struct {
	Levels []string          // empty means every level
	Text   string            // free text over message and raw line
	Fields map[string]string // field:value, all must match
	// From and To narrow the view to a time range, usually pulled open in the
	// histogram. Zero means unbounded on that side.
	From time.Time
	To   time.Time
}

func (q Query) matches(entry Entry) bool {
	if !q.From.IsZero() || !q.To.IsZero() {
		// A line without a usable timestamp cannot be placed on the axis. It is
		// dropped from a time-narrowed view rather than guessed into it; the
		// counters still cover every line, so nothing disappears silently.
		if entry.Time.IsZero() {
			return false
		}
		if !q.From.IsZero() && entry.Time.Before(q.From) {
			return false
		}
		// To is exclusive: dragging up to a bucket boundary should not pull in
		// the first entry of the next bucket.
		if !q.To.IsZero() && !entry.Time.Before(q.To) {
			return false
		}
	}
	if len(q.Levels) > 0 {
		found := false
		for _, level := range q.Levels {
			if entry.Level == level {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if q.Text != "" {
		needle := strings.ToLower(q.Text)
		if !strings.Contains(strings.ToLower(entry.Message), needle) &&
			!strings.Contains(strings.ToLower(entry.Raw), needle) {
			return false
		}
	}
	for name, want := range q.Fields {
		if entry.Fields[name] != want {
			return false
		}
	}
	return true
}

// Analyse parses every line once, then answers with the filtered view plus the
// counters. The counters are computed over *all* lines on purpose: a filter
// that hides the errors must not also hide that they exist.
func Analyse(lines []string, query Query, buckets int) Result {
	result := Result{
		Counts:  map[string]int{},
		Formats: map[string]int{},
		Entries: make([]Entry, 0, len(lines)),
	}
	parsed := make([]Entry, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		entry := Parse(line)
		parsed = append(parsed, entry)
		result.Counts[entry.Level]++
		result.Formats[entry.Format]++
	}
	result.Total = len(parsed)

	for _, entry := range parsed {
		if query.matches(entry) {
			result.Entries = append(result.Entries, entry)
		}
	}
	result.Matched = len(result.Entries)
	result.Histogram, result.BucketMS = histogram(result.Entries, buckets)
	result.Fields = topFields(result.Entries)
	return result
}

// histogram spreads the entries over a fixed number of buckets between the
// first and the last timestamp, so the bars line up with the table below.
func histogram(entries []Entry, buckets int) ([]Bucket, int64) {
	if len(entries) == 0 || buckets < 1 {
		return []Bucket{}, 0
	}
	var first, last time.Time
	for _, entry := range entries {
		if entry.Time.IsZero() {
			continue
		}
		if first.IsZero() || entry.Time.Before(first) {
			first = entry.Time
		}
		if entry.Time.After(last) {
			last = entry.Time
		}
	}
	if first.IsZero() {
		return []Bucket{}, 0
	}
	span := last.Sub(first)
	if span <= 0 {
		span = time.Second
	}
	width := span / time.Duration(buckets)
	if width < time.Second {
		width = time.Second
	}
	out := make([]Bucket, 0, buckets+1)
	index := map[int64]int{}
	for _, entry := range entries {
		if entry.Time.IsZero() {
			continue
		}
		slot := entry.Time.Sub(first) / width
		key := int64(slot)
		position, ok := index[key]
		if !ok {
			out = append(out, Bucket{Time: first.Add(time.Duration(key) * width), Counts: map[string]int{}})
			position = len(out) - 1
			index[key] = position
		}
		out[position].Counts[entry.Level]++
		out[position].Total++
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	return out, width.Milliseconds()
}

// topFields is the Kibana sidebar: which fields occur, and what their most
// common values are. It answers "what status codes are in here" without anyone
// writing a query first.
func topFields(entries []Entry) []Field {
	counts := map[string]map[string]int{}
	totals := map[string]int{}
	for _, entry := range entries {
		for name, value := range entry.Fields {
			if value == "" || len(value) > 120 {
				continue
			}
			if counts[name] == nil {
				counts[name] = map[string]int{}
			}
			counts[name][value]++
			totals[name]++
		}
	}
	fields := make([]Field, 0, len(counts))
	for name, values := range counts {
		list := make([]FieldValue, 0, len(values))
		for value, count := range values {
			list = append(list, FieldValue{Value: value, Count: count})
		}
		sort.Slice(list, func(i, j int) bool {
			if list[i].Count != list[j].Count {
				return list[i].Count > list[j].Count
			}
			return list[i].Value < list[j].Value
		})
		if len(list) > 5 {
			list = list[:5]
		}
		fields = append(fields, Field{Name: name, Values: list, Total: totals[name]})
	}
	// Fields with few distinct values are the useful filters, so put the ones
	// that carry the most entries first.
	sort.Slice(fields, func(i, j int) bool {
		if fields[i].Total != fields[j].Total {
			return fields[i].Total > fields[j].Total
		}
		return fields[i].Name < fields[j].Name
	})
	if len(fields) > 12 {
		fields = fields[:12]
	}
	return fields
}
