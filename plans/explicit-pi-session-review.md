---
id: explicit-pi-session-review
status: active
risk: high
current_phase: 2
phase_status: awaiting_approval
updated: 2026-09-02
---

# Explicit Pi Session Review Provenance

## 1. Goal

Let a developer manually choose an unreviewed Pi Session for the current working directory, explicitly bind that Session to a separately chosen Git changeset, and complete the existing evidence-backed learning loop while recording only a bounded, source-free Pi Session ID as durable provenance.

The Git selection and the existing evidence preview remain the sole authority for code scope. Pi LearnLoop must never infer a changeset from Session timestamps, names, messages, summaries, tool calls, or filesystem activity.

## 2. Background

`PROJECT.md` still lists two connected goals that are not implemented: selecting unreviewed Pi Sessions and associating a Session with a Git changeset. The current `/learn` command starts with a Git working-tree or commit-range choice and has no Pi Session concept.

The repository already has three useful seams:

- `internal/evidence` is a deep module whose small interface owns Git verification, bounded changed-Go analysis, and the inspectable preview;
- daemon-owned preview continuation preserves the exact reviewed evidence without trusting a client-built bundle;
- `internal/history` owns the protected SQLite schema, source-free assessment lifecycle, recovery, and repository-scoped lookup.

This task must deepen those seams without widening `evidence.Bundle`, evaluator schemas, prompts, RPC, or the generic learning-history interface. ADR-0006 was accepted on 2026-09-02, and Phase 1 was explicitly authorized on the same date.

## 3. Verified Current Behavior

Verified on 2026-09-02 from commit `530e651ed23d0ff87acc2c374e24533f0c12df6e`, the current repository, and the installed Pi 0.84.3 package:

- `main`, `origin/main`, and `HEAD` all resolve to `530e651`; the working tree was clean before this plan was written.
- `/learn` accepts no arguments and offers exactly `Working tree` or `Commit range`. Both require an explicit base revision; commit range also requires an explicit head revision.
- `DaemonEvidenceClient.preview` sends the current `ctx.cwd` and Git selection to strict authenticated `POST /v1/evidence-previews`.
- The daemon calls `evidence.Preview`, retains the exact bounded `evidence.Result` for five minutes, consumes it once in `POST /v1/question-sets`, and only then builds `evidence.Bundle` and evaluator input.
- `assessment.Provenance` currently carries the canonical repository root and released prompt provenance. `history.Start` records the repository, resolved changeset, evidence manifest, evaluator provenance, and lifecycle facts, but no Pi Session identity.
- Schema v1 has `repositories`, `learning_attempts`, and `question_outcomes`. `PRAGMA user_version` is authoritative. A newer schema fails closed without downgrade, repair, replacement, or deletion; the rest of the learning loop degrades when history is unavailable.
- `POST /v1/learning-history-queries` returns a bounded repository-scoped source-free record list. Its strict response does not contain Pi Session data.
- Pi 0.84.3 exports `SessionManager` and `SessionInfo`. `SessionManager.list(cwd, sessionDir?, onProgress?)` returns `Promise<SessionInfo[]>`, filters a custom Session directory by cwd, and sorts newest activity first.
- A Pi Session header contains `id`, `timestamp`, `cwd`, and optional `parentSession`; it has no Git base, Git head, repository manifest, changed-file set, or other trustworthy changeset identity.
- Pi creates current Session IDs with UUIDv7. Its declared validation accepts non-empty ASCII alphanumeric IDs with internal `.`, `_`, and `-`, while requiring alphanumeric first and last characters. Pi does not currently impose the LearnLoop storage bound proposed here.
- The extension context exposes a read-only Session manager, including the current Session ID/header/entries. No read-only interface maps another Session to Git changes.
- Pi 0.84.3's `SessionManager.list` is not metadata-only: it reads every candidate JSONL file and materializes `path`, `cwd`, `name`, `parentSessionPath`, timestamps, message count, `firstMessage`, and concatenated `allMessagesText` before returning. Pi LearnLoop needs only the ID and order, but cannot prevent this upstream in-memory materialization through the current interface.

The installed package manifest, lockfile, declarations, implementation, documentation, example, and `node_modules/.bin/pi --version` all identify Pi 0.84.3.

## 4. Relevant Call Chains and State

Current Git-only flow:

