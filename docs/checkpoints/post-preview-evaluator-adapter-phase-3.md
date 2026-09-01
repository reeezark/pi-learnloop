---
id: post-preview-evaluator-adapter-phase-3
plan: post-preview-evaluator-adapter
phase: 3
status: current
updated: 2026-09-01
---

# Post-Preview Evaluator Adapter Phase 3

## Goal

Replace the production deterministic seam with the bounded, isolated Pi 0.84.3 RPC adapter accepted in ADR-0003 while retaining deterministic, no-provider automated verification.

## Current Phase

Phase 3 is complete. The `post-preview-evaluator-adapter` plan is complete with `phase_status: complete`; there is no authorized next phase in this plan.

Repository snapshot:

- checkout: `/Users/bytedance/workspace/pi-learnloop`
- branch: `main`, tracking `origin/main`
- Phase 2 baseline: `eac2633` (`feat: add deterministic post-preview questions`)
- Phase 3 commit: the commit containing this checkpoint

## Completed

- Embedded the exact immutable `evaluator-question-generation@1.0.0` released prompt into the Go binary without adding a repository-relative runtime file lookup or duplicating the prompt.
- Added `PiRPCEvaluator`, which resolves `pi` from daemon startup `PATH`, converts it to an absolute symlink-resolved regular executable, freezes that path, and requires exact `0.84.3` stdout from a two-second `--version` preflight.
- Moved preflight before listener and discovery publication. Missing, non-executable, mismatched, or timed-out Pi leaves preview available while continuation reports `evaluator_unavailable`.
- Replaced only the daemon's production `QuestionEvaluator` wiring. `DeterministicEvaluator` remains a no-model test fixture behind the same interface; there is no second production path.
- Added direct no-shell process invocation with the accepted fixed deny arguments, released system prompt, and validated non-secret provider/model/thinking values. Pi credentials remain Pi-managed and never enter daemon HTTP, argv, prompts, logs, persistence, or model-visible provenance.
- Added correlated `set_auto_retry(false)`, `set_auto_compaction(false)`, and empty `get_commands` checks before exactly one prompt. The adapter sends LF-only JSONL, waits for the prompt response and `agent_settled`, obtains one final assistant text, and applies the existing strict question-set validator.
- Added fail-closed handling for invalid/duplicate RPC JSON, mismatched responses, discovered commands, tool calls/executions, unknown or retry/compaction/extension events, malformed assistant output, child failure, missing model/auth behavior, deadline, and cancellation.
- Enforced fixed 120-second evaluator, 2-MiB stdout, 64-KiB stderr, and 64-KiB final-text bounds. The stdout cap is applied at the reader before an unterminated record can allocate beyond the accepted limit. Every exit path closes stdin, terminates when needed, waits for the child, and exposes only safe errors.
- Updated `/learn` confirmation copy to disclose that the selected excerpts will reach the configured model, may incur provider cost, and can be affected by external Pi/provider retry settings.
- Added an explicit opt-in live smoke procedure to `README.md`. No live smoke was run during this phase.
- Updated `README.md` and `PROJECT.md` with the completed production flow, prerequisites, privacy boundary, module responsibilities, test strategy, and remaining non-goals.

## Modified Files

Production implementation:

- `agent/prompts/assets.go`
- `internal/evaluator/pi_rpc.go`
- `internal/daemon/daemon.go`
- `extensions/lib/learn-command.ts`

Automated verification:

- `internal/evaluator/pi_rpc_test.go`
- `internal/daemon/daemon_test.go`
- `tests/extension/learn-command.test.ts`

Stable and lifecycle documentation:

- `README.md`
- `PROJECT.md`
- `plans/post-preview-evaluator-adapter.md`
- `docs/checkpoints/post-preview-evaluator-adapter-phase-2.md`
- `docs/checkpoints/post-preview-evaluator-adapter-phase-3.md`

No dependency, generated-code, released-prompt content, capability-policy, daemon protocol, persistence, CI/CD, or release file changed.

## Important Decisions

