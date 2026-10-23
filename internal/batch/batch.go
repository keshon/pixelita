// Package batch discovers image inputs and schedules bounded file work.
package batch

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
)

var ErrNoInput = errors.New("no input paths given")

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
// memory-heavy, so jobs of zero uses at most two workers.
func Each(n, jobs int, fn func(i int)) {
	workers := jobs
	if workers <= 0 {
		workers = min(runtime.NumCPU(), 2)
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
