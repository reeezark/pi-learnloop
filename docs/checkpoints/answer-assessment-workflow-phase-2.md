---
id: answer-assessment-workflow-phase-2
plan: answer-assessment-workflow
phase: 2
status: current
updated: 2026-09-01
---

# Answer Assessment Workflow Phase 2

## Goal

Implement the bounded volatile assessment lifecycle, additive authenticated protocol, answer/follow-up UI, and deterministic end-to-end verification without enabling a production assessment model process.

## Current Phase

Phase 2 is complete. The plan now points to Phase 3 with `phase_status: awaiting_approval`.

Repository snapshot before this phase:

- checkout: `/Users/bytedance/workspace/pi-learnloop`
- branch: `main`, tracking `origin/main`
- Phase 2 baseline: `22e547b`
- Phase 2 commit: the commit containing this checkpoint

## Completed

- Added `internal/assessment.Service` with `Start`, `Submit`, and `Close` as the single owner of volatile answer state.
- Retained owned validated evaluator input, Q1/Q2/Q3 question set, and fixed model selection behind `as1-` plus 32 random base64url bytes.
- Enforced the accepted 30-minute lifetime, eight-live-entry limit, and 1-MiB aggregate evidence limit without evicting live entries. Expiry is checked before admission and submission, old stage copies are released during transitions, and shutdown clears all state.
- Implemented atomic `awaiting_answers → evaluating_initial → complete | awaiting_follow_up → evaluating_follow_up → complete` transitions. Invalid requests preserve state; a started evaluator failure invalidates it; concurrent and replayed submissions cannot start a second call.
- Added an assessment descriptor to successful question-set responses and strict authenticated `POST /v1/assessment-turns` handling for either exact initial answers or exact F1 fields.
- Reused safe evaluator error categories, mapped unknown/expired/completed/failed/malformed/concurrent IDs to `409 assessment_unavailable`, and added no daemon retry.
- Wired production with an assessment service that has no evaluator. It reports `evaluator_unavailable`, retains no assessment, and never falls back to the deterministic fixture.
- Extended the daemon client with strict descriptor/result validation and a one-attempt assessment request.
- Extended `/learn` to collect exactly Q1/Q2/Q3, stop locally on cancellation/empty/invalid input, disclose evidence/answer sharing and possible cost, submit one initial turn, render at most one F1, submit F1 once, and display the daemon-provided verdicts plus Go-derived label.
- Added deterministic service, protocol, concurrency, failure, cancellation, no-retry, malformed-response, follow-up, and final-rendering tests without contacting Pi or a provider.
- Updated stable user and project documentation while keeping the Phase 3 production boundary explicit.

## Modified Files

Production implementation:

- `internal/assessment/service.go`
- `internal/daemon/daemon.go`
- `internal/daemon/server.go`
- `extensions/lib/daemon-client.ts`
- `extensions/lib/learn-command.ts`

Automated verification:

- `internal/assessment/service_test.go`
- `internal/daemon/assessment_test.go`
- `tests/extension/daemon-client.test.ts`
- `tests/extension/learn-command.test.ts`

Stable and lifecycle documentation:

- `README.md`
- `PROJECT.md`
- `plans/answer-assessment-workflow.md`
- `docs/checkpoints/answer-assessment-workflow-phase-1.md`
- `docs/checkpoints/answer-assessment-workflow-phase-2.md`

No dependency, prompt, prompt embedding, evaluator process, evidence expansion, persistence, SQLite, SSE, background job, Session, CI/CD, release, or generated file changed.

## Important Decisions

- `internal/assessment` is the deep state-machine boundary. HTTP and TypeScript do not own lifecycle, evidence, model binding, follow-up eligibility, or label rules.
- The service creates an owned assessment only from the exact already validated input and question result. The client can send only an opaque ID and stage-specific answers.
- A stage leaves its awaiting state before the evaluator is called. A lost response or evaluator failure is intentionally not retryable because the paid call may already have occurred.
- The entry releases the prior retained representation when it moves to an evaluating state, so an F1 assessment does not keep two long-lived copies of the evidence while still accounting against only one capacity unit.
- The existing question response remains additively compatible: clients that ignore `assessment` still receive the unchanged `question_set`.
- Production constructs `assessment.New(nil)` until Phase 3. Deterministic evaluation is available only through explicit test composition.
- The extension validates and renders results but never derives the public label or accepts a second F1.

