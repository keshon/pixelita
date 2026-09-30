# pixelita

Command-line tools for images. Pure Go, no cgo, no libwebp and no
libimagequant — one command builds them on any machine.

Pixelita follows three rules:

1. **Measure before writing.** Every writing tool has `-dry-run`. Candidates
   below the configured gain or fidelity thresholds are skipped.
2. **One engine, multiple interfaces.** The current commands share the
   operations in `internal/ops` and emit one JSON schema. The intended CLI,
   agent interface, and GUI share typed requests, policy, results, and
   verification rather than parsing or duplicating one another.
3. **Measured claims.** Performance and quality claims below include the
   reference tool and corpus.

| Tool | What it does |
|---|---|
| [`img-scan`](cmd/img-scan) | looks at a folder and **measures** what each tool would save |
| [`img-quant`](cmd/img-quant) | quantises a PNG to a palette |
| [`img-webp`](cmd/img-webp) | PNG and JPEG → WebP |
| [`img-jpeg`](cmd/img-jpeg) | shrinks a JPEG **losslessly**; not one pixel changes |
| [`img-resize`](cmd/img-resize) | resizing, and sets of derived sizes |
| [`img-diff`](cmd/img-diff) | compares two images: PSNR, SSIM, a difference map |
| [`img-look`](cmd/img-look) | makes an image **visible**: one PNG to open, or the pixel values |

A web interface exists on the `web-ui` branch and is not part of the set yet:
the CSS kit it is built on has not settled.

## Build

```bash
go build -o bin/ ./cmd/...
```

Image operations live in [`internal/ops`](internal/ops). Each current command
parses flags, calls an operation, and renders a report.

## Architecture direction

The current `pixelita` binary dispatches to seven `img-*` commands. It does not
yet accept a goal, create a plan, select a capability, explain routing, execute,
and verify through one task model. That canonical task interface is the main
remaining product milestone. The existing commands will remain as thin
Unix-style wrappers. A future GUI will call the same typed engine or a stable
API.

See
[`docs/architecture.md`](docs/architecture.md) for the implemented/target
boundary, layers, compatibility plan, and schema rules. The measured baseline,
defects, and evaluation matrix are in
[`docs/agent-evaluation.md`](docs/agent-evaluation.md).

## The usual order of work

Start with `img-scan`. It measures codec candidates and the encoded cost at
configured display widths:

```
img-webp:  7 files, would save 2.6 MB
at  640px:  177.6 KB for the whole set (95%), 7 files are wider than that
at 1280px:  446.4 KB for the whole set (87%), 7 files are wider than that
at 1920px:  802.4 KB for the whole set (76%), 7 files are wider than that
```

In this measurement, conversion saved 83% and a 640px delivery variant saved
95%. `-widths` supplies layout-specific widths. Each result comes from an actual
resize and encode.

```bash
img-scan ./public/img                    # what is here and what can be won
img-resize -max-width 1920 ./public/img  # nothing wider than 1920
img-jpeg ./public/img                    # free bytes: the picture does not change
img-webp ./public/img                    # convert what pays off
img-diff -min-psnr 35 ./src ./public/img # check that nothing broke
```

## The JSON contract

Any tool given `-json` emits one object of the same shape (`pixelita` forwards,
so `pixelita scan -json …` works too):

```json
{
  "tool": "img-quant",
  "schema": "2",
  "dryRun": true,
  "items": [
    {
      "path": "public/img/hero.png",
      "output": "public/img/hero-min.png",
      "status": "would",
      "bytesBefore": 253747,
      "bytesAfter": 72285,
      "gainPercent": 71.5,
      "metrics": { "colors": 256, "psnr": 37.8 }
    }
  ],
  "summary": {
    "files": 1, "changed": 1, "skipped": 0, "failed": 0,
    "bytesBefore": 253747, "bytesAfter": 72285, "gainPercent": 71.5
  },
  "totals": { "atWidth": { "640": 177600 }, "atWidthTotal": 2600000 }
}
```

