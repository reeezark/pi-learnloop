---
id: durable-learning-history
status: draft
risk: high
current_phase: 1
phase_status: planned
updated: 2026-09-01
---

# Durable Learning History and Safe Recovery

## 1. Goal

Add a local, repository-scoped learning history that survives daemon restarts and records whether an answer-assessment completed, failed, or was interrupted, while preserving the existing rule that source, questions, answers, feedback, prompts, RPC streams, credentials, and raw model output are never persisted.

Recovery in this task means recovering committed history and converting unfinished source-free attempt markers to `interrupted`. It does not mean resuming an assessment or automatically repeating a model call whose provider outcome may be unknown.

## 2. Background

The completed `answer-assessment-workflow` plan proves the full learning interaction, but the result exists only in one HTTP response. The daemon removes the assessment entry after a complete turn, clears all volatile entries on shutdown, and has no history read operation. A restart, lost response, or process crash therefore leaves no local evidence that the user completed or attempted the learning loop.

Persistence is high risk because it creates a long-lived data and compatibility boundary, introduces the first third-party Go runtime dependency, changes startup behavior, and must not weaken the privacy and no-retry guarantees accepted in ADR-0003 and ADR-0004. ADR-0005 is proposed with this plan and must be accepted before implementation.

## 3. Current Behavior

Verified on 2026-09-01 from the current code and documentation:

- `evidence.Preview` returns a canonical repository root, resolved base/head revisions, bounded changed declarations, and source excerpts.
- `evidence.BuildBundle` and `evaluator.NewInput` intentionally omit the repository root before model evaluation. The runtime bundle has a stable manifest SHA-256 plus resolved revisions.
- `continuationStore` retains an owned `evidence.Result` for five minutes and atomically consumes it in `handleQuestionSet`.
- `handleQuestionSet` still has the server-verified canonical repository root, but calls `assessment.Service.Start` with only evaluator input, question set, and model selection. Repository provenance is therefore lost at the assessment boundary.
- `assessment.Service.Submit` atomically enters an evaluating state before a provider call, permits at most one F1, invalidates the entry after evaluator failure, and removes it after a complete result.
- The final `assessment.Result` contains only the validated turn and Go-derived label. No durable record or repository identity is returned.
- `handleAssessmentTurn` returns exactly `protocol_version`, `assessment_turn`, and, for a complete result, `label`.
- `DaemonEvidenceClient` validates that exact response shape. `/learn` renders feedback and then returns; no history command exists.
- `daemon.Run` owns only a volatile runtime directory containing a lock, descriptor, and Instance Token. It constructs no database or durable store.
- Production defaults to `os.UserConfigDir()/pi-learnloop/runtime`. The runtime path is protected against symlinks, wrong owners, and permissive modes, and runtime files are removed at shutdown.
- `go.mod` declares Go 1.21 and no third-party module. No `go.sum` exists.
- The default local Go 1.21.13 toolchain has a recorded macOS external-linker failure for network-enabled test binaries. The established `CGO_ENABLED=0` and `-tags netgo` verification paths pass, so a CGO-only SQLite driver would break the existing supported test/build path.
- README and ADR-0004 explicitly state that no learning record, source, answer, feedback, or model output is saved.

Authoritative SQLite documentation confirms that WAL mode adds `-wal` and `-shm` files, requires a local filesystem, persists across reopen, and treats the WAL as part of database state. SQLite documents `synchronous=FULL` as ACID in WAL mode and `foreign_keys` as a per-connection setting that applications must set explicitly. `PRAGMA user_version` is reserved for application-managed schema versioning.

## 4. Relevant Call Chain

Current terminal path:

```text
/learn
→ POST /v1/evidence-previews
→ evidence.Preview returns canonical repository root and bounded source
→ continuationStore.retain
→ POST /v1/question-sets
→ continuationStore.consume
→ evidence.BuildBundle → evaluator.NewInput
→ isolated question evaluator
→ assessment.Service.Start(input, questions, model)
→ POST /v1/assessment-turns
→ assessment.Service.Submit
→ isolated initial evaluator
→ optional isolated F1 evaluator
→ derive label → remove volatile entry
→ strict HTTP response → render once → result disappears
```

Proposed path:

