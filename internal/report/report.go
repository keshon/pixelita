// Package report renders typed operation results as tables or schema-2 JSON.
//
// The tools are meant to be driven by a person at a terminal or by an agent.
// Every tool builds the same structure and renders it either as a table or as
// JSON, and neither rendering knows anything the other does not.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/keshon/pixelita/internal/result"
)

const Version = result.Version

type Status = result.Status

const (
	StatusDone    = result.StatusDone
	StatusWould   = result.StatusWould
	StatusSkipped = result.StatusSkipped
	StatusFailed  = result.StatusFailed
)

type NextAction = result.NextAction
type BuildInfo = result.BuildInfo
type CandidateDecision = result.CandidateDecision
type Verification = result.Verification
type Item = result.Item
type Summary = result.Summary
type Report = result.Report

func New(tool, verb string, dry bool) *Report { return result.New(tool, verb, dry) }

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

// WriteJSON emits the report as one JSON object. Paths use forward slashes so
// the output is the same on every platform and reads sanely in a browser.
func WriteJSON(r *Report, w io.Writer) error {
	out := *r
	out.NextAction = normalizedAction(r.NextAction)
	out.Items = make([]Item, len(r.Items))
	for i, it := range r.Items {
		it.Path = filepath.ToSlash(it.Path)
		it.Output = filepath.ToSlash(it.Output)
		it.NextAction = normalizedAction(it.NextAction)
		out.Items[i] = it
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func normalizedAction(action *NextAction) *NextAction {
	if action == nil {
		return nil
	}
	out := &NextAction{Command: action.Command, Args: make([]string, len(action.Args))}
	for i, arg := range action.Args {
		out.Args[i] = filepath.ToSlash(arg)
	}
	return out
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
func WriteTable(r *Report, w io.Writer, cols []Column, verbose bool) {
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
	if r.Totals != nil {
		fmt.Fprintf(w, "inputs: %v, outcomes: %v; known input bytes: %v\n", r.Totals["inputs"], r.Totals["outcomes"], r.Totals["inventoryBytes"])
		if n, ok := r.Totals["writtenOutputs"].(int); ok && n > 0 {
			fmt.Fprintf(w, "written: %d outputs, %v bytes\n", n, r.Totals["writtenBytes"])
		}
		if n, ok := r.Totals["predictedOutputs"].(int); ok && n > 0 {
			fmt.Fprintf(w, "predicted: %d outputs, %v bytes\n", n, r.Totals["predictedBytes"])
		}
	}
	for _, n := range r.Notes {
		fmt.Fprintln(w, n)
	}
}

// Emit writes the report in whichever form was asked for and returns the
// process exit code.
func Emit(r *Report, w io.Writer, cols []Column, asJSON, verbose bool) int {
	r.Finish()
	if asJSON {
		if err := WriteJSON(r, w); err != nil {
			fmt.Fprintln(w, err)
			return 1
		}
	} else {
		WriteTable(r, w, cols, verbose)
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
