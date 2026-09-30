# Compare image fidelity

Use `pixelita compare a.png b.png` for a whole-image answer. Use `img-diff` when
you need regional diagnostics, ranking, or tonal-level analysis.

```bash
img-diff -worst 5 -levels original.png compressed.png
img-diff -crop "1150,1560,220,90 1280,2816,256,256" -levels a.png b.png
img-look -crop "1150,1560,220,90 1280,2816,256,256" a.png b.png
```

Rectangles use `x,y,w,h` everywhere. `-worst` reports source coordinates that
can be pasted directly into `-crop`; do not rescale coordinates by eye.

Choose the ranking that matches the question:

- `-by ssim` finds lost structure and flattened texture.
- `-by psnr` finds large pixel-level error.
- `-by levels` finds lost tonal headroom.

Read `p95/p99` beside the maximum delta. A large maximum with low percentiles is
an isolated outlier; elevated percentiles show distributed damage.

Approximate photographic calibration:

| Assessment | PSNR | SSIM |
|---|---:|---:|
| Indistinguishable | 40 dB or more | 0.99 or more |
| Fine for web delivery | 35–40 dB | 0.97–0.99 |
| Visible on inspection | 30–35 dB | 0.93–0.97 |
| Visibly damaged | below 30 dB | below 0.93 |

These ranges are not universal. Flat graphics and UI screenshots expose errors
that photographs can hide at the same score. Inspect the worst regions before
making a final quality claim.

Dimension-mismatched pairs are resampled from the second image to the first in
linear light and marked `resampled`. Use `-strict-size` when any mismatch must
fail. Directory pairing ignores extensions and recognizes Pixelita's generated
size suffixes.
