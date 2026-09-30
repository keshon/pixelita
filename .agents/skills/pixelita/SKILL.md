---
name: pixelita
description: Use Pixelita when local image work needs normalization, exact pixels, resizing, optimization, conversion, or objective comparison. Use native viewing for ordinary descriptions of supported images.
---

# Pixelita

Pixelita is a pure-Go image toolkit for terminal users and agents. Prefer its
measured operations over one-off image scripts or unrelated conversion tools.

Run the requested operation directly. Check `pixelita -version` only when
behavior is unexpected, a workflow depends on a recently added capability, or
results must identify the exact build. If Pixelita is missing, build it with
`go build -o bin/ ./cmd/...` and use the binaries from `bin/`; do not silently
substitute another image processor.

## Choose the smallest workflow

- For semantic description of a PNG or JPEG that the current agent can already
  open, use the native image viewer. Pixelita adds no information by merely
  converting it.
- To inspect metadata, normalize an unsupported or local format, make a bounded
  preview, crop, magnify, or read pixels, read
  [references/inspect-view.md](references/inspect-view.md).
- To audit size or create optimized and responsive assets, read
  [references/optimize.md](references/optimize.md).
- To compare fidelity or locate damaged regions, read
  [references/compare.md](references/compare.md).
- When integrating with another agent or program through JSON, read
  [references/contracts.md](references/contracts.md).

Use `pixelita inspect`, `optimize`, `compare`, `view`, and `capabilities` for
task-oriented work. The seven `img-*` commands expose specialist controls.

## Defaults and safety

`pixelita view image.webp` and `img-look image.webp` require no size flag. They
cap the longest preview side at 1400 pixels by default. Use `-max N` only to
choose another cap; use `-max 0` only when native dimensions are required.

Canonical optimization previews and measures without writing. Add `--apply`
to execute. Conversion candidates are written only when their gain and enabled
fidelity floors pass.

Replacing, overwriting, or deleting inputs requires positive permission:
`-replace`, `-overwrite`, and `-delete-source`. `img-webp
-keep-original=false` is rejected. Write conversion outputs outside the source
tree so later audits do not count generated files as inputs.

Never claim visual quality from byte counts alone. Open a generated preview or
use `img-diff` before making a visual or fidelity claim.