`status` is one of `done`, `would`, `skipped`, `failed`. Numbers common to every
tool are named fields; anything tool-specific lives in `metrics`, so the schema
does not grow a column every time a tool learns to measure something. Paths use
forward slashes on every platform. `totals` carries machine-readable rollups
(`img-scan` per-width byte counts) alongside the human `notes`. Exit codes:
`0` completed (including deliberate threshold skips), `1` at least one item
failed, `2` the arguments were wrong.

`pixelita scan …` forwards to `img-scan`; the other six subcommands work the
same way. The `img-*` binaries also run directly.

## Photographs arrive rotated

Decoded operations apply EXIF orientation in
[`internal/imgio`](internal/imgio/exif.go). `ReadHeader` reports the oriented
dimensions. `img-jpeg` copies the coefficient stream without decoding and
retains the original orientation tag.

---

# img-scan

`img-scan` inventories inputs and encodes candidates without writing them. It
reports per-file recommendations and aggregate savings.

```
file                                        size format  type         pixels  colours best
------------------------------------------------------------------------------------------------------
alpha-ios.png                       128.4 KB png     rgba        350x402    23347 webp -91%
cover-212crm.png                    434.3 KB png     rgb        1480x740    40862 webp -88%
photo-rpharm.png                    969.6 KB png     rgba      1999x1000   78220+ webp -81%
site_crm_about@1.png                170.7 KB png     palette    1643x946      256 webp -28% *
ui-kanban5.png                      167.1 KB png     rgba       1920x948    14796 quant -69%
------------------------------------------------------------------------------------------------------
would be improved: 13, skipped: 0, failed: 0
size: 5.6 MB -> 1.1 MB, saved 4.5 MB (80%)
img-quant: 12 files, would save 3.9 MB
img-webp:  13 files, would save 4.5 MB
metadata:  268 B in 6 files, dropped by any re-encode
* img-webp leaves palette PNGs alone unless told otherwise: -skip-palette=false
```

`-quick` reads headers without encoding candidates. A plus sign on the colour
count marks a lower bound after the histogram stops exact counting at roughly
one hundred thousand colours.

---

# img-quant

`img-quant` reduces a PNG to a palette of at most 256 colours.

## How it is built

The implementation uses four stages:

1. **Histogram.** Open addressing over flat slices rather than a `map`: a
   photograph can have millions of distinct colours. Fully transparent pixels
   collapse into one — PNG files are full of transparent pixels carrying junk
   RGB, and without this the histogram lies about how many colours the image
   really has.
2. **Median cut.** The box costing the most error is split along the axis of
   greatest variance. The split point is found by scanning rather than taken at
   the median: one extra linear pass puts the boundary where the colours
   actually separate.
3. **k-means.** The palette is refined with weighted Lloyd iterations. Median
   cut initializes box centres; refinement moves entries toward the actual
   colour distribution.
4. **Remap with adaptive dithering.** Error is diffused into the neighbours, but
   only where the palette genuinely missed.

## Why the dithering is adaptive

Uniform dithering can replace compressible flat runs with noise. Adaptive
dithering reduces diffusion where a palette entry already matches closely.

| Variant | Size | PSNR |
|---|---|---|
| uniform dithering | 2398 KB | 44.6 dB |
| adaptive dithering | **1576 KB** | **44.6 dB** |

Both variants measured 44.6 dB; adaptive dithering was 34% smaller.

## Comparison with pngquant

The reference is `pngquant 2.17.0`, the official build. The set is 12
true-colour PNGs from a real project (covers, mockups, screenshots, photographs
with transparency), 5.5 MB. Both programs on their defaults, 256 colours. PSNR
is computed by the same code for both results.

| | Size | PSNR | Time |
|---|---|---|---|
| img-quant | **1576 KB** | **44.6 dB** | 7.1 s |
| pngquant 2.17 | 1908 KB | 44.0 dB | **4.6 s** |

At 256 colours, `img-quant` was 17% smaller and 0.6 dB higher in PSNR at 1.5x
the run time. At 64 colours, the differences were −22% and +0.7 dB.

On already-palette PNGs,
where both programs are lossless and only the PNG writing is measured, Go loses
to libpng by **0.7%** — the standard `compress/flate` is barely behind zlib
here. With dithering off on both sides, what is left is a clean comparison of
the palettes: **−4% and +0.8 dB** in our favour.

