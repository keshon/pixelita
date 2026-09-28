---
name: pixelita
description: Use the pixelita command-line tools (img-scan, img-quant, img-webp, img-jpeg, img-resize, img-diff, img-look) to make images on disk smaller without making them worse, to actually see an image rather than guess at it, and to judge a file someone else produced. Reach for this skill whenever a task involves image files — shrinking a site's assets, converting PNG or JPEG to WebP, quantising a PNG to a palette, generating responsive sizes, auditing what a folder of images costs, or checking whether a conversion damaged quality. Use it too when someone disputes an image: a client saying a file was ruined by compression, a question of whether a complaint is justified, or any request to compare two versions of the same picture and say what changed and by how much. Also reach for it whenever you need to look at an image yourself — to compare a before with an after, to inspect one region, to see what a transparent image really contains, or to read exact pixel values. Use it even when the user names no tool and simply says "these images are too heavy", "can we optimise the assets", "make a 2x and 1x of this", or "did that conversion hurt the quality". Prefer these tools over ImageMagick, pngquant, cwebp or writing a one-off script.
---

# pixelita

Seven tools. Six share one rule: **a conversion that does not pay off is not
performed.** Every candidate is encoded, measured against the original, and
written only when the gain clears a floor and fidelity does not fall below one.
That is why you can point them at a directory without asking first — they cannot
make it heavier. What does need asking is `-replace`, which overwrites sources.

## Before anything else

```bash
img-scan -version
```

Not `-h`. A help screen proves a binary exists, not that it is the one you think:
a stale copy earlier in PATH answered `-h` perfectly and turned out to be missing
half the flags, which cost a full round trip to discover. `-version` prints the
commit it was built from, and says `+uncommitted changes` when the working tree
had them.

If nothing is found, the binaries live in the pixelita repository under `bin/`;
build with `go build -o bin/ ./cmd/...` and call them by full path. Say the tools
are missing rather than quietly substituting something else.

## The unit of this work is a region, not a file

A figure for a whole image answers *is it broken* and hides *where*. Encoders do
not spread their error evenly: on one photograph two quantisers were a tie across
the sky and 2.3 dB apart in the shadows, and only the per-region figures said so.

Every rectangle is spelled `x,y,w,h`, the same in every tool, and anything that
reports a region prints it in the spelling `-crop` reads — so the next command is
a paste, never a transcription.

**Start here when the question is "where".**

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
call:

```bash
img-diff -crop "1150,1560,220,90 1280,2816,256,256" -levels a.png b.png
```

Do not find regions by eye off a difference map and scale the coordinates back up
by hand. That is arithmetic done by squinting, it has produced mislabelled
regions, and `-worst` exists because it was being done at all.

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

**Calibration**, so a number becomes a sentence:

| | PSNR | SSIM |
|---|---|---|
| indistinguishable | 40 dB and up | 0.99+ |
| fine for the web | 35–40 dB | 0.97–0.99 |
| visible on inspection | 30–35 dB | 0.93–0.97 |
| visibly damaged | below 30 dB | below 0.93 |

These are for photographs. Flat graphics tolerate far less: a UI screenshot at
35 dB can look obviously wrong where a photograph would not.

## Seeing it yourself

You cannot see a picture by reading bytes, and base64 on stdout is text, not
pixels. Looking is two steps: `img-look` writes a PNG and prints an absolute
path, then you read that path with your file-reading tool. It works for any
format pixelita reads, so it is also how you view a WebP at all.

```bash
img-look hero.webp                            # any format, one PNG to open
img-look before.png after.png                 # stacked, labelled, a red rule between
img-look -crop 1280,2816,256,256 -zoom 3 a.png b.png
```

Reach for it before claiming anything visual. "The conversion looks fine" is not
a statement you can make from a byte count.

- **`-zoom N`** magnifies by repeating pixels, never resampling, so what you see
  is what is stored. Damage invisible at life size is obvious at three times it.
