# Inspect and view

Use the native viewer for a straightforward description when it already accepts
the file. Use Pixelita when the agent cannot open the source directly or when a
repeatable crop, comparison layout, bounded preview, statistic, or pixel value
is part of the task.

## Inventory

`pixelita inspect` is the task-oriented entry point. Add `--measure` when actual
candidate encodes are worth the extra time. For a fast header-only specialist
report, use:

```bash
img-scan -quick -json image-or-directory
```

The scan reports dimensions, format, byte size, palette information, relevant
PNG chunks, and `coloursAtLeast` when exact colour counting stopped early.

## View

```bash
img-look hero.webp
img-look before.png after.png
img-look -crop 700,380,460,210 a.png b.png
img-look -crop 4600,1380,180,105 -zoom 4 a.png b.png
img-look -at '450,300 20,40' shot.png
```

Open the absolute PNG path printed by `img-look`. Output is written under the
system temporary directory unless `-out` is supplied.

The size cap is optional. Omitting `-max` uses the 1400-pixel long-side default.
`-max N` selects another cap and `-max 0` preserves native dimensions. `-zoom`
uses exact repeated pixels and disables the default cap unless a cap was set
explicitly.

- `-crop` accepts one or several space-separated `x,y,w,h` rectangles.
- `-stats` reports linear-light mean colour, luma range, and per-channel levels.
- `-dry-run` reports without writing a preview.
- `-stretch` maps the first panel's range to full scale and applies the same
  mapping to every panel.
- `-at` prints exact RGBA and hex values; add `-json` for structured output.
- Transparent pixels are shown on a checkerboard by default.

Do not infer visual content from scan metadata or byte counts. Open the preview.
