---
id: ADR-0006
status: proposed
date: 2026-09-02
supersedes: none
---

# ADR-0006: Explicit Pi Session Provenance

## Context

Pi LearnLoop currently lets a user explicitly select a Git working tree or commit range, inspect bounded changed-Go evidence, complete an isolated interview, and view source-free repository history. It does not let the user select an unreviewed Pi Session or associate a Session with the reviewed Git changeset.

Pi 0.84.3 provides stable Session IDs, a read-only Session manager in extension context, and `SessionManager.list(cwd, sessionDir?)`. Its Session header contains `id`, `timestamp`, `cwd`, and optional `parentSession`, but no trustworthy Git base/head or changed-file identity. Session messages, summaries, tool calls, and timestamps are not proof of a Git changeset.

The listing interface also has a material privacy/resource limitation: before returning `SessionInfo[]`, Pi 0.84.3 reads candidate Session files and materializes message-derived `firstMessage` and `allMessagesText`, plus paths and other metadata. LearnLoop needs only bounded IDs and ordering.

ADRs 0002–0005 already require strict authenticated local requests, exact server-owned evidence, model isolation, no product retry, and allowlisted source-free history. Any Session design must preserve those decisions.

## Decision

This ADR is proposed. It does not authorize an implementation phase.

### 1. Associate explicitly; never infer

Pi Session provenance is a user assertion made by two separate visible choices:

1. choose a Pi Session ID from a manual current-cwd list;
2. choose a Git working tree or commit range through the existing selection interface.

The existing `evidence.Preview` result remains the sole authority for code scope. LearnLoop does not infer Git changes from Session time, cwd, name, parent, leaf, prompts, messages, assistant output, tool calls/results, summaries, or filesystem activity. It does not claim that the chosen Session produced every selected change; it records only the user's explicit association for this review.

The existing Git-only `/learn` path remains available.

### 2. Persist only a bounded source-free Session ID

Schema v2 adds one nullable `pi_session_id` to the assessment attempt. A valid ID is 1–128 ASCII bytes, begins and ends with an ASCII alphanumeric character, and otherwise contains only ASCII alphanumerics plus `.`, `_`, and `-`.

This follows Pi 0.84.3's declared character set while adding a fixed LearnLoop bound. It does not require UUIDv7 shape.

No Session path, cwd, name, timestamp, message count, parent Session, leaf/entry ID, prompt, user/assistant content, tool call/result, summary, first/all-messages value, or transcript may be persisted. The ID is not a foreign key to a Pi file and is not dereferenced during history read or recovery.

Migration from schema v1 to v2 is forward-only, transactional, and non-destructive. Every v1 row receives SQL `NULL` and all existing values remain unchanged. Git-only reviews continue to store NULL. A schema-v1 binary must treat v2 as newer and follow ADR-0005's fail-closed behavior without downgrade, repair, replacement, or deletion; non-history learning remains degradable.

No dependency is added or changed.

### 3. Define reviewed by completion only

A Session is reviewed only when the same server-canonicalized repository and Session ID have at least one `complete` assessment record.

`running`, `failed`, and `interrupted` records do not hide a Session. NULL v1/Git-only records do not match. The lookup is bounded to at most 20 unique candidate IDs and returns only the completed subset, without record details or other Session metadata.

Reviewed filtering is advisory UI state, not a uniqueness constraint. Multiple explicit completed reviews for the same repository and Session ID may exist.

### 4. Use independent additive authenticated routes

Add two strict authenticated HTTP v1 routes:

```text
POST /v1/pi-session-evidence-previews
POST /v1/pi-session-review-queries
```

The first accepts repository, bounded Session ID, and the existing explicit Git selection. It invokes the same evidence interface and fixed caps as the generic preview route and does not echo the Session ID.

The second accepts the current repository and 1–20 bounded candidate IDs, canonicalizes the repository through the existing Git seam, and returns only reviewed IDs in candidate order. If protected history is unavailable, it fails explicitly rather than treating every Session as unreviewed.

Existing strict preview, question, assessment, and generic history requests/responses gain no Session fields. Adding separate routes preserves their compatibility and makes the provenance-bearing seam inspectable.

### 5. Keep Session provenance outside the model boundary

The daemon retains the Session ID beside the exact preview result in its owned continuation. On consume, evidence alone enters `evidence.Bundle`, evaluator input, prompts, RPC, and model content; the ID travels separately through daemon-owned assessment provenance to a dedicated source-free history start.

