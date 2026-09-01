---
id: post-preview-evaluator-adapter-phase-1
plan: post-preview-evaluator-adapter
phase: 1
status: superseded
updated: 2026-09-01
---

# Post-Preview Evaluator Adapter Phase 1

## Goal

Freeze and prove the runtime evaluator boundary before adding continuation state, product behavior, subprocess execution, or a model call.

## Current Phase

Phase 1 is complete. The plan now points to Phase 2 with `phase_status: awaiting_approval`. Phase 2 is not authorized.

Repository snapshot:

- checkout: `/Users/bytedance/workspace/pi-learnloop`
- branch: `main`, tracking `origin/main`
- baseline commit: `32460aa` (`docs: plan post-preview evaluator`)
- working tree: Phase 1 remains uncommitted

## Completed

- Accepted `docs/decisions/ADR-0003-post-preview-evaluator-boundary.md` after resolving its protocol, retention, Pi version, model mapping, output, timeout, retry, and compatibility decisions.
- Added `internal/evaluator.NewInput` and the independently versioned `evaluator-input@1` JSON mapping.
- Added fail-closed validation for bundle identity, manifest/content hashes, revisions, budgets, counts, paths, enums, ranges, references, byte totals, copy ownership, and truncation.
- Kept `evidence.Bundle` as an internal domain type; no JSON tags or behavior were added to `internal/evidence`.
- Added strict `evaluator-question-set@1` parsing with a 64 KiB cap, UTF-8 checks, duplicate-key and unknown-field rejection, one-object framing, fixed Q1/Q2/Q3 IDs and kinds, 1,000-byte question limits, explicit reference arrays, bundle-reference validation, and an exact empty `insufficient_evidence` result.
- Ensured output-validation errors do not echo untrusted unknown field names.
- Added `BuildPiArguments` for exact Pi `0.84.3` model mapping and fixed `--mode rpc`, no-session, no-tools, resource-discovery deny, and no-approve flags.
- The Pi argument contract validates provider, model, thinking level, and prompt bounds but does not resolve an executable, spawn a process, read credentials, or call a provider.
- Released `agent/prompts/evaluator-question-generation/v1.0.0.md` for `evaluator-input@1` → `evaluator-question-set@1`.
- Added synthetic question-shape, evidence-reference, prompt-injection, and insufficient-evidence eval cases.
- Extended the Agent infrastructure validator and negative tests to enforce prompt path/version/status, runtime schema IDs, capability policy, and core safety language.
- Updated `PROJECT.md` and Agent module guides with stable Phase 1 facts.
- Ran the required `bits-unit-test-gen` workflow. Scope was non-diff and limited to `NewInput`, `ParseQuestionSet`, and `BuildPiArguments`; no pre-existing source defect was identified. Its coverage gate was skipped because neither the repository nor the request defines a CI coverage threshold and `EXEC_SOURCE` was empty. `utree flush` completed successfully.

## Modified Files

Runtime contract and tests:

- `internal/evaluator/contract.go`
- `internal/evaluator/contract_test.go`
- `internal/evaluator/pi_contract.go`
- `internal/evaluator/pi_contract_test.go`

Prompt and development eval assets:

- `agent/README.md`
- `agent/prompts/README.md`
- `agent/prompts/evaluator-question-generation/v1.0.0.md`
- `agent/evals/cases/question-generation-injection.json`
- `agent/evals/cases/question-generation-insufficient.json`
- `agent/evals/cases/question-set-unknown-reference.json`
- `agent/evals/cases/question-set-wrong-shape.json`

Governance and validation:

- `PROJECT.md`
- `docs/decisions/ADR-0003-post-preview-evaluator-boundary.md`
- `plans/post-preview-evaluator-adapter.md`
- `scripts/validate-agent-infra.sh`
- `scripts/test-agent-infra.sh`
- `docs/checkpoints/post-preview-evaluator-adapter-phase-1.md`

