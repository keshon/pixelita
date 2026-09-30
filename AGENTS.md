# pixelita

Pixelita is a pure-Go image toolkit. Build every command with:

```bash
go build -o bin/ ./cmd/...
```

The single build command and no-cgo constraint are product requirements.

## Product architecture

Pixelita has three clients: terminal users, agents, and a future GUI. They must
share typed operations, safety policy, schemas, routing, and verification.

The canonical workflow is:

1. inspect inputs;
2. plan for an explicit goal;
3. execute with safety checks;
4. verify the result.

Formats and codecs are capabilities within that workflow. New formats extend a
small compile-time capability catalog instead of creating a new product model.

`pixelita inspect`, `optimize`, `compare`, `view`, and `capabilities` implement
this workflow through an internal typed plan and a static capability catalog.
`optimize` previews by default and writes only with `--apply`. The plan is an
internal execution contract, not a public serialized format. See
[`docs/architecture.md`](docs/architecture.md).

Keep the existing `img-*` commands as thin Unix-style wrappers and compatibility
aliases. They may adapt presentation, but must not own policy or behavior that
differs from the typed engine. The GUI must call the engine or a stable API; it
must not parse human CLI output.

Safety and recovery rules belong in executable help, schemas, and errors. The
Pixelita skill may summarize them, but must not be the only source.

## Conversion policy

A conversion is written only when it clears `-min-gain` and its enabled
`-min-psnr` or `-min-ssim` thresholds. `img-scan` must recommend only candidates
that the corresponding converter would write.

Source overwrite and deletion require explicit user approval. `-replace`
overwrites a source. `-delete-source` deletes only after a verified WebP write;
`-keep-original=false` is rejected.

Plan all output paths before parallel writes. Duplicate destinations are
errors. Failed writes must not remove sources or leave partial outputs.

## Regions and verification

Regions use `x,y,w,h` everywhere. Commands that accept multiple regions use a
space-separated list. Commands that report regions emit the same spelling so
their output can be passed directly to `-crop`.

`img-diff` measures fidelity. `img-look` produces viewable images and pixel
data. Verify encoder changes with at least one of them.

`img-diff -worst` ranks damaged tiles by `ssim`, `psnr`, or `levels`.
`-by levels` replaced the rejected `-flattest` design: smoothness did not locate
the regions with the greatest loss of tonal levels.

Dimension-mismatched pairs are resampled from b to a in linear light and marked
`resampled`. `-strict-size` makes a mismatch fail. Directory comparisons ignore
extensions and recognize the tools' `-min`, `-320w`, and `-800x600` suffixes.

## Agent-facing contracts

Optimize for task completion and recovery, not flag count. Three runs of the
same audit used 26, 24, and 31 tool calls despite adding useful flags. The
multi-region `-crop`, `-worst`, and `-levels` interfaces reduced that workflow
to two calls.

Names must carry their caveats. An inexact colour count is `coloursAtLeast`,
not `colours` plus a separate boolean. Metadata reports name relevant chunks;
a byte count alone does not identify a colour-profile or orientation change.

JSON schema 2 uses one report envelope. Machine-readable rollups belong in
`totals`; `notes` are for human explanation. Paths use forward slashes.

Explicit unsupported files and invalid or conflicting options are argument
errors. Directories may contain unrelated files and may produce an empty result.

## Code layout

| Package | Responsibility |
|---|---|
| `internal/engine` | Typed plans, canonical routing, and the static capability catalog |
| `internal/ops` | Image operations and current per-file decisions |
| `internal/report` | Table and JSON report envelope |
| `internal/quant` | Histogram, median cut, k-means, and dithered remap |
| `internal/jpegopt` | Lossless JPEG coefficient-stream optimization |
| `internal/resize` | Linear-light resampling with premultiplied alpha |
| `internal/metric` | PSNR and SSIM |
| `internal/imgio` | Decode, encode, and header inspection |
| `internal/imgio/exif.go` | EXIF orientation applied during decode |
| `internal/cli` | Path collection and worker scheduling |

New operations start in the typed engine. Add `cmd/img-<name>` only when a
specialist wrapper improves the human workflow. Thin writing commands support
`-json`, `-dry-run`, and `-out-dir`.

## Measurements and tests

README performance and quality numbers must come from reproducible measurements.
Rerun affected measurements when an encoder change can move them. Do not publish
estimates as benchmark results.

The external corpus is selected through environment variables:

```bash
go test ./...
JPEGOPT_CORPUS=/path/to/jpegs go test ./internal/jpegopt -run TestCorpus -v
QUANT_BENCH_IMAGE=/path/to/photo.png go test ./internal/quant -bench Phases
```

`img-jpeg` is pixel-identical. Verify it with
`img-diff -min-psnr 200`; any failure rejects the change.

## Fixed constraints

- AVIF and JPEG XL remain out of scope while no production-quality pure-Go
  encoder is available. Adding cgo is not acceptable.
- Pixel-preserving operations retain metadata. Removing EXIF can rotate an
  image; removing an ICC profile can change rendered colour.
- Decode-time EXIF orientation is canonical for decoded operations.
  `img-jpeg` retains the original orientation tag because it does not decode.
- Specialist binaries retain the `img-` prefix.
- Do not use the `web-ui` branch. Build any future GUI from scratch after the
  engine contracts stabilize.

## Style

Comments explain constraints, measurements, and non-obvious decisions. They do
not narrate code that is already clear.
