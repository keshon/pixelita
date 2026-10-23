package engine

import (
	"context"
	"fmt"

	"github.com/keshon/pixelita/internal/batch"
	"github.com/keshon/pixelita/internal/imgio"
	"github.com/keshon/pixelita/internal/ops"
	"github.com/keshon/pixelita/internal/result"
)

type BatchRequest struct {
	Context  context.Context
	Paths    []string
	FromFile string
	Jobs     int
}

func runFiles(req BatchRequest, exts []string, tool string, dry, overwrite, replace bool, validate func() error, plan func(string) ([]ops.Destination, error), run func(string) []result.Item) (*result.Report, error) {
	if req.Jobs < 0 {
		return nil, fmt.Errorf("jobs must be non-negative")
	}
	if err := validate(); err != nil {
		return nil, &ops.PlanError{Code: "invalid_option", Detail: err.Error()}
	}
	files, err := batch.Roots(req.Paths, req.FromFile, exts...)
	if err != nil {
		return nil, err
	}
	var dests []ops.Destination
	for _, p := range files {
		dests = append(dests, ops.Destination{Source: p})
		if plan != nil {
			d, err := plan(p)
			if err != nil {
				return nil, err
			}
			dests = append(dests, d...)
		}
	}
	if err := ops.PreflightDestinations(dests, overwrite, replace); err != nil {
		return nil, err
	}
	items := make([][]result.Item, len(files))
	batch.Each(len(files), req.Jobs, func(i int) {
		if req.Context != nil && req.Context.Err() != nil {
			items[i] = []result.Item{{Path: files[i], Status: result.StatusFailed, Code: "cancelled", Reason: "cancelled before starting this file"}}
			return
		}
		items[i] = run(files[i])
	})
	rep := result.New(tool, "written", dry)
	for _, group := range items {
		for _, it := range group {
			rep.Add(it)
		}
	}
	return rep, nil
}
func Quant(req BatchRequest, o ops.QuantOptions) (*result.Report, error) {
	return runFiles(req, []string{".png"}, "img-quant", o.DryRun, o.Overwrite, o.Replace, o.Validate, func(p string) ([]ops.Destination, error) {
		return []ops.Destination{{Source: p, Output: chosenOutput(o.OutputPath(p), o.Output)}}, nil
	}, func(p string) []result.Item { return []result.Item{ops.Quant(p, o)} })
}
func JPEG(req BatchRequest, o ops.JPEGOptions) (*result.Report, error) {
	return runFiles(req, []string{".jpg", ".jpeg"}, "img-jpeg", o.DryRun, o.Overwrite, o.Replace, o.Validate, func(p string) ([]ops.Destination, error) {
		return []ops.Destination{{Source: p, Output: chosenOutput(o.OutputPath(p), o.Output)}}, nil
	}, func(p string) []result.Item { return []result.Item{ops.JPEG(p, o)} })
}
func WebP(req BatchRequest, o ops.WebPOptions) (*result.Report, error) {
	return runFiles(req, []string{".png", ".jpg", ".jpeg"}, "img-webp", o.DryRun, o.Overwrite, false, o.Validate, func(p string) ([]ops.Destination, error) {
		return []ops.Destination{{Source: p, Output: chosenOutput(o.OutputPath(p), o.Output)}}, nil
	}, func(p string) []result.Item { return []result.Item{ops.WebP(p, o)} })
}
func Resize(req BatchRequest, o ops.ResizeOptions) (*result.Report, error) {
	return runFiles(req, imgio.Extensions, "img-resize", o.DryRun, o.Overwrite, o.Replace, o.Validate, func(p string) ([]ops.Destination, error) { return ops.PlanResizeDestinations(p, o) }, func(p string) []result.Item { return ops.Resize(p, o) })
}
func Scan(req BatchRequest, o ops.ScanOptions) (*result.Report, error) {
	r, e := runFiles(req, imgio.Extensions, "img-scan", true, false, false, o.Validate, nil, func(p string) []result.Item { return []result.Item{ops.Scan(p, o)} })
	if e == nil {
		ops.ScanSummary(r, o)
	}
	return r, e
}

func chosenOutput(planned, explicit string) string {
	if explicit != "" {
		return explicit
	}
	return planned
}
