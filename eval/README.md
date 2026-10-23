# Current acceptance workflow

Build Pixelita, then run the deterministic corpus and CLI harness:

```powershell
go build -o bin/ ./cmd/...
python eval/generate_corpus.py "$env:TEMP/pixelita-eval/corpus"
python eval/refactor_check.py bin/pixelita.exe "$env:TEMP/pixelita-eval/corpus" "$env:TEMP/pixelita-eval/results.json"
```

Pillow is an evaluation-only dependency for generating the corpus. Pixelita's
build and runtime remain pure Go. The harness runs 14 CLI calls and records
arguments, exit codes, elapsed time, and full JSON results. It tests preview/apply
parity, JPEG identity, measured format selection, resize-to-WebP, worst regions,
multi-region viewing, and invalid threshold recovery. Temporary outputs are
removed automatically; inputs are retained.

This is deterministic acceptance, not an agent usability trial or a representative
compression benchmark. Timings include process startup and describe one local
run. No broad performance claim is made.

## Historical milestone-two comparison

The measurements below predate the refactor and its updated fidelity and
metadata policy. They are retained for provenance, not current benchmark claims.

### Evaluation harness

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
