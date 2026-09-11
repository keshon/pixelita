// Package report is the shared output contract for every tool in this repo.
//
// The tools are meant to be driven by a person at a terminal, by a web front
// end, and by an agent, and only the first of those can read a formatted table.
// So every tool builds the same structure and renders it either as a table or
// as JSON, and neither rendering knows anything the other does not.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// Version is the schema version of the JSON output. Consumers should check it.
const Version = "1"

type Status string

const (
	StatusDone    Status = "done"    // the file was written
	StatusWould   Status = "would"   // dry run: it would have been written
	StatusSkipped Status = "skipped" // deliberately left alone
	StatusFailed  Status = "failed"
)

// Item is one file's outcome. Numbers common to every tool are named fields;
// anything tool-specific goes in Metrics, so the schema does not have to grow a
// column every time a tool learns something new.
type Item struct {
	Path        string         `json:"path"`
	Output      string         `json:"output,omitempty"`
	Status      Status         `json:"status"`
	Reason      string         `json:"reason,omitempty"`
	BytesBefore int64          `json:"bytesBefore,omitempty"`
	BytesAfter  int64          `json:"bytesAfter,omitempty"`
	GainPercent float64        `json:"gainPercent,omitempty"`
	Metrics     map[string]any `json:"metrics,omitempty"`
	Error       string         `json:"error,omitempty"`
}

func (i Item) Changed() bool { return i.Status == StatusDone || i.Status == StatusWould }

// Action renders an outcome for a table cell: what happened, and why if the
// tool decided not to act.
func Action(i Item, verb string) string {
	s := string(i.Status)
	switch i.Status {
	case StatusDone:
		s = verb
	case StatusWould:
		s = "would be " + verb
	}
	if i.Reason != "" {
		return s + ": " + i.Reason
	}
	return s
}

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
	case int64:
		return float64(v), true
	}
	return 0, false
}

// Str reads a string metric.
func (i Item) Str(key string) string {
	s, _ := i.Metrics[key].(string)
	return s
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
	Tool    string   `json:"tool"`
	Schema  string   `json:"schema"`
	DryRun  bool     `json:"dryRun"`
	Items   []Item   `json:"items"`
	Summary Summary  `json:"summary"`
	Notes   []string `json:"notes,omitempty"`

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
	if s.BytesBefore > 0 {
		s.GainPercent = (1 - float64(s.BytesAfter)/float64(s.BytesBefore)) * 100
	}
	r.Summary = s
}

// WriteJSON emits the report as one JSON object. Paths use forward slashes so
// the output is the same on every platform and reads sanely in a browser.
func (r *Report) WriteJSON(w io.Writer) error {
	out := *r
	out.Items = make([]Item, len(r.Items))
	for i, it := range r.Items {
		it.Path = filepath.ToSlash(it.Path)
		it.Output = filepath.ToSlash(it.Output)
		out.Items[i] = it
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// Column describes one table column.
type Column struct {
	Title string
	Width int
	Right bool
	Value func(Item) string
}

// WriteTable renders the report for a human. Unchanged files are hidden unless
// verbose, because the interesting output is what the tool decided to do.
func (r *Report) WriteTable(w io.Writer, cols []Column, verbose bool) {
	line := func(cells []string) {
		var b strings.Builder
		for i, c := range cols {
			if i > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(pad(cells[i], c.Width, c.Right))
		}
		fmt.Fprintln(w, strings.TrimRight(b.String(), " "))
	}

	width := len(cols) - 1
	titles := make([]string, len(cols))
	for i, c := range cols {
		titles[i] = c.Title
		width += c.Width
	}
	line(titles)
	fmt.Fprintln(w, strings.Repeat("-", width))

	for _, it := range r.Items {
		if !it.Changed() && it.Error == "" && !verbose {
			continue
		}
		cells := make([]string, len(cols))
		for i, c := range cols {
			cells[i] = c.Value(it)
		}
		line(cells)
		if it.Error != "" {
			fmt.Fprintf(w, "%s%s\n", strings.Repeat(" ", cols[0].Width+1), it.Error)
		}
	}

	fmt.Fprintln(w, strings.Repeat("-", width))
	verb := r.Verb
	if r.DryRun {
		verb = "would be " + verb
	}
	fmt.Fprintf(w, "%s: %d, skipped: %d, failed: %d\n",
		verb, r.Summary.Changed, r.Summary.Skipped, r.Summary.Failed)
	if r.Summary.BytesBefore > 0 {
		fmt.Fprintf(w, "size: %s -> %s, saved %s (%.0f%%)\n",
			Size(r.Summary.BytesBefore), Size(r.Summary.BytesAfter),
			Size(r.Summary.BytesBefore-r.Summary.BytesAfter), r.Summary.GainPercent)
	}
	for _, n := range r.Notes {
		fmt.Fprintln(w, n)
	}
}

// Emit writes the report in whichever form was asked for and returns the
// process exit code.
func (r *Report) Emit(w io.Writer, cols []Column, asJSON, verbose bool) int {
	r.Finish()
	if asJSON {
		if err := r.WriteJSON(w); err != nil {
			fmt.Fprintln(w, err)
			return 1
		}
	} else {
		r.WriteTable(w, cols, verbose)
	}
	if r.Summary.Failed > 0 {
		return 1
	}
	return 0
}

func pad(s string, width int, right bool) string {
	if n := runeLen(s); n > width {
		return truncate(s, width)
	} else if right {
		return strings.Repeat(" ", width-n) + s
	} else {
		return s + strings.Repeat(" ", width-n)
	}
}

// truncate keeps the tail of a path, which is the part that identifies it.
func truncate(s string, width int) string {
	r := []rune(s)
	if width <= 1 {
		return string(r[:max(width, 0)])
	}
	return "…" + string(r[len(r)-width+1:])
}

func runeLen(s string) int { return len([]rune(s)) }

// Size formats a byte count for a report column.
func Size(n int64) string {
	if n == 0 {
		return "-"
	}
	neg := ""
	if n < 0 {
		neg, n = "-", -n
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%s%d B", neg, n)
	}
	div, exp := int64(unit), 0
	for size := n / unit; size >= unit; size /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%s%.1f %cB", neg, float64(n)/float64(div), "KMG"[exp])
}

// Percent formats a gain for a report column, blank when there is nothing to say.
func Percent(v float64, known bool) string {
	if !known {
		return ""
	}
	return fmt.Sprintf("%.0f%%", v)
}