```text
successful question generation
→ assessment.Start also receives server-owned, non-model-visible provenance
→ user submits initial answers
→ atomic volatile transition to evaluating_initial
→ history store creates one source-free running attempt before provider call
→ isolated evaluator call (never automatically retried)
→ optional F1 leaves the attempt running
→ complete: one transaction writes terminal label and Q1/Q2/Q3 verdicts
→ failed: one transaction writes a bounded safe failure code
→ final response reports whether history was saved

daemon startup
→ open and validate protected SQLite database
→ apply forward migrations transactionally
→ atomically convert leftover running attempts to interrupted
→ serve normal learning and authenticated history queries

/learn-history
→ authenticated repository-scoped query
→ newest source-free records only
→ thin extension rendering; no model call
```

## 5. Relevant Files

- `AGENTS.md`: high-risk task, phase authorization, dependency, testing, checkpoint, and commit rules.
- `PROJECT.md`: local-first architecture, planned SQLite history, privacy constraints, compatibility requirements, and recorded macOS Go toolchain limitation.
- `README.md`: current user-visible guarantee that nothing is saved.
- `plans/answer-assessment-workflow.md`: completed volatile predecessor and explicit persistence exclusion.
- `docs/checkpoints/answer-assessment-workflow-phase-3.md`: current handoff and next-priority guidance.
- `docs/decisions/ADR-0002-local-daemon-protocol-security.md`: protected local runtime and authenticated v1 protocol.
- `docs/decisions/ADR-0003-post-preview-evaluator-boundary.md`: exact evidence, isolated evaluator, credentials, and no-retry rules.
- `docs/decisions/ADR-0004-answer-assessment-lifecycle.md`: volatile state machine, sensitive-data prohibition, and deterministic labels.
- `internal/evidence/evidence.go`: canonical repository root and resolved selection provenance.
- `internal/evidence/bundle.go`: manifest identity and repository-root removal.
- `internal/evaluator/contract.go`: evaluator input and question-set schemas.
- `internal/evaluator/assessment_contract.go`: strict assessment turn, verdict, feedback, and label types.
- `internal/assessment/service.go`: volatile ownership, atomic states, expiry, failure invalidation, and terminal removal.
- `internal/daemon/continuation.go`: current bounded owned-copy pattern.
- `internal/daemon/server.go`: provenance loss, assessment HTTP response, strict authentication, and error mapping.
- `internal/daemon/daemon.go`: production composition and startup/shutdown lifecycle.
- `internal/daemon/runtime.go`: protected-directory/file validation and atomic runtime writes.
- `extensions/lib/daemon-client.ts`: exact assessment response validation and no-retry behavior.
- `extensions/lib/learn-command.ts`: manual interaction and final rendering.
- `agent/prompts/README.md` and `agent/prompts/assets.go`: released prompt identities and run-provenance requirement.
- `go.mod`: current Go 1.21, standard-library-only runtime baseline.

## 6. Scope

- Add one deep `internal/history` module backed by SQLite and `database/sql`.
- Add a protected production data directory separate from ephemeral runtime discovery files.
- Add embedded, ordered, forward-only migrations and schema compatibility checks.
- Persist a minimal repository identity, selection identity, evaluator provenance, safe attempt status, final label, and Q1/Q2/Q3 kinds/verdicts.
- Create a source-free running marker immediately before the first answer-assessment model call when storage is available.
- Convert unfinished markers to `interrupted` on the next daemon startup without replaying work.
- Preserve terminal completed or failed attempts across compatible daemon upgrades and restarts.
- Add an additive final-response history-save descriptor so a completed assessment is still returned when storage fails.
- Add one authenticated, bounded repository-history query and one manually triggered `/learn-history` command.
- Add deterministic unit, integration, permission, migration, corruption, interruption, protocol, and extension tests without a provider call.
- Update README, PROJECT, plan metadata, ADR status, and one checkpoint after each authorized phase.

## 7. Out of Scope

- Persisting source excerpts, file/declaration paths inside evidence, question text, user answers, F1 text/answer, feedback, prompt bodies, RPC frames, raw model output, credentials, tokens, or Session transcripts.
- Restoring a live assessment after restart or reconstructing evaluator input from history.
- Automatically retrying question generation, initial assessment, or F1 after timeout, connection loss, process exit, daemon crash, or an `interrupted` marker.
- A durable worker queue, leases, background job execution, SSE, polling orchestration, daemon autostart, or automatic reminders.
- Remote synchronization, mobile access, multi-user storage, cloud backup, telemetry, or analytics.
- Encryption at rest or OS keychain integration. The first boundary relies on a protected local directory and file ownership; encryption requires its own threat model and ADR.
- Automatic retention, pruning, deletion, export, or database repair. No destructive policy is inferred without a user-facing requirement.
- Schema downgrade, destructive migration, or automatically replacing a corrupt/newer database.
- Changing evidence limits, assessment labels, follow-up rules, evaluator prompts, Pi version support, or model-call semantics.
- Upgrading the Go language/toolchain baseline, package publication, CI/CD, or unrelated refactoring.

