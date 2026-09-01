---
id: ADR-0005
status: proposed
date: 2026-09-01
supersedes: none
---

# ADR-0005: Local Learning History and Safe Interruption Recovery

## Context

Pi LearnLoop now completes a manual evidence-backed learning loop, but the outcome is volatile. The daemon retains source-bearing assessment state only in memory, removes a completed entry before returning its final response, and clears all entries on shutdown. The extension renders that response once. There is no database, durable record, history route, or recovery behavior.

The existing architecture imposes strict constraints:

- ADR-0002 limits the daemon to authenticated IPv4 loopback and protects ephemeral discovery/token files under a same-owner runtime directory.
- ADR-0003 forbids product retries after an evaluator call may have reached the provider and keeps credentials, source, prompts, RPC streams, and model output out of logs and persistence.
- ADR-0004 keeps source, questions, answers, F1 content, feedback, and assessment state volatile. It explicitly requires a later storage ADR to define an allowlisted schema instead of serializing runtime structs.
- The server verifies a canonical repository root during preview, but `evaluator.Input` deliberately omits it before model evaluation. The current assessment boundary therefore cannot produce a repository-scoped durable record without new server-owned provenance.
- The module uses Go 1.21 and has no third-party Go dependency. The supported local verification path includes `CGO_ENABLED=0` because the default macOS Go 1.21.13 external linker has a recorded `missing LC_UUID` failure for network-enabled binaries.
- A lost HTTP response or daemon crash must not be treated as proof that a paid evaluator call did not happen.

SQLite's official documentation states that WAL mode is persistent, uses associated `-wal` and `-shm` files, requires all users to be on the same host, and relies on checkpointing. It states that `synchronous=FULL` in WAL mode adds a sync after each transaction commit and provides ACID durability, while `NORMAL` may lose the latest transaction after power loss. SQLite also documents that applications must explicitly set per-connection foreign-key enforcement and may use `PRAGMA user_version` for their own schema version.

The inspected Go driver choices have material trade-offs:

- `github.com/mattn/go-sqlite3` is mature but requires CGO and a C compiler, conflicting with the repository's established pure-Go verification path.
- current `modernc.org/sqlite` is CGO-free but its newest releases require newer Go versions; tagged `v1.35.0` declares Go 1.21 and pins its paired `modernc.org/libc` dependency.
- inspected `github.com/ncruces/go-sqlite3` Go-1.21 tags include a Go 1.23 toolchain directive and a broader runtime graph, while newer tags require newer Go versions.
- upgrading the repository's Go baseline only to obtain a newer SQLite driver would combine two high-risk compatibility changes.

## Decision

This decision remains proposed. Acceptance authorizes the architecture and persisted schema boundary, but implementation still requires the explicit phase gates in `plans/durable-learning-history.md`.

### 1. Persist only allowlisted, source-free learning facts

Pi LearnLoop will persist one repository-scoped record for an answer-assessment attempt when storage is available. Schema version 1 may contain only:

- a random durable record ID;
- the canonical local repository root;
- start/finish timestamps;
- `running`, `complete`, `failed`, or `interrupted` status and an optional bounded safe failure code;
- resolved base/head revisions and the evidence manifest SHA-256;
- question-set and assessment-turn schema versions;
- released question/assessment prompt identifiers, versions, and SHA-256 hashes;
- non-secret Pi version, provider, model ID, and thinking level;
- whether F1 was used;
- the Go-derived final label;
- Q1/Q2/Q3 IDs, kinds, and verdicts.

The schema must not contain source excerpts, changed file/declaration paths, evidence-reference details, question text, user answers, F1 text or answer, feedback, prompt bodies, RPC frames, raw model output, credentials, Instance Tokens, executable paths, or Session transcripts. Implementation must construct a dedicated validated history value; it must never serialize `evidence.Result`, `evaluator.Input`, `QuestionSet`, `AssessmentInput`, `AssessmentTurn`, or an internal assessment entry as a blob.

