---
id: durable-learning-history-phase-3
plan: durable-learning-history
phase: 3
status: current
updated: 2026-09-02
---

# Durable Learning History Phase 3

## Goal

Expose the accepted schema-v1 history through one authenticated, bounded, repository-scoped read route and a manually triggered `/learn-history` command, without starting a model call or adding destructive history behavior.

## Current Phase

Phase 3 and the `durable-learning-history` plan are complete. The working baseline was Phase 2 commit `8ced24009457677c34fa947071837cb40103f9a8`; the Phase 3 commit is the commit containing this checkpoint.

## Completed

- Exposed one narrow `evidence.ResolveRepositoryRoot` interface that reuses the existing Git `--show-toplevel` verification and canonicalization path for both preview and history lookup.
- Added strict authenticated `POST /v1/learning-history-queries` with a 4-KiB request limit, required absolute repository path, required positive limit, and maximum limit 50.
- Passed the already-open degradable `history.Store` into the daemon handler; no new persistence seam, database adapter, dependency, or schema change was added.
- Canonicalized the requested Git working tree before capability lookup, queried only the matching canonical root newest-first, and returned an explicit `history_unavailable` error when the protected store was unavailable.
- Added an exact source-free response containing record ID, timestamps, lifecycle status/safe failure code, revisions and manifest identity, schema/prompt/model provenance, follow-up use, optional label, and Q1/Q2/Q3 kinds/verdicts. The stored canonical root is never returned.
- Added `DaemonEvidenceClient.history`, which performs one non-retryable authenticated request and validates the exact response, state-dependent nullability, ordering, record IDs, bounds, versions, hashes, provenance, and complete Q1/Q2/Q3 outcome shape.
- Added a manual `/learn-history` command that accepts no arguments, requires the trusted interactive project, requests 20 newest records for the current working directory, performs no confirmation or model call, renders concise source-free history, treats empty history as normal, and explains unavailable storage without repair.
- Updated README and PROJECT with the implemented route/command, privacy boundary, database location, interruption semantics, and SQLite WAL backup caution.
- Did not add deletion, export, pruning, reminders, dashboards, analytics, remote sync, retry/resume, SSE, dependencies, package publication, prompt changes, or evaluator behavior.

## Modified Files

Go repository verification, daemon protocol/composition, and tests:

- `internal/evidence/evidence.go`
- `internal/evidence/evidence_test.go`
- `internal/daemon/server.go`
- `internal/daemon/daemon.go`
- `internal/daemon/daemon_test.go`
- `internal/daemon/history_query_test.go`
- `internal/daemon/history_query_integration_test.go`

Thin extension client/UI and tests:

- `extensions/lib/daemon-client.ts`
- `extensions/lib/learn-command.ts`
- `extensions/pi-learnloop.ts`
- `tests/extension/daemon-client.test.ts`
- `tests/extension/learn-command.test.ts`
- `tests/extension/extension-entry.test.ts`

Stable and lifecycle documentation:

- `README.md`
- `PROJECT.md`
- `plans/durable-learning-history.md`
- `docs/checkpoints/durable-learning-history-phase-2.md`
- `docs/checkpoints/durable-learning-history-phase-3.md`

## Important Decisions

- Repository identity is verified before querying history by reusing the evidence module's real Git seam. The daemon does not duplicate `git` invocation logic, accept a client-provided database key, or route history through source-bearing assessment state.
- The production handler receives the concrete optional history store already owned by `daemon.Run`. Tests use real temporary protected SQLite databases; no hypothetical storage interface or mock database was introduced.
- The route accepts up to 50 records as fixed by ADR-0005. The manual UI intentionally requests 20 and exposes no argument or cross-repository browser.
- Response nullability follows lifecycle invariants: `running` has no terminal fields, `complete` has a label and exactly three outcomes, `failed` has only a safe failure code, and `interrupted` has no label/failure code/outcomes.
- The response deliberately omits the canonical repository root even for the requested repository. The TypeScript client rejects unknown fields, including an accidentally added `canonical_root`.
- History discovery and query are attempted once. Unlike preview discovery, history has no startup-race retry, and it can never start an evaluator.

## Tests / Verification

Focused red-green tests passed during implementation for:

- canonical nested Git-root resolution and empty-path rejection;
- repository isolation and exact source-free history response;
- production store composition with zero evaluator calls;
- invalid/non-repository requests, strict JSON, method/auth/media/body limits, limit 50, empty history, and unavailable storage;
- one authenticated TypeScript request, exact response validation, rejection of extra canonical-root metadata, and no retry on `history_unavailable`;
- manual rendering, empty history, unavailable-history messaging, and registration of only `/learn` plus `/learn-history`.

Final verification passed:

- `CGO_ENABLED=0 go test -count=1 ./...`
- `go test -race -count=1 -tags netgo ./...`
- `go vet -tags netgo ./...`
- `go build -tags netgo -o /tmp/pi-learnloop-phase3 ./cmd/pi-learnloop`
- `npm run typecheck`
- `npm test` (33 tests)
- `npm pack --dry-run --json`
- `scripts/test-agent-infra.sh`
- `scripts/validate-agent-infra.sh`
- `git diff --check`

No verification contacts a provider or creates a production history database.

## Known Issues

- There is no retention, deletion, pruning, export, backup command, repair, downgrade, dashboard, analytics, remote sync, reminder, retry/resume, durable worker, or SSE stream.
- Source-bearing live assessments still cannot resume after restart. A leftover `running` marker becomes `interrupted`, and the user must explicitly start a new `/learn` flow.
- A raw manual backup must account for `history.db-wal` and `history.db-shm`; copying only `history.db` while the daemon is running may not preserve all committed state.

## Remaining Work

No work remains in `durable-learning-history`. Any next product capability must begin with repository investigation and a new approved plan. Candidate target areas already recorded in PROJECT include Session-to-changeset selection, richer Go type/dependency evidence, SSE, and durable worker coordination; none is authorized by this checkpoint.

## Next Step

Commit and push this completed phase. A later Agent should restore context from `AGENTS.md`, `PROJECT.md`, ADR-0002 through ADR-0005, this checkpoint, and Git status before proposing the next active plan.

## Do Not Change

- Do not persist or return source, changed paths, questions, answers, F1 content, feedback, prompt bodies, RPC/model output, credentials, tokens, executable paths, Session transcripts, or another repository's canonical root/records.
- Do not resume or retry interrupted/failed assessments or history queries, infer provider completion, or add a durable job queue without a separate approved plan.
- Do not weaken exact HTTP/client validation, authentication, canonical Git-root verification, query cap, schema/value preflight, same-owner/mode/symlink/hard-link/local-filesystem checks, WAL/FULL/foreign-key/trusted-schema settings, or no-repair/no-downgrade behavior.
- Do not change schema version 1, `lr1-` IDs, persisted enums, prompt bodies, evaluator semantics, dependencies, Go baseline, CI/CD, release configuration, deletion/export, or retention behavior without separately reviewed authorization.