## 8. Proposed Changes

### 8.1 Select one CGO-free SQLite dependency

Use `modernc.org/sqlite v1.35.0` as the only direct runtime dependency in Phase 1, with its transitive module graph recorded by `go.mod`/`go.sum`. Its tagged `go.mod` declares Go 1.21 and pins `modernc.org/libc v1.61.13`; later inspected releases require newer Go baselines. The package is a `database/sql` driver and does not require CGO, preserving the repository's authoritative `CGO_ENABLED=0` verification path.

Do not add a migration framework, ORM, query builder, repository abstraction hierarchy, or alternate driver. SQL and migration ordering stay private to `internal/history`.

This version is a deliberate compatibility pin rather than the newest driver release. A later driver upgrade must review its required Go version, paired `modernc.org/libc`, generated SQLite version, licenses, and supported targets.

### 8.2 Separate durable data from ephemeral runtime state

Production data lives under:

```text
os.UserConfigDir()/pi-learnloop/data/
└── history.db
```

The existing sibling `runtime/` directory remains discoverable and ephemeral. The database is never published in `daemon.json` and never deleted during normal daemon cleanup.

The data directory must be a real, same-owner directory with mode `0700`. The database is pre-created as a regular same-owner file with mode `0600` before SQLite opens it. WAL auxiliary files remain inside the protected directory and are treated as inseparable database state. Tests use an explicit absolute temporary data directory; no environment variable or client-supplied database path is added.

The database must be local, not a network filesystem. The store owns one `database/sql` connection for the initial workload and verifies rather than assumes these settings:

```text
PRAGMA journal_mode = WAL
PRAGMA synchronous = FULL
PRAGMA foreign_keys = ON
PRAGMA trusted_schema = OFF
PRAGMA busy_timeout = 5000
```

`synchronous=FULL` favors committed-history durability over write throughput; the workload is small and human-paced.

### 8.3 Release an explicit schema version 1

Migration 1 creates three private tables. Names below are part of the proposed persisted format; implementation may adjust SQL syntax but not the data boundary without updating ADR-0005 before acceptance.

```text
repositories
  id                    integer primary key
  canonical_root        text unique not null
  created_at_unix_ms    integer not null

learning_attempts
  record_id                         text primary key
  repository_id                     integer not null → repositories.id
  started_at_unix_ms                integer not null
  finished_at_unix_ms               integer nullable
  status                            running | complete | failed | interrupted
  failure_code                      nullable bounded safe enum
  base_revision                     text not null
  head_revision                     text not null
  evidence_manifest_sha256          text not null
  question_schema_version           integer not null
  assessment_schema_version         integer not null
  question_prompt_id/version/hash   text not null
  assessment_prompt_id/version/hash text not null
  pi_version                        text not null
  provider                          text not null
  model_id                          text not null
  thinking_level                    text not null
  follow_up_used                    integer not null check 0/1
  label                             nullable understood | partial | review_needed

question_outcomes
  record_id              text not null → learning_attempts.record_id
  question_id            Q1 | Q2 | Q3
  question_kind          code_specific | go_backend
  verdict                demonstrated | partial | not_demonstrated
  primary key (record_id, question_id)
```

`record_id` is `lr1-` plus 32 random bytes in unpadded base64url. It is distinct from volatile `as1-` capabilities and is safe to expose to the authenticated local client.

The canonical repository root is persisted because the server needs an unambiguous local lookup key and already verifies/returns it during preview. It is sensitive local metadata, so it never enters model input or logs and is protected by the database directory. The history deliberately does not store evidence item paths, references, question/answer/feedback text, or model output.

Provider/model/thinking and prompt identifiers/hashes are non-secret provenance needed to interpret results across evaluator changes. Prompt bodies remain embedded immutable assets and are not copied into SQLite.

### 8.4 Use transactional, forward-only migrations

`PRAGMA user_version` is the schema-version authority. On open, the store:

1. validates the path and database header access;
2. establishes and verifies connection settings;
3. reads `user_version`;
4. applies each missing embedded migration in order under an immediate write transaction;
5. updates `user_version` in the same transaction;
6. runs a foreign-key check and a bounded quick integrity check before enabling history writes.

An empty database migrates to version 1. A supported older database migrates forward. A newer schema version, failed migration, failed setting, invalid owner/mode, symlink, or integrity failure disables the history capability without modifying or deleting the database. Preview and volatile learning remain available. There is no downgrade or automatic repair.

Every attempt-state update and its question outcomes are one transaction. A complete record never becomes visible with a partial set of verdicts.

### 8.5 Record safe lifecycle facts without making model calls retryable

The question handler must bind server-owned provenance to the volatile assessment when calling `Start`: canonical repository root, resolved revisions, manifest hash, schema versions, released prompt metadata, and fixed model selection. This provenance is retained in daemon memory but is not sent to the evaluator unless it already belongs to the accepted evaluator input.

After initial answers pass validation and the assessment atomically enters `evaluating_initial`, the daemon asks history to create one `running` record before starting the evaluator. The record contains no answers or source. If history is unavailable, the evaluator may still run under the existing volatile contract; the eventual result reports that it was not saved.

An F1 result keeps the durable marker `running` and sets `follow_up_used=1`. A complete result atomically sets `complete`, stores the Go-derived label and exactly three question outcomes, then returns the result. A known evaluator failure may set `failed` with only a stable safe failure code. Error strings and provider content are not stored.

At startup, every leftover `running` row is atomically changed to `interrupted`. This is evidence of an uncertain or abandoned attempt, not permission to resume it. The user must start a new visible `/learn` flow for any further provider call.

The integration must use one durable record ID for the whole initial/F1 lifecycle. A repeated terminal write with identical content may be treated as success; conflicting content for the same ID fails closed. This protects local bookkeeping from ambiguous commit results without repeating model evaluation.

### 8.6 Preserve the primary assessment result when history fails

A model result must not be hidden merely because the local database becomes full, busy, corrupt, or unavailable after the provider call. A complete assessment response additively reports one strict descriptor:

```json
{"history":{"saved":true,"record_id":"lr1-<43 base64url characters>"}}
```

or:

```json
{"history":{"saved":false,"reason":"storage_unavailable"}}
```

The validated assessment turn, feedback, and deterministic label are still returned. The extension renders a concise warning only for `saved:false`; it never retries the assessment or a history write. Follow-up responses need not expose the internal running record.

This field changes the currently exact response validator, so Phase 2 must update Go and TypeScript protocol tests together. The response remains under authenticated v1 and does not change the existing assessment request.

### 8.7 Add a bounded read-only history capability

Phase 3 adds strict authenticated `POST /v1/learning-history-queries` with:

```json
{"repository":"<current cwd>","limit":20}
```

The server canonicalizes and verifies the repository instead of trusting the path as a database key. `limit` is required, positive, and capped at 50. The response is newest-first and contains only the source-free record fields intended for display: record ID, timestamps, status/failure code, resolved revisions, label, follow-up flag, evaluator provenance, and three question kinds/verdicts. It never returns canonical roots for other repositories or any excluded text.

The extension registers `/learn-history` as a manual command. It performs no model call, computes no score, and renders the daemon result. An empty history is a normal result. No automatic reminder, startup notification, deletion command, or export is added.

## 9. Compatibility

- Existing evidence selection, preview, continuation, evaluator schemas, prompts, assessment request, verdicts, follow-up limit, label mapping, and no-retry behavior remain unchanged.
- ADR-0004 remains authoritative for volatile source-bearing state. ADR-0005 creates a narrow exception only for the explicitly listed source-free persisted fields.
- `history.db` schema version 1 becomes a compatibility commitment after Phase 1 ships. Fields cannot be repurposed; later changes require forward migrations and ADR review.
- The final assessment response gains an additive field, but the current client is exact and must be updated in the same Phase 2 commit. No already released package compatibility is assumed.
- `/learn-history` and its query route are new capabilities. `/learn` continues to reject arguments and continues to work when history is unavailable.
- A compatible newer daemon must preserve existing committed records. A daemon encountering a newer database must not downgrade or rewrite it.
- The Go module remains at language version 1.21. `modernc.org/sqlite v1.35.0` and its paired transitive graph become approved core dependencies only if the user explicitly accepts ADR-0005 and Phase 1.

