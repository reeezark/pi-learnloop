---
id: answer-assessment-workflow-phase-1
plan: answer-assessment-workflow
phase: 1
status: current
updated: 2026-09-01
---

# Answer Assessment Workflow Phase 1

## Goal

Accept ADR-0004 and establish provider-independent answer-assessment contracts, deterministic label aggregation, a deterministic evaluator fixture, a draft assessment prompt, and synthetic review cases without adding daemon, extension, or production Pi behavior.

## Current Phase

Phase 1 is complete. The plan now points to Phase 2 with `phase_status: awaiting_approval`.

Repository snapshot before this phase:

- checkout: `/Users/bytedance/workspace/pi-learnloop`
- branch: `main`, tracking `origin/main`
- Phase 1 baseline: `5d38e10`
- Phase 1 commit: the commit containing this checkpoint

## Completed

- Accepted ADR-0004's volatile lifecycle, strict answer/turn schemas, one-follow-up limit, deterministic verdict aggregation, privacy boundary, and phase gates.
- Added `evaluator-assessment-input@1` with owned validated evaluator input, fixed Q1/Q2/Q3 question set, exactly three ordered non-empty answers, and an optional single F1 exchange for the final stage.
- Added `evaluator-assessment-turn@1` strict parsing with exact fields, duplicate-key and trailing-content rejection, UTF-8 and byte bounds, ordered verdicts, reference validation, and a prohibition on a second follow-up.
- Added the compatibility-sensitive Go label rule: all demonstrated is `understood`; at least two not demonstrated is `review_needed`; every other valid combination is `partial`.
- Added a separate `AssessmentEvaluator` interface and `DeterministicAssessmentEvaluator` test fixture. No production code path constructs or invokes it.
- Added exhaustive label tests for all 27 verdict combinations and focused contract tests for owned copies, bounds, stages, references, malformed output, cancellation, model selection, and deterministic follow-up behavior.
- Added draft `evaluator-answer-assessment@1.0.0`. It is neither embedded nor released and cannot be invoked by the product.
- Added seven synthetic development cases covering unsupported answers, answer injection, over-crediting, necessary and unnecessary follow-ups, second-follow-up rejection, and malformed output.
- Updated stable project and evaluator-asset documentation with the implemented Phase 1 boundary and remaining unavailable product behavior.

## Modified Files

Runtime contracts and deterministic test seam:

- `internal/evaluator/assessment_contract.go`
- `internal/evaluator/assessment_contract_test.go`
- `internal/evaluator/assessment_evaluator.go`
- `internal/evaluator/assessment_evaluator_test.go`

Development assets:

- `agent/prompts/evaluator-answer-assessment/v1.0.0.md`
- `agent/prompts/README.md`
- `agent/evals/README.md`
- seven `assessment-*.json` cases under `agent/evals/cases/`
- `agent/README.md`

Stable and lifecycle documentation:

- `PROJECT.md`
- `plans/answer-assessment-workflow.md`
- `docs/decisions/ADR-0004-answer-assessment-lifecycle.md`
- `docs/checkpoints/answer-assessment-workflow-phase-1.md`

No daemon, command, extension, existing production prompt, prompt embedding, dependency, persistence, SQLite, SSE, CI/CD, release, or generated file changed.

## Important Decisions

- Assessment inputs revalidate and own the complete prior evaluator input and question set. They accept no repository root, credentials, executable path, source rebuilt by a client, or mutable aliases.
- Empty runtime slice representations are normalized back to the evidence manifest's canonical form before bundle-hash validation; this preserves validation of legitimate bundles without weakening content or metadata checks.
- Initial and final assessment stages are distinct values. Only the initial stage can return F1; the final stage must return three evaluations.
- The model never controls the public label. The Go mapping is exhaustive and has no score threshold.
- The deterministic adapter is explicitly a test fixture and is not a fallback for missing production evaluation.
- The prompt stays `draft` until Phase 3 releases and embeds it after the separately authorized lifecycle work.
- The existing development-case schema is reused; no runtime assessment values are persisted as fixtures or promoted into a product protocol.

## Tests / Verification

Passed:

- focused assessment constructor, parser, label, and deterministic-evaluator tests;
- complete `CGO_ENABLED=0 go test -count=1 ./internal/evaluator`;
- complete `CGO_ENABLED=0 go test -count=1 ./...`;
- complete `go test -count=1 -race -tags netgo ./...` on the final rerun;
- `go vet ./...`;
- `npm run typecheck`;
- `npm test`: 18/18 passed;
- required `bits-unit-test-gen` Step 1-7 workflow and `utree flush`;
- `scripts/test-agent-infra.sh`;
- `scripts/validate-agent-infra.sh`;
- `git diff --check`.

The first full race run hit the existing two-second Pi preflight boundary in one fake-executable test. The unchanged focused test passed immediately, and the unchanged complete race command then passed. No Pi RPC source or timeout was modified.

Coverage enforcement was skipped because the repository, user request, and execution source define no coverage threshold. No live Pi process, provider, credential, paid request, or model network call was used.

## Known Issues

- The product still stops after rendering Q1/Q2/Q3. It cannot collect answers or return an assessment.
- There is no volatile assessment state, assessment identifier, assessment HTTP route, answer UI, production Pi assessment adapter, prompt embedding, or product cost confirmation for the additional turns.
- The draft prompt has not been exercised against a live model and must not be described as released behavior.
- Assessment state remains intentionally non-durable in ADR-0004; SQLite and crash recovery are separate future work.
- The existing Pi fake-executable preflight test can approach its two-second bound under race load; this phase did not alter that unrelated security-sensitive code.

## Remaining Work

- Phase 2: implement the bounded in-memory assessment lifecycle, additive authenticated protocol, answer/follow-up UI, and deterministic end-to-end tests while production assessment remains unavailable.
- Phase 3: release/embed the prompt and add the isolated Pi RPC assessment adapter with fake-process coverage.
- Later separate work: durable learning history, SQLite jobs, SSE, Session selection, reminders, and evidence enrichment.

## Next Step

Stop after committing and pushing Phase 1. Wait for explicit authorization before implementing Phase 2.

When Phase 2 is authorized, restore context from `AGENTS.md`, `PROJECT.md`, ADR-0004, `plans/answer-assessment-workflow.md`, this checkpoint, Git status, and the Phase 1 commit. Confirm the exact allowed-file list before editing.

## Do Not Change

- Do not begin Phase 2 without explicit authorization.
- Do not add daemon routes, assessment stores, extension answer UI, or production Pi assessment invocation in this checkpoint.
- Do not embed or release the draft assessment prompt.
- Do not use the deterministic adapter as a production fallback.
- Do not accept evidence, questions, model selection, prompt version, repository paths, credentials, or executable paths from future assessment submissions.
- Do not allow more than Q1/Q2/Q3, one F1, one F1 answer, or model-selected public labels.
- Do not retry or repair paid assessment calls automatically.
- Do not add dependencies, persistence, SQLite, SSE, background workers, Session indexing, source expansion, reminders, CI/CD, release work, or unrelated refactoring.
