package engine

import (
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/keshon/pixelita/internal/batch"
	"github.com/keshon/pixelita/internal/imgio"
	"github.com/keshon/pixelita/internal/ops"
	"github.com/keshon/pixelita/internal/result"
)

type CompareRequest struct {
	A, B             string
	MinPSNR, MinSSIM float64
	StrictSize       bool
	Regions          []image.Rectangle
	Worst, Tile      int
	By               ops.Ranking
	Levels           bool
	Out              string
	Overwrite        bool
	Amplify          float64
	Jobs             int
}

func Compare(req CompareRequest) (*result.Report, error) {
	if err := (ops.CandidatePolicy{MinPSNR: req.MinPSNR, MinSSIM: req.MinSSIM}).Validate(); err != nil {
		return nil, err
	}
	if req.Worst < 0 || req.Tile < 0 || req.Jobs < 0 || math.IsNaN(req.Amplify) || math.IsInf(req.Amplify, 0) || req.Amplify < 0 {
		return nil, fmt.Errorf("invalid comparison options")
	}
	if req.Tile == 0 {
		req.Tile = 256
	}
	if req.By == "" {
		req.By = ops.RankSSIM
	}
	if req.Amplify == 0 {
		req.Amplify = 8
	}
	if req.By != ops.RankSSIM && req.By != ops.RankPSNR && req.By != ops.RankLevels {
		return nil, fmt.Errorf("by must be ssim, psnr or levels")
	}
	for _, r := range req.Regions {
		if r.Empty() || r.Min.X < 0 || r.Min.Y < 0 {
			return nil, fmt.Errorf("regions must have positive dimensions and non-negative coordinates")
		}
	}
	regional := req.Worst > 0 || len(req.Regions) > 0 || req.Levels
	if req.Worst > 0 && len(req.Regions) > 0 {
		return nil, fmt.Errorf("choose worst regions or explicit crops")
	}
	if regional && req.Out != "" {
		return nil, fmt.Errorf("difference maps and region reports require separate requests")
	}
	pairs, err := pairUp(req.A, req.B)
	if err != nil {
		return nil, err
	}
	var destinations []ops.Destination
	for _, p := range pairs {
		destinations = append(destinations, ops.Destination{Source: p.a}, ops.Destination{Source: p.b})
	}
	outputs := make([]string, len(pairs))
	for i, p := range pairs {
		if req.Out != "" {
			outputs[i] = req.Out
			if len(pairs) > 1 {
				outputs[i] = filepath.Join(req.Out, stem(p.a)+".png")
			}
			destinations = append(destinations, ops.Destination{Output: outputs[i]})
		}
	}
	if err := ops.PreflightDestinations(destinations, req.Overwrite, false); err != nil {
		return nil, err
	}
	results := make([][]result.Item, len(pairs))
	batch.Each(len(pairs), req.Jobs, func(i int) {
		p := pairs[i]
		if !regional {
			o := ops.DefaultDiff()
			o.MinPSNR = req.MinPSNR
			o.MinSSIM = req.MinSSIM
			o.StrictSize = req.StrictSize
			o.Out = outputs[i]
			o.Overwrite = req.Overwrite
			o.Amplify = req.Amplify
			results[i] = []result.Item{ops.Diff(p.a, p.b, o)}
			return
		}
		fail := func(err error) {
			results[i] = []result.Item{{Path: p.a, Output: p.b, Status: result.StatusFailed, Code: "comparison_failed", Error: err.Error()}}
		}
		a, _, _, err := imgio.Load(p.a)
		if err != nil {
			fail(err)
			return
		}
		b, _, _, err := imgio.Load(p.b)
		if err != nil {
			fail(err)
			return
		}
		mismatch := a.Bounds().Size() != b.Bounds().Size()
		if mismatch && req.StrictSize {
			fail(fmt.Errorf("image dimensions differ"))
			return
		}
		if req.Worst > 0 {
			results[i], err = ops.WorstRegions(p.a, p.b, req.Worst, req.Tile, req.By, req.Levels || req.By == ops.RankLevels)
		} else {
			rects := req.Regions
			if len(rects) == 0 {
				rects = []image.Rectangle{a.Bounds()}
			}
			results[i], err = ops.MeasureRegions(p.a, p.b, rects, req.Levels)
		}
		if err != nil {
			fail(err)
			return
		}
		for j := range results[i] {
			it := &results[i][j]
			if it.Status == result.StatusFailed {
				continue
			}
			psnr, ok := it.Num("psnr")
			if !ok {
				psnr = math.Inf(1)
			}
			ssim, has := it.Num("ssim")
			alpha, hasAlpha := it.Num("psnrAlpha")
			if !hasAlpha {
				alpha = math.Inf(1)
			}
			v := ops.EvaluateCandidate(ops.Candidate{PSNR: psnr, HasPSNR: true, PSNRAlpha: alpha, HasPSNRAlpha: true, SSIM: ssim, HasSSIM: has}, ops.CandidatePolicy{MinPSNR: req.MinPSNR, MinSSIM: req.MinSSIM})
			if !v.Accept {
				it.Status = result.StatusFailed
				it.Code = v.Code
				it.Reason = "region below requested fidelity"
			}
			if mismatch {
				it.Metrics["resampled"] = "b to a"
			}
		}
	})
	rep := result.New("pixelita compare", "compared", false)
	for _, items := range results {
		for _, it := range items {
			rep.Add(it)
		}
	}
	return rep, nil
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
		if _, err := batch.Collect([]string{a, b}, imgio.Extensions...); err != nil {
			return nil, err
		}
		return []pair{{a, b}}, nil
	}
	if ai.IsDir() != bi.IsDir() {
		return nil, fmt.Errorf("compare two files or two directories, not one of each")
	}

	left, err := batch.Collect([]string{a}, imgio.Extensions...)
	if err != nil {
		return nil, err
	}
	right, err := batch.Collect([]string{b}, imgio.Extensions...)
	if err != nil {
		return nil, err
	}
	// Each output gets its own source, so responsive variants are all checked.
	// Relative paths disambiguate repeated basenames; flat ambiguous collections
	// fail instead of silently choosing whichever file was indexed last.
	var pairs []pair
	for _, q := range right {
		best := ""
		score := -1
		ambiguous := false
		qr, _ := filepath.Rel(b, q)
		for _, p := range left {
			if sameFile(p, q) {
				continue
			}
			if stem(p) != stem(q) && stem(p) != stripSuffix(stem(q)) && stripSuffix(stem(p)) != stripSuffix(stem(q)) {
				continue
			}
			rank := 0
			if stem(p) == stem(q) {
				rank += 4
			}
			if filepath.Ext(p) == filepath.Ext(q) {
				rank++
			}
			pr, _ := filepath.Rel(a, p)
			if filepath.Dir(pr) == filepath.Dir(qr) {
				rank += 8
			}
			if rank > score {
				best = p
				score = rank
				ambiguous = false
			} else if rank == score {
				ambiguous = true
			}
		}
		if ambiguous {
			return nil, fmt.Errorf("ambiguous source for %s; compare explicit files or preserve relative directories", q)
		}
		if best != "" {
			pairs = append(pairs, pair{best, q})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].a == pairs[j].a {
			return pairs[i].b < pairs[j].b
		}
		return pairs[i].a < pairs[j].a
	})
	if len(pairs) == 0 {
		return nil, fmt.Errorf("no file names in common between %s and %s", a, b)
	}
	return pairs, nil
}

