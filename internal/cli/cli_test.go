package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
