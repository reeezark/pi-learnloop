---
id: explicit-pi-session-review-phase-1
plan: explicit-pi-session-review
phase: 1
status: superseded
updated: 2026-09-02
---

# Explicit Pi Session Review Phase 1

## Context

### Goal

Release the accepted source-free history foundation for explicit Pi Session provenance: schema v2 with one nullable bounded ID, a dedicated Session-bound assessment start, and a completion-only repository/candidate lookup, without changing daemon routes, assessment propagation, evidence, evaluators, extensions, dependencies, or current Git-only behavior.

### Current Phase

Phase 1 is complete. The active `explicit-pi-session-review` plan now points to Phase 2 with `phase_status: awaiting_approval`. Phase 2 is not authorized.

The working baseline was design commit `31d74c9bd49978fce82e54cb207c95f164244fd2`, which accepted no implementation. The completed Phase 1 changes are currently in the working tree and have not been committed or pushed.

## Completed

- Accepted ADR-0006 and recorded the user's explicit Phase 1 authorization before implementation.
- Added embedded ordered migration `002_pi_session_provenance.sql` and advanced the current schema authority from v1 to v2.
- Made opening an empty store apply migrations 1 and 2 in order, and made opening v1 verify the exact existing schema before transactionally applying v2.
- Added only nullable `learning_attempts.pi_session_id`, constrained to 1–128 ASCII bytes, ASCII alphanumeric first and last characters, and internal ASCII alphanumerics plus `.`, `_`, and `-`.
- Preserved every v1 value and relationship during migration; every old row receives SQL `NULL`. Migration failure rolls back both the added column and `PRAGMA user_version`.
- Kept the existing `Create` path Git-only and SQL-NULL. Added `CreateWithPiSession` as the narrow Session-bound history start and `ValidPiSessionID` as the shared bounded identity contract.
- Kept generic `Start`, `Record`, `List`, and their consumers Session-free.
- Added `ReviewedPiSessionIDs`, accepting 1–20 unique validated candidates and returning only IDs with at least one `complete` attempt in the same canonical repository, in candidate order. `running`, `failed`, `interrupted`, SQL-NULL, and other-repository rows do not match.
- Added full opening preflight for every stored non-NULL ID before connection configuration or running-record recovery. Externally bypassed constraints fail closed without rewriting or echoing the invalid value.
- Preserved protected-path checks, exact-schema verification, WAL/FULL/foreign-key/trusted-schema/busy-timeout settings, integrity checks, idempotent terminal writes, running-to-interrupted recovery, future-schema rejection, and no-repair/no-downgrade behavior.
- Updated stable README and PROJECT facts to schema v2 while stating that current Git-only daemon behavior still stores NULL and Session routes/UI remain unimplemented.
- Added no dependency, route, protocol field, database index, evidence/model field, Session metadata/content field, background work, hook, snapshot, marker, extension storage, or Session-file access/write.

## Modified Files

History implementation and migration:

- `internal/history/migrations.go`
- `internal/history/migrations/002_pi_session_provenance.sql`
- `internal/history/records.go`
- `internal/history/store.go`
- `internal/history/validation.go`

Focused history verification:

- `internal/history/migration_test.go`
- `internal/history/pi_session_test.go`
- `internal/history/safety_test.go`
- `internal/history/store_internal_test.go`
- `internal/history/validation_test.go`

Stable and lifecycle documentation:

- `README.md`
- `PROJECT.md`
- `plans/explicit-pi-session-review.md`
- `docs/decisions/ADR-0006-explicit-pi-session-provenance.md`
- `docs/checkpoints/explicit-pi-session-review-phase-1.md`

## Important Decisions

- The history module exposes two narrow start paths instead of widening generic `Start`: `Create` always stores NULL, while `CreateWithPiSession` requires one valid non-empty ID. This preserves every current caller and generic response by construction.
- Migration ordering and exact-schema verification are private to history. A versioned store is verified against the schema produced by the embedded ordered migrations before any later migration is applied.
- The completion lookup returns only a candidate-order ID subset rather than generic records, timestamps, statuses, changesets, labels, evaluator provenance, or repository metadata.
- The 20-candidate lookup is deliberately unindexed at this human-scale bound; adding a speculative persisted index was not required for Phase 1.
- ID validation is byte-oriented and excludes Unicode, control bytes, separators, leading/trailing punctuation, and values over 128 bytes. SQL constraints repeat the boundary, and open-time validation defends against externally bypassed constraints.
- The pre-Phase-1 schema-v1 implementation at `31d74c9` was executed from a temporary repository export and passed its existing future-schema safety test against version 2, directly confirming ADR-0005 fail-closed compatibility. The export was removed afterward.

## Tests / Verification

Passed:

- `CGO_ENABLED=0 go test -count=1 ./internal/history/...`
- `CGO_ENABLED=0 go test -count=1 ./...`
- `go test -race -count=1 -tags netgo ./...`
- `go vet -tags netgo ./...`
- `go build -tags netgo -o /private/tmp/pi-learnloop-phase1.SmN50J/pi-learnloop ./cmd/pi-learnloop`
- schema-v1 compatibility export at commit `31d74c9bd49978fce82e54cb207c95f164244fd2`: `CGO_ENABLED=0 go test -count=1 ./internal/history/...`
- `scripts/test-agent-infra.sh`
- `scripts/validate-agent-infra.sh`
- `git diff --check`

The temporary build and compatibility-export directories were inspected and removed. No verification contacted a provider, used a real Pi Session, wrote a production history database, or changed dependencies.

One earlier full-suite attempt reported `TestNewPiRPCAssessmentEvaluator` unavailable after two full suites were inadvertently running concurrently and the fixed two-second fake-Pi version preflight timed out. The exact focused test then passed in 1.09 seconds, and a clean non-concurrent full-suite rerun passed all packages. No implementation change was made in response.

TypeScript typecheck/tests and npm pack were not run because Phase 1 changed no extension, TypeScript, package, or publication file. They remain required in the applicable later phase.

## Known Issues

- No Session-aware daemon route, preview continuation, assessment provenance propagation, or extension selection exists. Current `/learn` remains Git-only and writes SQL NULL.
- The completion lookup is an internal history capability only; storage-unavailable protocol behavior remains Phase 2 work.
- Pi 0.84.3 `SessionManager.list` still materializes message-derived values while scanning. The accepted design requires a separate explicit resource/privacy decision before Phase 3; Phase 1 does not call it.
- There is no uniqueness constraint: multiple complete reviews for the same canonical repository and Session ID remain intentionally valid.

## Remaining Work

- Phase 2: add the two independent strict authenticated routes and server-owned provenance propagation while proving Session ID absence from evidence, evaluator, prompt, RPC, logs, errors, and generic history.
- Phase 3: only after its explicit privacy/resource review and authorization, add manual bounded current-cwd Session selection and explicit Git binding in the extension.

## Next Step

Review and commit the completed Phase 1 working tree if requested. Do not begin Phase 2 until the user explicitly authorizes `explicit-pi-session-review` Phase 2.

## Do Not Change

- Do not add Session fields to existing strict preview, question, assessment, or generic history requests/responses.
- Do not place the Session ID in `evidence.Result`, `evidence.Bundle`, evaluator values, prompts, RPC, model content, logs, errors, or generic history output.
- Do not infer Git changes from Session time, cwd, name, messages, summaries, tool calls, or filesystem activity.
- Do not add a dependency, index, hook, snapshot, marker, reminder, background worker, Session-file parser/write, extension-owned store, repair, downgrade, or destructive migration.
- Do not start Phase 2 without a new explicit high-risk authorization.
