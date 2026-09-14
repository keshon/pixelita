---
name: pixelita
description: Use the pixelita command-line tools (img-scan, img-quant, img-webp, img-jpeg, img-resize, img-diff, img-look) to make images on disk smaller without making them worse, and to actually see an image rather than guess at it. Reach for this skill whenever a task involves image files in a project — shrinking a site's assets, converting PNG or JPEG to WebP, quantising a PNG to a palette, generating responsive sizes, stripping weight from a photo folder, auditing what a folder of images costs, or checking whether a conversion damaged quality. Use it even when the user does not name a tool and simply says things like "these images are too heavy", "can we optimise the assets", "make a 2x and 1x of this", or "did that conversion hurt the quality". Use it too when someone disputes an image: a client saying a file was ruined by compression, a question of whether a complaint is justified, or any request to compare two versions of the same picture and say what changed and by how much. Also reach for it whenever you need to look at an image yourself — to compare a before with an after, to inspect one region of a screenshot, to see what a transparent image really contains, or to read exact pixel values. Prefer these tools over ImageMagick, pngquant, cwebp or writing a one-off script.
---

# pixelita

Seven small tools. Six share one rule: **a conversion that does not pay off is
not performed.** Every candidate is encoded, measured against the original, and
written only when the gain clears a floor and fidelity does not fall below one.

That rule is why you can point them at a directory without asking permission
first — they cannot make it heavier. What does need asking is `-replace`, which
overwrites sources in place.

## Before anything else: are they on PATH?

```bash
img-scan -h
```

If that fails, the binaries live in the pixelita repository under `bin/`. Build
them with `go build -o bin/ ./cmd/...` from the repo root, then either add that
`bin` to PATH or call them by full path. Do not silently fall back to
ImageMagick — say that the tools are missing and offer to build them.

## Seeing an image yourself

You cannot see a picture by reading its bytes, and base64 on stdout is text, not
pixels — it costs a fortune and shows you nothing. The way to actually look at
an image is two steps, and `img-look` exists to make the first one one command:

```bash
img-look before.png after.png     # prints an absolute path
```
then read that path with your file-reading tool. It decodes the PNG and puts the
picture in front of you. That works for **any** format pixelita can read — WebP,
JPEG, palette PNG — so this is also how you view a WebP at all.

Reach for it before claiming anything visual. "The conversion looks fine" is not
a statement you can make from a byte count.

```bash
img-look hero.webp                          # any format → one PNG to open
img-look before.png after.png               # stacked, labelled, a red rule between
img-look -crop 700,380,460,210 a.png b.png  # the same region of both, magnified by cropping
img-look -max 0 -crop 0,0,64,64 icon.png    # native pixels, no scaling
img-look -across tall-a.png tall-b.png      # side by side instead of stacked
img-look -crop 4600,1380,180,105 -zoom 4 a.png b.png   # that region, 4x, pixels repeated
img-look -crop 800,2400,500,250 -stats a.png b.png     # and what it averages to
```

**`-zoom` is the one to remember.** Compression and quantisation damage is
usually invisible at life size and obvious at four times it. Pixels are repeated,
never resampled, so what you see is what is stored — which is the point when the
thing you suspect is a rounding error. It implies `-max 0`.

**`-stats` also answers "is this file still editable?"** — a question that
otherwise turns into an argument. It reports `levels`, the number of distinct
values each channel still uses in the region. That is the tonal headroom:

```bash
img-look -dry-run -stats -crop 1150,1560,220,90 original.png compressed.png
  original    levels 69/90/90
  compressed  levels 19/21/20     ← a gradient with twenty levels will band
```

Measure a **smooth** region for this, not the whole image: across a whole frame
the figure stays high and says nothing, while the gradient where it matters has
already collapsed. Do not prove the point by applying a tone curve and counting
colours afterwards — it works, but the curve is yours to choose and the client
can say so. The level count needs no curve.

`-stretch` is the picture that goes with it: the region's own range mapped to
full scale, as auto-levels would, which brings out whatever the headroom was
hiding. `-dry-run` gives the numbers without writing an image.

**`-stats` when your eye might be fooling you.** It reports the mean colour of
the region (in linear light) and its luma range. Reach for it the moment you
want to say something like "the shadows got crushed" or "the tint shifted" — it
is a two-second check that turns an impression into a number, and impressions
about images are wrong more often than they feel.

