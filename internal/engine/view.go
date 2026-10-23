package engine

import (
	"fmt"
	"image"

	"github.com/keshon/pixelita/internal/batch"
	"github.com/keshon/pixelita/internal/imgio"
	"github.com/keshon/pixelita/internal/ops"
	"github.com/keshon/pixelita/internal/result"
)

type LookRequest struct {
	Paths             []string
	Options           ops.LookOptions
	Regions           []image.Rectangle
	Points            []image.Point
	DryRun, Overwrite bool
}

func Look(req LookRequest) (*result.Report, error) {
	if req.Options.Max < 0 || req.Options.Zoom < 0 || req.Options.Zoom > 64 {
		return nil, fmt.Errorf("max must be non-negative and zoom must be 0..64")
	}
	switch req.Options.Background {
	case "checker", "white", "black", "none":
	default:
		return nil, fmt.Errorf("background must be checker, white, black or none")
	}
	if len(req.Points) > 0 && (len(req.Regions) > 0 || req.Options.Out != "") {
		return nil, fmt.Errorf("pixel probes cannot be combined with crops or output")
	}
	paths, err := batch.Roots(req.Paths, "", imgio.Extensions...)
	if err != nil {
		return nil, err
	}
	rep := result.New("pixelita view", "shown", req.DryRun || len(req.Points) > 0)
	if len(req.Points) > 0 {
		for _, p := range paths {
			for _, it := range ops.ProbeItems(p, req.Points) {
				rep.Add(it)
			}
		}
		return rep, nil
	}
	regions := req.Regions
	if len(regions) == 0 {
		regions = []image.Rectangle{{}}
	}
	if len(regions) > 1 && req.Options.Out != "" {
		return nil, fmt.Errorf("multiple crops use generated output paths; omit out")
	}
	options := make([]ops.LookOptions, len(regions))
	var dests []ops.Destination
	for _, p := range paths {
		dests = append(dests, ops.Destination{Source: p})
	}
	for i, r := range regions {
		o := req.Options
		o.Crop = r
		if o.Out == "" {
			o.Out = ops.LookPath(paths, o)
		}
		options[i] = o
		dests = append(dests, ops.Destination{Output: o.Out})
	}
	if err := ops.PreflightDestinations(dests, req.Overwrite || req.DryRun, false); err != nil {
		return nil, err
	}
	for _, o := range options {
		img, items, err := ops.Look(paths, o)
		if err != nil {
			rep.Add(result.Item{Path: paths[0], Status: result.StatusFailed, Code: "view_failed", Error: err.Error()})
			continue
		}
		if !req.DryRun {
			out, size, err := ops.WriteLook(img, o.Out, req.Overwrite)
			if err != nil {
				rep.Add(result.Item{Path: paths[0], Status: result.StatusFailed, Code: "write_failed", Error: err.Error()})
				continue
			}
			for i := range items {
				if items[i].Status == result.StatusFailed {
					continue
				}
				items[i].Output = out
				items[i].Written = true
				items[i].BytesAfter = size
			}
			rep.Note("open %s (%d bytes)", out, size)
		}
		for _, it := range items {
			rep.Add(it)
		}
	}
	return rep, nil
}
