package ops

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Destination struct {
	Source string
	Output string
}

type PlanError struct {
	Code   string
	Path   string
	Source string
	Other  string
	Detail string
}

func (e *PlanError) Error() string {
	if e.Code == "output_collision" {
		return fmt.Sprintf("duplicate planned destination %s for %s and %s", e.Path, e.Source, e.Other)
	}
	if e.Code == "output_overlaps_input" {
		return fmt.Sprintf("planned destination %s for %s is another input: %s", e.Path, e.Source, e.Other)
	}
	if e.Other != "" {
		return fmt.Sprintf("%s: %s conflicts with %s", e.Detail, e.Path, e.Other)
	}
	if e.Path == "" {
		return e.Detail
	}
	return fmt.Sprintf("%s: %s", e.Detail, e.Path)
}

// PreflightDestinations validates a complete batch before any encoder writes.
func PreflightDestinations(planned []Destination, overwrite, replace bool) error {
	seen := map[string]Destination{}
	sources := map[string]string{}
	for _, d := range planned {
		if d.Source == "" {
			continue
		}
		key, err := canonicalPath(d.Source)
		if err != nil {
			return &PlanError{Code: "invalid_source", Path: d.Source, Detail: err.Error()}
		}
		sources[key] = d.Source
	}
	for _, d := range planned {
		if d.Output == "" {
			continue
		}
		key, err := canonicalPath(d.Output)
		if err != nil {
			return &PlanError{Code: "invalid_destination", Path: d.Output, Detail: err.Error()}
		}
		if prior, ok := seen[key]; ok {
			return &PlanError{Code: "output_collision", Path: d.Output, Source: prior.Source, Other: d.Source, Detail: "duplicate planned destination"}
		}
		seen[key] = d

		sourceKey, err := canonicalPath(d.Source)
		if err == nil && sourceKey == key {
			if !replace {
				return &PlanError{Code: "source_replacement_requires_permission", Path: d.Output, Detail: "destination replaces its source; use explicit replacement permission"}
			}
			continue
		}
		if source, ok := sources[key]; ok {
			return &PlanError{Code: "output_overlaps_input", Path: d.Output, Source: d.Source, Other: source, Detail: "planned destination is another input"}
		}
		if outInfo, err := os.Stat(d.Output); err == nil {
			for _, source := range sources {
				sourceInfo, e := os.Stat(source)
				if e == nil && os.SameFile(outInfo, sourceInfo) {
					return &PlanError{Code: "output_overlaps_input", Path: d.Output, Source: d.Source, Other: source, Detail: "destination aliases an input"}
				}
			}
		}
		if _, err := os.Stat(d.Output); err == nil && !overwrite {
			return &PlanError{Code: "destination_exists", Path: d.Output, Detail: "destination exists; use explicit overwrite permission"}
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return &PlanError{Code: "destination_unavailable", Path: d.Output, Detail: err.Error()}
		}
	}
	return nil
}

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	// Resolve existing ancestors as output directories may be symlinked while
	// the destination itself has not been created yet.
	resolved, err := resolveAncestors(abs)
	if err != nil {
		return "", err
	}
	abs = resolved
	if runtime.GOOS == "windows" {
		abs = strings.ToLower(abs)
	}
	return abs, nil
}

// AtomicWrite writes and verifies a same-directory temporary before replacing
// the final path. Existing destinations are preserved on every failure path.
func AtomicWrite(path string, data []byte, overwrite bool, verify func([]byte) error) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tmp)
		}
	}()
	if err := f.Chmod(0o644); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	written, err := os.ReadFile(tmp)
	if err != nil {
		return err
	}
	if !bytes.Equal(written, data) {
		return fmt.Errorf("written bytes differ from candidate")
	}
	if verify != nil {
		if err := verify(written); err != nil {
			return fmt.Errorf("verification failed: %w", err)
		}
	}

	// Link publishes without replacing a destination created after preflight.
	if !overwrite {
		if err := os.Link(tmp, path); err != nil {
			return err
		}
		return nil
	}
	// Rename replaces in one filesystem operation; never move the original away
	// first, which leaves a missing destination if the process is interrupted.
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	removeTemp = false
	return nil
}

func resolveAncestors(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return path, nil
	}
	resolved, err = resolveAncestors(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(path)), nil
}