```text
/learn
→ explicit working-tree or commit-range selection
→ POST /v1/evidence-previews
→ evidence.Preview (canonical repository + exact bounded Git evidence)
→ daemon continuation retains evidence.Result
→ POST /v1/question-sets consumes continuation
→ evidence.BuildBundle → evaluator.NewInput → isolated question evaluator
→ assessment.Start(server-owned canonical repository provenance)
→ POST /v1/assessment-turns
→ history.Create before the first assessment evaluator call
→ complete / failed / interrupted source-free record
```

Proposed Pi Session flow:

```text
manual /learn Pi Session path
→ SessionManager.list(ctx.cwd, current Session directory) once
→ immediately project the newest at-most-20 candidates to validated IDs only
→ POST /v1/pi-session-review-queries(current repository, candidate IDs)
→ hide only IDs with at least one complete record in the same canonical repository
→ user explicitly selects one remaining Pi Session ID
→ user separately selects working tree or commit range
→ POST /v1/pi-session-evidence-previews(repository, Pi Session ID, Git selection)
→ evidence.Preview remains the code-scope authority
→ daemon continuation retains evidence.Result plus separate source-free Session provenance
→ existing POST /v1/question-sets request remains unchanged
→ evidence.Result alone enters BuildBundle/evaluator input
→ Session ID travels separately through assessment provenance
→ dedicated history start stores the validated ID on the attempt
→ only a complete record makes repository + Session ID "reviewed"
```

The completed-ID query is a UI filtering aid, not a uniqueness constraint or authorization decision. A stale query or an explicit later review may create another complete assessment for the same repository and Session ID.

## 5. Relevant Files

- `AGENTS.md`, `PROJECT.md`, and `plans/README.md`: lifecycle, high-risk phase authorization, stable goals, and plan format.
- `docs/decisions/ADR-0002-local-daemon-protocol-security.md`: authenticated loopback protocol, strict requests, additive v1 rules, safe errors, and no logging of request bodies.
- `docs/decisions/ADR-0003-post-preview-evaluator-boundary.md`: exact retained evidence, evaluator isolation, model-content boundary, and no retry.
- `docs/decisions/ADR-0004-answer-assessment-lifecycle.md`: daemon-owned assessment provenance/state and one-follow-up lifecycle.
- `docs/decisions/ADR-0005-local-learning-history.md`: source-free allowlist, schema-v1 safety, degradation, migration, and recovery semantics.
- `docs/checkpoints/durable-learning-history-phase-3.md`: current completed baseline and protected history-query guarantees.
- `extensions/pi-learnloop.ts`: manual command registration.
- `extensions/lib/learn-command.ts`: current Git-only `/learn` selection, preview, confirmation, answer, and history UI.
- `extensions/lib/daemon-client.ts`: strict authenticated client and exact v1 response validation.
- `internal/daemon/server.go`: current route dispatch, strict request handling, evidence-to-continuation flow, and assessment provenance construction.
- `internal/daemon/continuation.go`: bounded daemon-owned exact evidence retention.
- `internal/assessment/service.go`: volatile assessment state and history-start construction.
- `internal/evidence/evidence.go` and `internal/evidence/bundle.go`: Git scope authority and model-visible bundle projection, which must remain Session-agnostic.
- `internal/history/migrations/001_initial.sql`, `migrations.go`, `store.go`, `records.go`, `types.go`, and `validation.go`: schema v1, fail-closed opening, source-free lifecycle, repository query, and full stored-value validation.
- `node_modules/@earendil-works/pi-coding-agent/dist/core/session-manager.d.ts` and `.js`: authoritative installed declarations and behavior for `SessionManager.list`, `SessionInfo`, header fields, ID validation, and list-time message materialization.
- `node_modules/@earendil-works/pi-coding-agent/docs/session-format.md`, `docs/sessions.md`, and `examples/sdk/11-sessions.ts`: installed Session format and supported listing usage.

## 6. Task Contract

### Goal

Add an explicit, inspectable Pi Session-to-Git selection path while preserving Git evidence as the only code authority and persisting only a bounded Session ID.

### Scope

- Schema v1-to-v2 migration for one nullable source-free Pi Session ID.
- Completion-only repository/Session reviewed lookup.
- Two independent strict authenticated routes for Session-bound preview and reviewed-ID lookup.
- Daemon-owned Session provenance propagation outside model-visible values.
- A manual extension path that lists at most 20 current-cwd candidates, filters completed IDs, and requires a separate explicit Git selection.

