// Command img-webp converts PNG and JPEG images to WebP.
//
// It is built around one idea: a conversion that makes a file bigger, or barely
// smaller, is not worth doing. Every candidate is encoded, measured against the
// original and only written when it actually pays off.
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
	opt := ops.DefaultWebP()
	var verbose, asJSON bool
	var showVersion bool
	var jobs int
	var listFile string

	flag.IntVar(&opt.Quality, "quality", opt.Quality, "lossy quality, 1..100")
	flag.StringVar(&opt.Mode, "mode", opt.Mode, "encoding mode: lossy, lossless or near-lossless")
	flag.BoolVar(&opt.SkipPalette, "skip-palette", opt.SkipPalette,
		"skip palette PNGs: quantised images rarely gain and often grow")
	flag.Float64Var(&opt.MinGain, "min-gain", opt.MinGain,
		"minimum size reduction in percent, below that the original is kept")
	flag.BoolVar(&opt.DryRun, "dry-run", false, "measure and report, write nothing")
	flag.BoolVar(&opt.KeepOriginal, "keep-original", opt.KeepOriginal,
		"keep the source file next to the .webp")
	flag.BoolVar(&verbose, "v", false, "list skipped files too")
	flag.StringVar(&opt.OutDir, "out-dir", "",
		"write results here instead of next to the source")
	flag.BoolVar(&showVersion, "version", false, "print which build this is and exit")
	flag.BoolVar(&asJSON, "json", false, "emit the report as JSON")
	flag.IntVar(&jobs, "jobs", 0, "parallel encoders, 0 means one per CPU core")
	flag.StringVar(&listFile, "from-file", "",
		"read paths from a file, one per line, instead of or in addition to arguments")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "img-webp — converts PNG and JPEG to WebP when it pays off\n\n")
		fmt.Fprintf(os.Stderr, "usage: img-webp [flags] <file-or-directory>...\n\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nexamples:\n")
		fmt.Fprintf(os.Stderr, "  img-webp -dry-run ./public/img\n")
		fmt.Fprintf(os.Stderr, "  img-webp -quality 90 ./public/img/site/download\n")
		fmt.Fprintf(os.Stderr, "  img-webp -mode near-lossless -skip-palette=false ./ui-shots\n")
	}
	flag.Parse()

	if showVersion {
		cli.Version(os.Stdout, "img-webp")
		return
	}

	if _, err := opt.EncoderOptions(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	files, err := cli.Roots(flag.Args(), listFile, ".png", ".jpg", ".jpeg")
	if err != nil {
		if errors.Is(err, cli.ErrNoInput) {
			flag.Usage()
		} else {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		os.Exit(2)
	}

	rep := report.New("img-webp", "converted", opt.DryRun)
	items := make([]report.Item, len(files))
	cli.Each(len(files), jobs, func(i int) { items[i] = ops.WebP(files[i], opt) })
	for _, it := range items {
		rep.Add(it)
	}
	os.Exit(rep.Emit(os.Stdout, columns, asJSON, verbose))
}

var columns = []report.Column{
	{Title: "file", Width: 48, Value: func(i report.Item) string { return i.Path }},
	{Title: "type", Width: 8, Value: func(i report.Item) string { return i.Str("colourType") }},
	{Title: "before", Width: 10, Right: true, Value: func(i report.Item) string { return report.Size(i.BytesBefore) }},
	{Title: "after", Width: 10, Right: true, Value: func(i report.Item) string { return report.Size(i.BytesAfter) }},
	{Title: "gain", Width: 6, Right: true, Value: func(i report.Item) string {
		return report.Percent(i.GainPercent, i.BytesAfter > 0)
	}},
	{Title: "action", Width: 22, Value: func(i report.Item) string { return report.Action(i, "converted") }},
}