### Continuous-tone stress case

A 17.9-megapixel, 34.2 MB night photograph with 100,591 distinct colours was
used as a continuous-tone stress case. `img-scan` selected `img-webp` (−80%)
over `img-quant` (−69%).

| | Size | PSNR | SSIM | Time |
|---|---|---|---|---|
| img-quant | 11 050 349 | **33.2 dB** | **0.958** | 9.8 s |
| pngquant 2.x | **11 036 728** | 31.6 dB | 0.932 | **8.4 s** |

The quantized output was 0.12% larger than pngquant and measured +1.6 dB PSNR
and +0.026 SSIM. On noisy continuous-tone material both
programs hit the same floor, because what PNG can do with 256 dithered colours
over 17.9 megapixels is bounded by the dithering noise, not by the palette.

Per-region measurements:

| Region | img-quant | pngquant |
|---|---|---|
| smooth gradient sky | 34.9 dB / 0.952 | 34.8 dB / **0.963** |
| grass, mid-tone texture | **30.5 dB / 0.957** | 29.1 dB / 0.935 |
| foliage, deep shadow | **33.0 dB / 0.930** | 30.7 dB / **0.859** |

The gradient is a tie — pngquant is fractionally better there by SSIM. The gap
is entirely in the dark textured areas, and the mean colour of one shadow patch
says why:

```
original    32 19  9
img-quant   31 23 10    luminance held, a slight lift towards green
pngquant    27 16  7    about 15% darker — the shadows are crushed
```

A palette spent on a bright sky leaves nothing for dark neutral greens, and they
collapse towards black taking the texture with them. That collapse is what SSIM
0.859 is measuring. The k-means refinement is what buys those entries back, and
it is the step the off-the-shelf Go quantisers omit.

The measured size difference depends on the source material. This corpus showed
a consistent fidelity increase.

## Flags

| Flag | Default | What it does |
|---|---|---|
| `-colors` | `256` | maximum palette size, 2–256 |
| `-dither` | `1` | Floyd–Steinberg strength, `0` turns it off |
| `-effort` | `6` | 1–10, how long to spend refining the palette |
| `-min-gain` | `10` | minimum size reduction in percent |
| `-min-psnr` | `30` | refuse to write below this fidelity in dB; `0` disables |
| `-replace` | `false` | overwrite the source instead of writing beside it |
| `-suffix` | `-min` | suffix for the output name |
| `-out-dir` | — | write results here instead of next to the source |

---

# img-jpeg

`img-jpeg` rewrites baseline JPEG Huffman tables without changing coefficients.

A JPEG stores blocks of quantised coefficients entropy-coded with Huffman
tables. The coefficients are the picture; the tables are only how it was written
down. Most encoders ship the example pair from the standard's annex instead of
tables fitted to the image. The coefficients are copied unchanged, so decoded
pixels remain identical.

Measured on 40 real JPEGs from a project, 20.9 MB:

| | |
|---|---|
| Size | 20.9 MB → 18.7 MB, **−10.5%** |
| Pixels changed | **0** |
| Individual files | up to −22% |

Tests decode both files and compare their pixels byte for byte.

Metadata is kept deliberately. Dropping EXIF lays a photograph on its side;
dropping an ICC profile changes the colours a browser paints. Neither belongs in
an operation that promises to change nothing.

Progressive JPEGs are skipped.

| Flag | Default | What it does |
|---|---|---|
| `-min-gain` | `1` | minimum size reduction in percent |
| `-replace` | `false` | overwrite the source instead of writing beside it |
| `-suffix` | `-min` | suffix for the output name |
| `-out-dir` | — | write results here instead of next to the source |

---

# img-webp

`img-webp` converts PNG and JPEG inputs when the candidate clears the configured
gain and fidelity thresholds. Palette PNGs are skipped by default because WebP
often makes them larger.

Measurements supporting the palette default:

| File | PNG | WebP q100 | WebP q90 |
|---|---|---|---|
| interface screenshot 2528×1456 | 919 KB | **1045 KB** | 544 KB |
| interface screenshot 2528×1456 | 275 KB | **334 KB** | 219 KB |

Measured true-colour inputs saved up to 90%. PNG colour type is read from the
header before decoding.

