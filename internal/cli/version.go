package cli

import (
	"fmt"
	"io"
	"runtime/debug"
)

// Version reports which build this is.
//
// It exists because of a failure that cost a full round trip and could not be
// detected from inside: a stale copy of the binaries sat earlier in PATH than
// the fresh ones, `img-scan -h` succeeded, and the reader concluded the tools
// were present and current. They were present. A flag that answers "is this the
// build I think it is" is the difference between finding that out in one
// command and finding it out when a flag that should exist does not.
//
// The revision comes from the module's own build stamp, which `go build`
// records without anything being passed to it, so there is nothing to remember
// at release time and nothing to go stale on its own.
func Version(w io.Writer, tool string) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		fmt.Fprintf(w, "%s (build information unavailable)\n", tool)
		return
	}
	var revision, when string
	dirty := false
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.time":
			when = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if revision == "" {
		fmt.Fprintf(w, "%s %s (built outside a repository)\n", tool, info.Main.Version)
		return
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	mark := ""
	if dirty {
		mark = " +uncommitted changes"
	}
	fmt.Fprintf(w, "%s %s%s %s\n", tool, revision, mark, when)
}