## 10. Risks

- **Sensitive local paths:** canonical repository roots reveal directory names. Mitigation: a same-owner `0700` directory, `0600` database, no logs/model exposure, and repository-scoped queries only.
- **Accidental content persistence:** internal structs contain source, questions, answers, and feedback. Mitigation: construct a dedicated allowlisted history record; never JSON-serialize evaluator or assessment structs into a blob.
- **Duplicate paid calls:** crash recovery can be mistaken for retry authorization. Mitigation: startup only marks `running` as `interrupted`; no stored row can trigger an evaluator.
- **Ambiguous result/save boundary:** a provider call may succeed while the database commit fails. Mitigation: return the assessment plus explicit `saved:false`; never replay the model call.
- **Schema corruption or newer versions:** automatic repair could destroy history. Mitigation: disable only the history capability and preserve the database unchanged.
- **WAL handling:** copying only `history.db` while it is open can lose committed transactions. Mitigation: document that `-wal` is database state and add no raw-copy/export feature in this task.
- **Connection-specific pragmas:** pooled connections could omit foreign keys or durability settings. Mitigation: one connection initially and verified settings before enabling the store.
- **Dependency age:** v1.35.0 preserves Go 1.21 but is not the latest modernc release. Mitigation: exact pin, license/version record, focused integration tests, and a later explicit Go/driver upgrade plan.
- **Database growth:** no retention policy exists. Mitigation: records contain bounded metadata only; deletion/pruning remains a separate user-visible decision.
- **Protocol drift:** the TypeScript client currently rejects unknown root fields. Mitigation: Go and TS protocol changes/tests land together in Phase 2.

## 11. Implementation Phases

### Phase 1 — Protected SQLite foundation

Goal: implement and verify the storage module, schema version 1, and safe startup checks without connecting it to daemon assessment behavior.

Allowed files:

- `go.mod`, new `go.sum`;
- new `internal/history/` source, embedded migrations, and tests;
- narrowly shared protected-path helpers if investigation proves reuse safer than duplication;
- plan, ADR, PROJECT/README dependency facts, and the Phase 1 checkpoint.

Required work:

- add exactly `modernc.org/sqlite v1.35.0` and the resolved transitive graph;
- implement protected data-directory/database creation and validation;
- implement open/close, verified pragmas, schema migration, future-version refusal, integrity checks, source-free record validation, transactional create/terminal-update/query operations, and startup interruption marking;
- test permissions, symlinks, migrations, rollback, idempotent terminal writes, conflicts, corruption/newer schema, WAL reopen, interruption marking, and forbidden-content absence;
- do not construct the store from `daemon.Run` yet.

Forbidden in Phase 1:

- daemon, assessment, evaluator, extension, HTTP protocol, prompt, runtime behavior, model call, CI, or release changes;
- any dependency other than the approved SQLite graph;
- writing a production database during tests.

Phase 1 acceptance:

- the store passes with `CGO_ENABLED=0`;
- a committed record survives close/reopen and an uncommitted transaction does not;
- schema 1 contains only the approved allowlisted columns;
- unsupported/corrupt/protection failures do not delete or rewrite the database;
- one source-free running row becomes interrupted on recovery;
- no daemon behavior changes.

### Phase 2 — Assessment lifecycle recording

Goal: bind server-owned provenance, record safe running/terminal facts, and surface save status without changing evaluator semantics.

Allowed files:

- `internal/assessment/` and tests;
- `internal/daemon/` and tests;
- `agent/prompts/assets.go` only if needed to expose immutable prompt metadata without changing prompt content;
- `extensions/lib/daemon-client.ts`, `extensions/lib/learn-command.ts`, and focused extension tests for the additive save descriptor;
- README, PROJECT, plan, ADR status, and the Phase 2 checkpoint.

Required work:

- construct/open/close the store in daemon lifecycle with a test-only absolute data directory;
- preserve preview/question behavior when storage is unavailable;
- pass only server-owned provenance into assessment state;
- create one running record before the initial assessment call when possible;
- update it for F1, complete, and safe failures without storing excluded content;
- mark running records interrupted on restart;
- return and strictly validate/render `history.saved` without retrying evaluation;
- add cancellation, disk/storage failure, restart, response-loss, concurrency, and privacy tests.

Forbidden in Phase 2:

- history query route/UI, automatic reminder, retry/resume, durable worker, SSE, raw content persistence, new prompt, new dependency, or unrelated refactoring.

