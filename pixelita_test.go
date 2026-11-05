package pixelita_test

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/keshon/pixelita"
)

func TestPublicAPIPreviewAndApply(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.png")
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewNRGBA(image.Rect(0, 0, 16, 16))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	req := pixelita.OptimizeRequest{Context: t.Context(), Paths: []string{src}, Widths: []int{8}, OutDir: filepath.Join(dir, "out")}
	rep, err := pixelita.Optimize(req)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.DryRun || len(rep.Items) != 1 || rep.Items[0].Written || rep.Summary.Files != 1 || rep.Summary.Changed != 1 || rep.Totals["predictedOutputs"] != 1 {
		t.Fatalf("not a preview: %+v", rep)
	}
	if _, err := os.Stat(req.OutDir); !os.IsNotExist(err) {
		t.Fatal("preview created output")
	}
	req.Apply = true
	rep, err = pixelita.Optimize(req)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Summary.Failed != 0 || len(rep.Items) != 1 || !rep.Items[0].Written || rep.Totals["writtenOutputs"] != 1 {
		t.Fatalf("apply failed: %+v", rep)
	}
	if _, err := os.Stat(rep.Items[0].Output); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(src)
	if !bytes.Equal(after, data.Bytes()) {
		t.Fatal("source changed")
	}
}
