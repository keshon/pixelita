# Agent-facing evaluation

## Milestone-two protocol (fixed before implementation)

The milestone-two comparison uses one generated corpus: a flat RGBA PNG, a
photographic RGB PNG, a baseline JPEG, a progressive JPEG, two inputs with the
same basename, and one unsupported text file. Keep the corpus outside the
repository and retain the generator or exact source commands with the results.

Run these six scenarios on the milestone-one baseline and the completed
milestone, repeating each agent-driven run three times where practical:

1. discover the installed build and supported capabilities;
2. inspect the mixed corpus and produce an optimization plan;
3. preview, apply, and verify safe same-format optimization;
4. request an explicit WebP conversion and an explicit resize variant;
5. recover from a flag after a path, unsupported input, and corrupt input;
6. reject basename collisions, existing destinations, replacement, and source
   deletion before an unauthorized write.

Compare four workflows: the milestone-one specialist commands, the canonical
interface without the skill, the canonical interface with the shipped skill,
and an agent-written Pillow script. For every run record completion, source
preservation, unsafe behavior, Pixelita or shell invocations, invalid calls,
recovery turns, elapsed wall time, input and output bytes, PSNR/SSIM where
applicable, and setup or dependency work. Treat each file as one observation
for size-quality-time comparisons; do not require one implementation to win
every file. The standard inspect, optimize-preview, optimize-apply case passes
the weak-model gate only when it completes in at most three Pixelita invocations
without reading the long skill.

Record skipped or unavailable comparisons as limitations, not zero-valued
measurements. Automated repository tests cover deterministic contracts; model
trials and Pillow timing remain manual evaluation evidence.

## Milestone-two verification status

The milestone-one commit `698306e` passed `go test ./...`, `go vet ./...`, and
the eight-binary build before implementation. The milestone-two checkout passes
the same three gates plus focused tests for interspersed flags, JSON argument
errors, collision and existing-destination preflight, atomic verification
failure, cross-input output overlap, explicit deletion, plan/apply routing
equivalence, deterministic plan ordering, capability output, and shared
candidate verdicts.

Windows end-to-end checks exercised all five canonical commands, a specialist
command with flags after its path, and rejection of
`img-webp -keep-original=false`. A baseline JPEG produced by the Go JPEG encoder
was previewed as `would`, applied as `done` with `jpeg-huffman`, and compared at
`-min-psnr 200` as pixel-identical. The standard preview, apply, and verify path
therefore takes three Pixelita invocations.

The deterministic Pillow 12.3.0 comparison is recorded in
[`eval/README.md`](../eval/README.md). Pillow made both PNGs smaller, at lower
measured fidelity. On the baseline JPEG, Pixelita produced a pixel-identical
319,773-byte result while Pillow produced a 319,755-byte result at 71.26 dB.
Pillow made the progressive JPEG 5.35% larger; Pixelita skipped it.

Repeated weak-model trials and the skill/no-skill comparison have not been
executed because no weak-model runner is available in the evaluation
environment. No comparative agent-performance result is claimed.

## Scope and baseline

This evaluation measures command discovery, workflow selection, argument
formation, result interpretation, error recovery, and quality verification by a
materially weaker model without hidden prompt knowledge.

The authoritative baseline was `origin/main` at commit `43ebea6` on Windows 11.
The original checkout was not modified. Tests used deterministic generated PNG
and JPEG fixtures plus temporary output directories; no repository image was
overwritten.

Verification results:

```text
go test ./...                       pass (15 packages)
go vet ./...                        pass
go build -o <temp>/ ./cmd/...       pass (8 binaries)
```

The checked-out source and installed tools did not match. `PATH` resolved all
seven `img-*` commands to a September 14 build at commit `5a1aeaf` with
uncommitted changes, while the evaluated source was the September 29 commit
`43ebea6`. `pixelita` was not on `PATH`. The documented version check detects
this, but only after an agent already knows the check exists.

## Milestone-one baseline verdict

