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

## Agent workflow trials

`agent_trials.py` runs fresh ephemeral Codex CLI sessions against synthetic
fixtures. It covers optimization, recovery from duplicate output destinations,
and damaged-region discovery with a preview. Guided sessions receive a copy of
the bundled skill; help-only sessions do not. The installed global Pixelita
skill is disabled per invocation, without changing user configuration. Other
global instructions may remain. This measures explicit guidance, not automatic
skill discovery. The runner uses the CLI-default model and records that it is
not pinned; do not infer a general model or skill advantage from a small batch.

```powershell
python eval/agent_trials.py "$env:TEMP/pixelita-agent-trials" --repeat 2
python -m unittest discover -s eval -p test_agent_trials.py -v
```

Requires an authenticated Codex CLI with working approved shell execution in
workspace-write mode, built Pixelita binaries, and Pillow for test fixtures.
Trial logs, prompts, and outputs stay in the specified directory. The runner
uses no personal corpus data. It records command events and token usage; artifact
graders independently check source hashes, distinct valid outputs, size/fidelity
floors, and damage-region overlap. Trace review is still needed to assess the
requested preview/verification sequence and the quality of explanations.
Command-event counts are not total tool-call counts.

The first two attempts on 2026-10-23 were infrastructure-blocked. In both
conditions, the nested CLI rejected shell creation before any Pixelita command
executed. Authentication and connectivity checks passed, but did not establish
the cause of the policy rejection. The runner now stops on such a rejection;
it does not disable rules or weaken the sandbox to continue. No agent task
success rate, guidance benefit, or tool efficiency is claimed. Aggregate
evidence is in `agent-trials-2026-10-23.json`; raw traces remain local.

The three deterministic grader tests passed, including negative controls for
missing outputs, changed sources, and an incorrect damage region. These tests
establish grader behavior, not agent usability. After the execution environment
is resolved, rerun in a fresh directory. Resume requires matching binary and
skill hashes and refuses to silently skip an earlier blocked attempt.

The trace-and-artifact approach follows the
[official skill evaluation guidance](https://developers.openai.com/blog/eval-skills).

## Private real-world corpus

Keep the corpus and output directory outside the checkout. The harness stores
source paths, hashes, detailed reports, and generated images in that directory.
Only the aggregate summary is suitable for committing.

```powershell
python eval/corpus_check.py E:/your/images "$env:TEMP/pixelita-corpus" --sample 24
python eval/corpus_check.py E:/your/images "$env:TEMP/pixelita-corpus"
python eval/corpus_summary.py "$env:TEMP/pixelita-corpus" --out summary.json
```

Sampling rotates through format, alpha, profile, and pixel-count strata. The
full run resumes completed records, requires unchanged executable hashes and
policy, and writes every input to its own output directory. PNGs use the same
floors as compare_png.py. JPEGs use 1% gain and independent pixel-identity
verification at 200 dB. Failed or unsupported inputs are recorded without
stopping the remaining corpus. A timeout kills that child after five minutes.
The summary fails on source changes, metadata mismatches, or failed output
verification. Check `completed == inputCount` before treating it as a full run.

Metadata verification compares PNG ancillary chunks except palette/transparency
encoding chunks, and JPEG APP/COM segments before the first scan, byte for byte.
The Windows harness reads the OS peak working set for each child process. Other
platforms report null memory values. Comparison memory is recorded separately
from pngquant encoding memory; neither is an entire batch's simultaneous peak.

The first private corpus contained 144 unique files (91 PNG and 53 JPEG),
including related screenshots, photographs, logos, gradients, and artwork.
`private-corpus-aggregate-2026-10-23.json` contains only aggregate results, with
no source names, paths, or image hashes. Pixelita wrote 86 PNGs and 33 JPEGs;
all 119 passed independent comparison and exact metadata checks. Four PNGs were
refused because of an unknown unsafe-to-copy iDOT chunk. One PNG and 12 JPEGs
missed the gain floor; eight progressive JPEGs were skipped. All 144 source
hashes remained unchanged.

Counting retained originals, PNG bytes fell from 62,621,206 to 25,446,998 and
JPEG bytes from 138,566,531 to 135,920,949. The pngquant reference accepted 90
PNGs and retained one, yielding 25,794,728 bytes; its metadata equivalence was
not assessed. This is a local workload result, not a general encoder ranking.
Worst-region spot checks showed visible dithering in photographic detail at
3x zoom despite passing whole-image quality floors. No perceptual-losslessness
claim is warranted.

After a metric change, replay the saved output comparisons without re-encoding:

```powershell
python eval/replay_comparisons.py "$env:TEMP/pixelita-corpus"
```

Replay requires identical metrics and exit statuses, checks source hashes,
and records before/after per-process memory in the private directory.

The first replay validated all 210 saved comparisons exactly. Replacing two
full-frame SSIM luma arrays with rolling row buffers reduced the maximum
comparison working set from 1,358,282,752 to 499,900,416 bytes. Re-optimizing the
largest image (3186 by 16820) produced identical output bytes and metrics, with
peak working set falling from 1,437,802,496 to 632,807,424 bytes. Raw aggregate
evidence is in `rolling-ssim-2026-10-23.json`. These are single local runs; no
controlled speedup is claimed. Source decoding still scales with image area.
`go test ./internal/metric -run '^$' -bench BenchmarkSSIMRows -benchmem`
checks SSIM allocation growth independently of file I/O.

## iDOT compatibility and quality-policy follow-up

`idot-recheck-2026-10-23.json` records rechecking all four previously refused
private screenshots through canonical preview and apply. All four wrote and
passed independent PSNR/SSIM verification, with identical preview/apply metrics
and unchanged sources. Combined size fell from 6,030,258 to 1,445,429 bytes.
Every ancillary chunk except the explicitly reported iDOT hint matched exactly.
The corpus metadata verifier permits only this named exception; it still rejects
undeclared removal, stale retained offsets, or changes to other chunks.

`policy_sweep.py` re-evaluates saved candidates without re-encoding:

```powershell
python eval/policy_sweep.py "$env:TEMP/pixelita-corpus" --out policy.json
# --overrides accepts a private directory of newer per-image records.
```

`quality-policy-sweep-2026-10-23.json` includes the four corrected candidates.
At 10% gain, the 30 dB / 0.95 floor accepts 90 of 91 PNGs and saves 41,759,037
bytes. The 35 dB / 0.98 floor accepts 87 and saves 39,100,314 bytes; 40 dB / 0.99
accepts 78 and saves 32,277,149 bytes. These are explicit evaluation scenarios,
not CLI defaults. The strictest scenario rejects the three inspected candidates
that showed dithering at 3x zoom. This supports offering a conservative command,
not claiming perceptual losslessness or changing the default for every workload.

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