### Allowed Files

Only the files named by the currently authorized phase may change. Across the complete plan, the allowed file groups are:

- Phase 1: `internal/history/**`, focused history tests, this plan, ADR-0006, Phase 1 checkpoint, and stable README/PROJECT facts after implementation.
- Phase 2: `internal/daemon/continuation.go`, `internal/daemon/server.go`, `internal/assessment/service.go`, their focused tests/integration tests, this plan, ADR-0006, Phase 2 checkpoint, and stable README/PROJECT facts after implementation.
- Phase 3: `extensions/pi-learnloop.ts`, `extensions/lib/daemon-client.ts`, `extensions/lib/learn-command.ts`, focused extension tests, this plan, ADR-0006, Phase 3 checkpoint, and stable README/PROJECT/user documentation after implementation.
- Existing manifests may be read for verification but must not change.

If implementation proves another file is essential, stop and amend the active phase contract before editing it.

### Forbidden Changes

- No dependency addition, removal, or upgrade; no Go, Node, TypeScript, SQLite driver, or Pi version change.
- No change to evidence selection semantics, evidence caps, `evidence.Result`, `evidence.Bundle`, evaluator input/output schemas, prompts, RPC frames, model arguments, feedback, labels, or retry rules.
- No Session timestamp/message/name/tool-based Git inference.
- No automatic lifecycle hook, Session event listener, background Session index, Git snapshot, reminder, startup notification, daemon autostart, polling worker, or SSE.
- No writes to Pi Session files: no marker, label, name, custom entry, custom message, transcript mutation, or Session deletion.
- No extension-owned persistence, cache, mapping file, database, local storage, telemetry, or remote synchronization.
- No persistence of Session path, cwd, name, created/modified time, message count, parent Session, leaf/entry IDs, prompt, user/assistant content, tool call/result, summary, transcript, or first/all-messages projections.
- No Session ID in evidence bundles, evaluator values, prompts, RPC, model-visible content, logs, errors, generic `/v1/evidence-previews`, generic `/v1/learning-history-queries`, or `/learn-history` output.
- No destructive migration, downgrade, database repair/replacement, pruning, export, retention change, or record deletion.
- No unrelated refactor, formatting, public command, release, CI/CD, packaging, or cross-platform work.

### Acceptance Criteria

The three authorized phases satisfy Sections 8, 11, and 12; every existing Git-only, privacy, isolation, history-degradation, and no-retry guarantee remains true.

### Verification

Run each phase's focused tests and the final repository-supported suites in Section 13. No verification may call a provider or use real Session transcript content in committed fixtures.

## 7. Deep-Module Design

This plan keeps three small interfaces and hides implementation behind them:

1. **History Session provenance module** — accepts one validated optional ID at the dedicated history-start seam and answers one bounded completion-only repository/ID query. SQL, migration, NULL handling, and status filtering remain private. The interface does not return generic records merely to answer a boolean reviewed question.
2. **Daemon continuation/provenance module** — retains exact evidence plus a separate optional source-free provenance value. Callers consume one value; evidence goes to `BuildBundle`, provenance goes to assessment history. The Session ID cannot cross the evaluator seam by construction.
3. **Pi Session listing adapter** — uses Pi's supported `SessionManager.list` once and immediately projects to at most 20 IDs. All UI ordering and filtering stay in the thin extension; Pi file parsing and directory rules remain Pi's implementation.

The external HTTP seam is intentionally additive rather than making existing strict request interfaces carry conditional Session fields. Production and focused test adapters exercise the same daemon interfaces. No hypothetical repository, evaluator, Session-storage, or database abstraction is added.

## 8. Proposed Changes

### 8.1 Bounded Pi Session identity

LearnLoop accepts a Pi Session ID only when its ASCII byte representation:

- is 1 through 128 bytes;
- begins and ends with `[A-Za-z0-9]`;
- contains only `[A-Za-z0-9._-]`.

This mirrors Pi 0.84.3's declared character contract while adding a storage and request bound. It deliberately does not freeze UUIDv7 shape, so a future compatible Pi ID within the same bounded character interface need not force a schema change. Both daemon input and history storage validate the same rule. Invalid IDs receive only generic safe errors and are never echoed.

### 8.2 Schema v2 and v1 migration

Schema v2 adds exactly one nullable `pi_session_id` text value to `learning_attempts`, constrained to the bounded ID rule when non-NULL.