The image operations and measurement commands passed the tested workflows. The
baseline interface was not model-agnostic. A weaker model still depended on the
repository skill for routing and safety rules. The baseline `pixelita` binary
only forwards to seven specialist binaries; it does not implement the target
inspect, plan, execute, and verify workflow. Implementing that canonical
workflow is the main remaining product milestone.

The seven commands remain useful specialist tools for humans. Weaker agents and
future codec additions need a stable task interface organized around inspect,
plan, execute, and verify. Codecs belong in a compile-time capability catalog. Specialist
commands remain thin compatibility adapters.

## Evaluation matrix

| Dimension | Assessment | Confirmed evidence |
|---|---|---|
| Discovery | Weak | One dispatcher exists, but it was absent from `PATH`; all installed specialists were stale. |
| Happy-path operations | Strong | Quick/deep scan, quantization, WebP, lossless JPEG, responsive resize, look/probe, whole-image diff, crops, and worst regions worked. |
| Conversion safety | Mixed | Gain and fidelity floors worked. Output collisions and a source-delete mode remain unsafe. |
| Structured output | Good | Shared schema 2, forward-slash paths, scan totals, probe JSON, stable item statuses, and partial failures are useful. |
| Argument recovery | Mixed | The commands reject unsupported explicit files, invalid ranges/formats, and conflicting resize modes. Flags after a positional argument still fail as a bogus path. |
| Result interpretation | Mixed | Metrics are rich, but summary counters describe outcomes rather than unique sources and quick scans report zero summary bytes while notes contain the inventory total. |
| Task routing | Weak | `pixelita` forwards to codec/operation commands; it does not accept a goal, compare strategies, serialize a plan, or explain a route. |
| GUI readiness | Partial | Operations are shared in `internal/ops`, but there is no stable public typed API, planner, or capability catalog. |
| Format extensibility | Weak | The visible model remains a flat tool list. More format-named binaries would compound discovery and routing cost. |
| Determinism/composability | Mixed | File collection is sorted and JSON paths are stable; basename collisions and prose-only recovery knowledge break reliable composition. |

## Confirmed strengths

- The project builds in pure Go with one command and no external codec tools.
- Operations are separated from flag parsing, which provides a base for one
  engine and multiple interfaces.
- Deep scan measures real encoder output rather than projecting savings.
- Lossy WebP and PNG quantization enforce fidelity floors; scan reports the
  same WebP PSNR/SSIM evidence used by the converter.
- `img-diff` combines whole-image metrics, worst-region search, reusable crop
  coordinates, percentiles, and tonal levels. This materially reduces agent
  calls compared with one-region-at-a-time tooling.
- `img-look` provides visual artifacts, multi-region composites, statistics,
  and structured pixel probes instead of forcing agents to invent viewers.
- JSON remains valid on per-item failures, and exit code 1 distinguishes partial
  execution failure from a clean run.
- Directory traversal is stable, paths are normalized, and unrelated files in
  a directory are intentionally ignored.
- Lossless JPEG optimization verified pixel identity in tests and in a fixture
  comparison.

## Confirmed baseline defects and hazards

### P0: output collisions are not preflighted

Two inputs named `a/same.png` and `b/same.png` were quantized concurrently with
one `-out-dir`. Both items reported `done` and the summary counted two outputs,
but both named `same-min.png`; only one file existed. The report was false and
one result was lost. Plan and collision-check every output path before parallel
encoding or writing.

### P0: destructive WebP behavior is disguised as retention policy

`img-webp -keep-original=false` successfully deleted the source fixture. The
skill previously said only `-replace` required approval. Documentation in this
branch now states the truth, but the interface remains wrong. Deletion should be
an explicit destructive request represented in the plan and result, with
failure-safe ordering and atomic output.

### P1: command discovery is not self-sufficient

The top-level binary was absent from `PATH`, while stale specialist binaries
were present. `-version` reveals the mismatch but there is no machine-readable
installation/capability check and no warning from the stale executable. A weak
model cannot use a check it has not discovered.

### P1: the front door exposes tools, not goals

