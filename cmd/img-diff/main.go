// Command img-diff measures how far one image is from another.
//
// It is the tool that keeps the rest of this repo honest. Every other command
// promises not to make a file worse; this one is how that promise is checked,
// by a person, by CI, or by an agent verifying its own work.
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
	cli.ConfigureDefaultFlags()
	opt := ops.DefaultDiff()
	var out, cropSpec string
	var worst, tile int
	var levels bool
	var by string
	var verbose, asJSON bool
	var showVersion bool
	var jobs int

	flag.StringVar(&out, "out", "",
		"write difference maps here: a file for one pair, a directory for many")
	flag.BoolVar(&opt.Overwrite, "overwrite", false, "replace an existing difference map")
	flag.StringVar(&cropSpec, "crop", "",
		"measure only these regions, as x,y,w,h; several may be given, separated by spaces")
	flag.IntVar(&worst, "worst", 0,
		"instead of one figure, find the N most damaged regions and print where they are")
	flag.StringVar(&by, "by", "ssim",
		"what -worst ranks by: ssim (structure lost), psnr (error in level), "+
			"or levels (tonal headroom lost)")
	flag.BoolVar(&levels, "levels", false,
		"also report tonal levels per channel, before and after: the headroom left")
	flag.IntVar(&tile, "tile", 256, "region size in pixels for -worst")
	flag.Float64Var(&opt.Amplify, "amplify", opt.Amplify, "how much to brighten the difference map")
	flag.Float64Var(&opt.MinPSNR, "min-psnr", 0, "fail below this PSNR in dB, 0 disables")
	flag.Float64Var(&opt.MinSSIM, "min-ssim", 0, "fail below this SSIM, 0 disables")
	flag.BoolVar(&opt.StrictSize, "strict-size", false, "fail when dimensions differ instead of resampling b to a")
	flag.BoolVar(&verbose, "v", false, "list identical pairs too")
	flag.BoolVar(&showVersion, "version", false, "print which build this is and exit")
	flag.BoolVar(&asJSON, "json", false, "emit the report as JSON")
	flag.IntVar(&jobs, "jobs", 0, "parallel comparisons, 0 means at most two workers")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "img-diff — measures how far one image is from another\n\n")
		fmt.Fprintf(os.Stderr, "usage: img-diff [flags] <a> <b>\n\n")
		fmt.Fprintf(os.Stderr, "Two files are compared directly. Two directories are paired up by\n")
		fmt.Fprintf(os.Stderr, "file name ignoring the extension, so a folder of PNGs can be checked\n")
		fmt.Fprintf(os.Stderr, "against the WebP files made from it.\n")
		fmt.Fprintf(os.Stderr, "Pairs of different dimensions are resampled (b to a) and marked\n")
		fmt.Fprintf(os.Stderr, "resampled; -strict-size fails instead.\n\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nexamples:\n")
		fmt.Fprintf(os.Stderr, "  img-diff before.png after.png\n")
		fmt.Fprintf(os.Stderr, "  img-diff -out diff.png before.png after.png\n")
		fmt.Fprintf(os.Stderr, "  img-diff -min-psnr 35 ./src ./converted\n")
	}
	cli.ParseDefaultFlags("img-diff")
	if err := (ops.CandidatePolicy{MinPSNR: opt.MinPSNR, MinSSIM: opt.MinSSIM}).Validate(); err != nil {
		cli.ExitArgument("img-diff", "invalid_threshold", err, nil)
	}

	if showVersion {
		cli.Version(os.Stdout, "img-diff")
		return
	}

	rects, err := ops.ParseRects(cropSpec)
	if err != nil {
		cli.ExitArgument("img-diff", "invalid_region", err, nil)
	}

	if flag.NArg() != 2 {
		cli.ExitArgument("img-diff", "expected_two_inputs", fmt.Errorf("expected two paths"), nil)
	}
	rep, err := engine.Compare(engine.CompareRequest{A: flag.Arg(0), B: flag.Arg(1), MinPSNR: opt.MinPSNR, MinSSIM: opt.MinSSIM, StrictSize: opt.StrictSize, Regions: rects, Worst: worst, Tile: tile, By: ops.Ranking(by), Levels: levels, Out: out, Overwrite: opt.Overwrite, Amplify: opt.Amplify, Jobs: jobs})
	if err != nil {
		cli.ExitArgument("img-diff", "invalid_arguments", err, nil)
	}
	shape := columns
	if len(rects) > 0 || worst > 0 {
		shape = regionColumns(levels || by == "levels")
	}
	os.Exit(report.Emit(rep, os.Stdout, shape, asJSON, verbose))
}

