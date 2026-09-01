package deploy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Record is what a successful deploy actually rolled out. It survives a restart
// of Mission Control, and that is the whole point.
//
// Two tickets needed the same fact and neither could have it. MC-0018: the
// change detection compared the source against the *running* binary, so
// restarting the service to verify a change consumed the deploy - the button
// then had nothing left to do. HF-0104: a rollback needs a target, and nothing
// remembered what the last good state was.
type Record struct {
	Pipeline  string    `json:"pipeline"`
	Version   string    `json:"version"`
	Commit    string    `json:"commit,omitempty"`
	At        time.Time `json:"zeit"`
	RunID     int       `json:"lauf"`
	StartedBy string    `json:"gestartetVon"`
	Tickets   []string  `json:"tickets,omitempty"`
}

// Journal keeps the successful deploys per pipeline, newest first.
//
// It is a file and not a database on purpose: Mission Control has no
// dependencies and is not going to grow one for a handful of rows. The file
// lives outside the repository because it is runtime state, not source.
type Journal struct {
	path    string
	mu      sync.Mutex
	entries map[string][]Record
}

// keepPerPipeline is small deliberately. A rollback target is the previous
// entry; a longer history would invite picking one at random.
const keepPerPipeline = 10

func NewJournal(path string) *Journal {
	j := &Journal{path: path, entries: map[string][]Record{}}
	j.load()
	return j
}

func (j *Journal) load() {
	raw, err := os.ReadFile(j.path)
	if err != nil {
		// No file yet is the normal first start, not an error worth shouting
		// about. A broken file is: it is dropped rather than half-parsed,
		// because a half-read journal would answer "last good version" wrongly.
		return
	}
	var entries map[string][]Record
	if json.Unmarshal(raw, &entries) == nil && entries != nil {
		j.entries = entries
	}
}

// Put records a successful deploy. A failed or cancelled run never gets here -
// the journal answers "what is known to work", not "what ran last".
func (j *Journal) Put(record Record) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	list := append([]Record{record}, j.entries[record.Pipeline]...)
	if len(list) > keepPerPipeline {
		list = list[:keepPerPipeline]
	}
	j.entries[record.Pipeline] = list
	return j.save()
}

// save writes through a temporary file and renames it. A half-written journal
// is worse than none: it would silently claim a version that never shipped.
func (j *Journal) save() error {
	if j.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(j.path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(j.entries, "", "  ")
	if err != nil {
		return err
	}
	tmp := j.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, j.path)
}

// Last is the version a pipeline is known to have shipped.
func (j *Journal) Last(pipeline string) (Record, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	list := j.entries[pipeline]
	if len(list) == 0 {
		return Record{}, false
	}
	return list[0], true
}

// Previous is the rollback target: the state before the current one. Without a
// second entry there is nothing to go back to, and saying so is better than
// offering a button that cannot work.
func (j *Journal) Previous(pipeline string) (Record, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	list := j.entries[pipeline]
	if len(list) < 2 {
		return Record{}, false
	}
	return list[1], true
}

// History is the whole record for one pipeline, newest first.
func (j *Journal) History(pipeline string) []Record {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]Record{}, j.entries[pipeline]...)
}
