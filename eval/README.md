# Evaluation harness

These scripts create the fixed milestone-two corpus outside the repository and
run the comparison Pillow workflow. They are evaluation dependencies, not
Pixelita runtime dependencies.

```powershell
python eval/generate_corpus.py $env:TEMP/pixelita-eval/corpus
python eval/pillow_baseline.py $env:TEMP/pixelita-eval/pillow `
  $env:TEMP/pixelita-eval/corpus/flat-rgba.png `
  $env:TEMP/pixelita-eval/corpus/photo-rgb.png `
  $env:TEMP/pixelita-eval/corpus/baseline.jpg `
  $env:TEMP/pixelita-eval/corpus/progressive.jpg
```

The corpus generator is deterministic. Pillow 12.3.0 produced the following
results on Windows 11:

| Input | Input bytes | Pillow bytes | Gain | PSNR |
|---|---:|---:|---:|---:|
| `flat-rgba.png` | 4,650 | 2,547 | 45.2% | 41.56 dB |
| `photo-rgb.png` | 954,522 | 317,853 | 66.7% | 36.59 dB |
| `baseline.jpg` | 325,703 | 319,755 | 1.83% | 71.26 dB |
| `progressive.jpg` | 303,513 | 319,755 | -5.35% | 71.26 dB |

On the same files, `pixelita optimize` wrote 3,736 bytes at 74.06 dB, 347,007
bytes at 38.02 dB, and 319,773 pixel-identical bytes for the first three files.
It skipped the progressive JPEG instead of making it larger. Preview, apply,
and comparison used three Pixelita invocations. One local timing run took 2.49
seconds for the Pillow optimization, 0.75 seconds for the Pixelita preview, and
0.78 seconds for Pixelita apply. These timings describe this corpus and machine;
they are not general performance claims.
