# Pixelita architecture

## Current dependency direction

Command adapters parse flags and render results. `internal/engine` owns typed
requests, batch orchestration, planning, candidate selection, and discovery.
It calls `internal/ops`, which owns encoding, shared verdicts, verification, and
safe publication. Codec and metric packages retain their existing boundaries.

`internal/batch` owns path collection and bounded scheduling independently of
CLI parsing. `internal/result` owns typed outcomes and accounting;
`internal/report` renders them as JSON or tables. Neither the engine nor image
operations import the CLI or rendering packages.

Specialists use engine entry points for quantization, JPEG, WebP, resize, scan,
comparison, and previews. Parser-specific diagnostics may remain in commands;
non-CLI callers are also validated by the shared entry points. The API remains
internal and is not a versioned public contract.

## Planning and selection

`optimize` preserves format/dimensions by default. `format=auto` explicitly allows
same-format and WebP candidates; actual encoded sizes and fidelity verdicts
select the winner. Candidate decisions are returned with the selected result.
There is no plugin registry or general-purpose optimization language.

Plans hold source hashes, output destinations, strategy identifiers, and the
request. All outputs are preflighted before parallel work, including outputs
that could alias another input. Execution checks source hashes. Preview and
apply are separate measurements; there is no public serialized plan.

Format conversion and resize can be combined explicitly. Exact-pixel and
metadata-preservation constraints currently restrict automatic optimization to
baseline JPEG. Unsupported guarantees are reported as skips, never assumed.

## Safety boundaries

Candidate policy rejects non-finite thresholds and missing requested evidence.
PNG quantization and WebP measure decoded candidates; PSNR includes an
alpha-aware gate. JPEG publication checks decoded pixel identity.

Compatible PNG ancillary chunks are retained. Palette-dependent chunks,
unsupported critical chunks, and animation prevent palette conversion. ICC
conversion and animated PNG transformation are unsupported for current WebP
and resize paths. Inspect reports named PNG and JPEG metadata.

Publication uses a same-directory verified temporary. Without overwrite,
a hard link publishes without replacing a concurrently created destination.
With overwrite, rename replaces the destination directly; the previous file
is never moved away into a temporary backup first. Unsupported filesystem
operations fail rather than falling back to destructive copying.

This is per-output safety, not a batch transaction or protection against an
adversarial process changing inputs between checks. Already completed outputs
remain when later files fail or queued work is cancelled.

## Results and compatibility

Schema 2 retains legacy summary meanings. Additive totals distinguish unique
inputs, observed inventory bytes, outcomes, predicted outputs, and actual
writes. A successful comparison is not counted as a file write.

Verification and candidate decisions are typed fields. Detailed operation
measurements remain in metrics. Human notes are explanatory. Paths in JSON use
forward slashes; skipped and failed items have stable codes.

Directory comparison pairs each derivative, preferring relative-directory
and same-format matches; ambiguous matches fail. Region comparison applies the
same fidelity policy and reports dimension resampling.

## Deliberate limits

There is no GUI, stable public API, new codec, or speculative encoder rewrite.
The encoder algorithms remain unchanged. Runtime defaults cap concurrent files
at two and decoded/generated preview size at 64 million pixels. Optimization
and specialist writing/scan requests accept cancellation between files;
interrupting an individual codec mid-operation is not implemented.

Cross-platform CI is configured; a local Windows run is not proof that the
hosted Linux job passed. Real-corpus and agent usability trials remain separate
acceptance evidence; see refactor-status.md.
