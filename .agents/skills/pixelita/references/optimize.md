# Optimize and resize

Use the canonical workflow for ordinary tasks:

```bash
pixelita inspect ./images
pixelita optimize ./images
pixelita optimize ./images --apply
```

`optimize` previews and measures by default. Review its decisions before adding
`--apply`. Same-format JPEG optimization and PNG palette quantization are
automatic. WebP conversion and dimension changes are explicit:

```bash
pixelita optimize ./images --format webp
pixelita optimize ./images --widths 640,1280,1920
```

Use specialist tools when their narrower controls matter:

- `img-quant` for flat PNG graphics, screenshots, icons, and UI assets.
- `img-webp` for photographs, gradients, and images with alpha.
- `img-jpeg` for pixel-identical JPEG Huffman optimization.
- `img-resize` for explicit dimensions or responsive variants.
- `img-scan` to measure candidate savings before choosing.

Every candidate must clear its gain and enabled fidelity floors. Useful guards
include `-min-gain`, `-min-psnr`, `-min-ssim`, `-dry-run`, and `-out-dir`.
Loosen them deliberately, not reflexively.

Read per-width scan results before recommending only a codec. Serving an
appropriately sized source often saves more than recompression. Resize from the
original, not from an already converted derivative, and note that responsive
variants need `srcset` or equivalent client selection.

Ask before source replacement, destination overwrite, or source deletion.
`-delete-source` is allowed only after a verified WebP write. Keep outputs
outside the input tree so rescans do not include generated variants.

AVIF and JPEG XL are out of scope because Pixelita requires production-quality
pure-Go codecs and no cgo dependency.
