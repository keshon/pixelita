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
	"path/filepath"
	"strings"

	"github.com/keshon/pixelita/internal/cli"
	"github.com/keshon/pixelita/internal/imgio"
	"github.com/keshon/pixelita/internal/ops"
	"github.com/keshon/pixelita/internal/report"
)

func main() {
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
	flag.StringVar(&cropSpec, "crop", "",
		"measure only these regions, as x,y,w,h; several may be given, separated by spaces")
	flag.IntVar(&worst, "worst", 0,
		"instead of one figure, find the N most damaged regions and print where they are")
	flag.StringVar(&by, "by", "ssim",
		"what -worst ranks by: ssim (structure lost), psnr (error in level), "+
			"or levels (tonal headroom lost)")
	flag.BoolVar(&levels, "levels", false,
		"also report tonal levels per channel, before and after: the headroom left")
	flag.IntVar(&tile, "tile", 256, "region size in pixels for -worst and -flattest")
	flag.Float64Var(&opt.Amplify, "amplify", opt.Amplify, "how much to brighten the difference map")
	flag.Float64Var(&opt.MinPSNR, "min-psnr", 0, "fail below this PSNR in dB, 0 disables")
	flag.Float64Var(&opt.MinSSIM, "min-ssim", 0, "fail below this SSIM, 0 disables")
	flag.BoolVar(&verbose, "v", false, "list identical pairs too")
	flag.BoolVar(&showVersion, "version", false, "print which build this is and exit")
	flag.BoolVar(&asJSON, "json", false, "emit the report as JSON")
	flag.IntVar(&jobs, "jobs", 0, "parallel comparisons, 0 means one per CPU core")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "img-diff — measures how far one image is from another\n\n")
		fmt.Fprintf(os.Stderr, "usage: img-diff [flags] <a> <b>\n\n")
		fmt.Fprintf(os.Stderr, "Two files are compared directly. Two directories are paired up by\n")
		fmt.Fprintf(os.Stderr, "file name ignoring the extension, so a folder of PNGs can be checked\n")
		fmt.Fprintf(os.Stderr, "against the WebP files made from it.\n\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nexamples:\n")
		fmt.Fprintf(os.Stderr, "  img-diff before.png after.png\n")
		fmt.Fprintf(os.Stderr, "  img-diff -out diff.png before.png after.png\n")
		fmt.Fprintf(os.Stderr, "  img-diff -min-psnr 35 ./src ./converted\n")
	}
	flag.Parse()

	if showVersion {
		cli.Version(os.Stdout, "img-diff")
		return
	}

	rects, err := ops.ParseRects(cropSpec)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
	if len(rects) == 1 {
		opt.Crop = rects[0]
	}

	// Three routes ask for a set of regions rather than one figure: find the
	// damaged ones, find the smooth ones, or name them. They print the same
	// table, because the answer wanted is the same shape in every case — a row
	// per place, with every number that belongs on that row.
	if worst > 0 || len(rects) > 1 {
		if flag.NArg() != 2 {
			flag.Usage()
			os.Exit(2)
		}
		var items []report.Item
		var note string
		if worst > 0 {
			ranking := ops.Ranking(by)
			switch ranking {
			case ops.RankSSIM, ops.RankPSNR, ops.RankLevels:
			default:
				fmt.Fprintf(os.Stderr, "error: -by must be ssim, psnr or levels, not %q\n", by)
				os.Exit(2)
			}
			items, err = ops.WorstRegions(flag.Arg(0), flag.Arg(1), worst, tile, ranking,
				levels || ranking == ops.RankLevels)
			note = "worst by " + by + " first"
		} else {
			items, err = ops.MeasureRegions(flag.Arg(0), flag.Arg(1), rects, levels)
			note = "in the order given"
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		rep := report.New("img-diff", "regions", false)
		for _, it := range items {
			rep.Add(it)
		}
		rep.Note("%s; paste a region into -crop to measure it again, "+
			"or into img-look -crop to see it", note)
		os.Exit(rep.Emit(os.Stdout, regionColumns(levels || ops.Ranking(by) == ops.RankLevels),
			asJSON, verbose))
	}

	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(2)
	}

	pairs, err := pairUp(flag.Arg(0), flag.Arg(1))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	rep := report.New("img-diff", "compared", false)
	items := make([]report.Item, len(pairs))
	cli.Each(len(pairs), jobs, func(i int) {
		o := opt
		if out != "" {
			o.Out = out
			if len(pairs) > 1 {
				o.Out = filepath.Join(out, stem(pairs[i].a)+".png")
			}
		}
		items[i] = ops.Diff(pairs[i].a, pairs[i].b, o)
	})
	for _, it := range items {
		rep.Add(it)
	}
	shape := columns
	if !opt.Crop.Empty() {
		shape = cropColumns
	}
	os.Exit(rep.Emit(os.Stdout, shape, asJSON, verbose))
}

type pair struct{ a, b string }

// pairUp resolves the two arguments into the list of comparisons to make.
func pairUp(a, b string) ([]pair, error) {
	ai, err := os.Stat(a)
	if err != nil {
		return nil, err
	}
	bi, err := os.Stat(b)
	if err != nil {
		return nil, err
	}
	if !ai.IsDir() && !bi.IsDir() {
		return []pair{{a, b}}, nil
	}
	if ai.IsDir() != bi.IsDir() {
		return nil, fmt.Errorf("compare two files or two directories, not one of each")
	}

	left, err := cli.Collect([]string{a}, imgio.Extensions...)
	if err != nil {
		return nil, err
	}
	right, err := cli.Collect([]string{b}, imgio.Extensions...)
	if err != nil {
		return nil, err
	}
	// Match on the name without its extension: the point is to compare a source
	// against whatever it was converted into.
	index := map[string]string{}
	for _, p := range right {
		index[stem(p)] = p
	}

	var pairs []pair
	for _, p := range left {
		if q, ok := index[stem(p)]; ok {
			pairs = append(pairs, pair{p, q})
		}
	}
	if len(pairs) == 0 {
		return nil, fmt.Errorf("no file names in common between %s and %s", a, b)
	}
	return pairs, nil
}

func stem(p string) string {
	base := filepath.Base(p)
	return strings.TrimSuffix(base, filepath.Ext(base))
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
				b, ok1 := i.Metrics["levelsBefore"].([]int)
				a, ok2 := i.Metrics["levels"].([]int)
				if !ok1 || !ok2 {
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
		if i.Reason != "" {
			return string(i.Status) + ": " + i.Reason
		}
		return ""
	}},
}
