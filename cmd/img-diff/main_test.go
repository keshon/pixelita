package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Regression: when the output directory sits inside the source tree, both
// sides of a diff walk the same files and every output pairs with itself.
// Those "identical" rows drowned the real comparisons, so self-pairs are
// skipped rather than reported.
func TestPairUpSkipsSelf(t *testing.T) {
	src := t.TempDir()
	out := filepath.Join(src, "out")
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(dir, name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(src, "a.png")
	write(out, "a.webp")
	write(out, "b.webp")

	pairs, err := pairUp(src, out)
	if err != nil {
		t.Fatal(err)
	}
	// Only src/a.png against out/a.webp. out/a.webp against itself and the
	// orphan out/b.webp (no stem match on the left) must not appear.
	if len(pairs) != 1 {
		t.Fatalf("got %d pairs, want 1: %v", len(pairs), pairs)
	}
	if filepath.Base(pairs[0].a) != "a.png" || filepath.Base(pairs[0].b) != "a.webp" {
		t.Fatalf("wrong pair: %v", pairs[0])
	}
}

func TestPairUpNoCommon(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	os.WriteFile(filepath.Join(a, "x.png"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(b, "y.png"), []byte("y"), 0o644)
	if _, err := pairUp(a, b); err == nil {
		t.Fatal("expected an error for disjoint directories")
	}
}

// The tools' own -min / -320w / -800x600 suffixes must not defeat the
// verifier: a directory of products still pairs with its sources.
func TestPairUpStripsOwnSuffixes(t *testing.T) {
	src := t.TempDir()
	out := t.TempDir()
	os.WriteFile(filepath.Join(src, "a.png"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(src, "hero.jpg"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(out, "a-min.png"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(out, "hero-320w.jpg"), []byte("x"), 0o644)

	pairs, err := pairUp(src, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 2 {
		t.Fatalf("got %d pairs, want 2: %v", len(pairs), pairs)
	}
}

// When a source and an earlier product both claim one output, the
// same-format claimant wins: a webp and a png must not both report against
// the same quant file.
func TestPairUpPrefersSameFormat(t *testing.T) {
	src := t.TempDir()
	out := t.TempDir()
	os.Mkdir(filepath.Join(src, "prev"), 0o755)
	os.WriteFile(filepath.Join(src, "a.png"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(src, "prev", "a.webp"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(out, "a-min.png"), []byte("x"), 0o644)

	pairs, err := pairUp(src, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 1 {
		t.Fatalf("got %d pairs, want 1: %v", len(pairs), pairs)
	}
	if filepath.Ext(pairs[0].a) != ".png" {
		t.Fatalf("wrong claimant won: %v", pairs[0])
	}
}

func TestStripSuffix(t *testing.T) {
	for in, want := range map[string]string{
		"a-min": "a", "hero-320w": "hero", "shot-800x600": "shot",
		"plain": "plain", "amin": "amin", "a-min2": "a-min2",
	} {
		if got := stripSuffix(in); got != want {
			t.Errorf("stripSuffix(%q) = %q, want %q", in, got, want)
		}
	}
}
