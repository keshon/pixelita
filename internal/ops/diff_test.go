package ops

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestDiffIndependentRGBAndAlphaMetrics(t *testing.T) {
	for _, tc := range []struct {
		name   string
		a, b   color.NRGBA
		metric string
	}{
		{"alpha-only", color.NRGBA{R: 90, A: 255}, color.NRGBA{R: 90, A: 128}, "psnrAlpha"},
		{"invisible-rgb", color.NRGBA{R: 90}, color.NRGBA{R: 180}, "psnr"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			paths := []string{filepath.Join(dir, "a.png"), filepath.Join(dir, "b.png")}
			for i, c := range []color.NRGBA{tc.a, tc.b} {
				im := image.NewNRGBA(image.Rect(0, 0, 8, 8))
				for y := 0; y < 8; y++ {
					for x := 0; x < 8; x++ {
						im.SetNRGBA(x, y, c)
					}
				}
				f, err := os.Create(paths[i])
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(f, im)
				closeErr := f.Close()
				if err != nil {
					t.Fatal(err)
				}
				if closeErr != nil {
					t.Fatal(closeErr)
				}
			}
			item := Diff(paths[0], paths[1], DiffOptions{MinPSNR: 200})
			if item.Status != "failed" {
				t.Fatalf("damage accepted: %+v", item)
			}
			if _, ok := item.Metrics[tc.metric]; !ok {
				t.Fatalf("missing %s: %+v", tc.metric, item)
			}
			if _, err := json.Marshal(item); err != nil {
				t.Fatal(err)
			}
		})
	}
}
