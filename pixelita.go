// Package pixelita exposes the same typed image operations used by the CLI.
// Optimize previews by default. Writing requires Apply; source replacement and
// deletion require their separate explicit flags.
package pixelita

import "github.com/keshon/pixelita/internal/engine"
import "github.com/keshon/pixelita/internal/result"

type InspectRequest = engine.InspectRequest
type OptimizeRequest = engine.OptimizeRequest
type Report = result.Report
type Item = result.Item
type Capability = engine.Capability

func Inspect(req InspectRequest) (*Report, error) { return finish(engine.Inspect(req)) }

func Optimize(req OptimizeRequest) (*Report, error) {
	plan, err := engine.BuildOptimizePlan(req)
	if err != nil {
		return nil, err
	}
	return finish(engine.ExecuteOptimize(plan), nil)
}

type CompareRequest struct {
	A, B             string
	MinPSNR, MinSSIM float64
	StrictSize       bool
}

func Compare(req CompareRequest) (*Report, error) {
	return finish(engine.Compare(engine.CompareRequest{A: req.A, B: req.B, MinPSNR: req.MinPSNR, MinSSIM: req.MinSSIM, StrictSize: req.StrictSize}))
}

// Native consumers need the same accounting that CLI rendering finalizes.
func finish(rep *Report, err error) (*Report, error) {
	if rep != nil {
		rep.Finish()
	}
	return rep, err
}

func Capabilities() []Capability { return engine.Capabilities() }
