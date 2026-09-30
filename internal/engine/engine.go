package engine

import (
	"os"
	"sort"
	"strings"

	"github.com/keshon/pixelita/internal/cli"
	"github.com/keshon/pixelita/internal/imgio"
	"github.com/keshon/pixelita/internal/ops"
	"github.com/keshon/pixelita/internal/report"
)

type OptimizeRequest struct {
	Paths        []string
	OutDir       string
	Apply        bool
	Overwrite    bool
	Replace      bool
	DeleteSource bool
	Format       string
	Widths       []int
	MinGain      float64
	MinPSNR      float64
	MinSSIM      float64
	Jobs         int
}

type InspectRequest struct {
	Paths   []string
	Measure bool
	Jobs    int
}

func Inspect(req InspectRequest) (*report.Report, error) {
	files, err := cli.Roots(req.Paths, "", imgio.Extensions...)
	if err != nil {
		return nil, err
	}
	o := ops.DefaultScan()
	o.Quick = !req.Measure
	rep := report.New("pixelita inspect", "inspected", true)
	items := make([]report.Item, len(files))
	cli.Each(len(files), req.Jobs, func(i int) { items[i] = ops.Scan(files[i], o) })
	for _, it := range items {
		rep.Add(it)
	}
	ops.ScanSummary(rep, o)
	return rep, nil
}

type CompareRequest struct {
	A, B       string
	MinPSNR    float64
	MinSSIM    float64
	StrictSize bool
}

func Compare(req CompareRequest) *report.Report {
	o := ops.DefaultDiff()
	o.MinPSNR, o.MinSSIM, o.StrictSize = req.MinPSNR, req.MinSSIM, req.StrictSize
	rep := report.New("pixelita compare", "compared", false)
	rep.Add(ops.Diff(req.A, req.B, o))
	return rep
}

type ViewRequest struct {
	Paths []string
	Out   string
	// Max is optional. Nil uses Pixelita's bounded preview default; a pointer
	// to zero explicitly preserves the source dimensions.
	Max       *int
	DryRun    bool
	Overwrite bool
}

func View(req ViewRequest) (*report.Report, error) {
	o := ops.DefaultLook()
	if req.Max != nil {
		o.Max = *req.Max
	}
	o.Out = req.Out
	if o.Out == "" {
		o.Out = ops.LookPath(req.Paths, o)
	}
	if !req.DryRun {
		if err := ops.PreflightDestinations([]ops.Destination{{Output: o.Out}}, req.Overwrite, false); err != nil {
			return nil, err
		}
	}
	img, items, err := ops.Look(req.Paths, o)
	rep := report.New("pixelita view", "shown", req.DryRun)
	for _, it := range items {
		rep.Add(it)
	}
	if err != nil {
		return rep, err
	}
	if !req.DryRun {
		out, size, err := ops.WriteLook(img, o.Out, req.Overwrite)
		if err != nil {
			return rep, err
		}
		for i := range rep.Items {
			rep.Items[i].Output = out
		}
		rep.Note("open %s (%s)", out, report.Size(size))
	}
	return rep, nil
}

type PlanItem struct {
	Source       string
	Destinations []string
	Strategy     string
	Verification string
	Automatic    bool
}

type Plan struct {
	Intent      string
	Apply       bool
	Permissions map[string]bool
	Items       []PlanItem
	Request     OptimizeRequest
}

