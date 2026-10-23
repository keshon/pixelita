package engine

import (
	"os"

	"github.com/keshon/pixelita/internal/imgio"
	"github.com/keshon/pixelita/internal/ops"
	"github.com/keshon/pixelita/internal/result"
)

// Selection measures only the explicitly permitted formats. Apply re-encodes
// the winner and enforces the same thresholds again before publishing it.
func executeBest(pi PlanItem, p *Plan) result.Item {
	raw, err := os.ReadFile(pi.Source)
	if err != nil {
		return result.Item{Path: pi.Source, Status: result.StatusFailed, Error: err.Error()}
	}
	h, err := imgio.ReadHeader(raw)
	if err != nil {
		return result.Item{Path: pi.Source, Status: result.StatusFailed, Error: err.Error()}
	}
	same := func(apply bool) result.Item {
		if h.Format == "png" {
			o := ops.DefaultQuant()
			o.DryRun = !apply
			o.Output = pi.Destinations[0]
			o.Overwrite = p.Request.Overwrite
			applyThresholds(&o.MinGain, &o.MinPSNR, &o.MinSSIM, p.Request)
			return ops.Quant(pi.Source, o)
		}
		o := ops.DefaultJPEG()
		o.DryRun = !apply
		o.Output = pi.Destinations[0]
		o.Overwrite = p.Request.Overwrite
		if p.Request.MinGain != nil {
			o.MinGain = *p.Request.MinGain
		}
		return ops.JPEG(pi.Source, o)
	}
	convert := func(apply bool) result.Item {
		o := ops.DefaultWebP()
		o.SkipPalette = false
		o.DryRun = !apply
		o.Output = pi.Destinations[1]
		o.Overwrite = p.Request.Overwrite
		applyThresholds(&o.MinGain, &o.MinPSNR, &o.MinSSIM, p.Request)
		return ops.WebP(pi.Source, o)
	}
	a, b := same(false), convert(false)
	decisions := []result.CandidateDecision{{Strategy: "same-format", Status: a.Status, Code: a.Code, Bytes: a.BytesAfter, Metrics: a.Metrics, Verification: a.Verification}, {Strategy: "webp", Status: b.Status, Code: b.Code, Bytes: b.BytesAfter, Metrics: b.Metrics, Verification: b.Verification}}
	chosen := a
	selected := "same-format"
	if b.Status == result.StatusWould && (a.Status != result.StatusWould || b.BytesAfter < a.BytesAfter) {
		chosen = b
		selected = "webp"
	}
	if chosen.Status == result.StatusWould && p.Apply {
		if selected == "webp" {
			chosen = convert(true)
		} else {
			chosen = same(true)
		}
	}
	chosen.Candidates = decisions
	if chosen.Metrics == nil {
		chosen.Metrics = map[string]any{}
	}
	chosen.Metrics["selectedCandidate"] = selected
	return chosen
}
