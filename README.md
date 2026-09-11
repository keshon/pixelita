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
| [`img-look`](cmd/img-look) | makes an image **visible**: one PNG to open, or the pixel values |

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

## Photographs arrive rotated

A phone held sideways does not turn the pixels it writes. It writes the sensor
as it is and records one EXIF tag saying which way was up, and every viewer is
expected to honour it. A tool that decodes, works, and encodes again without
honouring it produces a file that is sideways twice over: the pixels were never
turned, and the tag that would have turned them did not survive.

So the tag is applied at the moment of decoding, in
[`internal/imgio`](internal/imgio/exif.go). From there on the image in memory is
the image a person would see, every tool gets it upright without knowing EXIF
exists, and `img-scan` reports the size the photograph will appear at rather
than the one stored. `img-jpeg` is the exception on purpose: it never decodes,
so the original tag stays where it was and the file keeps working everywhere.

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

### And on material the palette hates

The set above is what a project actually contains. A single 17.9-megapixel
night photograph — a long exposure, star trails over a gradient sky, 100 591
distinct colours, 34.2 MB — is the opposite: the case nobody would sensibly
quantise, and the one where a palette has least to work with. `img-scan` says
as much before anything runs, and names `img-webp` (−80%) over `img-quant`
(−69%).

| | Size | PSNR | SSIM | Time |
|---|---|---|---|---|
| img-quant | 11 050 349 | **33.2 dB** | **0.958** | 9.8 s |
| pngquant 2.x | **11 036 728** | 31.6 dB | 0.932 | **8.4 s** |

**The size advantage is gone — a 0.12% loss — and the whole difference moves
into quality: +1.6 dB and +0.026 SSIM.** On noisy continuous-tone material both
programs hit the same floor, because what PNG can do with 256 dithered colours
over 17.9 megapixels is bounded by the dithering noise, not by the palette.

Where that quality sits is the interesting part, and it is not where one would
guess. Measured per region:

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

The honest summary of both tables: **the size win is material-dependent and the
fidelity win is not.**

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
img-diff -crop 4500,1300,500,250 a.png b.png # only that region
img-diff -min-psnr 35 ./src ./converted     # directories; exit 1 on a failure
```

`-crop` takes the same rectangle `img-look` takes, so a region can be looked at
and measured without restating it in different terms. It is worth reaching for
more often than it sounds: a whole-image average answers *is it broken* and
hides *where*. On the photograph measured under `img-quant`, two quantisers were
a tie across the smooth sky and 2.3 dB apart in the shadows, and only the
per-region figures said so.

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

# img-look

The tools above change files. This one changes nothing — it answers *let me see
it*, which turned out to be the thing most often written by hand around this
repo and thrown away again afterwards.

Four small programs kept being written during this project: one to stack a
before and an after, one to cut the same region out of both, one to drop a
transparent image onto a checkerboard, one to print a few pixel values. Ten
minutes each, deleted each time. They are all the same operation.

```bash
img-look hero.webp                            # any format in, one PNG to open
img-look before.png after.png                 # stacked, labelled, a red rule between
img-look -crop 700,380,460,210 a.png b.png    # the same region of both
img-look -max 0 -crop 0,0,64,64 icon.png      # native pixels, no scaling
img-look -crop 4600,1380,180,105 -zoom 4 a.png b.png   # that region, four times life size
img-look -crop 800,2400,500,250 -stats a.png b.png     # and what it averages to
img-look -at '450,300 20,40' shot.png         # the numbers instead of the picture
```

`-zoom` repeats pixels; it does not resample. The distinction matters because
the thing being inspected at four times life size is usually a suspected
rounding error, and a resampler would put a plausible picture in front of
someone trying to find out whether the pixels are right. It implies `-max 0`,
since magnifying and then capping the size would quietly undo the magnification.

`-stats` reports what the shown region averages to and how far its luma spreads.
The mean is taken **in linear light**, by handing the region to the same
resampler `img-resize` uses and asking for one pixel — so it agrees with
`img-resize -filter box -fit exact -width 1 -height 1` by construction, and a
test says so. An arithmetic mean of sRGB values would be a different number and
a wrong one: on the shadow patch measured above it reads 25 13 5 instead of
32 19 9.

The composite lands in the temporary directory — `%TEMP%\pixelita\look.png`,
`/tmp/pixelita/look.png` — and the absolute path is printed, and repeated in
`items[].output` under `-json`. Not the working directory: this produces
something to glance at and forget, and a glance should not leave a file for
`git status` to find later. `-out` puts it wherever you want it.

The output is a PNG because that opens anywhere and can be handed to anything
that reads images, and it is fitted to 1400px on the long side by default
because an eight-megapixel photograph tells a reader nothing a tenth of it would
not, at ten times the cost of looking. `-max 0` turns that off when the question
is about individual pixels.

Transparency is composited onto a checkerboard, because a convention that no
photograph can imitate is the only kind that cannot be misread as content.
Panels get the file name on a dark strip, so a label reads on a white image and
a black one alike; `-across` lays them side by side for tall images, and
`-label=false` removes the strip when it would cover the thing being examined.

`-at` is the other half of looking. Reading a picture tells you something is
wrong; reading the pixels tells you what:

```
img-look -at '450,300 20,40' screenshot.webp
  450,300  rgba 255 255 255 255  #ffffff
  20,40    rgba  38  41  40 255  #262928
```

Those four numbers are the ones that settled the colour-range investigation
described above — with this tool it would have been one command rather than an
afternoon.

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
