---
id: explicit-pi-session-review-phase-2
plan: explicit-pi-session-review
phase: 2
status: current
updated: 2026-09-02
---

# Explicit Pi Session Review Phase 2

## Context

### Goal

Add two independent strict authenticated daemon routes and propagate one bounded source-free Pi Session ID from the exact preview continuation to Session-aware history without placing it in Git evidence, evaluator values, prompts, RPC/model content, generic history, errors, or logs.

### Current Phase

Phase 2 is complete. The active `explicit-pi-session-review` plan now points to Phase 3 with `phase_status: awaiting_approval`. Phase 3 is not authorized and remains gated on the explicit Pi 0.84.3 listing privacy/resource review recorded in the plan and ADR-0006.

The Phase 2 implementation baseline is Phase 1 commit `2f3cd44845ce12a1320f330cf99c8fbf82a55137`, which is also the verified `origin/main`. The completed Phase 2 changes are currently in the working tree and have not been committed or pushed.

## Completed

- Added strict authenticated `POST /v1/pi-session-evidence-previews` with the existing repository and Git-selection interface plus one validated 1–128-byte Pi Session ID.
- Reused the existing evidence timeout, fixed 20-file/100-declaration/128-KiB caps, Git error mapping, preview response shape, loopback/Host/Origin defenses, Instance Token authentication, JSON uniqueness checks, and 16-KiB request cap.
- Kept the generic `/v1/evidence-previews` request and response unchanged. The Session-bound preview response also omits the Session ID.
- Deepened the daemon continuation module so atomic single-use consumption returns one owned value containing the exact cloned `evidence.Result` plus separate optional Session provenance. Generic preview stores empty provenance; Session preview validates before retention.
- Kept `evidence.Result`, `evidence.Bundle`, evaluator contracts, question-set request, assessment-turn request, and all model-visible schemas unchanged.
- Extended only server-owned `assessment.Provenance` with an optional validated ID. Evidence alone reaches `BuildBundle`, question evaluation, and answer evaluation; the ID reaches only `history.CreateWithPiSession` before the first assessment evaluator call.
- Preserved the existing Git-only assessment path through `history.Create`, so its schema-v2 Session column remains SQL `NULL`.
- Added strict authenticated `POST /v1/pi-session-review-queries`, accepting 1–20 unique validated IDs, canonicalizing the repository through the existing Git seam, and returning only completed matches in candidate order.
- Made absent/closed/failing protected history return explicit `503 history_unavailable`; repository verification occurs first, so an invalid repository is never reported as an empty unreviewed result.
- Proved through fake evaluator capture that Session identity is absent from question/assessment inputs and their exact JSON serialization used as Pi RPC runtime messages. Released prompt assets are unchanged and contain no runtime provenance.
- Proved that Session identity is absent from Session-preview, question, assessment, and generic history responses and from safe invalid-request errors. The daemon has no request-body or Session-provenance logging path.
- Proved completion-only filtering across `running`, `failed`, recovered `interrupted`, SQL-NULL/no-record, completed, candidate-order, nested-path canonicalization, and other-repository cases.
- Added no dependency, migration, database change, route field on an existing interface, evidence/model field, Session metadata/content, hook, snapshot, Session write, extension storage, background work, inference, retry, or extension behavior.

## Modified Files

Implementation:

- `internal/daemon/continuation.go`
- `internal/daemon/server.go`
- `internal/assessment/service.go`

Focused verification:

- `internal/daemon/continuation_test.go`
- `internal/daemon/pi_session_preview_test.go`
- `internal/daemon/pi_session_provenance_test.go`
- `internal/daemon/pi_session_review_query_test.go`
- `internal/assessment/history_test.go`

Stable and lifecycle documentation:

- `README.md`
- `PROJECT.md`
- `plans/explicit-pi-session-review.md`
- `docs/checkpoints/explicit-pi-session-review-phase-1.md`
- `docs/checkpoints/explicit-pi-session-review-phase-2.md`

ADR-0006 required no content change; its accepted decision already exactly describes the implemented Phase 2 interface and isolation.

## Important Decisions

