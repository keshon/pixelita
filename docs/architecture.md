# Pixelita product architecture

## Status

This document separates the current implementation from the target architecture
and defines compatibility rules for the migration.

The current implementation has a useful shared operation layer in
`internal/ops`, seven `img-*` commands, a `pixelita` dispatcher that forwards
to them, and JSON schema 2. Flag parsing, routing, validation, and some safety
policy still live in individual commands. The capability registry, task
planner, stable in-process API, and GUI integration remain unimplemented. The
canonical task interface is the main remaining product milestone.

## Product model

Pixelita is one image system with three clients:

- people using a terminal;
- agents that require low-ambiguity discovery and structured results;
- a future GUI.

The target architecture gives all three clients one typed engine for operations,
policy, capability metadata, routing, and verification. Interfaces may adapt
presentation without reimplementing decisions.

The durable workflow is:

1. **Inspect and understand.** Inventory inputs, read dimensions and metadata,
   identify constraints, and measure realistic opportunities.
2. **Choose a goal and plan.** Express an intent such as reduce transfer size,
   create display variants, preserve pixels exactly, or investigate damage.
   Evaluate compatible strategies and explain the choice.
3. **Execute safely.** Encode candidates, enforce gain and fidelity policy,
   preflight every output, and write only approved results.
4. **Verify.** Report quantitative fidelity, inspect relevant regions, and
   make the relationship between source, result, and decision explicit.

Formats are capabilities inside this workflow. Task intents remain stable when
PNG, JPEG, WebP, or future formats are added.

## Layers

### 1. Typed engine

The target engine owns typed requests and results for inspection, transformation,
encoding, comparison, presentation, planning, and verification. It also owns:

- safety policy: gain and fidelity floors, overwrite/delete rules, atomic
  output, collision detection, metadata promises, and deterministic ordering;
- one structured result envelope with stable status and error semantics;
- the capability registry and strategy selection;
- candidate measurement and post-operation verification.

Commands and the GUI must use engine routing and results. The existing
`internal/ops` package is the starting point. Filesystem discovery, option
validation, planning, and policy still need typed engine APIs.

### 2. Canonical task interface (target)

The current `pixelita` binary only dispatches to specialist commands. The target
interface becomes the primary entry point for agents and general use. It accepts
task intents and supports:

- inspect inputs and discover capabilities;
- plan toward an explicit goal without writing;
- execute a supplied or automatically selected plan;
- verify sources against results;
- present images or regions for visual inspection.

Routing decisions appear in brief human output and in structured fields:

- the normalized goal and input facts;
- considered strategies and why each was accepted or rejected;
- the selected strategy and capability identifier;
- predicted output, gain, fidelity, metadata, and dimensional effects;
- active thresholds and destructive permissions;
- fallback behavior when a candidate fails policy;
- verification performed and its outcome.

A plan is serializable and may be executed explicitly. This lets agents review
the decision without reconstructing it from prose and lets the GUI show the
same plan before applying it.

### 3. Specialist commands

The existing `img-scan`, `img-quant`, `img-webp`, `img-jpeg`, `img-resize`,
`img-diff`, and `img-look` commands may remain as concise Unix-style tools and
compatibility aliases. They translate flags into typed requests, call the
engine, and render its result. They do not own an encoder policy, output naming
rule, validation rule, or JSON variant.

The canonical interface lists the specialists for direct use. New codecs
normally add registry entries and strategy implementations without adding
top-level commands.

### 4. GUI and stable API

The GUI calls the typed engine in process or through a stable structured API.
It never spawns a CLI and parses human output. It may render previews, plans,
and results differently, but uses the same schema and policy decisions.

The CLI, agent protocol, and GUI therefore use the same behavior.

## Capability registry

Each capability has a stable identifier and machine-readable metadata:

- supported intents, input formats, and output formats;
- loss model: pixel-identical, lossless pixels, near-lossless, or lossy;
- alpha, animation, orientation, colour-profile, and metadata behavior;
- tunable parameters with types, ranges, defaults, and compatibility rules;
- availability and reason when unavailable;
- expected cost characteristics and whether a dry run requires full encoding;
- safety constraints, verification methods, and fallback capabilities.

The canonical interface exposes the registry as JSON and readable help. It
also exposes the build revision, engine version, and supported schema versions.
Clients discover support; they do not infer it from executable names.

Schema versions change only for incompatible meaning or shape. Additive fields
remain backward compatible. Every result declares its schema version and every
capability/parameter has stable semantics across interfaces.

## Safety contract

- Explicit files with unsupported types are argument errors. Directory scans
  may legitimately find no matching files.
- Invalid or conflicting options fail before decoding or writing. Values are
  never silently clamped to a different request.
- Every output path is planned before parallel work begins. Collisions fail the
  plan; two inputs never race to one destination.
- Source overwrite or deletion requires explicit destructive intent in the
  request and appears in both plan and result. Avoid negative retention flags
  for deletion.
- Writes are atomic where the platform permits. A failed write does not leave a
  partial result or remove a source.
- Threshold rejection is a successful safety decision (`skipped`), distinct
  from execution failure. Results state which threshold rejected the candidate.
- Human prose is explanatory. Agents do not need to parse it for a value,
  recovery step, output path, or routing decision.

## Migration without divergence

1. Keep schema 2 and all `img-*` commands compatible while moving validation,
   naming, filesystem planning, and policy into typed engine functions.
2. Add the capability registry and structured plan type. Make scan and direct
   converters consume the same candidate measurements and policy evaluator.
3. Implement the canonical task interface in `pixelita`. Continue forwarding
   legacy invocations during the transition.
4. Make every specialist command a thin adapter over the same requests and
   result renderer. Add equivalence tests between canonical and specialist
   paths.
5. Build the GUI only on the typed engine or stable API. Never copy command
   defaults or parse terminal output into GUI state.

The pure-Go, no-cgo, single `go build -o bin/ ./cmd/...` constraint remains.
Compatibility wrappers can still be separate binaries; package count does not
define the product model.

## Evaluation gates

The target interface must let a weaker model complete these cases without the
Pixelita skill:

1. Find the canonical command and prove which build it is using.
2. Inspect a mixed directory quickly, then request a measured plan.
3. Ask to reduce web transfer size and receive an explained choice between
   resize, palette quantization, lossless JPEG optimization, and lossy encoding.
4. Execute a dry run, apply the plan, and verify whole-image and worst-region
   fidelity without inventing a metric implementation.
5. Recover from an unsupported explicit file, invalid range, conflicting sizing
   modes, output collision, unwritable destination, and partial directory
   failure using structured recovery information.
6. Refuse a below-threshold candidate without treating the safe skip as a tool
   crash.
7. Require explicit authorization before overwrite or deletion and prove the
   source survives every failed operation.
8. Add a hypothetical codec through registry data and a strategy implementation
   without adding a new top-level workflow.

Track command count, tool-call count, invalid-call recovery count, prose parsing,
and task success. A feature that adds flags but does not reduce calls or recovery
work has not improved agent usability.