The Session ID may appear only in:

- the local extension's manual Session selection and dedicated authenticated requests;
- daemon-owned preview continuation and assessment provenance;
- dedicated history start/storage and completion-only reviewed-ID query/result.

It must not appear in `evidence.Result` or `evidence.Bundle`, evaluator schemas, prompts, RPC frames, model-visible content, logs, errors, generic `/v1/evidence-previews`, generic `/v1/learning-history-queries`, or `/learn-history` output.

### 6. Keep Session discovery manual, bounded, and read-only

The extension uses `SessionManager.list(ctx.cwd, ctx.sessionManager.getSessionDir())` only after a trusted interactive `/learn` action, never `listAll`. It immediately projects the newest at most 20 results to validated IDs, displays only IDs, queries completion state once, and requires a separate Git selection.

LearnLoop adds no automatic lifecycle hook, background index, Git snapshot, reminder, startup notification, polling, Session marker, Session-file write, or extension-owned persistence.

Pi 0.84.3's list-time full-file/message materialization is an accepted design limitation only if explicitly reviewed before Phase 3. LearnLoop never uses, logs, displays, persists, or transmits those message/metadata values. If the resource/privacy cost is unacceptable, Phase 3 stops for redesign from authoritative Pi capabilities; the implementation must not silently add a custom Session parser, hook, index, or dependency.

## Alternatives

### Infer Git changes from Session timestamps or filesystem activity

Rejected. Time overlap is not causality; unrelated edits, rebases, commits, branches, and concurrent tools make the association unreliable. It would present a guess as evidence.

### Infer Git changes from prompts, assistant text, summaries, or tool calls

Rejected. These values are untrusted, incomplete, branch-dependent, privacy-sensitive, and may describe intended rather than actual changes. Reading them also expands the data boundary without producing authoritative Git scope.

### Install lifecycle hooks and capture Git snapshots automatically

Rejected. Hooks/background snapshots change Session and repository lifecycle, create persistent source-bearing state, need cleanup/recovery semantics, and violate the manual local-first scope. Automatic reminders are also a project non-goal.

### Write a marker or custom entry into each Pi Session

Rejected. It mutates Pi-owned Session files, can enter user-visible or future model context depending on entry type, couples LearnLoop to Session format/lifecycle, and violates the read-only requirement.

### Store the mapping in extension-owned files or local storage

Rejected. It duplicates the daemon's protected storage, migration, canonical-repository, ownership, and recovery rules; creates another source of truth; and weakens locality of the history module.

### Persist full Session metadata or transcripts

Rejected. Paths, names, times, messages, prompts, tool calls, summaries, and transcripts are unnecessary for completion filtering and materially increase privacy, retention, migration, and breach impact.

### Add optional Session fields to existing strict routes

Rejected. Existing requests intentionally reject unknown fields. Conditional fields would widen well-established interfaces, blur Git-only versus provenance-bound behavior, and increase accidental leakage into generic history. Independent routes are additive and explicit.

### Use Session ID as the changeset authority

Rejected. Pi headers have no trusted Git base/head, and a Session can cover multiple branches, commits, working-tree states, or no code change. Session identity is provenance, not evidence.

### Maintain a background header-only Session index

Rejected for this capability. It adds lifecycle hooks, extension-owned persistence, staleness, cleanup, and migration. If Pi 0.84.3 listing is unacceptable, a replacement requires a new investigated design rather than an implicit index.

## Consequences

- Users can manually filter completed Pi Sessions and explicitly associate a chosen Session with an inspectable Git changeset.
- The association is honest but user-supplied; LearnLoop does not claim automatic authorship attribution.
- Git evidence, evaluator isolation, prompt/RPC schemas, labels, retry rules, and generic history stay unchanged.
- Schema v2 stores one additional source-free nullable value. Old rows are preserved with NULL; old binaries lose history capability on v2 but fail closed as already designed.
- Completion-only filtering keeps failed/interrupted work visible for another explicit review.
- Separate authenticated routes add protocol surface but avoid breaking current strict request interfaces.
- Session IDs remain local provenance and are not model-visible or returned by generic history.
- Pi 0.84.3 may temporarily materialize unused Session messages and metadata in extension memory during manual listing. That cost must be reviewed before Phase 3 and may force a redesign.
- No dependency, automatic hook, snapshot, marker, reminder, extension store, Session write, or background worker is introduced.
