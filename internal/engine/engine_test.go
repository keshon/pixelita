package engine

import (
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/keshon/pixelita/internal/ops"
	"github.com/keshon/pixelita/internal/report"
)

func writePNG(t *testing.T, path string) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	colors := []color.NRGBA{{R: 15, G: 30, B: 45, A: 255}, {R: 220, G: 50, B: 40, A: 255}, {R: 40, G: 190, B: 90, A: 255}, {R: 70, G: 80, B: 210, A: 255}}
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetNRGBA(x, y, colors[(x/8+y/8)%len(colors)])
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPlanOrderingAndApplyEquivalence(t *testing.T) {
	dir, out := t.TempDir(), filepath.Join(t.TempDir(), "out")
	a, b := filepath.Join(dir, "b.png"), filepath.Join(dir, "a.png")
	writePNG(t, a)
	writePNG(t, b)
	req := OptimizeRequest{Paths: []string{a, b}, OutDir: out}
	preview, err := BuildOptimizePlan(req)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(preview.Items[0].Source) != "a.png" || filepath.Base(preview.Items[1].Source) != "b.png" {
		t.Fatalf("not sorted: %#v", preview.Items)
	}
	cap, ok := capability(preview.Items[0].Strategy)
	if !ok || !cap.Automatic || cap.Verification != preview.Items[0].Verification {
		t.Fatalf("plan diverged from catalog: plan=%#v capability=%#v", preview.Items[0], cap)
	}
	pr := ExecuteOptimize(preview)
	pr.Finish()
	req.Apply = true
	applied, err := BuildOptimizePlan(req)
	if err != nil {
		t.Fatal(err)
	}
	ar := ExecuteOptimize(applied)
	ar.Finish()
	if len(pr.Items) != len(ar.Items) {
		t.Fatalf("item counts differ: %d %d", len(pr.Items), len(ar.Items))
	}
	for i := range pr.Items {
		if pr.Items[i].Output != ar.Items[i].Output || pr.Items[i].Str("strategy") != ar.Items[i].Str("strategy") {
			t.Fatalf("plan/apply differ: %#v %#v", pr.Items[i], ar.Items[i])
		}
		if pr.Items[i].Status != report.StatusWould || ar.Items[i].Status != report.StatusDone {
			t.Fatalf("plan/apply status mismatch: %#v %#v", pr.Items[i], ar.Items[i])
		}
	}
}

func TestPlanDetectsBatchCollisionAndExistingDestination(t *testing.T) {
	root, out := t.TempDir(), t.TempDir()
	left, right := filepath.Join(root, "left"), filepath.Join(root, "right")
	if err := os.MkdirAll(left, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(right, 0o755); err != nil {
		t.Fatal(err)
	}
	a, b := filepath.Join(left, "same.png"), filepath.Join(right, "same.png")
	writePNG(t, a)
	writePNG(t, b)
	_, err := BuildOptimizePlan(OptimizeRequest{Paths: []string{a, b}, OutDir: out})
	var pe *ops.PlanError
	if !errors.As(err, &pe) || pe.Code != "output_collision" {
		t.Fatalf("collision = %#v", err)
	}

	one := filepath.Join(t.TempDir(), "one.png")
	writePNG(t, one)
	q := ops.DefaultQuant()
	q.OutDir = out
	if err := os.WriteFile(q.OutputPath(one), []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = BuildOptimizePlan(OptimizeRequest{Paths: []string{one}, OutDir: out})
	if !errors.As(err, &pe) || pe.Code != "destination_exists" {
		t.Fatalf("existing = %#v", err)
	}
}

func TestCapabilitiesAreStableCopies(t *testing.T) {
	a, b := Capabilities(), Capabilities()
	if len(a) < 4 || !reflect.DeepEqual(a, b) {
		t.Fatalf("catalog unstable: %#v %#v", a, b)
	}
	a[0].ID = "changed"
	a[0].Intents[0] = "changed"
	if Capabilities()[0].ID == "changed" || Capabilities()[0].Intents[0] == "changed" {
		t.Fatal("catalog leaked mutable slice")
	}
}

func TestOptimizeReportsUnsupportedAutomaticFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.gif")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := gif.Encode(f, image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black}), nil); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	plan, err := BuildOptimizePlan(OptimizeRequest{Paths: []string{path}})
	if err != nil {
		t.Fatal(err)
	}
	rep := ExecuteOptimize(plan)
	rep.Finish()
	if len(rep.Items) != 1 || rep.Items[0].Code != "no_automatic_strategy" || rep.Items[0].Status != report.StatusSkipped {
		t.Fatalf("unsupported outcome = %#v", rep.Items)
	}
}

func TestOptimizeClassifiesCorruptInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.png")
	if err := os.WriteFile(path, []byte("not a png"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := BuildOptimizePlan(OptimizeRequest{Paths: []string{path}})
	var pe *ops.PlanError
	if !errors.As(err, &pe) || pe.Code != "unreadable_input" {
		t.Fatalf("corrupt input = %#v", err)
	}
}
