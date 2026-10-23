// Package result defines typed operation outcomes independently of CLI rendering.
package result

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

// Version is the schema version of the JSON output. Consumers should check it.
const Version = "2"

type Status string

const (
	StatusDone    Status = "done"    // the file was written
	StatusWould   Status = "would"   // dry run: it would have been written
	StatusSkipped Status = "skipped" // deliberately left alone
	StatusFailed  Status = "failed"
)

// NextAction is a shell-independent recovery step. Command and Args are kept
// separate so callers never need to parse or quote a command string.
type NextAction struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type BuildInfo struct {
	Revision      string `json:"revision,omitempty"`
	Time          string `json:"time,omitempty"`
	Modified      bool   `json:"modified"`
	ModuleVersion string `json:"moduleVersion,omitempty"`
	Available     bool   `json:"available"`
}

// Item is one file's outcome. Numbers common to every tool are named fields;
// anything tool-specific goes in Metrics, so the schema does not have to grow a
// column every time a tool learns something new.
type CandidateDecision struct {
	Metrics      map[string]any `json:"metrics,omitempty"`
	Verification *Verification  `json:"verification,omitempty"`
	Strategy     string         `json:"strategy"`
	Status       Status         `json:"status"`
	Code         string         `json:"code,omitempty"`
	Bytes        int64          `json:"bytes"`
}
type Verification struct {
	Status string `json:"status"`
	Method string `json:"method"`
}
type Item struct {
	Written      bool                `json:"written,omitempty"`
	Verification *Verification       `json:"verification,omitempty"`
	Candidates   []CandidateDecision `json:"candidates,omitempty"`
	Path         string              `json:"path"`
	Output       string              `json:"output,omitempty"`
	Status       Status              `json:"status"`
	Code         string              `json:"code,omitempty"`
	Reason       string              `json:"reason,omitempty"`
	Hint         string              `json:"hint,omitempty"`
	NextAction   *NextAction         `json:"nextAction,omitempty"`
	BytesBefore  int64               `json:"bytesBefore,omitempty"`
	BytesAfter   int64               `json:"bytesAfter,omitempty"`
	GainPercent  float64             `json:"gainPercent,omitempty"`
	Metrics      map[string]any      `json:"metrics,omitempty"`
	Error        string              `json:"error,omitempty"`
}

func (i Item) Changed() bool { return i.Status == StatusDone || i.Status == StatusWould }

// Num reads a numeric metric. Metrics are a loose map so the schema does not
// have to grow a field every time a tool learns to measure something.
func (i Item) Num(key string) (float64, bool) {
	switch v := i.Metrics[key].(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return f, true
		}
	}
	return 0, false
}

// Str reads a string metric.
func (i Item) Str(key string) string {
	s, _ := i.Metrics[key].(string)
	return s
}

// Ints reads a []int metric, tolerating the []any a JSON round-trip produces.
// Agents parse -json output after it has been decoded once already, so a strict
// []int assertion would silently drop levels and atWidth figures.
func (i Item) Ints(key string) ([]int, bool) {
	switch v := i.Metrics[key].(type) {
	case []int:
		return v, true
	case []int64:
		out := make([]int, len(v))
		for j, n := range v {
			out[j] = int(n)
		}
		return out, true
	case []any:
		out := make([]int, 0, len(v))
		for _, e := range v {
			f, ok := numAny(e)
			if !ok {
				return nil, false
			}
			out = append(out, int(f))
		}
		return out, true
	}
	return nil, false
}

func numAny(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

type Summary struct {
	Files       int     `json:"files"`
	Changed     int     `json:"changed"`
	Skipped     int     `json:"skipped"`
	Failed      int     `json:"failed"`
	BytesBefore int64   `json:"bytesBefore"`
	BytesAfter  int64   `json:"bytesAfter"`
	GainPercent float64 `json:"gainPercent"`
}

type Report struct {
	Tool         string         `json:"tool"`
	Schema       string         `json:"schema"`
	DryRun       bool           `json:"dryRun"`
	Items        []Item         `json:"items"`
	Summary      Summary        `json:"summary"`
	Notes        []string       `json:"notes,omitempty"`
	Totals       map[string]any `json:"totals,omitempty"`
	Capabilities any            `json:"capabilities,omitempty"`
	Build        *BuildInfo     `json:"build,omitempty"`
	NextAction   *NextAction    `json:"nextAction,omitempty"`

	// Verb is how this tool describes a finished item, as a past participle:
	// "written", "converted", "resized". A dry run turns it into "would be
	// written", which is why the participle rather than the infinitive.
	// Table output only.
	Verb string `json:"-"`
}

func New(tool, verb string, dryRun bool) *Report {
	return &Report{Tool: tool, Schema: Version, Verb: verb, DryRun: dryRun}
}

func (r *Report) Add(item Item) { r.Items = append(r.Items, item) }

func (r *Report) Note(format string, args ...any) {
	r.Notes = append(r.Notes, fmt.Sprintf(format, args...))
}

// Finish totals up the items. Only changed files count towards the byte totals:
// a skipped file keeps its original size, so counting it would dilute the
// saving into meaninglessness.
func (r *Report) Finish() {
	if r.Items == nil {
		r.Items = []Item{} // an empty run is an empty list, not a null
	}
	s := Summary{Files: len(r.Items)}
	for _, it := range r.Items {
		switch {
		case it.Status == StatusFailed:
			s.Failed++
		case it.Changed():
			s.Changed++
			s.BytesBefore += it.BytesBefore
			s.BytesAfter += it.BytesAfter
		default:
			s.Skipped++
		}
	}
	for i := range r.Items {
		if r.Items[i].Code != "" {
			continue
		}
		switch r.Items[i].Status {
		case StatusFailed:
			r.Items[i].Code = "operation_failed"
		case StatusSkipped:
			r.Items[i].Code = "not_selected"
		}
	}
	if s.BytesBefore > 0 {
		s.GainPercent = (1 - float64(s.BytesAfter)/float64(s.BytesBefore)) * 100
	}
	r.Summary = s
	if r.Totals == nil {
		r.Totals = map[string]any{}
	}
	inputs := map[string]int64{}
	written := map[string]int64{}
	predicted := map[string]int64{}
	for _, it := range r.Items {
		if it.Path != "" {
			key, _ := filepath.Abs(it.Path)
			if it.BytesBefore > inputs[key] {
				inputs[key] = it.BytesBefore
			} else if _, ok := inputs[key]; !ok {
				inputs[key] = 0
			}
		}
		if it.Written && it.Output != "" {
			written[it.Output] = it.BytesAfter
		}
		if it.Status == StatusWould && it.Output != "" {
			predicted[it.Output] = it.BytesAfter
		}
	}
	var inventory, actual, candidate int64
	known := 0
	for _, n := range inputs {
		inventory += n
		if n > 0 {
			known++
		}
	}
	for _, n := range written {
		actual += n
	}
	for _, n := range predicted {
		candidate += n
	}
	r.Totals["inputs"] = len(inputs)
	r.Totals["inventoryBytes"] = inventory
	r.Totals["inventoryKnownInputs"] = known
	r.Totals["outcomes"] = len(r.Items)
	r.Totals["writtenOutputs"] = len(written)
	r.Totals["writtenBytes"] = actual
	r.Totals["predictedOutputs"] = len(predicted)
	r.Totals["predictedBytes"] = candidate
}