No file under `cmd/`, `internal/daemon/`, `internal/evidence/`, `extensions/`, or `tests/extension/` changed. No dependency or generated file changed.

## Important Decisions

- ADR-0003 is accepted. Preview and evaluation remain separate authenticated operations, and later continuation must consume the exact retained preview once.
- Runtime evaluator schemas are Go-owned compatibility contracts, separate from the development-only eval-case and run-record schemas.
- Successful output is exactly Q1/Q2 `code_specific` and Q3 `go_backend`; the only alternate is an empty `insufficient_evidence` result.
- The first evaluator compatibility target is exactly Pi `0.84.3`.
- Phase 1 freezes the argument and model mapping only. Executable resolution, version preflight, RPC framing, deadlines, output caps, and process lifecycle remain Phase 3 work.
- The released prompt is immutable. Any behavior change requires a new prompt version and corresponding eval coverage.
- No current product caller can reach `NewInput`, `ParseQuestionSet`, or `BuildPiArguments`.

## Tests / Verification

Passed:

- `go test -count=1 ./internal/evaluator`
- `CGO_ENABLED=0 go test -count=1 ./...`
- `go test -count=1 -race -tags netgo ./...`
- `go vet ./...`
- `npm run typecheck`
- `npm test` outside the restricted sandbox: 12/12 passed
- `scripts/test-agent-infra.sh`
- `scripts/validate-agent-infra.sh`
- `git diff --check`
- required `utree flush` for the unit-test generation workflow

Recorded host/sandbox behavior:

- Default Go 1.21.13 `go test -count=1 ./...` and untagged race testing still hit the already documented macOS 26 `missing LC_UUID` issue for network-enabled command/daemon binaries. The evaluator and evidence packages pass in that run; the established pure-Go and `netgo` commands above pass.
- The first restricted `npm test` run could not listen on `127.0.0.1` and failed four existing loopback tests with `EPERM`. The same command passed all 12 tests outside the sandbox.

No Pi executable, RPC process, provider, API credential, or model was used.

## Known Issues

- The evaluator contracts are not connected to `/learn`, the daemon, or `evidence.BuildBundle`.
- There is no continuation store, confirmation UI, deterministic adapter, continuation route, or question renderer.
- There is no executable lookup, Pi version preflight, JSONL RPC implementation, output stream cap, timeout, cancellation, or process reaping.
- Existing development run-record fixtures remain placeholders; no runtime run record or source persistence was added.
- The host's documented Go 1.21.13 `LC_UUID` limitation remains unchanged.

## Remaining Work

Phase 2, only after explicit authorization:

- bounded in-memory continuation store;
- additive preview continuation metadata and authenticated `POST /v1/question-sets`;
- exact retained-result consume and `evidence.BuildBundle` → `evaluator.NewInput` connection;
- internal evaluator interface and deterministic fixture adapter;
- explicit Pi confirmation and deterministic three-question rendering;
- expiry, capacity, concurrency, decline, strict protocol, compatibility, and no-model integration tests.

Phase 3 remains separately gated and owns the real isolated Pi RPC process adapter.

## Next Step

Wait for explicit user authorization of `post-preview-evaluator-adapter` Phase 2. Before implementation, re-read `AGENTS.md`, `PROJECT.md`, ADR-0002, accepted ADR-0003, the active plan, and this checkpoint; then inspect `git status` and the complete diff.

## Do Not Change

- Do not implement Phase 2 or Phase 3 without separate explicit authorization.
- Do not add a daemon route, continuation store, extension UI, subprocess spawn, credential access, live model call, persistence, dependency, or retry in the current phase.
- Do not send any evidence before the user has seen the preview and explicitly confirmed continuation.
- Do not reread the repository after confirmation or accept client-built evidence.
- Do not weaken the fixed question schema, reference checks, Pi 0.84.3 gate, deny flags, or safe error behavior.
- Do not edit the released prompt in place; add a new version through an approved plan when behavior must change.
