package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/keshon/pixelita/internal/report"
)

func writeTestPNG(t *testing.T, path string) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 8), G: uint8(y * 8), B: uint8((x + y) * 4), A: 255})
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

func TestJSONArgumentFailureWithJSONAfterPath(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"optimize", "missing.png", "--json", "--min-gain"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit = %d", code)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON %q: %v", stdout.String(), err)
	}
	items := payload["items"].([]any)
	item := items[0].(map[string]any)
	if item["code"] != "invalid_arguments" {
		t.Fatalf("code = %#v", item["code"])
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestCapabilitiesJSONUsesCatalog(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"capabilities", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	var payload struct {
		Schema       string            `json:"schema"`
		Capabilities []map[string]any  `json:"capabilities"`
		Build        *report.BuildInfo `json:"build"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Schema != "2" || len(payload.Capabilities) < 4 || payload.Build == nil {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestSubcommandHelpIsSuccessfulAndGeneratedFromFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"optimize", "--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	for _, want := range []string{"usage: pixelita optimize", "-apply", "-format", "-min-psnr"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("help missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestOptimizePreviewIncludesApplyAction(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "input.png")
	writeTestPNG(t, path)

	var stdout, stderr bytes.Buffer
	code := run([]string{"optimize", path, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	var payload struct {
		DryRun     bool               `json:"dryRun"`
		NextAction *report.NextAction `json:"nextAction"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.DryRun || payload.NextAction == nil || payload.NextAction.Command != "pixelita" {
		t.Fatalf("preview contract = %#v", payload)
	}
	if got := payload.NextAction.Args[len(payload.NextAction.Args)-1]; got != "--apply" {
		t.Fatalf("last next argument = %q", got)
	}
}

func TestViewRecoveryKeepsTheViewCommand(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input.png")
	output := filepath.Join(dir, "preview.png")
	writeTestPNG(t, input)
	if err := os.WriteFile(output, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"view", input, "--out", output, "--json"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit=%d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	var payload struct {
		Items []struct {
			NextAction *report.NextAction `json:"nextAction"`
		} `json:"items"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Items) != 1 || payload.Items[0].NextAction == nil {
		t.Fatalf("missing recovery action: %s", stdout.String())
	}
	if got := payload.Items[0].NextAction.Args[0]; got != "view" {
		t.Fatalf("recovery command = %q, want view", got)
	}
}
