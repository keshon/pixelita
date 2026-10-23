package engine

import (
	"context"
	"crypto/sha256"
	"image"
	"os"
	"sort"
	"strings"

	"github.com/keshon/pixelita/internal/batch"
	"github.com/keshon/pixelita/internal/imgio"
	"github.com/keshon/pixelita/internal/ops"
	"github.com/keshon/pixelita/internal/result"
)

type OptimizeRequest struct {
	Context          context.Context
	Paths            []string
	OutDir           string
	Apply            bool
	Overwrite        bool
	Replace          bool
	DeleteSource     bool
	Format           string
	ExactPixels      bool
	PreserveMetadata bool
	Widths           []int
	MinGain          *float64
	MinPSNR          *float64
	MinSSIM          *float64
	Jobs             int
}

type InspectRequest struct {
	Context context.Context
	Paths   []string
	Measure bool
	Jobs    int
}

func Inspect(req InspectRequest) (*result.Report, error) {
	o := ops.DefaultScan()
	o.Quick = !req.Measure
	rep, err := Scan(BatchRequest{Paths: req.Paths, Jobs: req.Jobs, Context: req.Context}, o)
	if err == nil {
		rep.Tool = "pixelita inspect"
		rep.Verb = "inspected"
	}
	return rep, err
}

type ViewRequest struct {
	Regions                []image.Rectangle
	Points                 []image.Point
	Zoom                   int
	Stats, Across, Stretch bool
	Paths                  []string
	Out                    string
	// Max is optional. Nil uses Pixelita's bounded preview default; a pointer
	// to zero explicitly preserves the source dimensions.
	Max       *int
	DryRun    bool
	Overwrite bool
}

func View(req ViewRequest) (*result.Report, error) {
	o := ops.DefaultLook()
	if req.Max != nil {
		o.Max = *req.Max
	} else if req.Zoom > 1 {
		o.Max = 0
	}
	o.Out = req.Out
	o.Zoom = req.Zoom
	o.Stats = req.Stats
	o.Across = req.Across
	o.Stretch = req.Stretch
	return Look(LookRequest{Paths: req.Paths, Options: o, Regions: req.Regions, Points: req.Points, DryRun: req.DryRun, Overwrite: req.Overwrite})
}

