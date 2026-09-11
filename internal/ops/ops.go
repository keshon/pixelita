// Package ops holds what the tools actually do to a file.
//
// Every command under cmd/ is a flag parser wrapped around one of these, and so
// is the web interface. Keeping the work here rather than in the commands is
// what turns "the interface does the same thing as the command line" from a
// promise into a property of the code: there is one implementation to be right
// or wrong about, not two that can drift.
package ops

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/keshon/pixelita/internal/report"
)

// fail marks an item as failed, keeping the reason short for a table cell and
// the full error for whoever wants to read it.
func fail(item report.Item, err error, reason string) report.Item {
	item.Status = report.StatusFailed
	item.Reason = reason
	item.Error = err.Error()
	return item
}

func gain(before, after int64) float64 {
	if before <= 0 {
		return 0
	}
	return (1 - float64(after)/float64(before)) * 100
}

// sibling builds an output path next to the source, with a suffix and possibly
// a different extension.
func sibling(path, suffix, ext string) string {
	return strings.TrimSuffix(path, filepath.Ext(path)) + suffix + ext
}

func percent(v float64) string { return fmt.Sprintf("%.0f%%", v) }