func stem(p string) string {
	base := filepath.Base(p)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// stripSuffix removes the output suffixes this toolkit's own tools produce,
// so a verifier pairs sources with their products. Exact stems always win
// (see pairUp): this is only the fallback.
func stripSuffix(s string) string {
	if strings.HasSuffix(s, "-min") {
		return strings.TrimSuffix(s, "-min")
	}
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '-' {
			tail := s[i+1:]
			if isDims(tail) {
				return s[:i]
			}
			break
		}
		if !(s[i] >= '0' && s[i] <= '9' || s[i] == 'x' || s[i] == 'w') {
			break
		}
	}
	return s
}

// isDims reports -320w or -800x600 tails.
func isDims(tail string) bool {
	if strings.HasSuffix(tail, "w") {
		tail = strings.TrimSuffix(tail, "w")
	} else {
		parts := strings.Split(tail, "x")
		if len(parts) != 2 {
			return false
		}
		tail = parts[0] + parts[1]
	}
	if tail == "" {
		return false
	}
	for i := 0; i < len(tail); i++ {
		if tail[i] < '0' || tail[i] > '9' {
			return false
		}
	}
	return true
}

func sameFile(a, b string) bool {
	aa, err := filepath.Abs(a)
	if err != nil {
		return a == b
	}
	bb, err := filepath.Abs(b)
	if err != nil {
		return a == b
	}
	return aa == bb
}
