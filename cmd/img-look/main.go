// Command img-look prepares images to be looked at rather than processed.
//
// It decodes anything the toolkit reads and writes one PNG that can be opened
// anywhere: fitted to a size worth looking at, cropped to the region in
// question, transparency shown against a checkerboard, several files stacked
// with a separator between them and their names written on. With -at it prints
// pixel values instead, for when the question is what exactly is there rather
// than how it looks.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/keshon/pixelita/internal/ops"
	"github.com/keshon/pixelita/internal/report"
)

func main() {
	opt := ops.DefaultLook()
	var cropSpec, atSpec string
	var asJSON, dryRun bool

	flag.IntVar(&opt.Max, "max", opt.Max, "longest side of each panel in pixels, 0 keeps the original")
	flag.StringVar(&cropSpec, "crop", "", "region to show, as x,y,w,h; applied to every input")
	flag.StringVar(&opt.Background, "bg", opt.Background,
		"what transparency is shown against: checker, white, black or none")
	flag.BoolVar(&opt.Across, "across", false, "lay the panels side by side instead of stacked")
	flag.IntVar(&opt.Zoom, "zoom", 0,
		"magnify this many times by repeating pixels, no smoothing; implies -max 0")
	flag.BoolVar(&opt.Stats, "stats", false,
		"also report the mean colour, luma range and tonal levels of what is shown")
	flag.BoolVar(&opt.Stretch, "stretch", false,
		"map the region's own range onto the full scale, as auto-levels would; "+
			"every panel uses the first one's range so they stay comparable")
	flag.BoolVar(&opt.Label, "label", opt.Label, "write the file name on each panel")
	flag.StringVar(&opt.Out, "out", opt.Out,
		"where to write the result; the default is a name derived from the "+
			"inputs, under "+ops.LookDir())
	flag.StringVar(&atSpec, "at", "",
		"print the pixels at these points instead of writing an image, as x,y x,y")
	flag.BoolVar(&dryRun, "dry-run", false,
		"measure and report, write no image — for when only -stats is wanted")
	flag.BoolVar(&asJSON, "json", false, "emit the report as JSON")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "img-look — makes images visible\n\n")
		fmt.Fprintf(os.Stderr, "usage: img-look [flags] <file>...\n\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nexamples:\n")
		fmt.Fprintf(os.Stderr, "  img-look hero.webp                             # any format, as a PNG to open\n")
		fmt.Fprintf(os.Stderr, "  img-look before.png after.png                  # stacked, labelled, comparable\n")
		fmt.Fprintf(os.Stderr, "  img-look -crop 700,380,460,210 a.png b.png     # the same region of both\n")
		fmt.Fprintf(os.Stderr, "  img-look -max 0 -crop 0,0,64,64 icon.png       # native size, no scaling\n")
		fmt.Fprintf(os.Stderr, "  img-look -crop 4600,1380,180,105 -zoom 4 a.png b.png\n")
		fmt.Fprintf(os.Stderr, "                                                 # that region at four times life size\n")
		fmt.Fprintf(os.Stderr, "  img-look -crop 800,2400,500,250 -stats a.png b.png\n")
		fmt.Fprintf(os.Stderr, "                                                 # and what those pixels average to\n")
		fmt.Fprintf(os.Stderr, "  img-look -at '10,20 300,15' shot.png           # the numbers, not the picture\n")
	}
	flag.Parse()

	// Magnifying and then capping the size would silently undo the zoom: a
	// region cut from a large photograph is fitted to 1400px by default, and
	// -zoom 4 on top of that gives neither the region's own pixels nor four
	// times them. Asking to magnify is asking for native pixels — unless the
	// cap was named explicitly, in which case it was meant.
	if opt.Zoom > 1 {
		capped := false
		flag.Visit(func(f *flag.Flag) {
			if f.Name == "max" {
				capped = true
			}
		})
		if !capped {
			opt.Max = 0
		}
	}

	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}

	// The probe is a different question and gives a different answer: text.
	if atSpec != "" {
		points, err := ops.ParsePoints(atSpec)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(2)
		}
		exit := 0
		for _, path := range flag.Args() {
			if flag.NArg() > 1 {
				fmt.Println(path)
			}
			lines, err := ops.Probe(path, points)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				exit = 1
				continue
			}
			for _, l := range lines {
				fmt.Println(" ", l)
			}
		}
		os.Exit(exit)
	}

	var err error
	if opt.Crop, err = ops.ParseRect(cropSpec); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	if opt.Out == "" {
		opt.Out = ops.LookPath(flag.Args(), opt)
	}

	img, items, err := ops.Look(flag.Args(), opt)
	rep := report.New("img-look", "shown", dryRun)
	for _, it := range items {
		rep.Add(it)
	}
	if err != nil {
		// Still emit. A caller that asked for -json asked for a document, and
		// a run where everything failed is exactly when it wants to know which
		// file failed and why.
		rep.Emit(os.Stdout, columns, asJSON, false)
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	// Every other tool here reads -dry-run as "report, write nothing". Asking
	// for numbers should not oblige anyone to produce a picture they did not
	// want; the measurements above are already done.
	if dryRun {
		os.Exit(rep.Emit(os.Stdout, columns, asJSON, false))
	}

	out, size, err := ops.WriteLook(img, opt.Out)
	if err != nil {
		// Still emit: the measurements are valid, and a caller that asked for
		// -json asked for a document.
		rep.Emit(os.Stdout, columns, asJSON, false)
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	// Every input contributed to the one composite, so every item names it.
	// That is what lets a caller reading -json find the file without parsing
	// the sentence below.
	for i := range rep.Items {
		rep.Items[i].Output = out
	}
	rep.Note("open %s — %dx%d, %s",
		out, img.Rect.Dx(), img.Rect.Dy(), report.Size(size))

	os.Exit(rep.Emit(os.Stdout, columns, asJSON, false))
}

var columns = []report.Column{
	{Title: "file", Width: 44, Value: func(i report.Item) string { return i.Path }},
	{Title: "size", Width: 9, Right: true, Value: func(i report.Item) string {
		if n, ok := i.Num("bytes"); ok {
			return report.Size(int64(n))
		}
		return ""
	}},
	{Title: "from", Width: 12, Right: true, Value: func(i report.Item) string { return i.Str("from") }},
	{Title: "shown", Width: 12, Right: true, Value: func(i report.Item) string { return i.Str("to") }},
	{Title: "region", Width: 20, Value: func(i report.Item) string { return i.Str("crop") }},
	{Title: "mean", Width: 8, Value: func(i report.Item) string { return i.Str("mean") }},
	// Levels is the headroom left: how many distinct values each channel still
	// uses here. It is what "this file can no longer be edited" means when
	// stated as a measurement instead of an opinion.
	{Title: "levels rgb", Width: 14, Right: true, Value: func(i report.Item) string {
		if v, ok := i.Metrics["levels"].([]int); ok && len(v) == 3 {
			return fmt.Sprintf("%d/%d/%d", v[0], v[1], v[2])
		}
		return ""
	}},
	{Title: "luma", Width: 9, Right: true, Value: func(i report.Item) string {
		if v, ok := i.Metrics["luma"].([]int); ok && len(v) == 2 {
			return fmt.Sprintf("%d..%d", v[0], v[1])
		}
		return ""
	}},
	{Title: "note", Width: 16, Value: func(i report.Item) string {
		if i.Status == report.StatusFailed {
			return "failed: " + i.Reason
		}
		return i.Str("background")
	}},
}
