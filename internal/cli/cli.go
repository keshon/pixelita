// Package cli holds the parts every tool in this repo needs: finding the files
// to work on, spreading the work across cores, and printing sizes.
package cli

import (
	"errors"
	"fmt"
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