The canonical repository root is accepted as sensitive local metadata necessary for exact repository lookup. It remains server-verified, never becomes model input, is never returned for another repository, and is protected by the local database directory.

### 2. Use one protected local SQLite database

Production stores `history.db` in the durable sibling directory:

```text
os.UserConfigDir()/pi-learnloop/data/history.db
```

The data directory is a real same-owner `0700` directory. The database is pre-created and validated as a real same-owner `0600` file. It is separate from `runtime/`, is not published in the daemon descriptor, and is not deleted by runtime cleanup. WAL and shared-memory files are part of the database state and remain inside the protected directory.

The database must be on a local filesystem. The initial store uses one `database/sql` connection and verifies:

```text
journal_mode = WAL
synchronous = FULL
foreign_keys = ON
trusted_schema = OFF
busy_timeout = 5000 milliseconds
```

`FULL` is selected because the workload is small and the purpose of the database is to preserve committed learning history across crashes, not to maximize write throughput.

### 3. Use explicit schema version 1 and forward-only migrations

`PRAGMA user_version` is the persisted schema authority. Embedded migrations run in order under an immediate write transaction, and the schema version changes in the same transaction as its DDL/data migration.

Schema version 1 uses normalized `repositories`, `learning_attempts`, and `question_outcomes` tables with constraints for status, label, question IDs/kinds, verdicts, F1 use, foreign keys, and exactly one outcome per attempt/question ID. A terminal complete update and its three outcomes commit atomically.

Opening an empty or supported older database may migrate it forward. A newer schema, failed migration, failed required pragma, wrong owner/mode, symlink, or failed bounded integrity/foreign-key check disables the history capability without changing, deleting, renaming, or automatically repairing the database. The volatile preview and learning loop remain available.

No downgrade, destructive migration, automatic pruning, or retention policy is introduced by this ADR.

### 4. Recovery records interruption; it never resumes or retries evaluation

After a valid initial answer submission atomically leaves `awaiting_answers`, history may create one source-free `running` marker before the isolated assessment evaluator starts. The same durable record ID covers the optional F1 and terminal result.

A complete result atomically records the deterministic label and Q1/Q2/Q3 verdicts. A known evaluator failure may record `failed` with only a safe stable code. If a daemon later opens the database and finds `running` rows, it atomically changes them to `interrupted`.

An `interrupted` row is not a durable job and carries no resumable evaluator input. Startup recovery never starts Pi, contacts a provider, rebuilds source, or changes an assessment back to an awaiting state. The user must manually start a new `/learn` flow.

This rule applies even when the daemon cannot know whether an earlier provider call completed. It preserves visibility without creating duplicate cost or inconsistent results.

### 5. Storage failure cannot hide a successful assessment

History is a degradable capability. If the database cannot open, migrate, validate, create a running marker, or commit a terminal result, evidence preview and the volatile learning loop remain available.

If answer evaluation succeeds but the terminal history write does not, the daemon still returns the validated feedback and deterministic label. The complete response additively includes exactly one of:

```json
{"history":{"saved":true,"record_id":"lr1-<43 base64url characters>"}}
```

```json
{"history":{"saved":false,"reason":"storage_unavailable"}}
```

The extension may warn that history was not saved. It must not retry the assessment or submit a second model call. Repeating an identical terminal database update for the same durable ID may be idempotent; conflicting content for that ID fails closed.

### 6. History access is authenticated, bounded, repository-scoped, and manual

The daemon may add strict authenticated `POST /v1/learning-history-queries`. Its request contains the current repository path and a required positive limit capped at 50. The daemon canonicalizes and verifies the repository, then returns newest-first source-free records for that repository only.

The Pi package may add `/learn-history` as a manually triggered read command. It performs no model call and no score computation. Empty history is normal. This ADR does not authorize automatic reminders, startup notifications, deletion, export, dashboards, analytics, or cross-repository browsing.

### 7. Pin the Go-1.21-compatible CGO-free driver

