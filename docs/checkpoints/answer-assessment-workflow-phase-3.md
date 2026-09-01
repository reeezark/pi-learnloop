---
id: answer-assessment-workflow-phase-3
plan: answer-assessment-workflow
phase: 3
status: current
updated: 2026-09-01
---

# Answer Assessment Workflow Phase 3

## Goal

Release and embed the assessment prompt, add the production isolated Pi RPC assessment adapter, wire it through the volatile assessment service, and verify every model turn with fake processes without contacting a provider.

## Current Phase

Phase 3 and the `answer-assessment-workflow` plan are complete.

Repository snapshot before this phase:

- checkout: `/Users/bytedance/workspace/pi-learnloop`
- branch: `main`, tracking `origin/main`
- Phase 3 baseline: `b37af70`
- Phase 3 commit: the commit containing this checkpoint

## Completed

- Released and embedded `evaluator-answer-assessment@1.0.0` as the production `evaluator-assessment-input@1` to `evaluator-assessment-turn@1` prompt.
- Added `PiRPCAssessmentEvaluator` as a narrow production implementation of `AssessmentEvaluator`.
- Kept question and assessment interfaces separate while extracting only the shared RPC lifecycle into a package-private function.
- Preserved the frozen symlink-resolved Pi 0.84.3 executable, fixed no-session/no-tools arguments, disabled retry/compaction, empty command discovery, strict response correlation, event rejection, stream bounds, 120-second deadline, opaque errors, and termination/reaping behavior.
- Made each initial or F1 assessment call marshal exactly one validated assessment envelope into one fresh Pi process. No process survives while waiting for user input and no product retry or repair call exists.
- Wired the production daemon to construct the assessment adapter at startup and pass it to `internal/assessment`; deterministic adapters remain test-only fixtures and are never production fallbacks.
- Extended fake-Pi coverage for exact initial and follow-up stages, two distinct process starts, complete and F1 output, invalid JSON/schema/references, response correlation, discovered commands, tool/unknown events, authentication failure, timeout, cancellation, stdout/stderr/output caps, early exit, opaque errors, and reaping.
- Added a daemon integration test that drives preview, question generation, volatile assessment creation, production Pi assessment, and final Go-derived label through authenticated loopback HTTP.
- Changed fake `--version` responses to a lightweight shell path so race-instrumented helper startup cannot spuriously cross the unchanged two-second production preflight boundary.
- Updated stable project, security, prompt, and opt-in live-smoke documentation. The live procedure warns that assessment resends selected excerpts with answers and can add up to two model calls.

## Modified Files

Production implementation:

- `agent/prompts/assets.go`
- `agent/prompts/evaluator-answer-assessment/v1.0.0.md`
- `internal/evaluator/pi_rpc.go`
- `internal/daemon/daemon.go`

Automated verification:

- `internal/evaluator/pi_rpc_assessment_test.go`
- `internal/evaluator/pi_rpc_test.go`
- `internal/daemon/daemon_test.go`
- `internal/daemon/question_set_test.go`

Stable and lifecycle documentation:

- `README.md`
- `PROJECT.md`
- `agent/README.md`
- `agent/prompts/README.md`
- `plans/answer-assessment-workflow.md`
- `docs/checkpoints/answer-assessment-workflow-phase-2.md`
- `docs/checkpoints/answer-assessment-workflow-phase-3.md`

No dependency, protocol schema, extension behavior, persistence, SQLite, SSE, background job, Session, CI/CD, release automation, or generated file changed.

## Important Decisions

- `PiRPCAssessmentEvaluator` is separate from `PiRPCEvaluator`; only the unexported Pi RPC mechanics are shared. No general process-control interface was introduced.
- Both released prompts are immutable embedded assets. Future behavior changes require a new prompt version and compatibility review.
- Every assessment turn is one new process and one accepted model result. A final-stage F1, malformed result, timeout, child failure, or lost response cannot trigger a repair or retry.
- The production daemon may expose question generation while assessment is unavailable if the assessment adapter cannot pass startup preflight. It never uses deterministic evaluation as a fallback.
- The accepted 2-second version preflight and 120-second evaluator deadline remain unchanged. The race flake was removed at the fake-executable boundary instead of weakening production limits.

## Tests / Verification

Passed after the final implementation and fake-preflight fixture update:

- `CGO_ENABLED=0 go test -count=1 ./internal/evaluator`
- `CGO_ENABLED=0 go test -count=1 ./internal/assessment`
- `CGO_ENABLED=0 go test -count=1 ./internal/daemon`
- `CGO_ENABLED=0 go test -count=1 ./...`
- `go test -count=1 -race -tags netgo ./internal/evaluator`
- `go test -count=1 -race -tags netgo ./...`
- `go vet ./...`
- `npm run typecheck`
- `npm test`: 25/25 passed
- required `bits-unit-test-gen` Step 1-7 workflow
- `scripts/test-agent-infra.sh`
- `scripts/validate-agent-infra.sh`
- `git diff --check`

The first complete race run failed only because a race-instrumented fake Pi test binary took 2.02 seconds to answer `--version`. The fixture now answers version preflight directly in its shell wrapper while retaining the race-instrumented child for every RPC scenario. The focused evaluator race test and the complete race suite then passed with the unchanged production timeout.

Coverage enforcement was skipped because the repository, user request, and execution source define no coverage threshold. No real Pi provider request, credential, paid call, or live smoke test was used.

## Known Issues

- Assessment state and results remain volatile. Daemon restart, expiry, evaluator failure, or a lost response requires a new `/learn` flow.
- Pi 0.84.3 exposes only a string-returning input dialog, so the first UI does not guarantee multiline answer editing.
- There is no SQLite persistence, durable learning history, crash recovery, SSE, background worker, Session selection, reminder, or evidence enrichment.
- Pi/provider transport retry behavior remains external. The supported configuration keeps `retry.provider.maxRetries` at `0` because RPC cannot enforce it.

## Remaining Work

The volatile answer-assessment workflow has no remaining phase. Future work requires separate investigation, a new plan, and explicit authorization, beginning with durable learning-history and recovery design if that remains the next product priority.

## Next Step

Stop after committing and pushing Phase 3. A later Agent should restore context from `AGENTS.md`, `PROJECT.md`, ADR-0002, ADR-0003, ADR-0004, `plans/answer-assessment-workflow.md`, this checkpoint, Git status, and the Phase 3 commit before proposing the next plan.

## Do Not Change

- Do not persist source, answers, prompts, RPC streams, model output, credentials, or Session transcripts without a separately accepted storage ADR and plan.
- Do not add product retries, repair calls, process reuse across human input, a second follow-up, or a deterministic production fallback.
- Do not accept evidence, questions, model selection, prompt version, repository paths, credentials, executable paths, or labels from assessment clients.
- Do not weaken loopback authentication, strict JSON, fixed bounds, Pi 0.84.3 isolation, capability denies, safe errors, or process reaping.
- Do not add SQLite, SSE, background workers, Session indexing, source expansion, reminders, dependencies, CI/CD, or release work without a new approved scope.