## Tests / Verification

Passed:

- focused `internal/assessment` ownership, capacity, expiry, lifecycle, failure, close, and concurrency tests;
- focused daemon descriptor, strict request, unavailable, failure, F1, replay, and concurrent-submission tests;
- focused extension complete, F1, cancellation, unavailable, no-retry, and malformed-response tests;
- complete `CGO_ENABLED=0 go test -count=1 ./internal/evaluator ./internal/assessment ./internal/daemon`;
- complete `CGO_ENABLED=0 go test -count=1 ./...`;
- race-enabled `internal/assessment` and `internal/daemon` results within the complete race runs;
- `go vet ./...`;
- `npm run typecheck`;
- `npm test`: 25/25 passed with local loopback permission;
- required `bits-unit-test-gen` Step 1-7 workflow and `utree flush`;
- `scripts/test-agent-infra.sh`;
- `scripts/validate-agent-infra.sh`;
- `git diff --check`.

The unit-test workflow generated 31 Phase 2 test scenarios and all 31 passed. It found no product defect outside the planned missing implementation. Coverage enforcement was skipped because the repository, user request, and execution source define no coverage threshold.

The first sandboxed `npm test` attempt could not bind `127.0.0.1`; the unchanged command passed 25/25 with local-loopback permission.

The complete `go test -count=1 -race -tags netgo ./...` command was run three times. `internal/assessment` and `internal/daemon` passed every run, but the pre-existing `TestNewPiRPCEvaluator/resolves_a_symlink_and_freezes_a_supported_executable` fake-process preflight crossed its unchanged two-second ADR-0003 boundary during each full parallel run. The exact focused race test passed unchanged on a subsequent run. No Phase 2 source touches that adapter or timeout, so this checkpoint records the known load-sensitive test instead of weakening the security boundary or claiming a fully green race command.

No real Pi assessment process, provider, credential, paid request, or model network call was used.

## Known Issues

- Production answer assessment remains unavailable until Phase 3 releases/embeds the prompt and wires the isolated Pi assessment adapter.
- Volatile assessments are lost on daemon restart, expiry, evaluator failure, or a lost response and cannot be resumed. This is accepted for the pre-persistence slice.
- Pi 0.84.3 exposes only a string-returning input dialog, so answers use the current single-input UI rather than a guaranteed multiline editor.
- The existing race-enabled Pi preflight fixture can exceed its exact two-second boundary under full parallel test load; the focused unchanged test can pass and the ordinary full suite passes.
- There is no durable result, SQLite, learning history, SSE, background worker, Session selection, reminder, or evidence enrichment.

## Remaining Work

- Phase 3: release/embed the assessment prompt and implement the isolated Pi RPC assessment adapter with fake-process tests and no product retry.
- Later separate work: durable learning history, SQLite jobs, crash recovery, SSE, Session selection, reminders, and evidence enrichment.

## Next Step

Stop after committing and pushing Phase 2. Wait for explicit authorization before implementing Phase 3.

When Phase 3 is authorized, restore context from `AGENTS.md`, `PROJECT.md`, ADR-0002, ADR-0003, ADR-0004, `plans/answer-assessment-workflow.md`, this checkpoint, Git status, and the Phase 2 commit. Reconfirm the exact prompt, schema, timeout, and Pi invocation scope before editing.

## Do Not Change

- Do not begin Phase 3 without explicit authorization.
- Do not embed or release the draft assessment prompt and do not construct a production assessment evaluator yet.
- Do not use `DeterministicAssessmentEvaluator` as a production fallback.
- Do not accept evidence, questions, model selection, prompt version, repository paths, credentials, executable paths, or labels from assessment clients.
- Do not allow more than Q1/Q2/Q3, one F1, one F1 answer, or one evaluator call per accepted stage.
- Do not retry or repair failed, timed-out, invalid, concurrent, replayed, or lost-response assessment calls automatically.
- Do not weaken loopback authentication, strict JSON, fixed bounds, Pi process isolation, capability denies, or safe errors.
- Do not add dependencies, persistence, SQLite, SSE, background workers, Session indexing, source expansion, reminders, CI/CD, release work, or unrelated refactoring.