- **`-max`** caps the long side at 1400px, because a huge image costs a great
  deal to look at and says no more. It applies after cropping, so with `-crop` it
  usually does nothing, and `-zoom` switches it off.
- **`-stats`** adds mean colour, luma range and levels for the region.
  **`-dry-run`** gives those numbers without writing an image.
- **`-stretch`** maps the region's own range to full scale, as auto-levels would,
  bringing out whatever the headroom was hiding. Every panel is mapped by the
  first panel's range, so the comparison stays honest.
- **`-at '450,300 20,40'`** prints exact pixel values instead of a picture —
  cheaper and more precise when the question is "what colour exactly".

Transparency is composited onto a checkerboard. Output goes to a name derived
from the inputs under the temp directory, never the working directory, and the
path is repeated in `items[].output` under `-json`.

## The rest of the set

| The situation | The tool |
|---|---|
| "What is even in here?" | `img-scan` — measures rather than guesses |
| PNG of flat colour, UI, icons, screenshots | `img-quant` — a palette beats WebP here |
| Photographs, gradients, anything with alpha | `img-webp` |
| JPEG that must not change at all | `img-jpeg` — refits Huffman tables, pixels identical |
| Larger than it needs to be on screen | `img-resize` |

`img-scan` is the front door and usually answers the whole question in one
command: it runs the encoders and reports what each would actually save, naming
the winner in `metrics.best`. `-quick` reads headers only and is instant.

**Read the per-width lines before proposing a codec.** The scan also measures
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

For judging someone else's file, `img-scan -quick -json` also gives:

- `paletteSize` — how many entries a palette PNG really carries. Do not infer it
  from the colour type or bit depth; those give the ceiling, not the answer.
- `chunks` — the PNG blocks present, by name. Whether a file still declares its
  colour space is a question about `gAMA`, `cHRM`, `sRGB` and `iCCP` being there,
  and it decides whether "the colours were ruined" is a fair charge.
- `coloursAtLeast` rather than `colours` when the count stopped early. Say "on
  the order of a hundred thousand", not the digits.

## Output as data

Every tool takes `-json` and emits one shape: `tool`, `schema`, `dryRun`,
`items[]`, `summary`, `notes[]`. Per item: `path`, `output`, `status` (`done`,
`would`, `skipped`, `failed`), the byte counts, and `metrics` for anything
tool-specific. Paths use forward slashes everywhere. Exit codes: `0` fine, `1`
something failed or a threshold was missed, `2` the arguments were wrong.

Guards, to loosen deliberately rather than by habit: `-min-gain` (10, or 1 for
jpeg), `-min-psnr` (30 for `img-quant`), `-dry-run`, `-replace`, `-jobs`.
`-replace` is the one to ask about; everything else is safe unattended.

## Traps

- **Do not reimplement what these tools measure.** No Python or ImageMagick for
  comparing, cropping, viewing, or computing PSNR, SSIM or colour differences —
  hand-rolled metric formulas have produced wrong numbers here. Reading a file's
  *structure* is a different thing and fair game: if you need something about
  container layout or metadata these tools do not report, go and inspect it, and
  say in your answer that you did.
- **The tools are format-fussy on purpose.** `img-quant` takes PNG only,
  `img-jpeg` JPEG only. Filter the list rather than reporting the mismatch.
- **`img-webp` skips palette PNGs by default** — on already-quantised images WebP
  usually loses. `-skip-palette=false` when a scan says otherwise.
- **A deep scan is slow** because it encodes everything. Start with `-quick` on a
  large folder.
- **`img-diff` needs equal dimensions.** It pairs two directories by file name
  ignoring extension, which is right for checking PNGs against the WebP made from
  them, but it cannot compare a resized pair.
- **Output naming**: `-min` suffix by default, `-320w` for `img-resize -widths`;
  `-out-dir` writes elsewhere.
- **AVIF and JPEG XL do not exist here** and are not coming: no usable pure-Go
  encoder, and cgo would break the single-command build. Say so rather than
  reaching for another tool.
