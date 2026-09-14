// Command img-scan looks at a tree of images and says what is there and what
// could be done about it.
//
// It is the tool the others were missing. Running a converter over a directory
// and hoping is a poor way to work: some files are already optimal, some would
// grow, and the ones worth touching are usually a minority carrying most of the
// weight. img-scan answers that before anything is written, by actually running
// the encoders rather than guessing from the file extension.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/keshon/pixelita/internal/cli"
	"github.com/keshon/pixelita/internal/imgio"
	"github.com/keshon/pixelita/internal/ops"
	"github.com/keshon/pixelita/internal/report"
)

func main() {
	opt := ops.DefaultScan()
	var verbose, asJSON bool
	var showVersion bool
	var jobs int
	var listFile string

	flag.BoolVar(&opt.Quick, "quick", false,
		"read headers only: an inventory, without measuring what conversion would save")
	flag.IntVar(&opt.Colors, "colors", opt.Colors, "palette size to test quantisation with")
	flag.IntVar(&opt.Effort, "effort", opt.Effort, "1..10, effort for the quantisation estimate")
	flag.IntVar(&opt.WebPQuality, "webp-quality", opt.WebPQuality, "quality to test WebP with")
	flag.Float64Var(&opt.MinGain, "min-gain", opt.MinGain,
		"gain below this is not worth recommending, in percent")
	flag.Float64Var(&opt.MinPSNR, "min-psnr", opt.MinPSNR,
		"do not recommend a conversion below this fidelity, in dB")
	flag.BoolVar(&verbose, "v", false, "list files with nothing to gain too")
	flag.BoolVar(&showVersion, "version", false, "print which build this is and exit")
	flag.BoolVar(&asJSON, "json", false, "emit the report as JSON")
	flag.IntVar(&jobs, "jobs", 0, "parallel workers, 0 means one per CPU core")
	flag.StringVar(&listFile, "from-file", "", "read paths from a file, one per line")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "img-scan — inventories images and measures what could be saved\n\n")
		fmt.Fprintf(os.Stderr, "usage: img-scan [flags] <file-or-directory>...\n\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nexamples:\n")
		fmt.Fprintf(os.Stderr, "  img-scan ./public/img\n")
		fmt.Fprintf(os.Stderr, "  img-scan -quick -v ./public/img\n")
		fmt.Fprintf(os.Stderr, "  img-scan -json ./public/img > report.json\n")
	}
	flag.Parse()

	if showVersion {
		cli.Version(os.Stdout, "img-scan")
		return
	}

	files, err := cli.Roots(flag.Args(), listFile, imgio.Extensions...)
	if err != nil {
		if errors.Is(err, cli.ErrNoInput) {
			flag.Usage()
		} else {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		os.Exit(2)
	}

	rep := report.New("img-scan", "improved", true)
	items := make([]report.Item, len(files))
	cli.Each(len(files), jobs, func(i int) { items[i] = ops.Scan(files[i], opt) })
	for _, it := range items {
		rep.Add(it)
	}
	ops.ScanSummary(rep, opt)
	os.Exit(rep.Emit(os.Stdout, columns, asJSON, verbose))
}

var columns = []report.Column{
	{Title: "file", Width: 38, Value: func(i report.Item) string { return i.Path }},
	{Title: "size", Width: 9, Right: true, Value: func(i report.Item) string { return report.Size(i.BytesBefore) }},
	{Title: "format", Width: 7, Value: func(i report.Item) string { return i.Str("format") }},
	{Title: "type", Width: 7, Value: func(i report.Item) string { return i.Str("colourType") }},
	{Title: "pixels", Width: 11, Right: true, Value: func(i report.Item) string { return i.Str("size") }},
	{Title: "colours", Width: 8, Right: true, Value: func(i report.Item) string {
		n, ok := i.Num("colours")
		if !ok {
			return ""
		}
		if exact, _ := i.Metrics["coloursExact"].(bool); !exact {
			return fmt.Sprintf("%.0f+", n) // the histogram stopped counting
		}
		return fmt.Sprintf("%.0f", n)
	}},
	{Title: "best", Width: 16, Value: func(i report.Item) string {
		best := i.Str("best")
		switch {
		case best == "" && i.Status == report.StatusFailed:
			return "failed"
		case best == "":
			return "— " + i.Reason
		}
		out := fmt.Sprintf("%s %.0f%%", best, -i.GainPercent)
		if i.Reason != "" {
			out += " *"
		}
		return out
	}},
}
