// Command img-resize scales images, in linear light so that it does not darken
// them, and writes only the sizes it was actually asked for.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/keshon/pixelita/internal/cli"
	"github.com/keshon/pixelita/internal/imgio"
	"github.com/keshon/pixelita/internal/ops"
	"github.com/keshon/pixelita/internal/report"
	"github.com/keshon/pixelita/internal/resize"
)

func main() {
	opt := ops.DefaultResize()
	var filterName, widths, listFile string
	var verbose, asJSON bool
	var showVersion bool
	var jobs int

	flag.IntVar(&opt.Width, "width", 0, "target width in pixels, 0 derives it from the height")
	flag.IntVar(&opt.Height, "height", 0, "target height in pixels, 0 derives it from the width")
	flag.IntVar(&opt.MaxWidth, "max-width", 0, "shrink anything wider than this, leave the rest alone")
	flag.IntVar(&opt.MaxHeight, "max-height", 0, "shrink anything taller than this, leave the rest alone")
	flag.Float64Var(&opt.Scale, "scale", 0, "scale factor, for example 0.5")
	flag.StringVar(&widths, "widths", "", "comma-separated widths to produce, for example 320,640,1280")
	flag.StringVar(&opt.Fit, "fit", opt.Fit, "how to fit the box: inside, outside, cover or exact")
	flag.StringVar(&filterName, "filter", resize.CatmullRom.Name,
		"resampling filter: "+strings.Join(resize.FilterNames(), ", "))
	flag.BoolVar(&opt.AllowUpscale, "allow-upscale", false, "permit making an image larger")
	flag.StringVar(&opt.Format, "format", opt.Format, "output format: keep, png or jpeg")
	flag.IntVar(&opt.JPEGQuality, "jpeg-quality", opt.JPEGQuality, "quality for JPEG output, 1..100")
	flag.StringVar(&opt.OutDir, "out-dir", "", "write results here instead of next to the source")
	flag.StringVar(&opt.Suffix, "suffix", "", "suffix for the output name, default is the size")
	flag.BoolVar(&opt.Replace, "replace", false, "overwrite the source file")
	flag.BoolVar(&opt.DryRun, "dry-run", false, "report what would happen, write nothing")
	flag.BoolVar(&verbose, "v", false, "list skipped files too")
	flag.BoolVar(&showVersion, "version", false, "print which build this is and exit")
	flag.BoolVar(&asJSON, "json", false, "emit the report as JSON")
	flag.IntVar(&jobs, "jobs", 0, "parallel workers, 0 means one per CPU core")
	flag.StringVar(&listFile, "from-file", "", "read paths from a file, one per line")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "img-resize — scales images in linear light\n\n")
		fmt.Fprintf(os.Stderr, "usage: img-resize [flags] <file-or-directory>...\n\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nexamples:\n")
		fmt.Fprintf(os.Stderr, "  img-resize -max-width 1920 ./public/img\n")
		fmt.Fprintf(os.Stderr, "  img-resize -widths 320,640,1280 -out-dir ./dist hero.png\n")
		fmt.Fprintf(os.Stderr, "  img-resize -width 400 -height 400 -fit cover avatar.jpg\n")
	}
	flag.Parse()

	if showVersion {
		cli.Version(os.Stdout, "img-resize")
		return
	}

	var ok bool
	if opt.Filter, ok = resize.FilterByName(filterName); !ok {
		fmt.Fprintf(os.Stderr, "error: unknown filter %q, expected one of %s\n",
			filterName, strings.Join(resize.FilterNames(), ", "))
		os.Exit(2)
	}

	var err error
	if opt.Widths, err = parseWidths(widths); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
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

	rep := report.New("img-resize", "resized", opt.DryRun)
	results := make([][]report.Item, len(files))
	cli.Each(len(files), jobs, func(i int) { results[i] = ops.Resize(files[i], opt) })
	for _, items := range results {
		for _, it := range items {
			rep.Add(it)
		}
	}
	os.Exit(rep.Emit(os.Stdout, columns, asJSON, verbose))
}

func parseWidths(s string) ([]int, error) {
	if s == "" {
		return nil, nil
	}
	var out []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("bad width %q", part)
		}
		out = append(out, n)
	}
	return out, nil
}

var columns = []report.Column{
	{Title: "file", Width: 40, Value: func(i report.Item) string { return i.Path }},
	{Title: "from", Width: 11, Right: true, Value: func(i report.Item) string { return i.Str("from") }},
	{Title: "to", Width: 11, Right: true, Value: func(i report.Item) string { return i.Str("to") }},
	{Title: "before", Width: 10, Right: true, Value: func(i report.Item) string { return report.Size(i.BytesBefore) }},
	{Title: "after", Width: 10, Right: true, Value: func(i report.Item) string { return report.Size(i.BytesAfter) }},
	{Title: "action", Width: 24, Value: func(i report.Item) string { return report.Action(i, "resized") }},
}
