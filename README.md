# Pixelita

Optimize image assets, measure the damage, and inspect the worst regions before
accepting the result. Pure Go, no cgo or external codec executables.

## Build

```sh
go build -o bin/ ./cmd/...
```

Run `bin/pixelita` (`bin\pixelita.exe` on Windows), or add `bin` to PATH.
`build.cmd check` runs tests and vet before building on Windows. CI builds and
tests on Windows and Linux with `CGO_ENABLED=0`.

## One workflow

```sh
pixelita inspect ./images --json
pixelita optimize ./images --out-dir ./optimized --json
pixelita optimize ./images --out-dir ./optimized --apply --json
pixelita compare ./images ./optimized --min-psnr 30 --json
```

Optimization previews by default. `--apply` remeasures current inputs and writes
accepted results. Keep outputs outside the source tree. Defaults preserve file
format and dimensions; PNG palette conversion may change pixels, while baseline
JPEG Huffman optimization is pixel-identical. Progressive JPEGs are skipped.

Permit format changes explicitly to compare actual same-format and WebP
candidates, or request WebP directly:

```sh
pixelita optimize ./images --format auto --json
pixelita optimize hero.png --format webp --widths 320,640,1280 --out-dir ./variants
```

`auto` selects the smallest accepted candidate and reports both decisions. It
does not resize, replace, or delete sources. Resize plus WebP resamples the source
and encodes each derivative once. Resize thresholds are not interchangeable with
same-size fidelity thresholds: use `compare` to evaluate the derivative.

## Inspect the damage

```sh
pixelita compare before.png after.webp --worst 3 --by ssim --levels --json
pixelita view before.png after.webp --crop "0,0,64,64 100,100,64,64" --stats
pixelita view hero.png --at "10,20 30,40" --json
```

Region coordinates are always `x,y,w,h`. Paste comparison regions directly into
`view --crop`. Views default to a 1400-pixel long-side limit; `--max 0` preserves
native resolution. `--zoom` repeats pixels. Comparison resamples b to a when
sizes differ and reports it; `--strict-size` rejects mismatches.

Directory comparison checks every matched derivative, including resize suffixes.
Ambiguous sources are errors, rather than silently selected matches.

For visually sensitive PNG photographs or mixed photo/UI assets, preview with
stricter floors before applying:

```sh
pixelita optimize ./images --min-psnr 40 --min-ssim 0.99 --out-dir ./optimized
```

This retains originals when palette conversion misses either floor. It is a
conservative option, not a promise of invisible change: inspect worst regions
at native size as well as zoomed. PNG defaults remain 30 dB PSNR, 10% gain, and
no SSIM gate unless requested. RGB error under transparent pixels still counts.

## Safety and fidelity

- Omitted thresholds use strategy defaults. `--min-psnr 0` and `--min-ssim 0`
  disable their checks; `--min-gain 0` permits equal size but not growth.
- PSNR gates both RGB error and alpha-aware error. SSIM is measured even for
  tiny images. Requested measurements cannot silently be omitted.
- `--exact-pixels` and `--preserve-metadata` restrict same-format optimization.
  Currently only baseline JPEG has an eligible guaranteed strategy; other inputs
  are explicitly skipped in these modes.
- Quantization measures the decoded encoded PNG, retains compatible ancillary
  chunks, and refuses metadata that cannot survive palette changes.
  Stale Apple `iDOT` decoding offsets are omitted after re-encoding and named
  in `metrics.candidateRemovedChunks`; ICC, EXIF, and textual chunks are retained.
- Current WebP conversion and resize reject ICC-profiled or animated PNG inputs
  that their encoders cannot faithfully carry forward. These are not colour
  management tools. JPEG-to-WebP applies EXIF orientation before encoding.
- `--overwrite` permits replacing an existing destination. `--replace` permits
  replacing a source. `--delete-source` applies only to explicit WebP conversion
  and removes a source only after verified output and a source-change check.
- Complete output paths are checked before writes. Temporary files are verified
  before publication; no-overwrite publication cannot clobber a concurrent file.
- Default concurrency is at most two files; `--jobs` overrides it. Decodes and
  generated previews are limited to 64 million pixels. Ctrl+C cancels queued
  optimization/scan work; active codec calls finish safely.

No filesystem operation promises transaction isolation from another process
modifying inputs concurrently. Preview is not a persistent approval token.

## Automation

Use `pixelita capabilities --json` for build and strategy facts. Every command
supports schema-2 JSON. Exit codes: `0` completed (including threshold skips),
`1` processing failure, `2` invalid request.

`totals` separates `inputs`, `outcomes`, `inventoryBytes`, `inventoryKnownInputs`,
`writtenOutputs`, `writtenBytes`, `predictedOutputs`, and `predictedBytes`.
Legacy `summary` retains its outcome-based semantics for compatibility.
`items[].written` records actual writes; `verification` describes the performed
check. Missing numeric metrics alone never establish identity.

The seven `img-*` specialist binaries and legacy aliases remain available. They
share engine orchestration and operations; use their help for expert controls.

## Verification and development

```sh
go test ./...
go vet ./...
go test ./internal/imgio -run '^$' -fuzz FuzzReadHeader -fuzztime 10s
```

The reproducible CLI acceptance harness is in [eval](eval/README.md). Current
architecture and remaining acceptance work are in [architecture](docs/architecture.md)
and [refactor status](docs/refactor-status.md). Earlier algorithm explanations
and benchmark numbers are retained as [historical reference](docs/historical-reference.md),
not claims about the current build.

AVIF, JPEG XL, a GUI, and a stable public Go API remain out of scope. MIT license.
