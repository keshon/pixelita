# pixelita

Seven command-line tools for images, in pure Go with no cgo. Build them with
`go build -o bin/ ./cmd/...` — that is the only build step, and keeping it that
way is a constraint rather than a convenience.

## The rule everything answers to

**A conversion that does not pay off is not performed.** Every candidate is
encoded, measured against the original, and written only when the gain clears
`-min-gain` and the fidelity does not fall below `-min-psnr`. This is why the
tools are safe to point at a directory, and it is the property to protect when
changing anything.

`-crop` means the same rectangle in `img-look` and `img-diff` on purpose: the
unit of this work is a region, not a file, and looking at one and measuring it
should not require restating it. Anything else that learns to work on part of an
image takes the same flag with the same `x,y,w,h` spelling.

The two tools that answer questions rather than change files — `img-diff` and
`img-look` — are what makes that rule checkable. When a change to an encoder
needs verifying, measure with the first and look with the second; do not assert
that output is correct without having done one of the two.

## Where code goes

All the work lives in `internal/ops`. A command under `cmd/` is a flag parser
and a table of columns around one function there, and so is the web interface on
the `web-ui` branch. That is deliberate: if the interface and the command line
share an implementation, they cannot drift, and "the UI does what the CLI does"
stops being a promise anyone has to keep by hand.

To add a tool: write the operation in `internal/ops`, then `cmd/img-<name>` as
a thin caller, with `-json` and `-dry-run` from the first commit.

| Package | What it holds |
|---|---|
| `internal/ops` | What each tool does to a file, and what it decides |
| `internal/report` | The one output shape: table for a person, JSON for everything else |
| `internal/quant` | The palette quantiser: histogram, median cut, k-means, dithered remap |
| `internal/jpegopt` | JPEG rewritten at the coefficient level, pixels untouched |
| `internal/resize` | Resampling in linear light with premultiplied alpha |
| `internal/metric` | PSNR and SSIM |
| `internal/imgio` | Decoding, encoding, and reading headers without decoding |
| `internal/imgio/exif.go` | The one EXIF tag applied at decode: orientation |
| `internal/cli` | Walking paths, spreading work across cores |

## Claims are measurements

Every number in the README came from running the tool against a reference and
writing down what happened — pngquant for the quantiser, libpng for the PNG
writer, the decoder itself for losslessness. If you change something that moves
one of those numbers, re-measure and update it. An estimate dressed as a
measurement is worse than no number.

The corpus lives outside the repo. `internal/quant/bench_test.go` and the
`TestCorpus` in `internal/jpegopt` both take a directory through an environment
variable, so a benchmark run needs real images pointed at rather than committed.

```bash
go test ./...
JPEGOPT_CORPUS=/path/to/jpegs go test ./internal/jpegopt -run TestCorpus -v
QUANT_BENCH_IMAGE=/path/to/photo.png go test ./internal/quant -bench Phases
```

`img-diff` is how a change to an encoder gets checked: run the tool over a
corpus, then compare the results against the sources. For `img-jpeg` the bar is
absolute — `img-diff -min-psnr 200` must report zero failures, because identical
is the whole claim.

## Things settled, worth not relitigating

- **AVIF and JPEG XL are out of scope.** No usable pure-Go encoder exists, and
  cgo would break the single-command build. The predecessor project used cgo for
  exactly this and it is the pain being escaped.
- **Metadata is never stripped** by an operation that promises not to change the
  picture. Dropping EXIF turns a photograph on its side; dropping an ICC profile
  changes the colours a browser paints.
- **EXIF orientation is applied at decode**, not carried downstream. The tag
  does not survive re-encoding and half the formats written here cannot hold it
  at all, so the only honest place to honour it is the moment the pixels are
  read. `ReadHeader` reports the turned size to match. `img-jpeg` is exempt
  because it never decodes: the original tag is still in the file it writes.
- **Binaries keep the `img-` prefix** whatever the repository is called. It
  groups them in PATH and it is what gets typed.
- **The web interface is parked** on the `web-ui` branch until the CSS kit it is
  built on settles. Do not resume it without being asked.

## Style

Comments explain *why*, not *what* — the code already says what. Where a
decision looks arbitrary, the comment is the place the measurement that drove it
gets written down. Several of the subtler bugs in this repo were found because a
comment claimed something the code did not do.
