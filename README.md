# pixelita

Command-line tools for images. Pure Go, no cgo, no libwebp and no
libimagequant — one command builds them on any machine.

Not another converter. What is different is the combination of three things:

1. **Measure first, then touch.** Every tool has `-dry-run`, which reports the
   result honestly and writes nothing. No tool spoils a file quietly: if there
   is nothing to gain, the file is left exactly as it was.
2. **The command line is the API.** Every tool speaks `-json` with one shared
   schema, so a web interface, a CI job and an AI agent all drive the same
   binaries a person drives from a terminal. Nothing is "UI only".
3. **Numbers, not promises.** Every claim below is a reproducible measurement
   against a reference tool rather than an impression.

| Tool | What it does |
|---|---|
| [`img-scan`](cmd/img-scan) | looks at a folder and **measures** what each tool would save |
| [`img-quant`](cmd/img-quant) | quantises a PNG to a palette |
| [`img-webp`](cmd/img-webp) | PNG and JPEG → WebP |
| [`img-jpeg`](cmd/img-jpeg) | shrinks a JPEG **losslessly**; not one pixel changes |
| [`img-resize`](cmd/img-resize) | resizing, and sets of derived sizes |
| [`img-diff`](cmd/img-diff) | compares two images: PSNR, SSIM, a difference map |

A web interface exists on the `web-ui` branch and is not part of the set yet:
the CSS kit it is built on has not settled.

## Build

```bash
go build -o bin/ ./cmd/...
```

All the work lives in [`internal/ops`](internal/ops). Each command is a flag
parser around one function there, and the web interface calls the same
functions. That turns "the interface does what the command line does" from a
promise into a property of the code: there is one implementation, so there is
nothing to drift.

## The usual order of work

```bash
img-scan ./public/img                    # what is here and what can be won
img-resize -max-width 1920 ./public/img  # nothing wider than 1920
img-jpeg ./public/img                    # free bytes: the picture does not change
img-webp ./public/img                    # convert what pays off
img-diff -min-psnr 35 ./src ./public/img # check that nothing broke
```

## The JSON contract

Any tool given `-json` emits one object of the same shape:

```json
{
  "tool": "img-quant",
  "schema": "1",
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
  }
}
```

`status` is one of `done`, `would`, `skipped`, `failed`. Numbers common to every
tool are named fields; anything tool-specific lives in `metrics`, so the schema
does not grow a column every time a tool learns to measure something. Paths use
forward slashes on every platform. Exit codes: `0` fine, `1` something failed or
a threshold was missed, `2` the arguments were wrong.

---

# img-scan

The missing front door. Running a converter over a folder and hoping is a poor
way to work: some files are already optimal, some would grow, and the gain
usually sits in a minority of them. `img-scan` answers that before anything is
written — and it does not guess from file extensions, it **actually runs the
encoders**.

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

`-quick` reads headers only — an inventory with no measuring, instantly. A plus
on the colour count means the histogram stopped counting exactly: above roughly
a hundred thousand colours it trades precision for a bounded run, and from then
on it can only undercount.

---

# img-quant

Quantisation: an image is reduced to a palette of at most 256 colours, the way
`pngquant` does it. The difference is that there is no C and no cgo here.

## How it is built

The same pipeline libimagequant arrived at, for the same reasons:

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
   cut puts a colour at the centre of a box, and a box is what the algorithm
   carved out, not the shape the colours inside it have. **This is the step the
   off-the-shelf Go quantisers skip, and it is the main difference in quality.**
4. **Remap with adaptive dithering.** Error is diffused into the neighbours, but
   only where the palette genuinely missed.

## Why the dithering is adaptive

Uniform dithering at full strength looks correct and costs a great deal. In a
flat area whose colour is a hair off a palette entry it renders that colour as a
sparse grid of two neighbouring ones. Formally faithful — and in practice a long
run of identical bytes, which PNG stores for almost nothing, is replaced by
noise that does not compress at all.

| Variant | Size | PSNR |
|---|---|---|
| uniform dithering | 2398 KB | 44.6 dB |
| adaptive dithering | **1576 KB** | **44.6 dB** |

The same PSNR for a third less file.

## Against pngquant

The reference is `pngquant 2.17.0`, the official build. The set is 12
true-colour PNGs from a real project (covers, mockups, screenshots, photographs
with transparency), 5.5 MB. Both programs on their defaults, 256 colours. PSNR
is computed by the same code for both results.

| | Size | PSNR | Time |
|---|---|---|---|
| img-quant | **1576 KB** | **44.6 dB** | 7.1 s |
| pngquant 2.17 | 1908 KB | 44.0 dB | **4.6 s** |

**17% smaller and 0.6 dB more faithful, at 1.5x the time.** At 64 colours the
gap is the same: −22% and +0.7 dB.

The gap is broken down rather than presented as a win. On already-palette PNGs,
where both programs are lossless and only the PNG writing is measured, Go loses
to libpng by **0.7%** — the standard `compress/flate` is barely behind zlib
here. With dithering off on both sides, what is left is a clean comparison of
the palettes: **−4% and +0.8 dB** in our favour.

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

---

# img-jpeg

The only tool in the set with no quality setting, because there is nothing to
lose.