| Flag | Default | What it does |
|---|---|---|
| `-quality` | `90` | quality for the lossy mode, 1–100 |
| `-mode` | `lossy` | `lossy`, `lossless` or `near-lossless` |
| `-skip-palette` | `true` | skip palette PNGs |
| `-min-gain` | `10` | minimum size reduction in percent |
| `-min-psnr` | `30` | refuse lossy below this fidelity in dB; `0` disables |
| `-min-ssim` | `0` | refuse lossy below this SSIM; `0` disables |
| `-keep-original` | `true` | keep the source; `false` deletes it after success and is destructive |
| `-out-dir` | — | write results here instead of next to the source |

---

# img-resize

`img-resize` resamples in linear light. sRGB value 128 represents about one
fifth of full luminance, so averaging sRGB values directly darkens fine detail.

A halved black-and-white checkerboard should encode as sRGB 188:

| Resampler | A checkerboard halved |
|---|---|
| sRGB-value average | 128 |
| `img-resize` | **188** |

Alpha is premultiplied for the same class of reason: without it the colour of
fully transparent pixels leaks into the visible edge beside them. Both
properties are covered by tests.

```bash
img-resize -max-width 1920 ./public/img            # shrink only, only what is too big
img-resize -widths 320,640,1280 -out-dir dist hero.png
img-resize -width 400 -height 400 -fit cover avatar.jpg
```

| Flag | Default | What it does |
|---|---|---|
| `-width` / `-height` | `0` | the size; zero derives it from the aspect ratio |
| `-max-width` / `-max-height` | `0` | shrink what is bigger, leave the rest alone |
| `-scale` | `0` | a factor, for example `0.5` |
| `-widths` | — | comma-separated widths, a set of derivatives |
| `-fit` | `inside` | `inside`, `outside`, `cover` or `exact` |
| `-filter` | `catmull-rom` | `nearest`, `box`, `triangle`, `catmull-rom`, `lanczos` |
| `-allow-upscale` | `false` | permit making an image larger |
| `-format` | `keep` | `keep`, `png` or `jpeg` |

---

# img-diff

`img-diff` compares sources and candidates for manual review, CI, and agent
verification.

PSNR measures pixel error. SSIM compares local means, variances, and covariance
to measure structural change.

```bash
img-diff before.png after.png               # one pair
img-diff -out diff.png before.png after.png # plus a difference map
img-diff -crop 4500,1300,500,250 a.png b.png # only that region
img-diff -min-psnr 35 ./src ./converted     # directories; exit 1 on a failure
```

`-crop "x,y,w,h x,y,w,h"` measures several rectangles in one call. `-levels`
adds per-channel tonal levels before and after conversion.

`-worst N` divides the image into tiles and prints the lowest-ranked regions in
the `x,y,w,h` format accepted by `-crop` and `img-look`.

```
region (x,y,w,h)           psnr   ssim worst  p95/p99
1280,2816,256,256       30.0 dB  0.693    35    16/19
1280,3072,256,256       30.7 dB  0.697    26    14/17
```

Manual inspection of the reference photograph selected regions near SSIM 0.86;
the tile search found a region at 0.69. `-by` ranks by `ssim`, `psnr`, or
`levels`. Ranking by levels found a region at SSIM 0.923 that had lost four
fifths of its tonal levels.

The rejected `-flattest` design used smoothness as a proxy for tonal damage. On
the reference photograph it selected crushed shadows instead of the damaged
gradient. `-by levels` measures the relevant property directly.

`p95/p99` reports the error thresholds containing 95% and 99% of pixels. On the
reference photograph, `worst` was 92 while p95/p99 were 18/25, indicating an
outlier rather than a frame-wide shift.

`-crop` uses the same rectangle syntax as `img-look`. Cropped reports omit file
size columns because those values describe whole files. On the quantizer corpus,
two candidates tied across the smooth sky and differed by 2.3 dB in the shadows.

Directory comparison pairs file stems regardless of extension and recognizes
the `-min`, `-320w`, and `-800x600` output suffixes. A same-format claimant wins
ties. Dimension mismatches resample b to a in linear light and add the
`resampled` metric; `-strict-size` rejects them. Self-matches are skipped.

