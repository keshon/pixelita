# Pixelita product architecture

## Status

This document separates the current implementation from the target architecture
and defines compatibility rules for the migration.

The current implementation has a shared operation layer in `internal/ops`, an
internal typed planner and static capability catalog in `internal/engine`, five
canonical `pixelita` tasks, seven specialist `img-*` commands, and JSON schema
2. Specialist validation and presentation remain command-local. A stable public
Go API and GUI integration remain unimplemented.

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

The internal engine owns the optimization request, deterministic plan, routing,
and capability catalog. Shared operations own candidate encoding, measurement,
verification, and atomic output. Together they provide:

- safety policy: gain and fidelity floors, overwrite/delete rules, atomic
  output, collision detection, metadata promises, and deterministic ordering;
- one structured result envelope with stable status and error semantics;
- the compile-time capability catalog and strategy selection;
- candidate measurement and post-operation verification.

The plan is deliberately internal and is not a stable serialized contract. A
future GUI must call the engine or a stable API built over it; it must not parse
human CLI output.

### 2. Canonical task interface

The `pixelita` binary is the primary entry point for agents and general use. It
supports:

- inspect inputs and discover capabilities;
- preview an internally planned optimization without writing;
- apply the same routes with explicit permission;
- verify sources against results;
- present images or regions for visual inspection.

Routing decisions appear in brief human output and in structured item fields:

- the normalized goal and input facts;
- the selected strategy and capability identifier;
- output, measured gain and fidelity;
- verification method and automatic-selection status;
- stable skip/failure codes and structured recovery where one exists.

Preview and apply rebuild the same deterministic internal plan. No plan file is
required or exposed as a public format.

### 3. Specialist commands

The existing `img-scan`, `img-quant`, `img-webp`, `img-jpeg`, `img-resize`,
`img-diff`, and `img-look` commands may remain as concise Unix-style tools and
compatibility aliases. They translate flags into typed requests, call the
engine, and render its result. They do not own an encoder policy, output naming
rule, validation rule, or JSON variant.

The canonical interface lists the specialists for direct use. New codecs add a
catalog entry and a strategy implementation without adding a top-level command.

### 4. Future GUI and stable API

A future GUI will be built from scratch after the engine contracts stabilize.
It will call the typed engine in process or through a stable structured API. It
will not spawn a CLI, parse human output, or inherit an earlier UI branch.

The CLI, agent protocol, and GUI therefore use the same behavior.

## Capability catalog

Each capability has a stable identifier and machine-readable semantic facts:

- supported intents, input formats, and output formats;
- loss model: pixel-identical, lossless pixels, near-lossless, or lossy;
- alpha, animation, orientation, colour-profile, and metadata behavior;
- dimension, alpha, pixel, and metadata promises;
- verification method, destructive effects, and automatic-selection eligibility.

`pixelita capabilities --json` and the planner read the same compile-time
catalog. The response also identifies the build and schema. Clients discover
support; they do not infer it from executable names.

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

## Current boundary

- Canonical optimization and specialist writers share destination preflight,
  atomic writes, and candidate verdict policy.
- Legacy `pixelita scan`, `quant`, `webp`, `jpeg`, `resize`, `diff`, and `look`
  aliases remain available.
- Specialist commands keep their expert flags and presentation; moving every
  parser into the engine is not required for the task interface.
- A future GUI must use the typed engine or a stable API. GUI work and a public
  Go API are outside this milestone.

The pure-Go, no-cgo, single `go build -o bin/ ./cmd/...` constraint remains.
Compatibility wrappers can still be separate binaries; package count does not
define the product model.

## Evaluation gates

The canonical interface is evaluated on whether a weaker model completes these cases without the
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
8. Add a hypothetical codec through catalog data and a strategy implementation
   without adding a new top-level workflow.

Track command count, tool-call count, invalid-call recovery count, prose parsing,
and task success. A feature that adds flags but does not reduce calls or recovery
work has not improved agent usability.
