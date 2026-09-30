package cli

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseFlagsAfterPositionals(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	quick := fs.Bool("quick", false, "")
	quality := fs.Int("quality", 90, "")
	if err := ParseFlags(fs, []string{"a.png", "-quick", "b.png", "-quality", "75"}); err != nil {
		t.Fatal(err)
	}
	if !*quick || *quality != 75 {
		t.Fatalf("flags not parsed: quick=%v quality=%d", *quick, *quality)
	}
	if got := fs.Args(); !reflect.DeepEqual(got, []string{"a.png", "b.png"}) {
		t.Fatalf("paths = %v", got)
	}
}

func TestCollectRejectsExplicitUnsupportedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "photo.jpg")
	if err := os.WriteFile(path, []byte("not relevant"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Collect([]string{path}, ".png")
	if err == nil || !strings.Contains(err.Error(), "unsupported input") {
		t.Fatalf("Collect error = %v, want unsupported input", err)
	}
}

func TestCollectAllowsDirectoryWithNoMatches(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := Collect([]string{dir}, ".png")
	if err != nil || len(files) != 0 {
		t.Fatalf("Collect = %v, %v; want empty success", files, err)
	}
}
