// Package cli holds the parts every tool in this repo needs: finding the files
// to work on, spreading the work across cores, and printing sizes.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/keshon/pixelita/internal/batch"
	"github.com/keshon/pixelita/internal/report"
)

// ErrNoInput means the command was run with nothing to work on.
var ErrNoInput = batch.ErrNoInput

// ParseFlags accepts flags before or after positional arguments. The standard
// flag package stops at the first positional; image commands should not turn a
// later flag into a filesystem path.
func ParseFlags(fs *flag.FlagSet, args []string) error {
	ordered, err := interspersed(fs, args)
	if err != nil {
		return err
	}
	return fs.Parse(ordered)
}

// ConfigureDefaultFlags makes the package-level flag set recoverable. Call it
// before defining flags in a command main.
func ConfigureDefaultFlags() {
	flag.CommandLine.Init(os.Args[0], flag.ContinueOnError)
}

// ParseDefaultFlags parses the configured package-level set and exits with a
// schema-2 argument error when parsing fails.
func ParseDefaultFlags(tool string) {
	flag.CommandLine.SetOutput(io.Discard)
	err := ParseFlags(flag.CommandLine, os.Args[1:])
	flag.CommandLine.SetOutput(os.Stderr)
	if err != nil {
		w := io.Writer(os.Stderr)
		if WantsJSON(os.Args[1:]) {
			w = os.Stdout
		}
		WriteArgumentError(w, tool, "invalid_arguments", err, os.Args[1:], nil)
		os.Exit(2)
	}
}

func ExitArgument(tool, code string, err error, next *report.NextAction) {
	w := io.Writer(os.Stderr)
	if WantsJSON(os.Args[1:]) {
		w = os.Stdout
	}
	WriteArgumentError(w, tool, code, err, os.Args[1:], next)
	os.Exit(2)
}

func ExitInputArgument(tool string, err error) {
	code := "invalid_input"
	if errors.Is(err, ErrNoInput) {
		code = "no_input"
	} else if strings.Contains(err.Error(), "unsupported input") {
		code = "unsupported_input"
	}
	ExitArgument(tool, code, err, nil)
}

func interspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var flags, paths []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			paths = append(paths, args[i+1:]...)
			break
		}
		if len(a) < 2 || a[0] != '-' || a == "-" {
			paths = append(paths, a)
			continue
		}
		name := strings.TrimLeft(a, "-")
		hasValue := strings.Contains(name, "=")
		if hasValue {
			name = strings.SplitN(name, "=", 2)[0]
		}
		f := fs.Lookup(name)
		flags = append(flags, a)
		if f == nil || hasValue {
			continue
		}
		if b, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && b.IsBoolFlag() {
			continue
		}
		if i+1 >= len(args) {
			return nil, fmt.Errorf("flag needs an argument: -%s", name)
		}
		i++
		flags = append(flags, args[i])
	}
	return append(flags, paths...), nil
}

// WantsJSON is intentionally tolerant: it is used before parsing so malformed
// invocations can still return a structured error.
func WantsJSON(args []string) bool {
	for _, a := range args {
		if a == "-json" || a == "--json" || a == "-json=true" || a == "--json=true" {
			return true
		}
	}
	return false
}

// WriteArgumentError emits the shared schema for bad arguments in JSON mode.
func WriteArgumentError(w io.Writer, tool, code string, err error, args []string, next *report.NextAction) {
	if WantsJSON(args) {
		r := report.New(tool, "", false)
		r.Add(report.Item{Status: report.StatusFailed, Code: code, Reason: "invalid arguments", Error: err.Error(), NextAction: next})
		r.Finish()
		_ = report.WriteJSON(r, w)
		return
	}
	fmt.Fprintln(w, "error:", err)
}

// Roots turns command-line arguments plus an optional list file into the files
// to work on. An empty result is not an error: a tool run over a directory with
// nothing matching should report exactly that, and a JSON consumer should get a
// valid empty report rather than a failure.
func Roots(args []string, listFile string, exts ...string) ([]string, error) {
	return batch.Roots(args, listFile, exts...)
}

// Fail marks an item as failed, keeping the reason short for the table and the
// full error for whoever wants to read it.
func Fail(item report.Item, err error, reason string) report.Item {
	item.Status = report.StatusFailed
	item.Reason = reason
	item.Error = err.Error()
	return item
}

// Collect walks the given paths and returns every file with one of the given
// extensions, once, in a stable order.
func Collect(roots []string, exts ...string) ([]string, error) { return batch.Collect(roots, exts...) }
func ReadList(name string) ([]string, error)                   { return batch.ReadList(name) }
func Each(n, jobs int, fn func(int))                           { batch.Each(n, jobs, fn) }

// HumanSize formats a byte count for a report column.
func HumanSize(n int64) string {
	if n == 0 {
		return "-"
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for size := n / unit; size >= unit; size /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMG"[exp])
}
