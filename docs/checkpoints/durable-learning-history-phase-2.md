---
id: durable-learning-history-phase-2
plan: durable-learning-history
phase: 2
status: superseded
updated: 2026-09-02
---

# Durable Learning History Phase 2

## Goal

Connect the accepted schema-v1 history store to the answer-assessment lifecycle, bind only server-owned source-free provenance, and surface terminal save status without changing evaluator semantics or adding a history query.

## Current Phase

Phase 2 is complete. The active plan now points to Phase 3 with `phase_status: awaiting_approval`; the authenticated repository-history route and manual `/learn-history` UI remain blocked on a new explicit high-risk authorization.

Repository snapshot before this phase:

- checkout: `/Users/bytedance/workspace/pi-learnloop`
- branch: `main`, tracking `origin/main`
- Phase 2 baseline: `d194533`
- Phase 2 commit: the commit containing this checkpoint

## Completed

- Added immutable prompt metadata accessors that derive SHA-256 from the exact embedded question-generation and answer-assessment prompt bytes without changing either prompt body.
- Extended the assessment service with server-owned canonical-root and prompt provenance. Base/head revisions, manifest hash, schema versions, and model selection are derived from already validated internal values rather than accepted as additional client input.
- Connected one concrete `history.Store` directly to the assessment lifecycle without adding an ORM, port/adapter hierarchy, or alternate storage abstraction.
- After a valid initial submission atomically enters `evaluating_initial`, creates one source-free `running` record before the assessment evaluator starts when storage is available.
- Reuses the same durable record across F1, marks `follow_up_used`, records `complete` with the Go-derived label and exactly Q1/Q2/Q3 kinds/verdicts, and records evaluator failure/invalid-output/timeout with only stable safe codes.
- Preserves the validated assessment result when create, F1, or terminal history writes fail. Complete responses now strictly contain either `history.saved:true` plus a valid `lr1-` ID or `history.saved:false` plus `storage_unavailable`; no failure path retries an evaluator.
- Opens and closes production history at `os.UserConfigDir()/pi-learnloop/data/history.db`. `daemon.Config.DataDir` accepts only an explicit absolute test path. Unsafe, corrupt, newer, unavailable, or otherwise failed history open degrades persistence only and does not block preview, question generation, or volatile assessment.
- Retains existing startup recovery semantics: opening the store changes leftover `running` rows to `interrupted` before serving, without starting Pi, contacting a provider, resuming work, or rebuilding source-bearing state.
- Updated the TypeScript client to validate the exact additive save descriptor and the `/learn` UI to emit one concise warning only when a completed result was not saved. Assessment submissions remain non-retryable.
- Updated README and PROJECT stable facts. No history query route, `/learn-history` command, reminder, worker, SSE, retention, deletion, export, repair, prompt body, evaluator contract, dependency, CI/CD, or release behavior was added.

## Modified Files

Production prompt metadata:

- `agent/prompts/assets.go`

Assessment lifecycle and verification:

- `internal/assessment/service.go`
- `internal/assessment/service_test.go`
- `internal/assessment/history_test.go`

Daemon composition, protocol, and verification:

- `internal/daemon/daemon.go`
- `internal/daemon/server.go`
- `internal/daemon/assessment_test.go`
- `internal/daemon/daemon_test.go`

Thin extension protocol/UI and verification:

- `extensions/lib/daemon-client.ts`
- `extensions/lib/learn-command.ts`
- `tests/extension/daemon-client.test.ts`
- `tests/extension/learn-command.test.ts`

Stable and lifecycle documentation:

- `README.md`
- `PROJECT.md`
- `plans/durable-learning-history.md`
- `docs/checkpoints/durable-learning-history-phase-1.md`
- `docs/checkpoints/durable-learning-history-phase-2.md`

## Important Decisions