Phase 1 will use exactly `modernc.org/sqlite v1.35.0` as the direct SQLite dependency and accept only the transitive graph resolved from that tag, including its paired `modernc.org/libc v1.61.13`. `go.sum` becomes part of the reviewed dependency state.

No ORM or migration framework is added. SQLite SQL, migrations, connection settings, and driver details remain private to `internal/history`.

The Go language baseline remains 1.21. The driver pin is not interpreted as permission for unrelated dependencies or upgrades. Moving to a later SQLite driver that requires Go 1.23/1.24/1.25 or replacing the driver requires a separate compatibility and dependency review.

## Alternatives

### Persist the full volatile assessment and resume after restart

Rejected. Resumption requires source, questions, answers, follow-up state, prompt/model context, and exact evaluator semantics to be durable. That materially expands the privacy threat surface and still cannot safely determine whether an interrupted provider call should be repeated.

### Persist only completed results

Credible and simpler, but not selected. A source-free running marker allows the product to distinguish “never attempted” from “started but outcome uncertain” after a crash without persisting answers or authorizing retries. The additional state is intentionally limited to four statuses.

### Automatically retry interrupted evaluator calls

Rejected. A timeout, connection loss, or process crash does not prove the provider did not receive the original call. Automatic replay may duplicate cost and return inconsistent feedback.

### Store JSON files or append-only JSONL

Rejected. Atomic multi-row outcomes, schema constraints, indexed repository queries, forward migrations, crash recovery, and concurrent read/write behavior would have to be rebuilt around a custom file format.

### Put `history.db` in the runtime directory

Rejected. Runtime descriptor/token files are ephemeral and intentionally removed or replaced across instances. Durable history needs a distinct lifecycle and must never be removed by instance cleanup.

### Use `github.com/mattn/go-sqlite3`

Rejected for the current baseline because it requires CGO and a C compiler. That would invalidate the repository's established `CGO_ENABLED=0` build/test path and worsen the recorded local external-linker limitation.

### Upgrade Go and use the newest `modernc.org/sqlite`

Deferred. A Go baseline change affects every user, test, build, and future release. It should not be bundled into the first persisted schema. The selected compatible pin carries an acknowledged maintenance cost until a separate toolchain/driver upgrade is approved.

### Use a current `github.com/ncruces/go-sqlite3` release

Deferred. It is CGO-free, but inspected releases either add a newer Go toolchain requirement or require a newer Go language baseline, and the runtime model differs. It offers no required capability that justifies combining that migration with schema version 1.

### Fail the entire daemon when history is unavailable

Rejected. Preview and volatile learning are already useful and do not depend on persistence. A corrupt or future database must not prevent the user from inspecting code or completing an explicitly initiated learning flow.

### Silently ignore history write failures

Rejected. The user would reasonably believe a durable result exists. The additive save descriptor preserves the assessment result while making persistence failure visible.

## Consequences

- Pi LearnLoop gains a stable local data format and a manually inspectable repository learning history.
- Committed records survive restart, and leftover running markers become visible interrupted outcomes without any automatic model call.
- Live assessments remain non-resumable; a crash can still lose feedback and requires a new explicit `/learn` flow.
- The database contains sensitive repository paths and model-selection metadata, though not source or user/model text. Local account and filesystem security remain part of the threat model.
- WAL creates auxiliary files that must be kept with the database during any future backup/export work.
- A storage failure degrades history rather than the learning loop, and the client must understand the new save-status field.
- Schema version 1, statuses, persisted label/verdict values, record ID format, and history response become compatibility-sensitive after implementation.
- `modernc.org/sqlite v1.35.0` introduces the first third-party Go runtime dependency and a transitive graph. It preserves Go 1.21 and CGO-free verification but is older than current driver releases.
- The database grows until a separately approved retention/deletion design exists; records are bounded metadata rather than source-bearing blobs.
- Accepting this ADR does not authorize implementation. The user must separately authorize each phase, beginning with the exact Phase 1 dependency change.