func BuildOptimizePlan(req OptimizeRequest) (*Plan, error) {
	if req.Format == "" {
		req.Format = "keep"
	}
	if req.Format != "keep" && req.Format != "webp" {
		return nil, &ops.PlanError{Code: "invalid_format", Detail: "format must be keep or webp"}
	}
	if req.Format == "webp" && len(req.Widths) > 0 {
		return nil, &ops.PlanError{Code: "conflicting_strategies", Detail: "webp conversion and resize variants are separate plans"}
	}
	if req.DeleteSource && req.Format != "webp" {
		return nil, &ops.PlanError{Code: "delete_source_requires_webp", Detail: "source deletion is only valid with explicit webp conversion"}
	}
	files, err := cli.Roots(req.Paths, "", imgio.Extensions...)
	if err != nil {
		return nil, err
	}
	p := &Plan{Intent: "optimize", Apply: req.Apply, Request: req,
		Permissions: map[string]bool{"overwrite": req.Overwrite, "replace": req.Replace, "deleteSource": req.DeleteSource}}
	var destinations []ops.Destination
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, &ops.PlanError{Code: "input_unavailable", Path: path, Detail: err.Error()}
		}
		head, err := imgio.ReadHeader(raw)
		if err != nil {
			return nil, &ops.PlanError{Code: "unreadable_input", Path: path, Detail: err.Error()}
		}
		item := PlanItem{Source: path}
		switch {
		case len(req.Widths) > 0:
			cap, _ := capability("resize")
			if !supportsFormat(cap, head.Format) {
				return nil, &ops.PlanError{Code: "unsupported_strategy_input", Path: path, Detail: "resize variants accept png or jpeg"}
			}
			item.Strategy, item.Verification, item.Automatic = cap.ID, cap.Verification, cap.Automatic
			o := ops.DefaultResize()
			o.Widths, o.OutDir, o.Replace = append([]int(nil), req.Widths...), req.OutDir, req.Replace
			sizes, _ := o.Targets(head.Width, head.Height)
			format := strings.ToLower(head.Format)
			ext := ".png"
			if format == "jpeg" {
				ext = ".jpg"
			}
			for _, size := range sizes {
				out := o.OutputPath(path, ext, size[0], size[1])
				item.Destinations = append(item.Destinations, out)
				destinations = append(destinations, ops.Destination{Source: path, Output: out})
			}
		case req.Format == "webp":
			cap, _ := capability("webp")
			if !supportsFormat(cap, head.Format) {
				return nil, &ops.PlanError{Code: "unsupported_strategy_input", Path: path, Detail: "webp conversion accepts png or jpeg"}
			}
			item.Strategy, item.Verification, item.Automatic = cap.ID, cap.Verification, cap.Automatic
			o := ops.DefaultWebP()
			o.OutDir = req.OutDir
			item.Destinations = []string{o.OutputPath(path)}
			destinations = append(destinations, ops.Destination{Source: path, Output: item.Destinations[0]})
		default:
			cap, ok := automaticCapability(head.Format)
			if !ok {
				item.Strategy = "none"
				item.Verification = "none"
				p.Items = append(p.Items, item)
				continue
			}
			item.Strategy, item.Verification, item.Automatic = cap.ID, cap.Verification, cap.Automatic
			switch cap.ID {
			case "png-palette":
				o := ops.DefaultQuant()
				o.OutDir, o.Replace = req.OutDir, req.Replace
				item.Destinations = []string{o.OutputPath(path)}
				destinations = append(destinations, ops.Destination{Source: path, Output: item.Destinations[0]})
			case "jpeg-huffman":
				o := ops.DefaultJPEG()
				o.OutDir, o.Replace = req.OutDir, req.Replace
				item.Destinations = []string{o.OutputPath(path)}
				destinations = append(destinations, ops.Destination{Source: path, Output: item.Destinations[0]})
			}
		}
		p.Items = append(p.Items, item)
	}
	sort.Slice(p.Items, func(i, j int) bool { return p.Items[i].Source < p.Items[j].Source })
	if err := ops.PreflightDestinations(destinations, req.Overwrite, req.Replace); err != nil {
		return nil, err
	}
	return p, nil
}

func ExecuteOptimize(plan *Plan) *report.Report {
	rep := report.New("pixelita optimize", "optimized", !plan.Apply)
	results := make([][]report.Item, len(plan.Items))
	cli.Each(len(plan.Items), plan.Request.Jobs, func(index int) {
		pi := plan.Items[index]
		var items []report.Item
		switch pi.Strategy {
		case "png-palette":
			o := ops.DefaultQuant()
			o.DryRun, o.Output, o.Replace, o.Overwrite = !plan.Apply, pi.Destinations[0], plan.Request.Replace, plan.Request.Overwrite
			applyThresholds(&o.MinGain, &o.MinPSNR, nil, plan.Request)
			items = []report.Item{ops.Quant(pi.Source, o)}
		case "jpeg-huffman":
			o := ops.DefaultJPEG()
			o.DryRun, o.Output, o.Replace, o.Overwrite = !plan.Apply, pi.Destinations[0], plan.Request.Replace, plan.Request.Overwrite
			if plan.Request.MinGain > 0 {
				o.MinGain = plan.Request.MinGain
			}
			items = []report.Item{ops.JPEG(pi.Source, o)}
		case "webp":
			o := ops.DefaultWebP()
			o.DryRun, o.Output, o.DeleteSource, o.Overwrite = !plan.Apply, pi.Destinations[0], plan.Request.DeleteSource, plan.Request.Overwrite
			applyThresholds(&o.MinGain, &o.MinPSNR, &o.MinSSIM, plan.Request)
			items = []report.Item{ops.WebP(pi.Source, o)}
		case "resize":
			o := ops.DefaultResize()
			o.Widths, o.OutDir, o.DryRun, o.Replace, o.Overwrite = append([]int(nil), plan.Request.Widths...), plan.Request.OutDir, !plan.Apply, plan.Request.Replace, plan.Request.Overwrite
			items = ops.Resize(pi.Source, o)
		case "none":
			items = []report.Item{{Path: pi.Source, Status: report.StatusSkipped, Code: "no_automatic_strategy", Reason: "no safe same-format optimization"}}
		}
		results[index] = items
	})
	for index, pi := range plan.Items {
		items := results[index]
		for _, it := range items {
			if it.Metrics == nil {
				it.Metrics = map[string]any{}
			}
			it.Metrics["strategy"] = pi.Strategy
			it.Metrics["verification"] = pi.Verification
			it.Metrics["automatic"] = pi.Automatic
			rep.Add(it)
		}
	}
	if !plan.Apply {
		rep.Note("preview only; nothing was written; rerun with --apply to execute this plan")
	}
	return rep
}

func applyThresholds(gain, psnr, ssim *float64, req OptimizeRequest) {
	if req.MinGain > 0 {
		*gain = req.MinGain
	}
	if req.MinPSNR > 0 {
		*psnr = req.MinPSNR
	}
	if ssim != nil && req.MinSSIM > 0 {
		*ssim = req.MinSSIM
	}
}
