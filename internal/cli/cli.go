// Package cli holds the parts every tool in this repo needs: finding the files
// to work on, spreading the work across cores, and printing sizes.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/keshon/pixelita/internal/report"
)

// ErrNoInput means the command was run with nothing to work on.
var ErrNoInput = errors.New("no input paths given")

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
		_ = r.WriteJSON(w)
		return
	}
	fmt.Fprintln(w, "error:", err)
}

// Roots turns command-line arguments plus an optional list file into the files
// to work on. An empty result is not an error: a tool run over a directory with
// nothing matching should report exactly that, and a JSON consumer should get a
// valid empty report rather than a failure.
func Roots(args []string, listFile string, exts ...string) ([]string, error) {
	roots := args
	if listFile != "" {
		fromFile, err := ReadList(listFile)
		if err != nil {
			return nil, err
		}
		roots = append(roots, fromFile...)
	}
	if len(roots) == 0 {
		return nil, ErrNoInput
	}
	return Collect(roots, exts...)
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
func Collect(roots []string, exts ...string) ([]string, error) {
	seen := map[string]bool{}
	var files []string

	wanted := func(path string) bool {
		ext := strings.ToLower(filepath.Ext(path))
		for _, e := range exts {
			if ext == e {
				return true
			}
		}
		return false
	}

	add := func(path string) {
		if !wanted(path) {
			return
		}
		abs, err := filepath.Abs(path)
		if err != nil || seen[abs] {
			return
		}
		seen[abs] = true
		files = append(files, path)
	}

	for _, root := range roots {
		info, err := os.Stat(root)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			if !wanted(root) {
				return nil, fmt.Errorf("unsupported input %q: expected %s", root, strings.Join(exts, ", "))
			}
			add(root)
			continue
		}
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				add(path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	sort.Strings(files)
	return files, nil
}

// ReadList reads newline-separated paths, ignoring blanks and # comments.
func ReadList(name string) ([]string, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		paths = append(paths, line)
	}
	return paths, nil
}

// Each runs fn for every index below n, on jobs goroutines. The work here is
// CPU-bound, so jobs of zero means one per core.
func Each(n, jobs int, fn func(i int)) {
	workers := jobs
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	workers = min(max(workers, 1), max(n, 1))

	queue := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range queue {
				fn(i)
			}
		}()
	}
	for i := 0; i < n; i++ {
		queue <- i
	}
	close(queue)
	wg.Wait()
}

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