- Opening an empty database applies migration 1 and migration 2 in order.
- Opening schema v1 applies migration 2 transactionally and changes only `PRAGMA user_version` plus the new nullable column.
- Every pre-existing v1 row receives SQL `NULL`; every existing column value and row relationship remains unchanged.
- Git-only assessments created by the existing route continue to store SQL `NULL`.
- A Session-bound assessment stores exactly one validated ID before the first assessment evaluator call.
- Full opening preflight validates every non-NULL stored ID without exposing it through generic record queries.
- A failed v1-to-v2 migration rolls back and leaves schema v1 unchanged.
- Schema v2 retains WAL, `synchronous=FULL`, foreign keys, `trusted_schema=OFF`, busy timeout, same-owner/mode/link/local-filesystem checks, integrity checks, and recovery from `running` to `interrupted`.
- A schema-v1 binary sees schema v2 as newer and follows ADR-0005: history fails closed without rewriting, downgrading, repairing, renaming, or deleting the database; preview and the volatile learning loop remain available.

The durable allowlist gains only the Session ID. No Session metadata or content column is added.

### 8.3 Completion-only reviewed meaning

A Pi Session is `reviewed` only when at least one `learning_attempts` row has all of:

```text
canonical repository root = the server-verified current repository
pi_session_id = the candidate ID
status = complete
```

`running`, `failed`, and `interrupted` rows never make a Session reviewed. SQL `NULL` v1/Git-only rows never match. The query accepts 1 through 20 unique validated candidate IDs and returns only the matching IDs, in candidate order, with no record, status, timestamp, label, changeset, evaluator, or repository metadata.

### 8.4 Independent authenticated routes

Add two authenticated v1 routes governed by ADR-0002's loopback, Instance Token, Host/Origin/peer, strict JSON, cache, size, timeout, and safe-error rules:

```text
POST /v1/pi-session-evidence-previews
POST /v1/pi-session-review-queries
```

Session-bound preview request:

```json
{
  "repository": "/absolute/path/to/repository",
  "pi_session_id": "<bounded Pi Session ID>",
  "selection": {
    "kind": "commit_range",
    "base": "<Git revision>",
    "head": "<Git revision>"
  }
}
```

The working-tree variant uses the existing `{kind:"working_tree",base:"..."}` selection. The response uses the existing evidence-preview shape and does not echo the Session ID. The route calls the same `evidence.Preview` interface with the same fixed caps; it does not copy or alter Git/evidence rules.

Reviewed-ID request and response:

```json
{
  "repository": "/absolute/path/to/repository",
  "pi_session_ids": ["<ID 1>", "<ID 2>"]
}
```

```json
{
  "protocol_version": 1,
  "reviewed_pi_session_ids": ["<completed ID from the request>"]
}
```

The query verifies/canonicalizes the repository through the existing Git-root seam before consulting history. Cross-repository results are impossible. Unavailable storage returns the existing generic `history_unavailable` capability error and does not guess that candidates are unreviewed.

The existing strict requests and responses for `/v1/evidence-previews`, `/v1/question-sets`, `/v1/assessment-turns`, and `/v1/learning-history-queries` do not gain Session fields.

### 8.5 Server-owned provenance propagation and model isolation

The Session-bound preview route validates the ID and stores an owned copy beside, not inside, the exact retained `evidence.Result`. On atomic continuation consume:

- `evidence.Result` alone goes to `evidence.BuildBundle` and `evaluator.NewInput`;
- the ID stays in daemon-owned continuation/assessment provenance;
- the existing question-set request needs no Session field;
- assessment history start receives the ID through a dedicated source-free provenance value;
- question/assessment evaluators, schemas, prompts, RPC, model content, errors, logs, and HTTP responses never receive it.

The generic preview route retains empty provenance, so current Git-only behavior and v2 NULL storage remain intact.

### 8.6 Manual Pi extension interaction

The extension preserves the current Git-changeset-only path and adds a separate manually selected Pi Session path under `/learn`:

1. Require the existing no-argument, interactive-UI, and trusted-project gates.
2. Let the user choose Pi Session review or the existing Git changeset flow.
3. For the Pi Session path, call `SessionManager.list(ctx.cwd, ctx.sessionManager.getSessionDir())` once. Never call `listAll`.
4. Take the newest at most 20 results, immediately validate/project only their IDs, and discard references to the richer `SessionInfo` values.
5. Query reviewed IDs once for the current repository; hide only completed IDs.
6. Present only full Session IDs as selection values. Do not display or persist name, time, path, cwd, parent, message count, messages, prompts, tool calls, summaries, or leaf data.
7. After the user chooses one ID, require the same explicit working-tree or commit-range inputs used today.
8. Show the chosen Session ID and Git selection together in the local preview interaction so the association is inspectable, then use the dedicated Session-bound preview route.
9. Preserve the existing preview-before-model, data-sharing/cost confirmation, answer confirmation, no-retry, and assessment rendering behavior.

Cancellation before either selection sends no Session-bound preview. No Session list or history query starts a model.

### 8.7 Pi 0.84.3 privacy and resource limitation

Although LearnLoop uses only IDs, Pi 0.84.3 currently makes `SessionManager.list` read candidate Session files and materialize `firstMessage` and `allMessagesText` in extension-process memory, plus other unused metadata. It may scan more than the 20 entries later shown because the API returns the full list before LearnLoop can slice it.

LearnLoop must not log, persist, display, transmit, index, cache, summarize, or inspect those values. Tests use synthetic Sessions only. Before Phase 3 authorization, explicitly reassess the memory/privacy/resource cost against representative Session counts and sizes. If it is unacceptable, stop and redesign the listing adapter from authoritative Pi capabilities; do not silently parse Session files, add an index, install a hook, or weaken the boundary.

## 9. Compatibility

- Database schema changes from v1 to v2 through one forward-only non-destructive migration.
- All v1 records remain readable with `pi_session_id = NULL` and otherwise unchanged.
- Schema-v1 binaries fail closed on v2 exactly as ADR-0005 requires; they do not downgrade or repair.
- `modernc.org/sqlite v1.35.0`, Go 1.21, Pi 0.84.3, and the npm graph remain pinned.
- Existing strict request bodies do not change. The two new routes are additive authenticated HTTP v1 capabilities.
- Existing Git-only `/learn`, evidence caps, continuation IDs/lifetime, question and assessment schemas, prompts, labels, follow-up rules, history records, generic `/learn-history`, and no-retry behavior remain compatible.
- Session ID is local source-free provenance. It is not a durable foreign key to a Session file: deleting, moving, renaming, branching, or making a Session unavailable does not mutate or invalidate a completed history record.
- Reviewed lookup is scoped by canonical repository plus ID, so identical IDs in another repository do not hide a Session.
- No uniqueness guarantee is added for repository + Session ID; multiple explicit reviews remain representable.

## 10. Risks

- **False provenance:** a user can bind the wrong Git changeset. Mitigation: never infer; display both selections; keep evidence preview authoritative and require explicit confirmation.
- **Privacy/resource exposure during listing:** Pi 0.84.3 materializes message-derived values for all listed files. Mitigation: immediate ID-only projection, no use/transmission/persistence, synthetic tests, and a Phase 3 go/no-go review.
- **Session ID leakage:** a new field could accidentally enter generic history, model input, logs, or errors. Mitigation: separate provenance value and routes, exact response tests, evaluator/RPC absence tests, and safe generic errors.
- **Migration damage:** adding a column could partially alter schema v1. Mitigation: immediate transaction, rollback tests, exact schema verification, full stored-value preflight, and unchanged no-repair policy.
- **Old-binary incompatibility:** schema-v1 binaries cannot use v2 history. Mitigation: intentional fail-closed degradation documented by ADR-0005; no downgrade.
- **Incorrect reviewed filtering:** failed/interrupted attempts could hide useful work or cross-repository rows could leak. Mitigation: completion-only `EXISTS` lookup keyed by canonical repository and bounded candidate set.
- **Protocol drift:** optional Session fields on current strict routes would break callers or blur provenance. Mitigation: independent additive routes and unchanged current request/response contracts.
- **Stale filtering:** a Session can complete after the reviewed query. Mitigation: filtering is advisory, not locking; an explicit duplicate review is safe and source-free.
- **Unbounded UI/listing:** many Sessions could create slow or memory-heavy interaction. Mitigation: at most 20 candidates leave LearnLoop, no pagination/background index, and Phase 3 resource review.

## 11. Implementation Phases

Every phase is high risk. ADR-0006 acceptance does not authorize implementation. Each phase requires explicit authorization, a restated contract, allowed-file confirmation, verification, complete diff review, and a checkpoint. Stop after every phase.

