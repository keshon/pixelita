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

## PNG reference comparison

`compare_png.py` accepts PNG paths or directories and compares fixed encoder
settings using the same Pixelita decoded metrics and acceptance floors. It needs
pngquant on PATH and the built binaries. Inputs are hashed before and after;
reference outputs live in a temporary directory.

```powershell
$moduleCache = go env GOMODCACHE
$fixtures = "$moduleCache/golang.org/x/image@v0.45.0/testdata"
python eval/compare_png.py --report results.json --repeat 3 `
  "$fixtures/gopher-doc.with-alpha.png" "$fixtures/testpattern.png" `
  "$fixtures/tux.png" "$fixtures/video-001.png" "$fixtures/yellow_rose.png"
```

Default floors are 10% gain, 30 dB for both RGB and alpha-aware PSNR, and 0.95
SSIM. Both tools use 256 colours and full dithering; Pixelita effort 6 and
pngquant speed 4 are fixed settings, not equivalent search budgets. These are
matched acceptance constraints, not matched achieved quality. Pixelita runs a
verified preview; pngquant writes a temporary candidate measured by `compare`.
Reported timings therefore describe different workflows, not codec speed.
Metadata equivalence and perceptual quality are not assessed.

The checked-in `png-fixtures-2026-10-23.json` records three repetitions on five
dependency fixtures using pngquant 2.17.0. Both tools accepted four fixtures and
rejected yellow_rose. Retaining the rejected original, total bytes were 164,551
for Pixelita and 166,446 for pngquant, from 205,325 input bytes. The rose fixture
has transparency: its RGB PSNR is about 10.56 dB despite alpha-aware PSNR above
37 dB. The existing policy deliberately requires both; this is not evidence of
equally severe visible damage.

`png-synthetic-2026-10-23.json` uses flat-rgba.png and photo-rgb.png from
generate_corpus.py. Both accepted both files; Pixelita produced 350,743 bytes
and pngquant 331,790, from 959,172 input bytes. Neither small collection supports
a general compression advantage. Reports identify binary hashes, tool versions,
source hashes, every repetition, and the acceptance policy. Missing PSNR fields
denote infinity only after successful measurement, following schema 2.

## Quantizer profiling

Use the version-pinned dependency fixture without copying it into the repository:

```powershell
$env:CGO_ENABLED = "0"
$moduleCache = go env GOMODCACHE
$env:QUANT_BENCH_IMAGE = "$moduleCache/golang.org/x/image@v0.45.0/testdata/yellow_rose.png"
go test ./internal/quant -run '^$' -bench 'QuantizeFile|Phases' -benchmem -benchtime=1s -count=3
```

Individual phases can be selected, for example `-bench 'Phases/remap$'`.
Phase setup is independent of which other sub-benchmarks execute. Median cut
receives fresh histogram ordering each iteration; the reset is outside timing.
QuantizeFile excludes file decoding, PNG encoding, and verification. Allocation
bytes are cumulative Go allocations per operation, not peak resident memory.
The NRGBA fixture takes the no-copy path in the conversion phase.

The 2026-10-23 Windows run used Go 1.26.4, CGO disabled, and an i5-9600KF.
The fixture is 400 by 301 pixels, 125,392 bytes, with SHA-256
`c0a5788aea933d7bab215074684b899b4a20438205d1231d840e62f6e2aef600`.
Raw measurements are in `quant-baseline-2026-10-23.txt`. This is a dependency
fixture baseline, not a representative asset corpus or a codec comparison.
Timing varied substantially across the three repetitions; no speedup is claimed.

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
