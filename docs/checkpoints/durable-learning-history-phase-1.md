---
id: durable-learning-history-phase-1
plan: durable-learning-history
phase: 1
status: current
updated: 2026-09-01
---

# Durable Learning History Phase 1

## Goal

Implement and verify the protected SQLite history foundation accepted in ADR-0005 without connecting it to daemon assessment behavior.

## Current Phase

Phase 1 is complete. The active plan now points to Phase 2 with `phase_status: awaiting_approval`; Phase 2 remains blocked on a new explicit high-risk authorization.

Repository snapshot before this phase:

- checkout: `/Users/bytedance/workspace/pi-learnloop`
- branch: `main`, tracking `origin/main`
- Phase 1 baseline: `fdd2234`
- Phase 1 commit: the commit containing this checkpoint

## Completed

- Accepted ADR-0005 and recorded the explicit approval for `modernc.org/sqlite v1.35.0` plus its resolved transitive graph.
- Added one deep `internal/history` module with a narrow public lifecycle: `Open`, `Create`, `MarkFollowUp`, `Complete`, `Fail`, `List`, and `Close`.
- Added real same-owner `0700` data-directory and `0600` single-link regular-database checks, final-component symlink rejection, and local-filesystem checks. Existing unsafe paths fail closed without chmod or replacement.
- Added one owned SQLite connection and verified WAL, `synchronous=FULL`, foreign keys, `trusted_schema=OFF`, and a 5-second busy timeout.
- Added the embedded, immediate-transaction schema-v1 migration for repositories, learning attempts, and Q1/Q2/Q3 outcomes. `PRAGMA user_version` is authoritative and the exact migration schema is checked before enabling writes.
- Added read-only preflight for integrity, foreign keys, exact schema objects, repository values, and every stored attempt/outcome. Future, corrupt, unexpected, or invalid databases return typed errors before WAL or recovery writes and are never repaired, downgraded, deleted, or overwritten.
- Added cryptographic `lr1-` IDs, bounded source-free provenance validation, safe failure/status/label/verdict enums, immediate create/complete transactions, idempotent exact terminal writes, conflict detection, and bounded repository-scoped newest-first queries.
- Added startup conversion of all valid `running` rows to `interrupted` without any evaluator, Pi process, provider call, retry, or resume behavior.
- Preserved current production behavior: no daemon, assessment, evaluator, extension, HTTP protocol, prompt, runtime, model-call, CI/CD, or release code changed, and the daemon does not construct the store.

## Modified Files

Dependency manifest:

- `go.mod`
- `go.sum`

New production storage module:

- `internal/history/errors.go`
- `internal/history/migrations.go`
- `internal/history/migrations/001_initial.sql`
- `internal/history/path.go`
- `internal/history/path_darwin.go`
- `internal/history/path_linux.go`
- `internal/history/store.go`
- `internal/history/records.go`
- `internal/history/types.go`
- `internal/history/validation.go`

New automated verification:

- `internal/history/store_test.go`
- `internal/history/store_internal_test.go`
- `internal/history/safety_test.go`
- `internal/history/validation_test.go`

Stable and lifecycle documentation:

- `README.md`
- `PROJECT.md`
- `plans/durable-learning-history.md`
- `docs/decisions/ADR-0005-local-learning-history.md`
- `docs/checkpoints/durable-learning-history-phase-1.md`

## Important Decisions

- The store is a local-substitutable deep module, not a port plus adapter hierarchy. Tests use real temporary SQLite databases; no ORM, migration framework, mock database, or speculative storage interface was added.
- Phase 1 accepts an explicit absolute data directory. The ADR-accepted production path under `os.UserConfigDir()` remains unwired until Phase 2.
- Existing database compatibility is strict: schema version 1 must exactly match the embedded migration and all persisted values must pass the same source-free contract used for new writes.
- Rejected existing databases are inspected read-only before persistent SQLite settings or recovery. An empty new database is migrated before switching to WAL so migration failure can roll back without partially changing schema state.
- Recovery records interruption only. It cannot resume an assessment, invoke Pi, contact a provider, or infer a completed result.
- The only persisted repository-sensitive value is the accepted canonical root. Source excerpts, changed paths, questions, answers, F1 content, feedback, prompt bodies, RPC/model output, credentials, tokens, executable paths, and Session transcripts have neither API fields nor schema columns.

## Tests / Verification

Passed without a live provider call or production database:

- `CGO_ENABLED=0 go test -count=1 ./internal/history`
- `CGO_ENABLED=0 go test -count=1 ./...`
- `go test -race -count=1 -tags netgo ./...`
- `go vet -tags netgo ./...`
- `go build -tags netgo ./cmd/pi-learnloop`
- `go mod verify`
- `scripts/test-agent-infra.sh`
- `scripts/validate-agent-infra.sh`
- `git diff --check`

Focused history tests cover protected creation and reopen, exact schema and privacy fields, settings, future/corrupt/unexpected databases, path permissions and symlink/hard-link rejection, invalid stored-value preservation, migration and rollback behavior, record validation, repository isolation and limits, WAL persistence, terminal idempotency/conflicts, failure records, and running-to-interrupted recovery.

The build verification created a root-level untracked binary and it was immediately removed after its type and exact path were confirmed. No repository file or user data was deleted.

## Known Issues

- The foreground daemon does not open or close `internal/history`, so current `/learn` attempts remain volatile and no production history database is created.
- Prompt metadata accessors and server-owned assessment provenance are not yet wired; those are Phase 2 concerns.
- There is no authenticated history route, `/learn-history` UI, retention, deletion, export, pruning, repair, downgrade, resume, retry, remote sync, or automatic reminder.
- The initial supported product platform remains macOS. A conservative Linux filesystem implementation compiles with the package but does not expand the product compatibility promise.

## Remaining Work

- Phase 2: bind server-owned provenance to assessment state, construct the store in daemon lifecycle, write running/follow-up/terminal facts without affecting successful assessment results, and add crash/storage-failure/privacy integration tests.
- Phase 3: add the strict authenticated bounded repository history query and manual thin-client UI.
- Any later retention, deletion, export, repair, reminder, retry/resume, or analytics behavior requires a separate approved plan or ADR as applicable.

## Next Step

Stop after committing and pushing Phase 1. A later Agent must restore context from `AGENTS.md`, `PROJECT.md`, ADR-0002 through ADR-0005, `plans/durable-learning-history.md`, this checkpoint, Git status, and the Phase 1 commit. Do not begin Phase 2 until the user explicitly authorizes `durable-learning-history Phase 2`.

## Do Not Change

- Do not connect the store to daemon, assessment, evaluator, extension, HTTP, prompt, or production runtime code without Phase 2 authorization.
- Do not persist source, changed paths, questions, answers, F1 content, feedback, prompt bodies, RPC/model output, credentials, tokens, executable paths, or Session transcripts.
- Do not add repair, downgrade, retention, deletion, export, sync, resume, retry, background work, or automatic reminders.
- Do not weaken same-owner/mode/symlink/hard-link/local-filesystem checks, exact schema/value preflight, WAL/FULL/foreign-key/trusted-schema settings, typed failure behavior, or repository query isolation.
- Do not upgrade SQLite, add an ORM/migration framework, or change schema/version/enums without a separately reviewed compatibility decision.
