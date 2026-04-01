# 2026-04-01 Go Implementation Refactor Plan

## Goals

- Fix behavior bugs found during Go implementation review before making broader structural changes.
- Reduce hidden coupling between `app`, `shell`, `runtime`, and tool wiring.
- Keep diffs local and explicit, following the repo's Go style guidance.

## Confirmed Problems

1. `send_dmail` handler is attached after the executor copies tool handlers, so the tool silently degrades to a no-op.
2. Print mode writes the user prompt into history before `runtime.Run`, which causes the same prompt to be persisted twice.
3. Shell-triggered runtime execution starts from `context.Background()`, so shell cancellation does not reliably stop the active run.
4. Shell exit text suggests `fimi -resume <id>`, but the CLI only supports `--continue` / `-C`.
5. MCP subprocesses do not inherit the parent process environment, so `PATH`, `HOME`, and similar variables can disappear.

## Refactor Scope

### Phase 1: Behavior fixes

- Move D-Mail wiring earlier so the executor sees the real handler.
- Make print-mode store creation side-effect free; let `runtime.Run` own prompt persistence.
- Pass shell run context into model-triggered executions.
- Fix the shell resume hint to match the actual CLI.
- Preserve parent environment when spawning MCP servers.

### Phase 2: Local simplification

- Remove dead shell state that is no longer read.
- Stop reading session previews during render; prepare preview data during session listing.
- Reduce duplicated JSONL append code in `contextstore`.
- Reuse existing output-shaping helper paths in tool handlers instead of repeating the same message-append logic.

### Phase 3: Verification

- Add focused regression tests for:
  - D-Mail handler wiring
  - print-mode store initialization
  - shell runtime context propagation
  - MCP environment merging
  - session preview loading
- Run the relevant `go test` subsets and then `go test ./...`.

## Notes

- Avoid broad package reshuffles in this pass.
- Keep each helper narrow and close to the code that uses it.
- Prefer deleting unused state over introducing another abstraction layer.
