// Command pixelita is the single entry point agents look for first.
//
// It forwards to the img-* tool named as its first argument, so there is one
// name to discover and seven tools behind it: `pixelita scan …` runs img-scan.
// Direct img-* binaries keep working; this adds no new implementation, only
// discovery.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

var tools = []string{"scan", "quant", "webp", "jpeg", "resize", "diff", "look"}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	name := os.Args[1]
	if name == "-version" || name == "-h" || name == "help" || name == "--help" {
		usage()
		return
	}
	known := false
	for _, t := range tools {
		if name == t || name == "img-"+t {
			name = "img-" + trimPrefix(t)
			known = true
			break
		}
	}
	if !known {
		fmt.Fprintf(os.Stderr, "unknown tool %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	bin := resolve(name)
	cmd := exec.Command(bin, os.Args[2:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			os.Exit(exit.ExitCode())
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func trimPrefix(t string) string {
	if len(t) > 4 && t[:4] == "img-" {
		return t[4:]
	}
	return t
}

func resolve(name string) string {
	// Same directory first: `go build -o bin/ ./cmd/...` puts pixelita next
	// to img-*, so a checkout just works without PATH edits.
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), exeName(name))
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return name
}

func exeName(name string) string {
	if filepath.Ext(name) == "" && os.PathSeparator == '\\' {
		return name + ".exe"
	}
	return name
}

func usage() {
	fmt.Fprintf(os.Stderr, "pixelita — one entry point for the seven img-* tools\n\n")
	fmt.Fprintf(os.Stderr, "usage: pixelita <tool> [flags] <paths>...\n\n")
	fmt.Fprintf(os.Stderr, "tools:\n")
	fmt.Fprintf(os.Stderr, "  scan    what is here and what can be won\n")
	fmt.Fprintf(os.Stderr, "  quant   PNG to palette when it pays off\n")
	fmt.Fprintf(os.Stderr, "  webp    PNG/JPEG to WebP when it pays off\n")
	fmt.Fprintf(os.Stderr, "  jpeg    shrink JPEG losslessly\n")
	fmt.Fprintf(os.Stderr, "  resize  scale images in linear light\n")
	fmt.Fprintf(os.Stderr, "  diff    measure how far one image is from another\n")
	fmt.Fprintf(os.Stderr, "  look    make images visible / read pixel values\n\n")
	fmt.Fprintf(os.Stderr, "examples:\n")
	fmt.Fprintf(os.Stderr, "  pixelita scan ./public/img\n")
	fmt.Fprintf(os.Stderr, "  pixelita diff before.png after.png\n")
	fmt.Fprintf(os.Stderr, "img-* binaries work directly too.\n")
}
