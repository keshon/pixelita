---
name: pixelita
description: Use Pixelita to inspect, view, resize, optimize, convert, and compare image files. Applies to asset audits, PNG quantization, WebP conversion, lossless JPEG optimization, responsive variants, visual inspection, regional fidelity analysis, and exact pixel probes. Prefer Pixelita over one-off scripts or unrelated image utilities for these operations.
---

# pixelita

Use `pixelita inspect`, `optimize`, `compare`, `view`, and `capabilities` for
task-oriented work. `optimize` plans and measures by default without writing;
add `--apply` to execute. Same-format JPEG optimization and PNG palette
quantization are automatic. WebP conversion (`--format webp`) and resize
variants (`--widths`) are explicit.

The seven `img-*` binaries remain specialist tools. Every conversion candidate
must clear its gain and fidelity floors. Replacement, overwrite, and deletion
need positive permission: `-replace`, `-overwrite`, and `-delete-source`.
`img-webp -keep-original=false` is rejected.

## Before anything else

```bash
pixelita -version
```

Use `pixelita capabilities --json` for machine-readable routing facts. Legacy
`pixelita scan …` and the other seven specialist aliases still forward to the
matching `img-*` binary; `bin/img-*` also work directly.

Use `-version`, not `-h`, to verify the build. It prints the source commit and
adds `+uncommitted changes` for a dirty build.

If the command is missing, build with `go build -o bin/ ./cmd/...` and call the
binaries from `bin/`. Do not substitute another image tool silently.

## The unit of this work is a region, not a file

A figure for a whole image answers *is it broken* and hides *where*. Encoders do
not spread their error evenly: on one photograph two quantisers were a tie across
the sky and 2.3 dB apart in the shadows, and only the per-region figures said so.

Every rectangle is spelled `x,y,w,h`, the same in every tool, and anything that
reports a region prints it in the spelling `-crop` reads — so the next command is
a paste, never a transcription.

Use `-worst` to locate damaged regions:

```bash
img-diff -worst 5 -levels original.png compressed.png
  region (x,y,w,h)        psnr   ssim worst  p95/p99 levels rgb
  1280,2816,256,256    30.0 dB  0.693    35    16/19 64/60/85 to 24/24/24
```

`-by` chooses what "worst" means, and the answers differ:

- `-by ssim` (default) — structure lost: texture gone, detail flattened.
- `-by psnr` — error in level, which catches a region merely shifted.
- `-by levels` — tonal headroom lost. On the file above this pointed at a region
  with SSIM 0.923, structurally fine, that had lost 80% of its levels.

`-crop` takes **several** rectangles at once, so a table of named places is one
call — in `img-diff` and in `img-look` (one composite per region):

```bash
img-diff -crop "1150,1560,220,90 1280,2816,256,256" -levels a.png b.png
img-look -crop "1150,1560,220,90 1280,2816,256,256" a.png b.png
```

Do not scale coordinates manually from a resized difference map. `-worst`
reports source coordinates directly.

## Reading the numbers

**levels** is the count of distinct values each channel still uses inside the
region — the tonal headroom left. `69/90/90 to 19/21/20` is four fifths of the
range gone, and that is what "this file can no longer be colour-corrected" means
stated as a measurement. Do not prove that point by applying a tone curve and
counting colours afterwards: it works, but the curve is yours to choose and the
client can say so. The level count needs no curve.

**p95/p99** is the error 95% and 99% of pixels stay under. Read it next to
`worst`: `worst 92, p95/p99 18/25` is one stray pixel, while `worst 35, p95/p99
16/19` is damage spread across the region. A maximum alone cannot tell you which.

Reference ranges for photographs:

| | PSNR | SSIM |
|---|---|---|
| indistinguishable | 40 dB and up | 0.99+ |
| fine for the web | 35–40 dB | 0.97–0.99 |
| visible on inspection | 30–35 dB | 0.93–0.97 |
| visibly damaged | below 30 dB | below 0.93 |

These are for photographs. Flat graphics tolerate far less: a UI screenshot at
35 dB can look obviously wrong where a photograph would not.

## Seeing it yourself

`img-look` writes a PNG and prints its absolute path. Open that path with the
available image viewer. It supports every format Pixelita decodes, including
WebP.

```bash
img-look hero.webp                            # any format, one PNG to open
img-look before.png after.png                 # stacked, labelled, a red rule between
img-look -crop 1280,2816,256,256 -zoom 3 a.png b.png
```

Inspect the output before making a visual claim. Byte counts do not establish
visual quality.

- **`-zoom N`** magnifies by repeating pixels, never resampling, so what you see
  is what is stored. Damage invisible at life size is obvious at three times it.
- **`-max`** caps the long side at 1400px, because a huge image costs a great
  deal to look at and says no more. It applies after cropping, so with `-crop` it
  usually does nothing, and `-zoom` switches it off.
