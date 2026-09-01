---
id: post-preview-evaluator-adapter-phase-2
plan: post-preview-evaluator-adapter
phase: 2
status: current
updated: 2026-09-01
---

# Post-Preview Evaluator Adapter Phase 2

## Goal

Implement and prove preview-before-confirmation, exact retained evidence, atomic single-use consumption, and deterministic three-question delivery without starting Pi or contacting a model.

## Current Phase

Phase 2 is complete. The active plan points to Phase 3 with `phase_status: awaiting_approval`; Phase 3 is not authorized.

Repository snapshot:

- checkout: `/Users/bytedance/workspace/pi-learnloop`
- branch: `main`, tracking `origin/main`
- Phase 1 baseline: `dec0fd1` (`feat: define post-preview evaluator contracts`)
- Phase 2 commit: the commit containing this checkpoint

## Completed

- Added a daemon-owned in-memory continuation store with 32-byte random `pc1-` identifiers, five-minute RFC3339-nanosecond expiry, eight live entries, a 1 MiB aggregate excerpt limit, no live eviction, owned evidence copies, atomic single-use consume, expired-entry purge, and shutdown cleanup.
- Added an optional v1 preview `continuation` object. Empty evidence, capacity pressure, and unavailable evaluators use the accepted safe unavailable reasons without changing the preview payload.
- Added authenticated strict `POST /v1/question-sets` with a 4 KiB request limit, exact field names, Pi `0.84.3` model metadata validation, indistinguishable `409 continuation_unavailable` IDs, stable evaluator errors, a 120-second evaluation context, and a 130-second extension client deadline.
- Connected atomic consume directly to `evidence.BuildBundle` and `evaluator.NewInput`. The continuation request cannot submit a repository, revision, source, or client-built bundle.
- Added `evaluator.QuestionEvaluator`, shared `ValidateModelSelection`, and the deterministic Phase 2 adapter. It returns strict Q1/Q2 `code_specific` plus Q3 `go_backend` through the existing output validator and does not start a process or provider call.
- Updated daemon lifecycle wiring and write timeout for the accepted continuation deadline relationship.
- Extended the thin client with one non-retrying question-set request. Preview discovery still retains its existing one-race retry; continuation does not retry authentication, discovery, transport, evaluator, or invalid output.
- Extended `/learn` to render the preview first, validate active non-secret Pi version/provider/model/thinking metadata, ask explicit confirmation, send no request on decline, render the strict result, and stop without answers or follow-up.
- Kept older preview responses compatible: the extension accepts a response without `continuation` and preserves the preview-only outcome.
- Corrected an expiry precision defect found by the required unit-test workflow: the external `expires_at` now preserves the same nanosecond instant used by server-side expiration.
- Updated `README.md` and `PROJECT.md` with current Phase 2 behavior, data flow, privacy boundary, module responsibilities, and test coverage.

## Modified Files

Go implementation and tests:

- `internal/daemon/continuation.go`
- `internal/daemon/continuation_test.go`
- `internal/daemon/daemon.go`
- `internal/daemon/question_set_test.go`
- `internal/daemon/server.go`
- `internal/daemon/server_test.go`
- `internal/evaluator/evaluator.go`
- `internal/evaluator/evaluator_test.go`
- `internal/evaluator/pi_contract.go`

Extension implementation and tests:

- `extensions/lib/daemon-client.ts`
- `extensions/lib/learn-command.ts`
- `extensions/pi-learnloop.ts`
- `tests/extension/daemon-client.test.ts`
- `tests/extension/extension-entry.test.ts`
- `tests/extension/learn-command.test.ts`

Documentation and lifecycle:

- `README.md`
- `PROJECT.md`
- `plans/post-preview-evaluator-adapter.md`
- `docs/checkpoints/post-preview-evaluator-adapter-phase-1.md`
- `docs/checkpoints/post-preview-evaluator-adapter-phase-2.md`

No dependency, generated-code, prompt, capability-policy, persistence, CI/CD, or release file changed.

## Important Decisions