// regionColumns is its own set rather than a slice of the one below. A region
// row has no file size, and three columns of dashes would be noise where the
// whole point is that one rectangle and one number read at a glance.
func regionColumns(levels bool) []report.Column {
	out := append([]report.Column{
		{Title: "region (x,y,w,h)", Width: 22, Value: func(i report.Item) string { return i.Str("crop") }},
	}, measureColumns...)
	if levels {
		out = append(out, report.Column{Title: "levels rgb", Width: 26,
			Value: func(i report.Item) string {
				b, ok1 := i.Ints("levelsBefore")
				a, ok2 := i.Ints("levels")
				if !ok1 || !ok2 || len(b) != 3 || len(a) != 3 {
					return ""
				}
				return fmt.Sprintf("%d/%d/%d to %d/%d/%d",
					b[0], b[1], b[2], a[0], a[1], a[2])
			}})
	}
	return out
}

// cropColumns is the same idea for a single named region: the file still needs
// naming, its size does not, because the size is not what was measured.
var cropColumns = append([]report.Column{
	{Title: "file", Width: 40, Value: func(i report.Item) string { return i.Path }},
	{Title: "region (x,y,w,h)", Width: 22, Value: func(i report.Item) string { return i.Str("crop") }},
}, measureColumns...)

var measureColumns = []report.Column{
	{Title: "psnr", Width: 8, Right: true, Value: func(i report.Item) string {
		if v, ok := i.Num("psnr"); ok {
			return fmt.Sprintf("%.1f dB", v)
		}
		return "identical"
	}},
	{Title: "ssim", Width: 6, Right: true, Value: func(i report.Item) string {
		if v, ok := i.Num("ssim"); ok {
			return fmt.Sprintf("%.3f", v)
		}
		return ""
	}},
	{Title: "worst", Width: 5, Right: true, Value: func(i report.Item) string {
		if v, ok := i.Num("maxDelta"); ok {
			return fmt.Sprintf("%.0f", v)
		}
		return ""
	}},
	{Title: "p95/p99", Width: 8, Right: true, Value: func(i report.Item) string {
		p95, ok1 := i.Num("p95")
		p99, ok2 := i.Num("p99")
		if !ok1 || !ok2 {
			return ""
		}
		return fmt.Sprintf("%.0f/%.0f", p95, p99)
	}},
}

var columns = []report.Column{
	{Title: "file", Width: 40, Value: func(i report.Item) string { return i.Path }},
	{Title: "a", Width: 10, Right: true, Value: func(i report.Item) string { return report.Size(i.BytesBefore) }},
	{Title: "b", Width: 10, Right: true, Value: func(i report.Item) string { return report.Size(i.BytesAfter) }},
	{Title: "size", Width: 6, Right: true, Value: func(i report.Item) string {
		return report.Percent(i.GainPercent, i.BytesBefore > 0)
	}},
	{Title: "psnr", Width: 8, Right: true, Value: func(i report.Item) string {
		if v, ok := i.Num("psnr"); ok {
			return fmt.Sprintf("%.1f dB", v)
		}
		if i.Status == report.StatusFailed {
			return ""
		}
		return "identical"
	}},
	{Title: "ssim", Width: 6, Right: true, Value: func(i report.Item) string {
		if v, ok := i.Num("ssim"); ok {
			return fmt.Sprintf("%.3f", v)
		}
		return ""
	}},
	{Title: "worst", Width: 5, Right: true, Value: func(i report.Item) string {
		if v, ok := i.Num("maxDelta"); ok {
			return fmt.Sprintf("%.0f", v)
		}
		return ""
	}},
	// Beside the worst single pixel, the error nearly every pixel stays under.
	// One number without the other cannot separate a stray outlier from a shift
	// across a large part of the frame.
	{Title: "p95/p99", Width: 8, Right: true, Value: func(i report.Item) string {
		p95, ok1 := i.Num("p95")
		p99, ok2 := i.Num("p99")
		if !ok1 || !ok2 {
			return ""
		}
		return fmt.Sprintf("%.0f/%.0f", p95, p99)
	}},
	// Shown whenever a crop is in force. A PSNR figure means something quite
	// different for one corner of an image than for the whole of it, and a
	// reader who cannot see which was measured has been told half a fact.
	{Title: "region", Width: 20, Value: func(i report.Item) string { return i.Str("crop") }},
	{Title: "note", Width: 18, Value: func(i report.Item) string {
		if r := i.Str("resampled"); r != "" {
			if i.Reason != "" {
				return "resampled " + r + ": " + i.Reason
			}
			return "resampled " + r
		}
		if i.Reason != "" {
			return string(i.Status) + ": " + i.Reason
		}
		return ""
	}},
}