Defaults worth knowing: the composite goes to `%TEMP%\pixelita\look.png` (or
`/tmp/pixelita/look.png`), **not** the working directory — so it never dirties a
repository, and each look overwrites the last. Use `-out` when two must exist at
once. Long side is capped at 1400px because a huge image costs a great deal to
look at and tells you no more; `-max 0` turns that off when the question is
about individual pixels. Transparency is composited onto a checkerboard.
`items[].output` in `-json` carries the path, so nothing has to be parsed out of
prose.

**When the question is "what colour exactly", use `-at` instead** — it prints
numbers, which are cheaper and more precise than a picture:

```bash
img-look -at '450,300 20,40' screenshot.webp
  450,300  rgba 255 255 255 255  #ffffff
  20,40    rgba  38  41  40 255  #262928
```

This is the tool that settled a real bug in this repo: a decoder was returning
studio-range luma, and four printed numbers proved it in one command.

## The order of work: look, act, verify

**1. Look.** `img-scan` is the entry point and the reason the set exists.
It does not guess from file extensions — it runs the encoders and reports what
each would actually save.

```bash
img-scan ./public/img              # measures; slow and honest
img-scan -quick ./public/img       # headers only; an inventory, instant
img-scan -json ./public/img        # the same, machine-readable
```

When judging a file someone else produced, `img-scan -json` also reports
`paletteSize` — how many entries a palette PNG actually carries. Do not infer it
from the colour type or the bit depth; those give the ceiling, and "256 colours"
inferred when the truth is 64 is a claim you cannot support.

Watch for `coloursAtLeast` rather than `colours` in the JSON: on a rich image
the count stops early, and the field is named that way so the number cannot be
quoted as exact. Say "on the order of a hundred thousand", not the digits.

Read the summary before proposing anything. A folder is usually a minority of
files carrying most of the weight, plus a tail that is already optimal and would
only grow.

**2. Act.** Pick the tool the scan pointed at. Every tool defaults to writing
next to the source with a suffix, so nothing is destroyed:

```bash
img-quant  ./img     # PNG → palette PNG
img-webp   ./img     # PNG and JPEG → WebP
img-jpeg   ./img     # JPEG → smaller JPEG, pixels untouched
img-resize -max-width 1920 ./img
```

**3. Verify.** Especially after anything lossy, and always worth doing when the
user cares about a photograph:

```bash
img-diff ./before ./after                 # PSNR, SSIM, worst pixel
img-diff -min-psnr 35 ./before ./after    # exit 1 if any pair falls below
img-diff -out diff.png a.png b.png        # a difference map, brightened
img-diff -crop 4500,1300,500,250 a.png b.png   # only that region
```

**Start with `-worst` when the question is "where".** It divides the image into
tiles, measures each, and prints the most damaged ones in the exact spelling
`-crop` takes:

```bash
img-diff -worst 5 original.png compressed.png
  region (x,y,w,h)        psnr   ssim worst  p95/p99
  1280,2816,256,256    30.0 dB  0.693    35    16/19
```

Then paste a region straight into the next two commands — no arithmetic, no
reading coordinates off a scaled-down picture:

```bash
img-diff -crop 1280,2816,256,256 original.png compressed.png   # the number
img-look -crop 1280,2816,256,256 -zoom 3 original.png compressed.png   # the picture
```

Do not find regions by eye off a difference map and scale the coordinates back
up by hand. That is arithmetic, it is done wrong, and `-worst` exists because it
was being done at all — twice, by readers who both then picked regions markedly
better than the real worst.

`p95/p99` is the error 95% and 99% of pixels stay under. Read it next to
`worst`: `worst 92, p95/p99 18/25` is one stray pixel, while `worst 35, p95/p99
16/19` is damage spread across the region. A maximum on its own cannot tell you
which you have.

## Which tool for what

