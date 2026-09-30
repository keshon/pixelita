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
	"errors"
	"flag"
	"fmt"
	"image"
	"os"

	"github.com/keshon/pixelita/internal/cli"
	"github.com/keshon/pixelita/internal/ops"
	"github.com/keshon/pixelita/internal/report"
)

func main() {
	cli.ConfigureDefaultFlags()
	opt := ops.DefaultLook()
	var cropSpec, atSpec string
	var asJSON, dryRun bool
	var overwrite bool
	var showVersion bool

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
	flag.BoolVar(&overwrite, "overwrite", false, "replace an existing preview")
	flag.BoolVar(&showVersion, "version", false, "print which build this is and exit")
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
	cli.ParseDefaultFlags("img-look")

	if showVersion {
		cli.Version(os.Stdout, "img-look")
		return
	}

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
		cli.ExitInputArgument("img-look", cli.ErrNoInput)
	}

	// The probe is a different question and gives a different answer: values.
	// With -json it emits the shared report shape so agents parse one schema;
	// without it prints text for a person.
	if atSpec != "" {
		points, err := ops.ParsePoints(atSpec)
		if err != nil {
			cli.ExitArgument("img-look", "invalid_points", err, nil)
		}
		if asJSON {
			rep := report.New("img-look", "shown", true)
			for _, path := range flag.Args() {
				for _, l := range ops.ProbeItems(path, points) {
					rep.Add(l)
				}
			}
			os.Exit(rep.Emit(os.Stdout, probeColumns, true, false))
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

	rects, err := ops.ParseRects(cropSpec)
	if err != nil {
		cli.ExitArgument("img-look", "invalid_region", err, nil)
	}
	// One spelling everywhere: -crop reads "x,y,w,h x,y,w,h" here just as it
	// does in img-diff, so a -worst table pastes directly. Several rectangles
	// mean several views in one invocation, one composite per region.
	if len(rects) == 0 {
		rects = []image.Rectangle{{}}
	}
	explicitOut := opt.Out != ""
	var planned []ops.Destination
	for _, r := range rects {
		o := opt
		o.Crop = r
		if o.Out == "" || len(rects) > 1 {
			o.Out = ops.LookPath(flag.Args(), o)
		}
		planned = append(planned, ops.Destination{Output: o.Out})
	}
	if err := ops.PreflightDestinations(planned, overwrite, false); err != nil {
		code := "invalid_destination"
		var pe *ops.PlanError
		if errors.As(err, &pe) {
			code = pe.Code
		}
		cli.ExitArgument("img-look", code, err, nil)
	}
	rep := report.New("img-look", "shown", dryRun)
	failed := false
	for ri, r := range rects {
		o := opt
		o.Crop = r
		if o.Out == "" || (len(rects) > 1 && explicitOut) {
			o.Out = ops.LookPath(flag.Args(), o)
		} else if len(rects) > 1 {
			// Distinct files per region; LookPath already hashes the crop.
			o.Out = ops.LookPath(flag.Args(), o)
		}
		_ = ri
		img, items, err := ops.Look(flag.Args(), o)
		for _, it := range items {
			rep.Add(it)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			failed = true
			continue
		}
		// Every other tool here reads -dry-run as "report, write nothing".
		if dryRun {
			continue
		}
		out, size, err := ops.WriteLook(img, o.Out, overwrite)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			failed = true
			continue
		}
		// Tag just the items from this region with this composite. Items carry
		// their crop in metrics, so a multi-region run stays attributable.
		base := len(rep.Items) - len(items)
		for i := range items {
			rep.Items[base+i].Output = out
		}
		rep.Note("open %s — %dx%d, %s",
			out, img.Rect.Dx(), img.Rect.Dy(), report.Size(size))
	}
	if failed {
		rep.Emit(os.Stdout, columns, asJSON, false)
		os.Exit(1)
	}
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
		if v, ok := i.Ints("levels"); ok && len(v) == 3 {
			return fmt.Sprintf("%d/%d/%d", v[0], v[1], v[2])
		}
		return ""
	}},
	{Title: "luma", Width: 9, Right: true, Value: func(i report.Item) string {
		if v, ok := i.Ints("luma"); ok && len(v) == 2 {
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

var probeColumns = []report.Column{
	{Title: "file", Width: 30, Value: func(i report.Item) string { return i.Path }},
	{Title: "x,y", Width: 12, Value: func(i report.Item) string {
		x, ok1 := i.Num("x")
		y, ok2 := i.Num("y")
		if !ok1 || !ok2 {
			return ""
		}
		return fmt.Sprintf("%.0f,%.0f", x, y)
	}},
	{Title: "rgba", Width: 16, Value: func(i report.Item) string {
		r, ok1 := i.Num("r")
		g, ok2 := i.Num("g")
		b, ok3 := i.Num("b")
		a, ok4 := i.Num("a")
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return ""
		}
		return fmt.Sprintf("%.0f %.0f %.0f %.0f", r, g, b, a)
	}},
	{Title: "hex", Width: 8, Value: func(i report.Item) string { return i.Str("hex") }},
	{Title: "note", Width: 20, Value: func(i report.Item) string {
		if i.Status == report.StatusFailed {
			return "failed: " + i.Reason
		}
		return ""
	}},
}
