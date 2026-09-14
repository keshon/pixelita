// Command img-quant reduces PNG images to a palette, the way pngquant does, and
// writes the result only when it is both smaller and still faithful.
//
// The work itself lives in internal/ops, which is also what the web interface
// calls. This file is the flag parser and the table.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/keshon/pixelita/internal/cli"
	"github.com/keshon/pixelita/internal/ops"
	"github.com/keshon/pixelita/internal/report"
)

func main() {
	opt := ops.DefaultQuant()
	var verbose, asJSON bool
	var showVersion bool
	var jobs int
	var listFile string

	flag.IntVar(&opt.Colors, "colors", opt.Colors, "maximum palette size, 2..256")
	flag.Float64Var(&opt.Dither, "dither", opt.Dither, "Floyd-Steinberg strength, 0 turns it off")
	flag.IntVar(&opt.Effort, "effort", opt.Effort, "1..10, how long to spend refining the palette")
	flag.Float64Var(&opt.MinGain, "min-gain", opt.MinGain,
		"minimum size reduction in percent, below that the original is kept")
	flag.Float64Var(&opt.MinPSNR, "min-psnr", opt.MinPSNR,
		"refuse to write below this fidelity in dB, 0 disables the check")
	flag.BoolVar(&opt.DryRun, "dry-run", false, "measure and report, write nothing")
	flag.BoolVar(&opt.Replace, "replace", false, "overwrite the source file instead of writing next to it")
	flag.StringVar(&opt.Suffix, "suffix", opt.Suffix, "suffix for the output name")
	flag.BoolVar(&verbose, "v", false, "list skipped files too")
	flag.BoolVar(&showVersion, "version", false, "print which build this is and exit")
	flag.BoolVar(&asJSON, "json", false, "emit the report as JSON")
	flag.IntVar(&jobs, "jobs", 0, "parallel workers, 0 means one per CPU core")
	flag.StringVar(&listFile, "from-file", "",
		"read paths from a file, one per line, instead of or in addition to arguments")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "img-quant — quantises PNG to a palette when it pays off\n\n")
		fmt.Fprintf(os.Stderr, "usage: img-quant [flags] <file-or-directory>...\n\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nexamples:\n")
		fmt.Fprintf(os.Stderr, "  img-quant -dry-run ./public/img\n")
		fmt.Fprintf(os.Stderr, "  img-quant -colors 128 -replace ./public/img/site\n")
		fmt.Fprintf(os.Stderr, "  img-quant -json -dry-run ./public/img\n")
	}
	flag.Parse()

	if showVersion {
		cli.Version(os.Stdout, "img-quant")
		return
	}

	files, err := cli.Roots(flag.Args(), listFile, ".png")
	if err != nil {
		if errors.Is(err, cli.ErrNoInput) {
			flag.Usage()
		} else {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		os.Exit(2)
	}

	rep := report.New("img-quant", "written", opt.DryRun)
	items := make([]report.Item, len(files))
	cli.Each(len(files), jobs, func(i int) { items[i] = ops.Quant(files[i], opt) })
	for _, it := range items {
		rep.Add(it)
	}
	os.Exit(rep.Emit(os.Stdout, columns, asJSON, verbose))
}

var columns = []report.Column{
	{Title: "file", Width: 46, Value: func(i report.Item) string { return i.Path }},
	{Title: "before", Width: 10, Right: true, Value: func(i report.Item) string { return report.Size(i.BytesBefore) }},
	{Title: "after", Width: 10, Right: true, Value: func(i report.Item) string { return report.Size(i.BytesAfter) }},
	{Title: "gain", Width: 6, Right: true, Value: func(i report.Item) string {
		return report.Percent(i.GainPercent, i.BytesAfter > 0)
	}},
	{Title: "colors", Width: 6, Right: true, Value: func(i report.Item) string {
		if n, ok := i.Num("colors"); ok {
			return fmt.Sprintf("%.0f", n)
		}
		return ""
	}},
	{Title: "psnr", Width: 8, Right: true, Value: func(i report.Item) string {
		if i.BytesAfter == 0 {
			return ""
		}
		if v, ok := i.Num("psnr"); ok {
			return fmt.Sprintf("%.1f dB", v)
		}
		return "lossless"
	}},
	{Title: "action", Width: 22, Value: func(i report.Item) string { return report.Action(i, "written") }},
}