### Phase 1 — Source-Free History Provenance and Schema v2

Goal: release schema v2 with one nullable bounded Pi Session ID and a completion-only repository/Session lookup.

Allowed files:

- `internal/history/**` and focused history tests;
- `plans/explicit-pi-session-review.md`;
- `docs/decisions/ADR-0006-explicit-pi-session-provenance.md`;
- `docs/checkpoints/explicit-pi-session-review-phase-1.md`;
- README/PROJECT only for facts implemented in this phase.

Required work:

- add embedded ordered migration 2 and move the current schema authority to v2;
- migrate v1 rows to SQL NULL without changing existing values;
- validate bounded ID writes and all stored non-NULL values;
- keep generic history record/list interfaces and responses Session-free;
- add a dedicated optional source-free history-start provenance input;
- add the bounded completion-only repository/candidate lookup;
- preserve running recovery, protection, pragmas, corruption/future-schema handling, idempotent terminal writes, and no-repair/no-downgrade semantics;
- add migration, rollback, exact-schema, old-row preservation, NULL, invalid-ID, status, repository-isolation, bound, ordering, and privacy tests.

Stop after Phase 1. Do not add daemon routes or extension behavior.

### Phase 2 — Independent Routes and Server-Owned Propagation

Goal: add the two strict authenticated routes and propagate Session provenance from exact preview continuation to history without model visibility.

Allowed files:

- `internal/daemon/continuation.go`, `internal/daemon/server.go`, and focused daemon/integration tests;
- `internal/assessment/service.go` and focused assessment/history tests;
- Phase 1 history files only if an investigation-backed correction to its accepted interface is first reflected in the phase contract;
- plan, ADR, Phase 2 checkpoint, README, and PROJECT lifecycle/fact updates.

Required work:

- add strict bounded Session-preview and reviewed-ID request/response handling;
- reuse existing authentication, browser defenses, repository canonicalization, evidence caps, timeouts, and safe errors;
- retain owned Session provenance separately from `evidence.Result`;
- pass it only through assessment provenance and the dedicated history-start seam;
- leave all existing route schemas exact and unchanged;
- prove generic history, bundles, evaluator inputs, prompts, serialized RPC, responses, logs, and errors contain no Session ID;
- prove `running`/`failed`/`interrupted` do not filter, `complete` does, and repository isolation holds;
- use fake evaluators only and never call a provider.

Stop after Phase 2. Do not list Pi Sessions or change `/learn`.

### Phase 3 — Manual Bounded Pi Session Selection

Goal: add the thin manual `/learn` Session path while preserving the existing Git-only path and every model confirmation gate.

Authorization prerequisite:

- explicitly review the Pi 0.84.3 full-file/message materialization cost described in Section 8.7; if unacceptable, amend this plan and ADR before implementation.

Allowed files:

- `extensions/pi-learnloop.ts`;
- `extensions/lib/daemon-client.ts`;
- `extensions/lib/learn-command.ts`;
- focused files under `tests/extension/`;
- plan, ADR, Phase 3 checkpoint, README, and PROJECT lifecycle/fact updates.

Required work:

- call the supported cwd/session-directory list interface only from a manual trusted interactive path;
- project and send at most 20 unique bounded IDs;
- filter completion-only reviewed IDs with one dedicated query;
- show only IDs and require a second explicit Git selection;
- send the bound request through the dedicated preview route;
- keep Git-only `/learn` and `/learn-history` compatible;
- handle empty lists, all-reviewed lists, invalid Pi data, history unavailable, Session listing failure, cancellation, and daemon/protocol errors without inference or hidden fallback;
- prove list/query/selection cause no model call and no extension/Pi Session write;
- use synthetic Session fixtures and never commit real transcript content.

Stop after Phase 3 and final verification. Do not add hooks, reminders, storage, pagination, or background work.

## 12. Acceptance Criteria

### History and migration

- Schema v2 contains exactly one new nullable bounded Pi Session ID and no Session content/metadata fields.
- Empty and v1 databases migrate forward transactionally; existing v1 rows have NULL IDs and every old value is unchanged.
- Migration failure leaves v1 unchanged; future/corrupt/unsafe stores still fail closed without repair.
- A v1 binary treats v2 as unsupported and leaves it unchanged under ADR-0005.
- Git-only assessment starts store NULL; Session-bound starts store the selected validated ID.
- Only complete rows in the same canonical repository are returned as reviewed; running, failed, interrupted, NULL, and other-repository rows are not.

