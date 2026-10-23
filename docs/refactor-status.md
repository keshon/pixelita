# Refactor status — 2026-10-23

## Implemented

- Reproducible baseline fixtures and a 14-call CLI acceptance harness.
- Shared finite threshold validation, explicit omitted-versus-zero semantics,
  decoded PNG measurements, SSIM for tiny regions, and alpha-aware PSNR gating.
- Verified JPEG pixel identity in preview and apply, PNG metadata retention,
  and explicit rejection of unsupported rendering metadata.
- Publication without a missing-destination backup window; no-clobber writes,
  symlink/hardlink preflight, input-overlap checks, and source-change checks.
- Shared engine entry points for all seven specialist commands.
- Separate batch infrastructure, typed results, and presentation packages.
- Explicit measured format selection, direct resize-to-WebP, exact-pixel and
  metadata constraints with conservative skips for unsupported guarantees.
- Canonical directory/region comparisons, all matched resize variants, multiple
  crop previews, pixel probes, and structured failures.
- Additive schema-2 accounting for unique inputs, predicted outputs, and actual
  writes; typed verification and candidate decision fields.
- Bounded default concurrency, decoded/preview/resize allocation limits, and
  cancellation of queued optimization and specialist writer/scan work.
- Windows/Linux no-cgo CI and downloadable build artifacts; revised README and
  synchronized bundled skill references. Historical benchmarks are archived.

## Verification performed locally

Windows tests, vet, and the single-command build have passed with CGO disabled.
The eight-command Linux/amd64 cross-build also passed.
The generated-corpus CLI harness passed 14 checks. JPEG verification includes
`compare --min-psnr 200`. A five-second ReadHeader fuzz run completed 179,102
executions without failure. Both bundled skill copies passed quick_validate.py.
See eval/README.md to reproduce the corpus run; the unit tests cover source
preservation, stale plans, alias destinations, cancellation, metadata, alpha,
and tiny-image metrics.

## Remaining acceptance work

The engineering changes do not complete the product-validation phase of the
agreed plan. Multiple licensed real-world asset collections, comparison against
reference tools at matched constraints, memory/throughput profiling, and repeated
agent trials with and without the skill have not been run. No external service
or model runner was configured for those trials. No market or usability advantage
is claimed from synthetic acceptance results.

Hosted CI has not been dispatched from this local checkout. Local Windows
execution and Linux cross-compilation do not establish Linux runtime behavior.

## Deliberate limitations

- Exact-pixel/metadata-guaranteed automatic optimization currently selects only
  baseline JPEG. PNG's compatible metadata retention is narrower than a blanket
  preservation guarantee.
- Resize has no same-size fidelity policy; comparisons disclose resampling.
- Auto-selection re-encodes its winner on apply and rechecks policy. Candidate
  caching and codec interruption remain unimplemented; measure their cost before
  adding complexity.
- Cancellation is cooperative between files, not inside codecs. Compare/view do
  not currently expose a cancellation context.
- The API and plan remain internal. No GUI, public Go API, AVIF, or JPEG XL.
