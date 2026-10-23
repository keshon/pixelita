package ops

import (
	"fmt"
	"math"
)

func (o QuantOptions) Validate() error {
	if o.Replace && o.OutDir != "" {
		return fmt.Errorf("replace cannot be combined with out-dir")
	}
	if err := (CandidatePolicy{o.MinGain, o.MinPSNR, o.MinSSIM}).Validate(); err != nil {
		return err
	}
	if o.Colors < 2 || o.Colors > 256 || o.Effort < 1 || o.Effort > 10 || math.IsNaN(o.Dither) || math.IsInf(o.Dither, 0) || o.Dither < 0 || o.Dither > 1 {
		return fmt.Errorf("colors must be 2..256, effort 1..10, dither finite and 0..1")
	}
	return nil
}
func (o WebPOptions) Validate() error {
	if err := (CandidatePolicy{o.MinGain, o.MinPSNR, o.MinSSIM}).Validate(); err != nil {
		return err
	}
	if o.Quality < 1 || o.Quality > 100 {
		return fmt.Errorf("quality must be 1..100")
	}
	if !o.KeepOriginal {
		return fmt.Errorf("keep-original=false is unsupported; use delete-source explicitly")
	}
	_, err := o.EncoderOptions()
	return err
}
func (o JPEGOptions) Validate() error {
	if o.Replace && o.OutDir != "" {
		return fmt.Errorf("replace cannot be combined with out-dir")
	}
	return (CandidatePolicy{MinGain: o.MinGain}).Validate()
}
func (o ResizeOptions) Validate() error {
	if o.Replace && o.OutDir != "" {
		return fmt.Errorf("replace cannot be combined with out-dir")
	}
	if o.Width < 0 || o.Height < 0 || o.MaxWidth < 0 || o.MaxHeight < 0 || o.Scale < 0 || math.IsNaN(o.Scale) || math.IsInf(o.Scale, 0) {
		return fmt.Errorf("sizes and scale must be finite and non-negative")
	}
	modes := 0
	for _, set := range []bool{o.Width > 0 || o.Height > 0, o.MaxWidth > 0 || o.MaxHeight > 0, o.Scale > 0, len(o.Widths) > 0} {
		if set {
			modes++
		}
	}
	if modes != 1 {
		return fmt.Errorf("choose one sizing mode: box, bounds, scale or widths")
	}
	for _, w := range o.Widths {
		if w <= 0 {
			return fmt.Errorf("widths must be positive")
		}
	}
	if o.Replace && len(o.Widths) > 0 {
		return fmt.Errorf("resize variants cannot replace a source")
	}
	if o.JPEGQuality < 1 || o.JPEGQuality > 100 {
		return fmt.Errorf("jpeg-quality must be 1..100")
	}
	switch o.Format {
	case "keep", "png", "jpeg", "webp":
	default:
		return fmt.Errorf("format must be keep, png, jpeg or webp")
	}
	switch o.Fit {
	case "inside", "outside", "cover", "exact":
	default:
		return fmt.Errorf("fit must be inside, outside, cover or exact")
	}
	return nil
}
func (o ScanOptions) Validate() error {
	q := DefaultQuant()
	q.Colors = o.Colors
	q.Effort = o.Effort
	q.MinGain = o.MinGain
	q.MinPSNR = o.MinPSNR
	q.MinSSIM = o.MinSSIM
	if err := q.Validate(); err != nil {
		return err
	}
	if o.WebPQuality < 1 || o.WebPQuality > 100 {
		return fmt.Errorf("webp-quality must be 1..100")
	}
	for _, w := range o.DisplayWidths {
		if w <= 0 {
			return fmt.Errorf("display widths must be positive")
		}
	}
	return nil
}
