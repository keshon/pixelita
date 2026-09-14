// Command img-jpeg makes JPEG files smaller without touching a single pixel.
//
// It refits the Huffman tables to the image in front of it. Most encoders ship
// the example tables from the standard's annex instead, and swapping in fitted
// ones costs nothing: the coefficients are copied across untouched, so the
// decoded image is identical to the last bit. There is no quality flag here
// because nothing is lost.
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
	opt := ops.DefaultJPEG()
	var verbose, asJSON bool
	var showVersion bool
	var jobs int
	var listFile string

	flag.Float64Var(&opt.MinGain, "min-gain", opt.MinGain,
		"minimum size reduction in percent, below that the original is kept")
	flag.BoolVar(&opt.DryRun, "dry-run", false, "measure and report, write nothing")
	flag.BoolVar(&opt.Replace, "replace", false, "overwrite the source file instead of writing next to it")
	flag.StringVar(&opt.Suffix, "suffix", opt.Suffix, "suffix for the output name")
	flag.BoolVar(&verbose, "v", false, "list skipped files too")
	flag.BoolVar(&showVersion, "version", false, "print which build this is and exit")
	flag.BoolVar(&asJSON, "json", false, "emit the report as JSON")
	flag.IntVar(&jobs, "jobs", 0, "parallel workers, 0 means one per CPU core")
	flag.StringVar(&listFile, "from-file", "", "read paths from a file, one per line")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "img-jpeg — shrinks JPEG files without changing the picture\n\n")
		fmt.Fprintf(os.Stderr, "usage: img-jpeg [flags] <file-or-directory>...\n\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nexamples:\n")
		fmt.Fprintf(os.Stderr, "  img-jpeg -dry-run ./public/img\n")
		fmt.Fprintf(os.Stderr, "  img-jpeg -replace ./public/img/photos\n")
	}
	flag.Parse()

	if showVersion {
		cli.Version(os.Stdout, "img-jpeg")
		return
	}

	files, err := cli.Roots(flag.Args(), listFile, ".jpg", ".jpeg")
	if err != nil {
		if errors.Is(err, cli.ErrNoInput) {
			flag.Usage()
		} else {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		os.Exit(2)
	}

	rep := report.New("img-jpeg", "written", opt.DryRun)
	items := make([]report.Item, len(files))
	cli.Each(len(files), jobs, func(i int) { items[i] = ops.JPEG(files[i], opt) })
	for _, it := range items {
		rep.Add(it)
	}
	os.Exit(rep.Emit(os.Stdout, columns, asJSON, verbose))
}

var columns = []report.Column{
	{Title: "file", Width: 52, Value: func(i report.Item) string { return i.Path }},
	{Title: "before", Width: 10, Right: true, Value: func(i report.Item) string { return report.Size(i.BytesBefore) }},
	{Title: "after", Width: 10, Right: true, Value: func(i report.Item) string { return report.Size(i.BytesAfter) }},
	{Title: "gain", Width: 6, Right: true, Value: func(i report.Item) string {
		return report.Percent(i.GainPercent, i.BytesAfter > 0)
	}},
	{Title: "quality", Width: 9, Value: func(i report.Item) string {
		if i.BytesAfter > 0 {
			return "identical"
		}
		return ""
	}},
	{Title: "action", Width: 20, Value: func(i report.Item) string { return report.Action(i, "written") }},
}
