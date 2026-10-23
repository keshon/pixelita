package engine

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/keshon/pixelita/internal/imgio"
	"github.com/keshon/pixelita/internal/ops"
	"github.com/keshon/pixelita/internal/result"
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
		if pr.Items[i].Status != result.StatusWould || ar.Items[i].Status != result.StatusDone {
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
	if len(rep.Items) != 1 || rep.Items[0].Code != "no_automatic_strategy" || rep.Items[0].Status != result.StatusSkipped {
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

func TestViewUsesBoundedDefaultUnlessMaxIsExplicit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wide.png")
	img := image.NewNRGBA(image.Rect(0, 0, 1600, 800))
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

	bounded, err := View(ViewRequest{Paths: []string{path}, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := bounded.Items[0].Str("to"); got != "1400x700" {
		t.Fatalf("view without Max produced %s, want the default 1400x700", got)
	}

	native := 0
	unbounded, err := View(ViewRequest{Paths: []string{path}, Max: &native, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := unbounded.Items[0].Str("to"); got != "1600x800" {
		t.Fatalf("view with Max=0 produced %s, want native 1600x800", got)
	}

	existing := filepath.Join(dir, "already-there.png")
	if err := os.WriteFile(existing, []byte("leave me alone"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := View(ViewRequest{Paths: []string{path}, Out: existing, DryRun: true}); err != nil {
		t.Fatalf("dry-run rejected an existing destination it will not write: %v", err)
	}
}

func TestExplicitZeroThresholds(t *testing.T) {
	z := 0.0
	req := OptimizeRequest{MinGain: &z, MinPSNR: &z, MinSSIM: &z}
	a, b, c := 10.0, 30.0, 0.9
	applyThresholds(&a, &b, &c, req)
	if a != 0 || b != 0 || c != 0 {
		t.Fatal("zero did not disable thresholds")
	}
	a, b, c = 10, 30, 0.9
	applyThresholds(&a, &b, &c, OptimizeRequest{})
	if a != 10 || b != 30 || c != 0.9 {
		t.Fatal("omitted thresholds replaced defaults")
	}
}

func TestOptimizeVariantsAndVerificationWorkflow(t *testing.T) {
	src := filepath.Join(t.TempDir(), "source.png")
	writePNG(t, src)
	out := t.TempDir()
	plan, err := BuildOptimizePlan(OptimizeRequest{Paths: []string{src}, OutDir: out, Format: "webp", Widths: []int{32}, Apply: true})
	if err != nil {
		t.Fatal(err)
	}
	rep := ExecuteOptimize(plan)
	if len(rep.Items) != 1 || rep.Items[0].Status != result.StatusDone {
		t.Fatalf("variant: %+v", rep.Items)
	}
	result := rep.Items[0].Output
	raw, err := os.ReadFile(result)
	if err != nil {
		t.Fatal(err)
	}
	h, err := imgio.ReadHeader(raw)
	if err != nil || h.Format != "webp" || h.Width != 32 {
		t.Fatalf("variant header: %+v %v", h, err)
	}
	compared, err := Compare(CompareRequest{A: src, B: result, Worst: 2, Tile: 16, Levels: true})
	if err != nil || len(compared.Items) != 2 {
		t.Fatalf("compare: %+v %v", compared, err)
	}
	if compared.Items[0].Str("resampled") == "" {
		t.Fatal("regional resampling not disclosed")
	}
	_, err = Look(LookRequest{Paths: []string{src, result}, Options: ops.DefaultLook(), Regions: []image.Rectangle{image.Rect(0, 0, 8, 8)}, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
}
func TestPlanRejectsChangedSource(t *testing.T) {
	src := filepath.Join(t.TempDir(), "source.png")
	writePNG(t, src)
	p, err := BuildOptimizePlan(OptimizeRequest{Paths: []string{src}, Apply: true})
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(src, []byte("changed"), 0600)
	r := ExecuteOptimize(p)
	if r.Items[0].Code != "source_changed" {
		t.Fatalf("stale plan executed: %+v", r.Items)
	}
}
func TestAutoSelectionAndExactPixels(t *testing.T) {
	src := filepath.Join(t.TempDir(), "source.png")
	writePNG(t, src)
	p, err := BuildOptimizePlan(OptimizeRequest{Paths: []string{src}, Format: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	r := ExecuteOptimize(p)
	if len(r.Items) != 1 || len(r.Items[0].Candidates) != 2 {
		t.Fatalf("missing decisions: %+v", r)
	}
	p, err = BuildOptimizePlan(OptimizeRequest{Paths: []string{src}, ExactPixels: true})
	if err != nil {
		t.Fatal(err)
	}
	if r := ExecuteOptimize(p); r.Items[0].Status != result.StatusSkipped {
		t.Fatal("exact mode quantized a PNG")
	}
}
func TestJPEGVerifiedIdentity(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.jpg")
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetNRGBA(x, y, color.NRGBA{uint8(x * 3), uint8(y * 3), 80, 255})
		}
	}
	f, err := os.Create(src)
	if err != nil {
		t.Fatal(err)
	}
	err = jpeg.Encode(f, img, &jpeg.Options{Quality: 90})
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	zero := 0.0
	p, err := BuildOptimizePlan(OptimizeRequest{Paths: []string{src}, Apply: true, ExactPixels: true, MinGain: &zero})
	if err != nil {
		t.Fatal(err)
	}
	r := ExecuteOptimize(p)
	if r.Items[0].Status != result.StatusDone {
		t.Fatalf("JPEG: %+v", r.Items)
	}
	c, err := Compare(CompareRequest{A: src, B: r.Items[0].Output, MinPSNR: 200})
	if err != nil || c.Items[0].Status != result.StatusDone {
		t.Fatalf("JPEG identity: %+v %v", c, err)
	}
}
func TestViewCannotOverwriteInput(t *testing.T) {
	src := filepath.Join(t.TempDir(), "source.png")
	writePNG(t, src)
	_, err := View(ViewRequest{Paths: []string{src}, Out: src, Overwrite: true})
	if err == nil {
		t.Fatal("preview may overwrite input")
	}
}

func TestCancellationKeepsSourcesAndReportsQueuedFiles(t *testing.T) {
	src := filepath.Join(t.TempDir(), "source.png")
	writePNG(t, src)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	o := ops.DefaultQuant()
	r, err := Quant(BatchRequest{Paths: []string{src}, Context: ctx}, o)
	if err != nil {
		t.Fatal(err)
	}
	if r.Items[0].Code != "cancelled" {
		t.Fatalf("queued work ran: %+v", r.Items)
	}
	if _, err := os.Stat(o.OutputPath(src)); !os.IsNotExist(err) {
		t.Fatal("cancelled operation wrote an output")
	}
}

func TestSpecialistExplicitOutputIsPreflighted(t *testing.T) {
	src := filepath.Join(t.TempDir(), "source.png")
	writePNG(t, src)
	o := ops.DefaultQuant()
	o.Output = src
	o.Overwrite = true
	if _, err := Quant(BatchRequest{Paths: []string{src}}, o); err == nil {
		t.Fatal("explicit output bypassed source protection")
	}
}