- Runtime uses the exact embedded released prompt asset. The released Markdown file remains immutable and is not copied into another production asset.
- Daemon discovery is not published until the bounded Pi preflight finishes, so a client cannot observe a descriptor for an HTTP server that is still blocked on evaluator setup.
- Production has one evaluator path. A failed startup preflight produces a nil evaluator and an unavailable continuation rather than falling back to deterministic questions.
- The adapter accepts only documented safe Pi 0.84.3 stream events. Tool, retry, compaction, queued, extension, error, unknown, and malformed shapes fail closed.
- Pi LearnLoop performs no evaluator retry or repair call. Pi's external `retry.provider.maxRetries` cannot be enforced through RPC and must remain `0` in the supported configuration.
- Automated tests re-exec the test binary through a temporary fake `pi` wrapper and never use the workstation's real Pi credentials or provider transport.

## Tests / Verification

Passed:

- focused `NewPiRPCEvaluator`, `PiRPCEvaluator.Evaluate`, JSONL cap, and daemon preflight-order tests;
- complete `CGO_ENABLED=0 go test -count=1 ./internal/evaluator`;
- complete `CGO_ENABLED=0 go test -count=1 ./internal/daemon`;
- `CGO_ENABLED=0 go test -count=1 ./...`;
- `go test -count=1 -race -tags netgo ./...`;
- `go vet ./...`;
- `npm run typecheck`;
- `npm test`: 18/18 passed when local loopback binding was permitted;
- required `bits-unit-test-gen` Step 1-7 workflow and `utree flush`;
- `scripts/test-agent-infra.sh`;
- `scripts/validate-agent-infra.sh`;
- `git diff --check` and complete Git status/stat/diff review.

The unit-test workflow initially produced three stable failures: discovery published before slow preflight, unknown assistant update types were accepted, and an unterminated stdout record read 4 MiB before enforcing the 2-MiB cap. The production fixes preserved the same assertions, and all three regression tests now pass. Coverage enforcement was skipped because the project, user request, and execution source define no coverage gate.

The first sandboxed `npm test` attempt could not bind `127.0.0.1` and reported seven `EPERM` infrastructure failures. The unchanged test command passed 18/18 when rerun with local loopback permission. No live Pi RPC process, provider, credential, paid request, or model network call was used.

## Known Issues

- Pi 0.84.3 must be named `pi` and visible on daemon startup `PATH`; other Pi versions and installations without that executable name are unsupported.
- Provider authentication and availability remain Pi-managed. A missing credential or provider failure returns a safe evaluator failure and consumes the single-use continuation; the user must review a new preview before trying again.
- Pi LearnLoop cannot enforce Pi/provider transport retries through RPC. The supported configuration requires `retry.provider.maxRetries: 0`, and confirmation discloses the external retry boundary.
- There is no answer collection, targeted follow-up, scoring, weakness label, persistence, SQLite, durable job, SSE, automatic reminder, or recovery of an in-flight evaluation.
- The recorded macOS Go 1.21.13 `LC_UUID` limitation remains unchanged; the established pure-Go and `netgo` verification paths pass.

## Remaining Work

No work remains in `post-preview-evaluator-adapter`.

Any answer collection, follow-up, scoring, persistence, or learning-history feature must begin with repository/code investigation, a new plan, compatibility and security review, and the authorization gates in `AGENTS.md`.

## Next Step

Stop after committing and pushing this completed Phase 3. For later work, restore context from `AGENTS.md`, `PROJECT.md`, ADR-0003, this checkpoint, Git status, and the Phase 3 commit, then create a new investigated plan.

## Do Not Change

- Do not add a deterministic production fallback when Pi is unavailable.
- Do not broaden supported Pi versions without adapter contract tests and compatibility review.
- Do not accept an executable path, credentials, source, or client-built bundle through HTTP.
- Do not retry a continuation, model call, failed process, or invalid result automatically.
- Do not weaken the fixed deny flags, pre-prompt checks, stream/output bounds, strict validator, or process-reaping guarantee.
- Do not persist source-bearing continuation state, prompts, RPC streams, model output, credentials, or Session transcripts.
- Do not implement answers, follow-up, scoring, SQLite, durable jobs, SSE, reminders, dependencies, or unrelated cleanup without a new authorized plan.