type PlanItem struct {
	SourceHash   [32]byte
	SourceBytes  int64
	SkipCode     string
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
	if err := validateOptimize(req); err != nil {
		return nil, err
	}
	if req.Format == "" {
		req.Format = "keep"
	}
	if req.Format != "keep" && req.Format != "webp" && req.Format != "auto" {
		return nil, &ops.PlanError{Code: "invalid_format", Detail: "format must be keep, webp or auto"}
	}

	if req.DeleteSource && req.Format != "webp" {
		return nil, &ops.PlanError{Code: "delete_source_requires_webp", Detail: "source deletion is only valid with explicit webp conversion"}
	}
	files, err := batch.Roots(req.Paths, "", imgio.Extensions...)
	if err != nil {
		return nil, err
	}
	p := &Plan{Intent: "optimize", Apply: req.Apply, Request: req,
		Permissions: map[string]bool{"overwrite": req.Overwrite, "replace": req.Replace, "deleteSource": req.DeleteSource}}
	var destinations []ops.Destination
	for _, path := range files {
		destinations = append(destinations, ops.Destination{Source: path})
	}
	for _, path := range files {
		if req.Context != nil && req.Context.Err() != nil {
			return nil, req.Context.Err()
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, &ops.PlanError{Code: "input_unavailable", Path: path, Detail: err.Error()}
		}
		head, err := imgio.ReadHeader(raw)
		if err != nil {
			return nil, &ops.PlanError{Code: "unreadable_input", Path: path, Detail: err.Error()}
		}
		item := PlanItem{Source: path, SourceHash: sha256.Sum256(raw), SourceBytes: int64(len(raw))}
		switch {
		case (req.ExactPixels || req.PreserveMetadata) && head.Format != "jpeg":
			item.SkipCode = "preservation_unavailable"
			item.Strategy = "none"
			item.Verification = "none"
		case len(req.Widths) > 0:
			cap, _ := capability("resize")
			if !supportsFormat(cap, head.Format) {
				return nil, &ops.PlanError{Code: "unsupported_strategy_input", Path: path, Detail: "resize variants accept png or jpeg"}
			}
			item.Strategy, item.Verification, item.Automatic = cap.ID, cap.Verification, cap.Automatic
			o := ops.DefaultResize()
			o.Widths, o.OutDir, o.Replace, o.Format = append([]int(nil), req.Widths...), req.OutDir, req.Replace, req.Format
			sizes, _ := o.Targets(head.Width, head.Height)
			format := strings.ToLower(head.Format)
			ext := ".png"
			if format == "jpeg" {
				ext = ".jpg"
			}
			if req.Format == "webp" {
				ext = ".webp"
			}
			for _, size := range sizes {
				out := o.OutputPath(path, ext, size[0], size[1])
				item.Destinations = append(item.Destinations, out)
				destinations = append(destinations, ops.Destination{Source: path, Output: out})
			}
		case req.Format == "auto":
			cap, ok := automaticCapability(head.Format)
			if !ok {
				item.Strategy = "none"
				item.Verification = "none"
				break
			}
			item.Strategy = "best"
			item.Verification = "candidate-dependent"
			if cap.ID == "png-palette" {
				o := ops.DefaultQuant()
				o.OutDir = req.OutDir
				item.Destinations = append(item.Destinations, o.OutputPath(path))
			} else {
				o := ops.DefaultJPEG()
				o.OutDir = req.OutDir
				item.Destinations = append(item.Destinations, o.OutputPath(path))
			}
			o := ops.DefaultWebP()
			o.OutDir = req.OutDir
			item.Destinations = append(item.Destinations, o.OutputPath(path))
			for _, dest := range item.Destinations {
				destinations = append(destinations, ops.Destination{Source: path, Output: dest})
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

func ExecuteOptimize(plan *Plan) *result.Report {
	rep := result.New("pixelita optimize", "optimized", !plan.Apply)
	results := make([][]result.Item, len(plan.Items))
	batch.Each(len(plan.Items), plan.Request.Jobs, func(index int) {
		pi := plan.Items[index]
		if plan.Request.Context != nil && plan.Request.Context.Err() != nil {
			results[index] = []result.Item{{Path: pi.Source, Status: result.StatusFailed, Code: "cancelled", Reason: "cancelled before starting this file"}}
			return
		}
		current, err := os.ReadFile(pi.Source)
		if err != nil || sha256.Sum256(current) != pi.SourceHash {
			results[index] = []result.Item{{Path: pi.Source, Status: result.StatusFailed, Code: "source_changed", Reason: "source changed since planning; create a new plan"}}
			return
		}
		var items []result.Item
		switch pi.Strategy {
		case "png-palette":
			o := ops.DefaultQuant()
			o.DryRun, o.Output, o.Replace, o.Overwrite = !plan.Apply, pi.Destinations[0], plan.Request.Replace, plan.Request.Overwrite
			applyThresholds(&o.MinGain, &o.MinPSNR, &o.MinSSIM, plan.Request)
			items = []result.Item{ops.Quant(pi.Source, o)}
		case "jpeg-huffman":
			o := ops.DefaultJPEG()
			o.DryRun, o.Output, o.Replace, o.Overwrite = !plan.Apply, pi.Destinations[0], plan.Request.Replace, plan.Request.Overwrite
			if plan.Request.MinGain != nil {
				o.MinGain = *plan.Request.MinGain
			}
			items = []result.Item{ops.JPEG(pi.Source, o)}
		case "webp":
			o := ops.DefaultWebP()
			o.DryRun, o.Output, o.DeleteSource, o.Overwrite = !plan.Apply, pi.Destinations[0], plan.Request.DeleteSource, plan.Request.Overwrite
			applyThresholds(&o.MinGain, &o.MinPSNR, &o.MinSSIM, plan.Request)
			items = []result.Item{ops.WebP(pi.Source, o)}
		case "resize":
			o := ops.DefaultResize()
			o.Widths, o.OutDir, o.DryRun, o.Replace, o.Overwrite, o.Format = append([]int(nil), plan.Request.Widths...), plan.Request.OutDir, !plan.Apply, plan.Request.Replace, plan.Request.Overwrite, plan.Request.Format
			items = ops.Resize(pi.Source, o)
		case "best":
			items = []result.Item{executeBest(pi, plan)}
		case "none":
			code := pi.SkipCode
			if code == "" {
				code = "no_automatic_strategy"
			}
			items = []result.Item{{Path: pi.Source, BytesBefore: pi.SourceBytes, Status: result.StatusSkipped, Code: code, Reason: "no available strategy satisfies the requested constraints"}}
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
	if req.MinGain != nil {
		*gain = *req.MinGain
	}
	if req.MinPSNR != nil {
		*psnr = *req.MinPSNR
	}
	if ssim != nil && req.MinSSIM != nil {
		*ssim = *req.MinSSIM
	}
}

func validateOptimize(req OptimizeRequest) error {
	if req.Replace && req.OutDir != "" {
		return &ops.PlanError{Code: "conflicting_permissions", Detail: "replace cannot be combined with out-dir"}
	}
	var p ops.CandidatePolicy
	applyThresholds(&p.MinGain, &p.MinPSNR, &p.MinSSIM, req)
	if err := p.Validate(); err != nil {
		return &ops.PlanError{Code: "invalid_threshold", Detail: err.Error()}
	}
	if req.Jobs < 0 {
		return &ops.PlanError{Code: "invalid_workers", Detail: "jobs must be non-negative"}
	}
	for _, w := range req.Widths {
		if w <= 0 {
			return &ops.PlanError{Code: "invalid_dimensions", Detail: "widths must be positive"}
		}
	}
	if len(req.Widths) > 0 && (p.MinGain > 0 || p.MinPSNR > 0 || p.MinSSIM > 0) {
		return &ops.PlanError{Code: "unsupported_threshold", Detail: "resize does not support optimization thresholds; compare the result separately"}
	}
	if len(req.Widths) > 0 && (req.Replace || req.DeleteSource || req.Format == "auto") {
		return &ops.PlanError{Code: "conflicting_options", Detail: "variants require keep or webp and cannot replace or delete a source"}
	}
	if (req.ExactPixels || req.PreserveMetadata) && (len(req.Widths) > 0 || req.Format == "webp" || req.Format == "auto") {
		return &ops.PlanError{Code: "conflicting_options", Detail: "exact pixels and metadata preservation require same-format optimization"}
	}
	if req.Format == "auto" && (req.Replace || req.DeleteSource) {
		return &ops.PlanError{Code: "conflicting_permissions", Detail: "automatic format selection writes separate outputs"}
	}
	if req.Replace && req.Format == "webp" {
		return &ops.PlanError{Code: "conflicting_permissions", Detail: "WebP conversion uses delete-source, not replace"}
	}
	return nil
}