Phase 2 acceptance:

- a successful result is returned even when history cannot be saved;
- a saved complete attempt has exactly three verdict rows and no excluded content;
- crash/restart changes a running marker to interrupted and starts no evaluator;
- concurrent/replayed assessment submissions still start at most one evaluator;
- existing volatile flow works when history is disabled.

### Phase 3 — Repository history query and UI

Goal: let the user manually inspect bounded local history without a model call.

Allowed files:

- `internal/history/` query code/tests if not completed in Phase 1;
- `internal/daemon/server.go` and focused route/integration tests;
- `extensions/pi-learnloop.ts`, `extensions/lib/daemon-client.ts`, `extensions/lib/learn-command.ts`, and focused extension tests;
- README, PROJECT, plan, accepted ADR, and the Phase 3 checkpoint.

Required work:

- add the strict authenticated bounded repository query;
- add `/learn-history` registration, client validation, empty/error handling, and concise rendering;
- ensure no cross-repository results, source-bearing values, or model calls are possible;
- document database location, privacy boundary, backup caution for WAL, and recovery semantics.

Forbidden in Phase 3:

- deletion/export/pruning, reminders, dashboards, analytics, remote sync, retry/resume, SSE, dependency changes, or package publication.

Phase 3 acceptance:

- history is manually queryable per canonical repository and newest-first;
- the response cap and exact schema are enforced;
- empty, unavailable, corrupt, and newer-schema cases are safe and understandable;
- `/learn` behavior and model-call count remain unchanged;
- the completed phase is committed and pushed.

## 12. Acceptance Criteria

- ADR-0005 is accepted and the exact Phase 1 dependency graph is explicitly authorized before code changes.
- The database is local, protected, forward-migrated, and never placed in the ephemeral runtime directory.
- Committed source-free records survive daemon restarts and compatible schema upgrades.
- Startup classifies unfinished attempts as interrupted without invoking Pi or any provider.
- No source, question/answer/F1/feedback text, prompt body, RPC/model output, credential, token, or Session content exists in schema, database fixtures, logs, or responses.
- Complete records contain one repository, one selection/manifest identity, immutable evaluator provenance, one deterministic label, and exactly Q1/Q2/Q3 verdicts.
- Storage failure cannot suppress a successfully validated assessment result and cannot trigger an automatic retry.
- History queries are authenticated, repository-scoped, bounded, strict, and manually triggered.
- Existing preview, question, assessment, isolation, cancellation, and no-retry tests continue to pass.
- Every phase has a completed checkpoint, reviewed diff, dedicated commit, and successful push before the next phase begins.

## 13. Verification

Run after every documentation or implementation phase as applicable:

```bash
./scripts/test-agent-infra.sh
./scripts/validate-agent-infra.sh
git diff --check
git status --short
git diff --stat
git diff
```

Phase 1 and later Go verification:

```bash
CGO_ENABLED=0 go test -count=1 ./internal/history
CGO_ENABLED=0 go test -count=1 ./...
go test -race -count=1 -tags netgo ./...
go vet -tags netgo ./...
go build -tags netgo ./cmd/pi-learnloop
```

Phase 2 and Phase 3 extension verification:

```bash
npm run typecheck
npm test
npm pack --dry-run --json
```

Focused recovery/manual scenarios use temporary repositories and temporary data directories only:

- create → commit → close → reopen;
- interrupt a running marker → reopen → verify `interrupted` and zero evaluator calls;
- corrupt/future-version database → verify history unavailable and preview still operational;
- fail a terminal write after evaluator success → verify result plus `saved:false` and no retry;
- query repository A while records for A and B exist → return A only;
- inspect database schema and values for excluded content;
- inspect directory/database/WAL/SHM ownership and effective protection.

No automated verification may contact a provider.

## 14. Open Questions

- `TODO / Need Confirmation` — Accept ADR-0005's persisted field allowlist, including the canonical local repository root and non-secret model/prompt provenance.
- `TODO / Need Confirmation` — Explicitly authorize `modernc.org/sqlite v1.35.0` and the exact transitive module graph resolved from its Go 1.21-compatible `go.mod`. Phase 1 must not run `go get` or modify dependencies before this approval.
- `TODO / Need Confirmation` — Accept the proposed non-destructive policy: no automatic retention, deletion, export, or repair in this task. Those capabilities require a later plan based on an explicit user need.