### Protocol and isolation

- Both new routes require the existing Instance Token and all ADR-0002 defenses.
- Requests are strict, bounded, duplicate-key/unknown-field/trailing-content rejecting, and validate 1–128-byte Pi IDs.
- Session-bound preview applies the exact current evidence limits and response shape and never echoes the ID.
- Existing strict route requests/responses remain unchanged.
- Session ID appears only in daemon-owned continuation, assessment provenance, dedicated history start/storage, and dedicated reviewed-ID query/result.
- It is absent from `evidence.Result`/`Bundle`, evaluator input, prompt, RPC, model content, logs, errors, generic history response, and `/learn-history` rendering.
- Storage unavailable never labels a candidate unreviewed and never blocks the existing Git-only learning loop.

### Extension behavior

- `/learn` remains manual, no-argument, trusted, and interactive.
- Existing Git-only selection remains available and compatible.
- The Session path uses `SessionManager.list` for current cwd/current Session directory, never `listAll`, and exposes at most 20 IDs.
- The extension does not use, display, persist, transmit, log, or index SessionInfo message/metadata fields.
- Completed IDs are filtered; running/failed/interrupted attempts remain selectable.
- The user explicitly chooses both a Session ID and a Git working tree/commit range and sees the association before model confirmation.
- Cancellation or listing/query failure starts no preview/evaluator as applicable; list/query alone never calls a model.
- No Session file, extension-owned store, dependency, hook, background index, snapshot, or reminder is introduced.

## 13. Verification

### Phase 1 focused verification

- focused `internal/history` tests for empty→v2 and v1→v2 migration, exact schema, rollback, value preservation, NULL behavior, ID validation, completion-only lookup, bounds/order, isolation, protection, future/corrupt schema, recovery, and privacy allowlist;
- `CGO_ENABLED=0 go test -count=1 ./internal/history/...`;
- inspect the complete schema and migration diff, including a schema-v1 fixture with representative running/complete/failed/interrupted rows.

### Phase 2 focused verification

- focused daemon, continuation, assessment, and history integration tests for strict auth/JSON/media/size/method/timeout/error behavior, exact preview fidelity, owned copies, propagation, storage degradation, repository isolation, and completion-only results;
- fake evaluator captures proving Session ID absence from all model-visible/serialized values and no provider contact;
- unchanged-route request/response regression tests.

### Phase 3 focused verification

- `npm run typecheck`;
- focused extension tests for at-most-20 ordering, completed filtering, ID-only projection/display, explicit Git binding, Git-only compatibility, empty/all-reviewed/history-unavailable/list-failure/cancel paths, strict responses, no retry, zero model calls before existing confirmations, and no Session/extension writes;
- synthetic bounded Session fixtures only;
- manual resource observation with representative local synthetic Session counts/sizes before accepting the `SessionManager.list` limitation.

### Final verification after each implemented phase as applicable

```text
CGO_ENABLED=0 go test -count=1 ./...
go test -race -count=1 -tags netgo ./...
go vet -tags netgo ./...
go build -tags netgo -o <temporary explicit path> ./cmd/pi-learnloop
npm run typecheck
npm test
npm pack --dry-run --json
scripts/test-agent-infra.sh
scripts/validate-agent-infra.sh
git diff --check
```

Use an explicit temporary build path outside the repository and remove it after inspection. Run only commands supported by the files changed in the current phase. No automated verification may call a provider, create production data, read real Session transcript content into a committed fixture, or claim unsupported platform coverage.

Before stopping each phase, inspect `git status`, `git diff --stat`, and the complete `git diff`; confirm every changed file is allowed and every phase acceptance criterion is addressed.

## 14. Open Questions

- `Resolved 2026-09-02` — The user accepted ADR-0006 and explicitly authorized Phase 1. Later phases remain unauthorized.
- `TODO / Need Confirmation before Phase 3` — Is Pi 0.84.3's full-file scan and materialization of `firstMessage`/`allMessagesText` acceptable for a manual ID-only list under representative Session volumes? If not, redesign from authoritative Pi capabilities before Phase 3; do not infer permission for a custom parser, index, hook, or dependency.
- No Phase 1 design dependency remains unresolved: schema v2 is nullable/non-destructive, the Session ID contract is bounded, reviewed semantics are completion-only, and no dependency change is proposed.