`pixelita` is a process launcher. Its help asks the caller to choose scan,
quant, WebP, JPEG, resize, diff, or look before the system has inspected the
problem. `img-scan` helps with one optimization route, but it is not a general
planner: it does not model presentation, exact preservation, metadata goals,
or future codecs as comparable capabilities.

### P1: option placement produces opaque recovery

Go's standard flag parser stops at the first positional argument. Running
`pixelita scan image.png -quick -json` treated `-quick` as a path and returned a
Windows `GetFileAttributesEx` error with no JSON. Either the canonical interface
must accept flags in ordinary positions or it must identify the ordering error
and return a structured correction.

### P1: the result envelope mixes source and outcome counts

Responsive resize of one source into two widths reported `summary.files: 2`
and counted the original bytes twice. Quick scan items carried `bytesBefore`,
but the summary reported zero bytes because skipped items are excluded; a note
held the actual inventory total. The field names do not distinguish sources
from outcomes. Schema 3 should separate input count, outcome count, inventory
bytes, candidate bytes, and changed bytes.

### P2: human notes still leak machine-relevant defects

A partial quick scan over one broken PNG and one valid PNG produced the note
`(1 , 1 png)`, because the failed item contributed an empty format key. The JSON
failure item was correct. The malformed note confirms that agents must not
depend on note text.

### P2: repository hygiene

The evaluated baseline tracked six input/output images under `temp/`, totalling
17,050,698 bytes (16.3 MiB). The reviewed changes remove them and ignore
`/temp/`.

## Validated behavior after corrections

- Explicit unsupported files now fail with exit code 2 and name the accepted
  extensions; empty matching directories remain a valid empty result.
- Quant, scan, and WebP reject invalid ranges rather than silently clamping or
  passing them to an encoder.
- Resize rejects negative dimensions, invalid formats/fits/quality, a missing
  sizing request, and conflicting sizing modes before reading inputs.
- Exit-code documentation now matches implementation: a deliberate threshold
  skip is success, an item failure is 1, and invalid arguments are 2.
- The destructive meaning of `-keep-original=false` is documented in the README
  and both shipped skill copies.
- The tracked `temp/` payload is removed and `/temp/` is ignored.

## Milestone-one recommended sequence

1. Fix output planning, collision detection, destructive intent, and atomic
   write/delete behavior before expanding codecs.
2. Define typed inspect/plan/execute/verify requests and results in the engine.
3. Add a compile-time capability catalog with loss and metadata behavior,
   availability, and verification requirements.
4. Turn `pixelita` into the canonical task interface with internal deterministic
   plans and explained routing. Keep `img-*` as adapters.
5. Move filesystem discovery, naming, validation, and policy out of command
   packages and test canonical/specialist equivalence.
6. Expose build, schema, and capabilities as structured discovery. Reduce the
   skill to hints rather than required operating knowledge.
7. Build the GUI directly on the typed engine or stable API.

## Original automated agent-evaluation cases

Run each case with a weaker model and without the Pixelita skill. Score task
success, tool-call count, invalid-call count, recovery turns, unstructured prose
parsing, source preservation, and whether verification was actually performed.

1. Discover the canonical executable and verify its build against the checkout.
2. Inventory a mixed directory quickly, then request a measured optimization
   plan without being told codec names.
3. Optimize a flat PNG, a photograph, and a baseline JPEG while preserving a
   source tree and explaining each chosen strategy.
4. Generate responsive widths from originals, emit deployment guidance, and
   verify content survival after dimension changes.
5. Find and inspect the three worst regions of a lossy conversion in no more
   than three tool calls.
6. Recover from an unsupported explicit file, a corrupt image, invalid range,
   conflicting resize modes, a flag after a path, and an unwritable output.
7. Detect same-basename output collisions before any write.
8. Refuse overwrite or deletion without explicit authorization and prove every
   failed operation preserves the source.
9. Interpret a threshold skip as a safety decision rather than a crash.
10. Add a hypothetical codec through capability data and one strategy without
    introducing a new top-level workflow.
