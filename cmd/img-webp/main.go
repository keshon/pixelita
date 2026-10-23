// Command img-webp converts PNG and JPEG images to WebP.
//
// It is built around one idea: a conversion that makes a file bigger, or barely
// smaller, is not worth doing. Every candidate is encoded, measured against the
// original and only written when it actually pays off.
//
// The work itself lives in internal/ops. This file is the flag parser and the
// table.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/keshon/pixelita/internal/cli"
	"github.com/keshon/pixelita/internal/engine"
	"github.com/keshon/pixelita/internal/ops"
	"github.com/keshon/pixelita/internal/report"
)

func main() {
	ctx, cancel := cli.WorkContext()
	defer cancel()
	cli.ConfigureDefaultFlags()
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
	flag.Float64Var(&opt.MinPSNR, "min-psnr", opt.MinPSNR,
		"refuse lossy below this fidelity in dB, 0 disables the check")
	flag.Float64Var(&opt.MinSSIM, "min-ssim", opt.MinSSIM,
		"refuse lossy below this SSIM, 0 disables the check")
	flag.BoolVar(&opt.DryRun, "dry-run", false, "measure and report, write nothing")
	flag.BoolVar(&opt.KeepOriginal, "keep-original", opt.KeepOriginal,
		"deprecated: false is rejected; use -delete-source")
	flag.BoolVar(&opt.DeleteSource, "delete-source", false,
		"delete the source only after a verified destination write")
	flag.BoolVar(&opt.Overwrite, "overwrite", false, "replace an existing destination")
	flag.BoolVar(&verbose, "v", false, "list skipped files too")
	flag.StringVar(&opt.OutDir, "out-dir", "",
		"write results here instead of next to the source")
	flag.BoolVar(&showVersion, "version", false, "print which build this is and exit")
	flag.BoolVar(&asJSON, "json", false, "emit the report as JSON")
	flag.IntVar(&jobs, "jobs", 0, "parallel encoders, 0 means at most two workers")
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
	cli.ParseDefaultFlags("img-webp")
	if err := (ops.CandidatePolicy{MinGain: opt.MinGain, MinPSNR: opt.MinPSNR, MinSSIM: opt.MinSSIM}).Validate(); err != nil {
		cli.ExitArgument("img-webp", "invalid_threshold", err, nil)
	}

	if showVersion {
		cli.Version(os.Stdout, "img-webp")
		return
	}
	if opt.Quality < 1 || opt.Quality > 100 {
		cli.ExitArgument("img-webp", "invalid_option", fmt.Errorf("-quality must be between 1 and 100"), nil)
	}
	if opt.MinGain < 0 || opt.MinPSNR < 0 || opt.MinSSIM < 0 || opt.MinSSIM > 1 {
		cli.ExitArgument("img-webp", "invalid_threshold", fmt.Errorf("thresholds must be non-negative and -min-ssim cannot exceed 1"), nil)
	}

	if _, err := opt.EncoderOptions(); err != nil {
		cli.ExitArgument("img-webp", "invalid_option", err, nil)
	}
	if !opt.KeepOriginal {
		next := &report.NextAction{Command: "img-webp", Args: append([]string{"-delete-source"}, flag.Args()...)}
		cli.ExitArgument("img-webp", "negative_retention_rejected", fmt.Errorf("-keep-original=false is unsafe; use explicit -delete-source"), next)
	}

	rep, runErr := engine.WebP(engine.BatchRequest{Context: ctx, Paths: flag.Args(), FromFile: listFile, Jobs: jobs}, opt)
	if runErr != nil {
		cli.ExitArgument("img-webp", engine.ErrorCode(runErr), runErr, nil)
	}
	os.Exit(report.Emit(rep, os.Stdout, columns, asJSON, verbose))
}

var columns = []report.Column{
	{Title: "file", Width: 40, Value: func(i report.Item) string { return i.Path }},
	{Title: "type", Width: 8, Value: func(i report.Item) string { return i.Str("colourType") }},
	{Title: "before", Width: 10, Right: true, Value: func(i report.Item) string { return report.Size(i.BytesBefore) }},
	{Title: "after", Width: 10, Right: true, Value: func(i report.Item) string { return report.Size(i.BytesAfter) }},
	{Title: "gain", Width: 6, Right: true, Value: func(i report.Item) string {
		return report.Percent(i.GainPercent, i.BytesAfter > 0)
	}},
	{Title: "psnr", Width: 8, Right: true, Value: func(i report.Item) string {
		if v, ok := i.Num("psnr"); ok {
			return fmt.Sprintf("%.1f dB", v)
		}
		return ""
	}},
	{Title: "action", Width: 24, Value: func(i report.Item) string { return report.Action(i, "converted") }},
}