- The exact `evidence.Result` is copied and retained at preview completion; confirmation never rereads Git or the working tree.
- Consume removes the entry under one mutex before bundle construction or evaluation. Build, validation, or evaluator failure never restores the grant.
- Malformed, unknown, expired, used, concurrent, wrong-instance, and post-restart IDs are intentionally indistinguishable.
- A capacity-limited or empty preview remains useful and reports an unavailable continuation rather than evicting another live grant.
- The current adapter is deterministic and local. Confirmation accurately says Phase 2 does not contact a model; Phase 3 must replace this copy when real data sharing and provider cost become possible.
- The extension sends Pi's exported version and active non-secret model identity only. Pi credentials never enter daemon JSON or the deterministic input.
- Phase 3 owns executable discovery, version preflight, RPC framing, the real released prompt, process caps/lifecycle, and model-backed output validation.

## Tests / Verification

Passed:

- focused deterministic evaluator tests;
- focused continuation store, exact retained working-tree, single/concurrent consume, strict request, authentication, empty evidence, restart loss, and BuildBundle-failure tests;
- `CGO_ENABLED=0 go test -count=1 ./...`;
- `go test -count=1 -race -tags netgo ./...`;
- `go vet ./...`;
- `npm run typecheck`;
- `npm test`: 18/18 passed;
- required `bits-unit-test-gen` workflow and `utree flush`;
- `scripts/test-agent-infra.sh` and `scripts/validate-agent-infra.sh` after checkpoint metadata was finalized;
- `git diff --check` and complete Git diff/dependency review.

The required unit-test workflow initially confirmed one P2 expiry precision defect with a stable failing test. Production formatting changed to `time.RFC3339Nano`; the same test and the full suites then passed.

Recorded host behavior:

- Default Go 1.21.13 network-enabled binaries still hit the documented macOS 26 `missing LC_UUID` linker issue. The established pure-Go and `netgo` race commands pass and are the authoritative Phase 2 results.
- No Pi executable, RPC process, provider, API credential, model, or live network evaluator was used.

## Known Issues

- The current questions are deterministic contract fixtures, not model-backed assessments.
- There is no Pi executable lookup, version preflight, JSONL RPC process, released-prompt invocation, output stream cap, termination, or process reaping.
- Decline sends no continuation request; because Phase 2 defines no deletion route, the unused in-memory grant expires within five minutes or is cleared on daemon shutdown.
- There is no answer collection, follow-up, scoring, persistence, durable job, SSE, or recovery of an evaluation.
- The host Go 1.21.13 `LC_UUID` limitation remains unchanged.

## Remaining Work

Phase 3, only after explicit authorization:

- replace the deterministic adapter with the accepted isolated Pi `0.84.3` RPC adapter;
- resolve and freeze `pi` from daemon startup `PATH` and run the exact version preflight;
- enforce deny flags, Pi-managed auth/model, correlated RPC setup, empty command list, one prompt, `agent_settled`, output caps, timeout, cancellation, termination, and reaping;
- validate one final assistant question set and preserve the existing continuation/no-retry/error boundary;
- test only with a fake Pi executable in automation and provide a separate opt-in live smoke procedure.

## Next Step

Wait for explicit user authorization of `post-preview-evaluator-adapter` Phase 3. Before implementation, restore context from `AGENTS.md`, `PROJECT.md`, ADR-0003, the active plan, this checkpoint, Git status, and the containing Phase 2 commit.

## Do Not Change

- Do not start Phase 3 without explicit authorization.
- Do not add a second evaluator path; replace the deterministic adapter at the single `QuestionEvaluator` seam.
- Do not retry continuation or evaluator execution.
- Do not reread the repository after preview or accept client-built evidence.
- Do not pass credentials through HTTP, argv, prompts, logs, persistence, or model-visible provenance.
- Do not persist raw source, prompts containing source, model output, continuation state, or Session transcripts.
- Do not implement answers, follow-up, scoring, SQLite, durable jobs, SSE, background work, dependencies, or unrelated cleanup.
