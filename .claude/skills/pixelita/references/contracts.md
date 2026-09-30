# Agent and JSON contracts

Use `pixelita capabilities --json` for machine-readable routing facts instead
of teaching an agent a hard-coded format matrix.

Every command supports `-json` and emits schema 2 with a shared envelope:
`tool`, `schema`, `dryRun`, `items`, `summary`, `notes`, and optional `totals` or
`nextAction`. Each item contains `path`, `output`, `status`, byte counts, and
operation-specific `metrics`. Paths use forward slashes.

Statuses are `done`, `would`, `skipped`, or `failed`. Exit codes are:

- `0`: completed, including a deliberate threshold skip.
- `1`: processing failure.
- `2`: invalid arguments or unsupported explicit input.

Prefer semantic fields whose names carry their caveats. For example,
`coloursAtLeast` means counting stopped at a lower bound. Relevant PNG metadata
is reported through named `chunks`; a byte count alone does not establish that
an ICC profile, gamma declaration, or orientation metadata survived.

Directories may contain unrelated files and legitimately produce an empty
result. An explicitly supplied unsupported file is an argument error.

Do not reimplement Pixelita's crops, previews, resize filters, PSNR, SSIM, tonal
levels, or pixel probing in Python or ImageMagick. Use the typed commands so
humans, local agents, cloud agents, and a future GUI share one behavior.