## WebP decoder regression

The original decoder path reported 22–30 dB and did not change between
`-quality 75` and `-quality 100`, although the file tripled in size.

White 255 decoded as 237 and dark 38 as 48. This matches
`16 + 219·v/255`, the studio range of BT.601. The encoder was not at fault:
Chrome decodes the same files **byte for byte identically to the source**. VP8
keeps luma between 16 and 235, while `golang.org/x/image/webp` hands the frame
back as an `image.YCbCr`, whose colour model is the full-range JPEG one.

[`internal/imgio`](internal/imgio/imgio.go) now converts with VP8 coefficients.
A regression test covers the shared lossy-WebP decode path.

Measured results after the fix:

| | PSNR | SSIM |
|---|---|---|
| before the fix | 22–30 dB | 0.95–0.99 |
| after | **33–44 dB** | **0.99+** |

The regression demonstrates why encoding and verification must remain separate
operations.

---

# img-look

`img-look` writes viewable PNG composites or reports pixel values. It does not
modify inputs.

```bash
img-look hero.webp                            # any format in, one PNG to open
img-look before.png after.png                 # stacked, labelled, a red rule between
img-look -crop 700,380,460,210 a.png b.png    # the same region of both
img-look -crop "0,0,64,64 100,300,64,64" a.png b.png  # several regions, one composite each
img-look -max 0 -crop 0,0,64,64 icon.png      # native pixels, no scaling
img-look -crop 4600,1380,180,105 -zoom 4 a.png b.png   # that region, four times life size
img-look -crop 800,2400,500,250 -stats a.png b.png     # and what it averages to
img-look -at '450,300 20,40' shot.png         # the numbers instead of the picture
img-look -json -at '450,300 20,40' shot.png   # the numbers as JSON
```

`-zoom` repeats pixels without resampling and implies `-max 0` unless `-max` was
set explicitly.

`-stats` reports the number of distinct values used by each channel in a region.
These levels measure remaining tonal headroom. On the reference photograph:

| Region | Original | Delivered |
|---|---|---|
| smooth sky gradient | 69/90/90 | **19/21/20** |
| grass | 64/60/85 | 24/24/24 |
| foliage | 256/256/254 | 106/115/112 |
| the whole image | 256/256/256 | 157/155/158 |

The whole image retained 157 levels while the sky gradient retained about 20 of
its original 90. Level counts require no selected tone curve.

`-stretch` maps the region's range onto the full scale. Every panel uses the
first panel's range to preserve the comparison. On the reference image it
revealed amplified dither noise instead of banding.

`-dry-run` reports the measurements and writes no image, for when only the
numbers are wanted.

`-stats` reports mean colour and luma range. The mean is computed in linear light
with the same resampler as
`img-resize -filter box -fit exact -width 1 -height 1`. A regression test keeps
the two results equal. On the reference shadow patch, an sRGB arithmetic mean
was 25/13/5 and the linear-light mean was 32/19/9.

The default composite path is `%TEMP%\pixelita\look.png` or
`/tmp/pixelita/look.png`. Human output prints the absolute path; JSON repeats it
in `items[].output`. `-out` selects another path.

Output is PNG. The default 1400px long-side limit bounds inspection cost;
`-max 0` preserves native dimensions.

Transparency is composited onto a checkerboard. Panels include the file name on
a dark strip. `-across` uses a horizontal layout, and `-label=false` removes the
strip.

`-at` reports exact pixel values:

```
img-look -at '450,300 20,40' screenshot.webp
  450,300  rgba 255 255 255 255  #ffffff
  20,40    rgba  38  41  40 255  #262928
```

## Unsupported formats

AVIF and JPEG XL remain unsupported because no production-quality pure-Go
encoder is available and cgo violates the build constraint.

## Pure-Go quality and performance

Quality depends on the algorithm. The evaluated Go quantisers (`go-quantize`,
`soniakeys/quant`, `esimov/colorquant`) are noticeably weaker, because they are
plain median cut or NeuQuant with no palette refinement and often no dithering.

Without SIMD, the measured float-heavy paths ran 1.5–2x slower than the C
references.

## License

MIT.