A JPEG stores blocks of quantised coefficients entropy-coded with Huffman
tables. The coefficients are the picture; the tables are only how it was written
down. Most encoders ship the example pair from the standard's annex instead of
tables fitted to the image in front of them. Fitting them costs nothing: the
coefficients are copied across untouched, so the decoded image is identical to
the last bit.

Measured on 40 real JPEGs from a project, 20.9 MB:

| | |
|---|---|
| Size | 20.9 MB → 18.7 MB, **−10.5%** |
| Pixels changed | **0** |
| Individual files | up to −22% |

The check is built into the tests and is simple: both files are decoded and the
bytes compared. Any difference is a failure, not an acceptable tolerance.

Metadata is kept deliberately. Dropping EXIF lays a photograph on its side;
dropping an ICC profile changes the colours a browser paints. Neither belongs in
an operation that promises to change nothing.

Progressive JPEGs are skipped: such a file has already been through an encoder
that cared.

| Flag | Default | What it does |
|---|---|---|
| `-min-gain` | `1` | minimum size reduction in percent |
| `-replace` | `false` | overwrite the source instead of writing beside it |
| `-suffix` | `-min` | suffix for the output name |

---

# img-webp

A PNG and JPEG to WebP converter that does not make things worse. Ordinary
converters convert everything, and an image that has already been through
quantisation often comes out of WebP at maximum quality **heavier** than the
original.

`-skip-palette` is on by default from measurement rather than caution:

| File | PNG | WebP q100 | WebP q90 |
|---|---|---|---|
| interface screenshot 2528×1456 | 919 KB | **1045 KB** | 544 KB |
| interface screenshot 2528×1456 | 275 KB | **334 KB** | 219 KB |

For true-colour images the picture is the opposite — WebP wins several-fold,
up to −90%. The colour type is read from the PNG header without decoding the
image.

| Flag | Default | What it does |
|---|---|---|
| `-quality` | `90` | quality for the lossy mode, 1–100 |
| `-mode` | `lossy` | `lossy`, `lossless` or `near-lossless` |
| `-skip-palette` | `true` | skip palette PNGs |
| `-min-gain` | `10` | minimum size reduction in percent |
| `-keep-original` | `true` | keep the source file beside the `.webp` |

---

# img-resize

Resizing **in linear light**. This is not a subtlety: sRGB values are not
proportional to light — 128 is not half the brightness of 255, it is about a
fifth. Averaging them directly darkens every image it touches, most visibly
where fine detail alternates light and dark: thin text, hairlines, foliage.

The test that shows it: a black-and-white checkerboard is exactly half the
light, and half the light in sRGB is **188, not 128**.

| Resampler | A checkerboard halved |
|---|---|
| naive, in sRGB values | 128 — the image got darker |
| **img-resize** | **188 — the brightness survived** |

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

The tool that keeps the others honest. Every other command promises not to make
a file worse; this is how that promise is checked — by a person, by CI, or by an
agent verifying its own work.

Two metrics, because they disagree usefully. PSNR is the mean squared error:
objective, comparable between tools, and blind to structure. SSIM compares local
means, variances and covariance — closer to what an eye notices, and it catches
a conversion that scored well and still looks wrong.

```bash
img-diff before.png after.png               # one pair
img-diff -out diff.png before.png after.png # plus a difference map
img-diff -min-psnr 35 ./src ./converted     # directories; exit 1 on a failure
```

Two directories are paired by file name ignoring the extension, so a folder of
PNGs can be checked directly against the WebP files made from it.

## What it found on its very first run

The first run reported 22–30 dB for WebP where 35–40 was expected and — more
tellingly — **the figure did not move between `-quality 75` and `-quality 100`**
although the file tripled in size. A constant error, not compression loss.

It turned out that white 255 came back as 237 and dark 38 as 48. That is exactly
`16 + 219·v/255`, the studio range of BT.601. The encoder was not at fault:
Chrome decodes the same files **byte for byte identically to the source**. VP8
keeps luma between 16 and 235, while `golang.org/x/image/webp` hands the frame
back as an `image.YCbCr`, whose colour model is the full-range JPEG one.

So the files were right and the measuring was wrong. Fixed in
[`internal/imgio`](internal/imgio/imgio.go) with a conversion using VP8's own
coefficients, and covered by a regression test. It fixed every tool at once:
any read of a lossy WebP went through the same path.

After the fix, what one expects of WebP q90:

| | PSNR | SSIM |
|---|---|---|
| before the fix | 22–30 dB | 0.95–0.99 |
| after | **33–44 dB** | **0.99+** |

This is the argument for a separate verifier: without it we would never have
learnt that our own numbers were lying.

---

## What will never be here

AVIF and JPEG XL. There is no pure-Go encoder for them fit for use, and cgo
breaks the premise that one command builds this on any machine. Better to say so
than to promise and then hit the wall.

## On quality in Go

The language has nothing to do with it. Quality is decided by the algorithm.
What is true is that the off-the-shelf Go quantisers (`go-quantize`,
`soniakeys/quant`, `esimov/colorquant`) are noticeably weaker, because they are
plain median cut or NeuQuant with no palette refinement and often no dithering.
You cannot take one off the shelf and get pngquant. You can write your own,
which is what happened here.

The one place C is genuinely ahead is speed: without SIMD the heavy float loops
run one and a half to two times slower. That costs time, not quality.

## License

MIT.