- The assessment service accepts only canonical root and the two immutable prompt identities as new provenance. It derives revisions, manifest, schema, Pi version, provider, model, and thinking level from validated values it already owns, reducing mismatch and spoofing risk.
- History remains a local-substitutable deep module. Production uses the concrete store and tests use real temporary protected SQLite databases; no hypothetical storage seam or mock database was introduced.
- Source-bearing assessment state remains volatile. The history API and schema still make source, paths, questions, answers, F1 text/answer, feedback, prompt bodies, RPC/model output, credentials, tokens, executable paths, and Session transcripts structurally unavailable for persistence.
- Running creation respects the assessment request context before the paid call. Best-effort F1/terminal bookkeeping uses a cancellation-detached context bounded to five seconds so a client disconnect or expired HTTP response cannot erase a result already obtained from the provider; this never authorizes another evaluator call.
- Follow-up HTTP responses keep their previous exact root shape. Only complete responses gain the required `history` object, and the extension rejects missing, extra, malformed, or contradictory descriptor fields.
- An F1 bookkeeping failure intentionally stops further durable updates for that attempt and ultimately reports `storage_unavailable`; any remaining running marker becomes interrupted on a later compatible open.
- Production history-open failure is not published in the runtime descriptor and does not change preview/question availability. The user sees persistence availability only through the terminal save descriptor in this phase.

## Tests / Verification

Passed without a live provider call or production database:

- `CGO_ENABLED=0 go test -count=1 ./internal/assessment ./internal/daemon ./internal/history`
- `CGO_ENABLED=0 go test -count=1 ./...`
- `go test -race -count=1 -tags netgo ./...`
- `go vet -tags netgo ./...`
- `go build -tags netgo -o /tmp/pi-learnloop-phase2 ./cmd/pi-learnloop`
- `npm run typecheck`
- `npm test` (27 tests)
- `npm pack --dry-run --json`
- `scripts/test-agent-infra.sh`
- `scripts/validate-agent-infra.sh`
- `git diff --check`

Focused Phase 2 verification covers running-before-evaluator ordering, direct completion, one-record F1 completion, safe failure codes, cancellation, terminal store failure after evaluator success, strict saved/unsaved response shapes, response loss after commit, concurrent/replayed submission with one evaluator and one record, server-owned prompt/schema/model provenance, excluded-content absence across SQLite files, daemon startup interruption recovery with zero evaluator calls, explicit absolute test data paths, and preview/question operation when history cannot open.

The build output was written only to `/tmp/pi-learnloop-phase2`; no repository build artifact or production history database was created.

## Known Issues

- There is no authenticated history query route or `/learn-history` command, so Phase 2 records are durable but not yet viewable through the product UI.
- History-open failure is intentionally degradable and not exposed until a completed assessment returns `storage_unavailable`; a dedicated availability/status surface is not part of the accepted Phase 2 protocol.
- There is no retention, deletion, export, repair, downgrade, remote sync, automatic reminder, evaluator retry/resume, durable worker, or background recovery.
- Source-bearing live assessments still cannot resume after restart. A running record becomes `interrupted`, and the user must manually start a new `/learn` flow.

## Remaining Work

- Phase 3: add strict authenticated `POST /v1/learning-history-queries`, canonical repository verification, a required bounded limit capped at 50, exact source-free response validation, and manual `/learn-history` rendering without any model call.
- Update README/PROJECT with the final user-visible history query, empty/unavailable behavior, WAL backup caution, and repository scoping after Phase 3 is authorized and implemented.
- Any retention, deletion, export, repair, reminder, retry/resume, analytics, remote sync, or new schema behavior requires a separate approved plan or ADR as applicable.

## Next Step

Stop after committing and pushing Phase 2. A later Agent must restore context from `AGENTS.md`, `PROJECT.md`, ADR-0002 through ADR-0005, `plans/durable-learning-history.md`, this checkpoint, Git status, and the Phase 2 commit. Do not begin Phase 3 until the user explicitly authorizes `durable-learning-history Phase 3`.

## Do Not Change

- Do not add the history query route/UI before Phase 3 authorization.
- Do not persist source, changed paths, questions, answers, F1 content, feedback, prompt bodies, RPC/model output, credentials, tokens, executable paths, or Session transcripts.
- Do not resume or retry interrupted/failed assessments, add a durable job queue, or infer provider completion from a lost response.
- Do not weaken exact response validation, same-owner/mode/symlink/hard-link/local-filesystem checks, schema/value preflight, WAL/FULL/foreign-key/trusted-schema settings, repository isolation, or safe failure codes.
- Do not change schema version 1, `lr1-` IDs, persisted enums, prompt bodies, evaluator semantics, dependency versions, Go baseline, CI/CD, or release configuration without a separately reviewed authorization.
