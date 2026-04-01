# Contextstore Shield Design

## Goal

Add a runtime-local shield equivalent for contextstore writes so history and checkpoint persistence can finish even if the outer run context is canceled.

## Scope

This design only protects contextstore operations triggered from `internal/runtime/runtime.go`.

Included:
- `AppendCheckpointWithMetadata`
- `Append`
- `AppendUsage`
- `RevertToCheckpoint`
- any `Rewrite*` call that runtime directly invokes in this flow

Excluded:
- LLM reply / streaming
- tool execution
- runtime event emission
- retry sleep
- general-purpose shielding inside `contextstore`

## Recommended Approach

Implement a small runtime-private shield layer in `internal/runtime/runtime.go`.

Because the current `contextstore.Context` API is synchronous and does not accept a `context.Context`,
the runtime will wrap each write in a closure-based helper that:
1. runs the concrete contextstore write to completion
2. checks the original run context after the write finishes
3. returns the original cancellation error after the write if cancellation happened

This keeps shielding tightly scoped to the runtime write boundary and avoids expanding the `contextstore` API.

## Why This Approach

### Option A — runtime-local shield wrapper (recommended)
- Smallest change
- Matches the Python reference intent of shielding context growth
- Keeps cancellation protection limited to persistence
- Avoids polluting storage APIs with runtime-only semantics

### Option B — shielded contextstore APIs
- Makes the behavior explicit at call sites
- But expands `contextstore.Context` with runtime-specific variants
- Increases API surface without adding real capability

### Option C — batch writes then commit atomically
- Could improve transaction semantics
- But is a larger architectural change and out of scope for this task

## Architecture

Add a tiny private helper layer in `internal/runtime/runtime.go`, for example:
- a helper that executes one grouped contextstore write closure
- a small test seam on `Runner` so cancellation can be injected after the write completes

The runtime remains responsible for deciding which writes must be shielded. `contextstore` stays unchanged.

Recommended implementation shape:
- execute the concrete write closure synchronously
- after the write, re-check the original context and surface cancellation before doing more work

A small private seam on `Runner` is acceptable for tests, such as a shield function wrapper.

## Data Flow Changes

### Run start
Shield these writes in `Run()`:
- initial checkpoint creation
- D-Mail checkpoint marker append
- initial user prompt append

### Per-step checkpointing
Shield these writes before step execution when D-Mail is enabled:
- per-step checkpoint creation
- per-step checkpoint marker append

### Step persistence
Shield these writes in `runStep()`:
- usage record append
- assistant text append for non-tool final replies

### Tool-step persistence
Shield these writes in `advanceRun()`:
- tool step records append
- tool failure records append

### D-Mail rollback path
Shield these writes in the rollback branch:
- `RevertToCheckpoint`
- post-rollback checkpoint creation
- post-rollback checkpoint marker append
- D-Mail user message append

## Error Handling

Shielding changes when cancellation is observed, not how storage errors behave.

Rules:
1. If the shielded contextstore write fails, return that write error.
2. If the write succeeds but the original run context was canceled, return the original cancellation error.
3. Otherwise continue normally.

This preserves the real failure cause for storage problems and prevents cancellation from masking disk or rename errors.

## Testing Strategy

Add runtime-focused tests that verify behavior rather than helper usage.

### Required cases
- If cancellation happens before a contextstore write begins, that write still completes, then the run stops.
- If cancellation happens during the D-Mail rollback path, rollback persistence completes, then the run stops.
- Non-contextstore operations remain unshielded and still respond directly to cancellation.
- A contextstore write error is returned as the real error rather than being replaced by `context.Canceled`.

### Test seams
A small private seam on `Runner` is acceptable to make this deterministic, such as:
- shield function wrapper

### Assertions
Prefer asserting on:
- resulting history/checkpoint file state
- returned runtime status or error
- no extra step progression after the shielded write completes

Do not center tests on whether a specific helper function was called.

## Non-Goals

This design does not attempt to:
- make multi-write sequences transactional
- shield event delivery or tool execution
- redesign `contextstore`
- change retry behavior

## Expected Outcome

After this change, runtime persistence will be resilient to cancellation at the exact point a history or checkpoint write is being committed, while the runtime still stops promptly once that write has completed.