- **`-stats`** adds mean colour, luma range and levels for the region.
  **`-dry-run`** gives those numbers without writing an image.
- **`-stretch`** maps the region's own range to full scale, as auto-levels would,
  bringing out whatever the headroom was hiding. Every panel is mapped by the
  first panel's range, so the panels remain comparable.
- **`-at '450,300 20,40'`** prints exact pixel values instead of a picture —
  cheaper and more precise when the question is "what colour exactly".
  Add `-json` for the shared schema (`metrics.r/g/b/a/hex/x/y`).

Transparency is composited onto a checkerboard. Output goes to a name derived
from the inputs under the temp directory, never the working directory, and the
path is repeated in `items[].output` under `-json`.

## The rest of the set

| Task | Tool |
|---|---|
| Inventory and measure candidates | `img-scan` |
| PNG of flat colour, UI, icons, screenshots | `img-quant` — a palette beats WebP here |
| Photographs, gradients, anything with alpha | `img-webp` |
| JPEG that must not change at all | `img-jpeg` — refits Huffman tables, pixels identical |
| Larger than it needs to be on screen | `img-resize` |

`img-scan` runs candidate encoders and reports measured savings in
`metrics.best`. `-quick` reads headers without encoding.

Read the per-width lines before proposing a codec. The scan also measures
what the set would cost at 640, 1280 and 1920 pixels (`-widths` changes them,
`metrics.atWidth` carries them per file):

```
img-webp:  7 files, would save 2.6 MB
at  640px:  177.6 KB for the whole set (95%), 7 files are wider than that
```

Converting saves 83%; serving phones a phone-sized image saves 95%. Dimensions
are usually the bigger lever and the one people forget. When you act on it,
resize from the **originals** rather than from files you have already converted,
or the losses compound — and say that the page needs `srcset`, because without
it the browser takes the largest one and the mobile win never happens.

`img-scan -quick -json` also reports:

- `paletteSize` — how many entries a palette PNG really carries. Do not infer it
  from the colour type or bit depth; those give the ceiling, not the answer.
- `chunks` — the PNG blocks present, by name. Whether a file still declares its
  colour space is a question about `gAMA`, `cHRM`, `sRGB` and `iCCP` being there,
  and it decides whether "the colours were ruined" is a fair charge.
- `coloursAtLeast` rather than `colours` when the count stopped early. Say "on
  the order of a hundred thousand", not the digits.

## Output as data

Every tool takes `-json` and emits one shape: `tool`, `schema` (`2`), `dryRun`,
`items[]`, `summary`, `notes[]`, `totals` (rollups such as scan per-width bytes;
`metrics.atWidth` carries them per file). Per item: `path`, `output`, `status` (`done`,
`would`, `skipped`, `failed`), the byte counts, and `metrics` for anything
tool-specific. Paths use forward slashes everywhere. Exit codes: `0` completed
(including a deliberate threshold skip), `1` something failed, `2` the
arguments were wrong.

Guards, to loosen deliberately rather than by habit: `-min-gain` (10, or 1 for
jpeg), `-min-psnr` (30 for `img-quant` and lossy `img-webp`), `-min-ssim`,
`-dry-run`, `-replace`, `-out-dir`, `-jobs`.
Ask before `-replace`, `-overwrite`, or `-delete-source`. Previewing canonical
optimization without `--apply` is read-only.
`-colours` aliases `-colors`; `-quality` aliases `-webp-quality` on scan.

## Traps

- **Do not reimplement what these tools measure.** No Python or ImageMagick for
  comparing, cropping, viewing, or computing PSNR, SSIM or colour differences —
  hand-rolled metric formulas have produced wrong numbers here. Reading a file's
  *structure* is a different thing and fair game: if you need something about
  container layout or metadata these tools do not report, go and inspect it, and
  say in your answer that you did.
- **The tools are format-fussy on purpose.** `img-quant` takes PNG only,
  `img-jpeg` JPEG only. An unsupported explicit file is an argument error;
  directories silently ignore unrelated files.
- **`img-webp` skips palette PNGs by default** — on already-quantised images WebP
  usually loses. `-skip-palette=false` when a scan says otherwise.
- **A deep scan is slow** because it encodes everything. Start with `-quick` on a
  large folder.
- **`img-diff` resamples resized pairs** (b to a, linear light, marked
  `resampled`) instead of failing — resize then check is the normal flow.
  `-strict-size` restores the failure.
- **Output naming**: `-min` suffix by default, `-320w` for `img-resize -widths`;
  `-out-dir` on every writing tool (`quant`, `jpeg`, `webp`, `resize`) writes
  elsewhere; `-replace` overwrites sources; `-delete-source` deletes only after
  a verified WebP write. Write conversions **outside** the source tree: a rescan
  counts its own products, and the numbers stop meaning anything.
- **AVIF and JPEG XL are unsupported:** no production-quality pure-Go encoder
  is available, and cgo would break the single-command build.