- Continuation consumption exposes one private owned value rather than adding a second lookup/consume interface. This keeps single-use atomicity and makes evidence/provenance separation local to the daemon module.
- The existing generic retention interface delegates to the same private implementation with empty provenance; the dedicated Session retention interface is the only nonempty path and validates again for defense in depth.
- The preview execution and response mapping live behind one shared private implementation. The two external HTTP interfaces decode separate exact request shapes, then reuse identical Git analysis, caps, errors, retention capacity, and response behavior.
- The assessment module validates optional provenance at `Start` and chooses `Create` versus `CreateWithPiSession` only at its private history-write seam. No history detail is exposed to evaluator adapters or clients.
- Reviewed lookup has a fixed 20-ID daemon bound matching the history interface. It returns a dedicated ID-only response instead of widening generic records.
- The dedicated review query uses the existing 30-second repository-verification context and the same 16-KiB bounded body as preview. Current maximum candidate IDs fit comfortably while preserving a fixed request cap.
- Tests replace model adapters with deterministic in-memory fakes and assert through the daemon HTTP and evaluator interfaces. They do not test past the external seams or contact a provider.

## Tests / Verification

Passed:

- focused new/changed behavior: `CGO_ENABLED=0 go test -count=1 -run 'TestContinuationStore|TestPiSession|TestServiceHistoryStoresPiSession|TestServiceRejectsInvalidPiSession' ./internal/daemon ./internal/assessment`
- complete affected modules: `CGO_ENABLED=0 go test -count=1 ./internal/daemon ./internal/assessment`
- `CGO_ENABLED=0 go test -count=1 ./...`
- `go test -race -count=1 -tags netgo ./...`
- `go vet -tags netgo ./...`
- `go build -tags netgo -o /private/tmp/pi-learnloop-phase2-final.9kZ1CM/pi-learnloop ./cmd/pi-learnloop`
- `scripts/test-agent-infra.sh`
- `scripts/validate-agent-infra.sh`
- `git diff --check`

The final temporary binary was inspected and `/private/tmp/pi-learnloop-phase2-final.9kZ1CM` was removed. No verification contacted a provider, read a real Pi Session, wrote production history, or changed dependencies.

One initial affected-package run inside the restricted filesystem could not open the default Go build cache. Repeating with an explicit writable temporary cache compiled successfully. One complete daemon run inside the restricted network sandbox then failed only because it could not bind `127.0.0.1`; the same complete affected-module suite passed outside that restriction. These were environment restrictions, not test assertions or implementation failures.

TypeScript typecheck/tests and npm pack were not run because Phase 2 changes no extension, TypeScript, package, or publication file. They remain required for Phase 3. No business behavior outside the authorized Go daemon/assessment phase was exercised manually.

## Known Issues

- The Pi extension does not call either new route and `/learn` remains Git-only. The new routes are local daemon capabilities awaiting Phase 3.
- Pi 0.84.3 `SessionManager.list` still materializes message-derived values while scanning. Phase 3 must not begin until the user explicitly reviews that privacy/resource cost; if unacceptable, the plan and ADR require redesign from authoritative Pi capabilities.
- Session-to-Git association remains an explicit user assertion. The daemon never infers it and cannot prove authorship beyond the separately selected Git evidence.
- There is intentionally no uniqueness constraint for canonical repository plus Session ID; multiple complete explicit reviews remain valid.

## Remaining Work

- Phase 3 only: after the required privacy/resource go/no-go review and a new explicit high-risk authorization, add the manual current-cwd at-most-20 ID selection path and explicit Git binding in the existing Pi extension.

## Next Step

Review and commit the completed Phase 2 working tree if requested. Do not begin Phase 3 from a generic “continue” instruction. Require explicit confirmation that Pi 0.84.3's list-time full-file/message materialization is acceptable for the manual bounded workflow and explicit authorization for `explicit-pi-session-review` Phase 3.

## Do Not Change

- Do not add Session fields to existing strict preview, question, assessment, or generic history requests/responses.
- Do not place Session ID in `evidence.Result`, `evidence.Bundle`, evaluator values, prompts, RPC/model content, logs, errors, or generic history output.
- Do not infer Git changes from Session time, cwd, name, messages, summaries, tool calls, or filesystem activity.
- Do not add a dependency, index, hook, snapshot, marker, reminder, background worker, Session-file parser/write, extension-owned store, repair, downgrade, or destructive migration.
- Do not begin Phase 3 without its explicit privacy/resource decision and new phase authorization.