| The situation | The tool | Why |
|---|---|---|
| PNG with flat colour, UI, icons, screenshots | `img-quant` | A palette beats WebP on this material |
| Photographs, gradients, anything with alpha | `img-webp` | Wins several-fold on true colour |
| JPEG that must not change at all | `img-jpeg` | Refits Huffman tables; pixels are identical |
| Anything larger than it needs to be on screen | `img-resize` | Resampling in linear light |
| "Did we break it?" | `img-diff` | The only tool that answers this |
| "What is even in here?" | `img-scan` | Measures rather than guesses |
| "Let me actually see it" | `img-look` | The only way you get pixels in front of you |

`img-jpeg` is the one with no downside to weigh: it cannot change the picture,
so any saving at all is worth taking. There is no quality flag on it for that
reason.

## What each tool refuses to do, and how to override

The guards are the point of the set. Loosen them deliberately, not by habit.

| Flag | Default | Meaning |
|---|---|---|
| `-min-gain` | 10 (1 for jpeg) | Below this percent saved, keep the original |
| `-min-psnr` | 30 (`img-quant`) | Below this fidelity in dB, refuse to write |
| `-dry-run` | off | Measure and report, write nothing |
| `-replace` | off | Overwrite the source instead of writing beside it |
| `-jobs` | one per core | Lower it on very large images; each worker holds a full decode |

`-replace` is the one to ask about. Everything else is safe to run unattended.

## Reading the output as data

Every tool takes `-json` and emits one schema, so you never have to parse a
table:

```json
{
  "tool": "img-quant", "schema": "1", "dryRun": true,
  "items": [{
    "path": "public/img/hero.png", "output": "public/img/hero-min.png",
    "status": "would", "bytesBefore": 253747, "bytesAfter": 72285,
    "gainPercent": 71.5, "metrics": { "colors": 256, "psnr": 37.8 }
  }],
  "summary": { "files": 1, "changed": 1, "skipped": 0, "failed": 0,
               "bytesBefore": 253747, "bytesAfter": 72285, "gainPercent": 71.5 }
}
```

`status` is `done`, `would`, `skipped` or `failed`. Anything tool-specific lives
in `metrics`, so the shape is stable across tools. Paths always use forward
slashes. Exit codes: `0` fine, `1` something failed or a threshold was missed,
`2` the arguments were wrong.

For a scan, `metrics.best` names the tool worth running and `metrics.quantBytes`
/ `metrics.webpBytes` hold what each would produce — enough to plan the whole
job from one JSON document.

## Traps worth knowing before you hit them

- **The tools are format-fussy on purpose.** `img-quant` takes PNG only,
  `img-jpeg` takes JPEG only. Pointing one at the wrong format is not an error
  worth reporting to the user — filter the list first.
- **`img-webp` skips palette PNGs by default.** On already-quantised images WebP
  usually loses, so it declines. `-skip-palette=false` if the scan says WebP
  still wins on a particular file.
- **A deep scan is genuinely slow** because it encodes everything. On a folder
  of hundreds, either warn the user or start with `-quick`.
- **`img-diff` needs equal dimensions.** It pairs two directories by file name
  ignoring extension, which is exactly right for checking a folder of PNGs
  against the WebP made from it — but it cannot compare a resized pair.
- **Output naming**: `-min` suffix by default (`hero.png` → `hero-min.png`),
  `-320w` for `img-resize -widths`. Use `-suffix` to change it.
- **Do not write a Python or ImageMagick script to view, crop or compare an
  image.** That is what `img-look` is; it was built precisely because those
  throwaway scripts kept being written and thrown away again.
- **AVIF and JPEG XL do not exist here** and are not coming: no usable pure-Go
  encoder, and cgo would break the single-command build. Say so rather than
  reaching for another tool.

## Worked example

The user says the site feels heavy.

```bash
img-scan -json ./public/img > /tmp/scan.json
```

Read `summary` for the total and `items[].metrics.best` for the split. Report it
in their terms — "340 files, 82 MB; 210 of them would drop to 21 MB, the other
130 are already optimal" — then propose the two or three commands that do it,
and run `img-diff` afterwards so the claim is checked rather than asserted.

When the work touches something the user will look at — a hero image, a logo, a
photograph — finish by looking at it yourself:

```bash
img-look -crop 400,200,600,400 hero.png hero-min.png
```
then read the printed path. A number said the conversion was fine; this is the
part that checks whether it *looks* fine.

Do not convert everything you can. A folder where a third of the files are
already optimal is normal, and touching them wastes time and churns the repo
history for nothing.
