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
	var out string
	var verbose, asJSON bool
	var jobs int

	flag.StringVar(&out, "out", "",
		"write difference maps here: a file for one pair, a directory for many")
	flag.Float64Var(&opt.Amplify, "amplify", opt.Amplify, "how much to brighten the difference map")
	flag.Float64Var(&opt.MinPSNR, "min-psnr", 0, "fail below this PSNR in dB, 0 disables")
	flag.Float64Var(&opt.MinSSIM, "min-ssim", 0, "fail below this SSIM, 0 disables")
	flag.BoolVar(&verbose, "v", false, "list identical pairs too")
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
	os.Exit(rep.Emit(os.Stdout, columns, asJSON, verbose))
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
	{Title: "note", Width: 18, Value: func(i report.Item) string {
		if i.Reason != "" {
			return string(i.Status) + ": " + i.Reason
		}
		return ""
	}},
}
