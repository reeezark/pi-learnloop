---
id: reviewable-multiline-answers
status: complete
risk: high
current_phase: 2
phase_status: complete
updated: 2026-09-03
---

# Reviewable Multiline Answers

## 1. Goal

Let a user compose bounded multiline answers to Q1, Q2, Q3, review and revise
those three drafts before the existing sharing confirmation, and compose the
optional F1 answer with the same editor, while preserving the existing evidence,
model-isolation, no-retry, and source-free-history guarantees.

The implementation must deepen the existing answer-collection behavior behind
the `learn-command` module's current interface. It must not introduce a custom
editor framework, another answer store, or a second owner for assessment rules.

## 2. Background

The implemented `/learn` flow displays an enriched evidence preview, obtains an
explicit question-generation confirmation, renders exactly Q1/Q2/Q3, and then
uses Pi's single-line `ctx.ui.input` dialog once per answer. A user cannot see a
previous answer while revising it or review all three answer slots before the
existing model-sharing confirmation. `PROJECT.md` identifies richer answer
editing as an unimplemented Pi-extension responsibility.

The locally installed and exactly pinned Pi package is
`@earendil-works/pi-coding-agent` 0.84.3. Its public
`ExtensionUIContext.editor(title, prefill?)` declaration returns
`Promise<string | undefined>`. The TUI implementation provides a multiline
editor, submits with Enter, inserts a newline with the configured newline
keybinding, cancels with Escape/Ctrl-C, and supports the configured external
editor. RPC mode exposes the same dialog as a blocking `extension_ui_request`.

That existing primitive is sufficient for the product interaction, but it
exposes two compatibility and privacy decisions that cannot be hidden as local
UI work:

- the current Go answer validator rejects every Unicode control character,
  including LF, and is shared with model-produced question/feedback validation;
- the authenticated assessment route currently reads at most 16 KiB even though
  three individually valid 4-KiB answers can serialize beyond that bound.

The external-editor shortcut also leaves Pi's in-memory UI path: Pi 0.84.3
creates a temporary `pi-editor-*/prompt.md`, starts the configured editor, reads
the result, and removes the directory on a best-effort basis. The editor itself
may create swap, backup, recovery, or telemetry data. This limitation must be
visible before the first answer editor opens and accepted in ADR-0008.

## 3. Current Behavior

Verified current behavior is:

- `LearnCommandContext.ui` exposes `select`, `input`, `confirm`, and `notify`,
  but not Pi's already public `editor` method.
- `collectAnswers` visits Q1, Q2, and Q3 in order and calls `collectAnswer` once
  for each. `collectAnswer` calls `ui.input(title, question)`, applies
  JavaScript `trim()`, and ends the flow on cancellation, empty text, a value
  over 4,096 UTF-8 bytes, or any C0/C1 control character.
- The optional F1 answer uses the same one-shot helper.
- The extension holds answers only long enough to request the existing explicit
  assessment-sharing confirmation and submit them through `LearnClient.assess`.
- `DaemonEvidenceClient.assess` serializes the strict request with
  `JSON.stringify` and performs no automatic retry.
- `handleAssessmentTurn` applies `http.MaxBytesReader` with a dedicated but
  currently 16-KiB bound before strict JSON decoding.
- `internal/assessment.Submit` owns the atomic lifecycle. It creates a validated
  initial or follow-up `evaluator.AssessmentInput` before invoking a fresh
  isolated Pi evaluator process.
- `internal/evaluator.validateAssessmentText` validates both user answers and
  model-produced F1/feedback. It accepts nonblank valid UTF-8 within its byte
  bound and rejects every `unicode.IsControl` rune, so LF is currently invalid.
- Assessment inputs and source-bearing state remain volatile. Durable history
  contains no question, answer, F1, feedback, prompt body, RPC frame, or model
  output.

The 16-KiB transport bound does not cover the official client's worst accepted
answer encoding. With a normal `as1-` identifier, the current Node
`JSON.stringify` request is approximately 12,486 bytes for three 4,096-byte
plain-ASCII answers but 24,774 bytes when those answers consist of quotes,
backslashes, or LF characters. The same follow-up requests are approximately
4,222 and 8,318 bytes respectively. This is an existing transport-budget gap
for quotes and backslashes and becomes user-visible for multiline input.

## 4. Relevant Call Chain

Initial answers:

```text
Pi /learn handler
→ createLearnCommand
→ client.questions(opaque continuation, fixed model selection)
→ render Q1/Q2/Q3
→ collectAnswers
→ collect/edit Q1, Q2, Q3 in extension memory
→ review menu (IDs only; no answer text)
→ existing answer-sharing confirmation
→ DaemonEvidenceClient.assess
→ JSON.stringify
→ POST /v1/assessment-turns
→ handleAssessmentTurn (body cap, strict JSON, authentication)
→ internal/assessment.Submit (atomic state transition)
→ evaluator.NewInitialAssessmentInput
→ fresh isolated Pi assessment evaluator
→ source-free terminal history write
```

Optional follow-up:

```text
validated F1 response
→ render F1
→ collect/edit one bounded F1 answer
→ DaemonEvidenceClient.assess (no retry)
→ same authenticated assessment route
→ internal/assessment.Submit
→ evaluator.NewFollowUpAssessmentInput
→ fresh isolated Pi assessment evaluator
→ source-free terminal history write
```

The answer editor must remain on the extension side of the existing sharing
gate. It must not become a daemon module, persisted draft, Session entry,
evaluator capability, or model interaction.

## 5. Relevant Files

- `PROJECT.md`: records richer answer editing as remaining work and defines the
  current module ownership.
- `extensions/lib/learn-command.ts`: owns the current answer collection,
  sharing confirmation, F1 interaction, and injected Pi UI interface.
- `extensions/lib/daemon-client.ts`: owns canonical request serialization,
  authenticated transport, deadlines, and no-retry behavior.
- `tests/extension/learn-command.test.ts`: exercises the command through the
  same injected UI/client interface used by production.
- `tests/extension/pi-session-review.test.ts`: constructs the same command
  context for the Session-bound path.
- `internal/daemon/server.go`: owns the per-route body reader and strict
  assessment request adapter.
- `internal/daemon/assessment_test.go`: verifies assessment authentication,
  strictness, size rejection, lifecycle, and state consumption.
- `internal/evaluator/assessment_contract.go`: owns both assessment-input and
  assessment-output text validation.
- `internal/evaluator/assessment_contract_test.go`: verifies answer and
  model-output contracts independently of transport.
- `internal/assessment/service.go` and its tests: own the existing volatile,
  single-consume assessment lifecycle and are an integration check, not a new
  editing seam.
- `agent/schemas/evaluator-assessment-input-v2.schema.json`: describes the
  existing v2 input shape; its answer strings already permit the proposed
  values, while Go retains the stricter UTF-8 byte authority.
- Pi 0.84.3 local declarations and implementation under
  `node_modules/@earendil-works/pi-coding-agent/dist/`: authoritative evidence
  for `ui.editor`, RPC forwarding, external-editor behavior, and cleanup limits.
- ADR-0002 through ADR-0007: fix transport security, exact evidence,
  no-retry assessment lifecycle, source-free persistence, Session isolation,
  and enriched-evidence behavior that this task must preserve.

## 6. Scope

This task may:

- distinguish the user-answer text contract from the model-output text
  contract inside `internal/evaluator`;
- allow internal LF (`U+000A`) only in Q1/Q2/Q3 and F1 user answers, while
  retaining valid UTF-8, nonblank, and 4-KiB limits;
- raise only the `POST /v1/assessment-turns` transport body bound from 16 KiB
  to 32 KiB, before strict decoding;
- require Pi 0.84.3's existing `ui.editor` method in the command's injected UI
  interface;
- add a bounded, local review/edit loop for the three initial answers;
- reuse the multiline editor for F1;
- add a one-time disclosure before opening the first answer editor about Pi's
  external-editor temporary file and external-process behavior;
- add focused regression tests and update stable public/project documentation;
  and
- create phase checkpoints and update this plan/ADR metadata according to the
  high-risk lifecycle.

### Allowed files by phase

Phase 1 may modify only:

- `internal/evaluator/assessment_contract.go`
- `internal/evaluator/assessment_contract_test.go`
- `internal/daemon/server.go`
- `internal/daemon/assessment_test.go`
- `internal/assessment/service_test.go` only if the existing public assessment
  seam needs an end-to-end LF regression
- `plans/reviewable-multiline-answers.md`
- `docs/decisions/ADR-0008-reviewable-multiline-answers.md`
- `docs/checkpoints/reviewable-multiline-answers-phase-1.md`

Phase 2 may modify only:

- `extensions/lib/learn-command.ts`
- `tests/extension/learn-command.test.ts`
- `tests/extension/pi-session-review.test.ts`
- `tests/extension/extension-entry.test.ts` only if its Pi command-context fake
  must satisfy the newly required editor interface
- `README.md`
- `PROJECT.md`
- `plans/reviewable-multiline-answers.md`
- `docs/decisions/ADR-0008-reviewable-multiline-answers.md`
- `docs/checkpoints/reviewable-multiline-answers-phase-1.md`
- `docs/checkpoints/reviewable-multiline-answers-phase-2.md`

Any additional file requires stopping and amending the approved plan before it
is changed.

## 7. Out of Scope

- New commands, question counts, F2 or free-form evaluator chat.
- Changes to evidence selection, previews, bundles, Go context, model selection,
  evaluator rubric, labels, prompt assets, output contracts, or citations.
- A custom `ctx.ui.custom` editor, `@earendil-works/pi-tui` import, custom
  keybindings, syntax highlighting, Markdown preview, or answer templates.
- Disabling or intercepting Pi's external-editor shortcut; Pi 0.84.3 does not
  expose a per-call switch through `ui.editor`.
- Answer autosave, drafts, recovery, Session entries, extension-owned storage,
  clipboard management, secure memory erasure, or durable assessment resume.
- Database schema or history-response changes; question and answer text remains
  forbidden from persistence.
- A new route, HTTP protocol major, request field, schema version, capability
  negotiation, SSE stream, polling job, worker, retry, or background process.
- A dependency addition, removal, or upgrade; Go remains 1.21 and Pi remains
  exactly 0.84.3 for implemented evaluator compatibility.
- Linux, Windows, web, remote-control, or non-loopback support.
- Fixing unrelated packaging, release automation, macOS AMD64 verification,
  deletion-side evidence, or other discovered issues.

## 8. Proposed Changes

### 8.1 Split user-answer validation from model-output validation

Keep `MaxAnswerTextBytes` at 4,096. Replace the shared use of
`validateAssessmentText` with two private policies behind the existing
constructors/parsers:

- user answers are valid UTF-8, nonblank after `strings.TrimSpace`, at most
  4,096 UTF-8 bytes, and may contain LF (`U+000A`) as their only Unicode control
  rune;
- F1 question text and per-question feedback remain nonblank, bounded, and
  reject every Unicode control rune exactly as they do today.

The rule applies identically to Q1/Q2/Q3 and F1 answers in both
`evaluator-assessment-input@1` and `@2`. CR, CRLF, tabs, NUL, DEL, C0/C1, and
other Unicode control runes remain invalid. The extension continues to apply
JavaScript `trim()` once to an accepted draft; internal LF and all other
non-control content are retained exactly after that existing boundary trim.
There is no silent newline flattening or CRLF normalization.

Tests must prove that LF succeeds for both initial and follow-up inputs, while
CR, tab, other controls, whitespace-only multiline text, invalid UTF-8 at the
Go boundary, and values over 4 KiB fail. Existing tests must continue to prove
that LF in model-produced F1 or feedback is rejected.

### 8.2 Give the existing assessment route a dedicated 32-KiB wire budget

Change only `maxAssessmentRequestBytes` from `16 * 1024` to `32 * 1024`.
`http.MaxBytesReader` remains ahead of allocation-heavy decoding and strict
field validation. Every other endpoint, header limit, timeout, response bound,
authentication rule, error code, and no-cache/no-log rule stays unchanged.

The 32-KiB value covers the official Node client's measured worst normal
`JSON.stringify` envelope for three accepted 4-KiB answers (24,774 bytes), plus
fixed request metadata and margin. It is not a promise to accept arbitrary
noncanonical JSON escaping or whitespace from another local client. A body over
32 KiB still receives the existing `413 request_too_large` before assessment
state is consumed.

Tests must cover an accepted initial request larger than 16 KiB whose decoded
answers remain individually valid, rejection immediately above 32 KiB, and a
valid request after that rejection to prove the awaiting assessment was not
consumed. The body-size change is a bounded, route-specific, server-side
acceptance relaxation under HTTP v1; ADR-0008 records the explicit exception to
ADR-0002's original uniform 16-KiB limit rule.

### 8.3 Keep editing inside the existing command module

Add exactly one method to the injected UI interface:

```text
editor(title, prefill?) → Promise<string | undefined>
```

Do not add a new public answer-editor interface or a custom Pi TUI module. Pi's
real TUI/RPC implementations and existing test fakes are the two adapters at
the already established `LearnCommandContext.ui` seam.

For initial collection:

1. Show one privacy/resource confirmation before opening Q1. It states that
   drafts remain local to the current interaction and are not saved by
   LearnLoop, but Pi's explicit external-editor shortcut writes a temporary
   `prompt.md`, launches the configured editor, and cleanup/editor artifacts
   are outside LearnLoop's guarantee.
2. Open `ui.editor("Answer Q1")`, Q2, and Q3 in fixed order without answer
   prefill on the first visit. The already rendered question set remains the
   question authority; question text is not copied into the answer draft.
3. Trim and validate each submitted draft locally. An invalid candidate is not
   sent, logged, displayed in a notification, or retained as the accepted
   answer. Show only a generic size/control warning and reopen with the previous
   valid bounded answer, or empty content when no valid answer exists.
4. After three valid answers, show a review selector containing only
   `Continue to sharing confirmation`, `Edit Q1`, `Edit Q2`, `Edit Q3`, and
   `Cancel`. Do not place answer text or excerpts in selector labels,
   notifications, errors, or Session entries.
5. Editing an answer opens the same editor with only that already accepted
   bounded answer as prefill. A valid submission replaces it. Cancellation
   preserves the previous answer and returns to the review selector.
6. `Cancel` discards the local collection and sends no assessment request.
   `Continue` reaches the unchanged explicit model/evidence/answer sharing
   confirmation; declining it also sends no assessment request.

For F1, render the validated follow-up as today and open the same bounded
multiline editor. Cancellation or an invalid/no-answer exit sends no follow-up
request. There is no second review selector because the editor already presents
the complete single F1 draft before submission. Existing assessment expiry,
single-consume, fixed-model, and no-retry rules remain unchanged.

### 8.4 Preserve model, Session, and persistence isolation

An accepted multiline answer follows the exact existing data path: extension
memory, authenticated loopback JSON, volatile daemon assessment state, one
isolated no-session/no-tools evaluator input, and Pi-managed provider transport.
LF does not enter evidence, become a Session identifier, change a prompt asset,
or authorize another model call.

LearnLoop must not append an answer to the development Session, save a draft,
log request bodies, persist input/output, or include answer text in history.
`ui.editor` in Pi 0.84.3 swaps a temporary interactive editor component and
returns its string; it does not append a Session entry. In RPC mode, Pi sends
the editor request and optional prefill to the controlling RPC client, which is
already the principal supplying the dialog response; LearnLoop adds no remote
transport or recipient.

Pi's external-editor path and an editor's own files are explicitly outside the
LearnLoop no-persistence guarantee after the user invokes that shortcut.
LearnLoop also cannot prevent Pi's editor process from materializing an
oversized pasted draft before validation. It validates immediately after the
dialog returns, retains only accepted bounded answers, gives no secure-erasure
guarantee for process memory, and adds no further copy.

### 8.5 Keep runtime schemas and prompts unchanged

The assessment request fields, response fields, strict IDs/order, answer count,
4-KiB logical answer bound, assessment input schema versions 1 and 2, turn
schema version 1, prompt identifiers/hashes, rubric, follow-up maximum, and
deterministic final-label mapping remain unchanged.

The checked-in v2 JSON schema already represents answers as bounded strings and
does not declare them single-line. Both released assessment prompts already
treat each answer as bounded untrusted data; their no-control-character rule
applies to model-produced output text. Allowing LF in input therefore requires
no new schema or prompt asset. Changing interpretation, shape, rubric, or model
output would require a separately versioned contract and is forbidden here.

## 9. Compatibility

- Existing single-line answers remain byte-for-byte valid after the current
  extension trim.
- The public `/learn` and `/learn-history` command names and argument behavior
  remain unchanged.
- The strict `POST /v1/assessment-turns` JSON shape, protocol version, status
  codes, and response shape remain unchanged. The server accepts a bounded
  superset of answer values and request sizes; no previously accepted request
  becomes invalid.
- The route-specific 32-KiB limit is a narrow exception to ADR-0002's initial
  uniform 16-KiB rule. All other routes retain their existing limits. A future
  limit reduction, required field, changed meaning, or response incompatibility
  still requires a protocol-version decision.
- An old extension continues to work with a new daemon. A new extension can
  submit a multiline or larger escaped request that an old daemon rejects. It
  must fail closed without retry/fallback and tell the user to update the daemon
  and extension together; it must not flatten or silently resubmit the answer.
- `evaluator-assessment-input@1/@2`, `evaluator-assessment-turn@1`, v1/v2 prompt
  assets, evidence contracts, history schema v2, and Pi Session routes remain
  unchanged.
- No stored-data migration, dependency change, build-baseline change, or
  configuration fallback is introduced.

## 10. Risks

- **Temporary disk disclosure:** invoking Pi's external editor writes the draft
  to a temporary `prompt.md`; cleanup is best effort, and the editor may create
  other artifacts. Mitigation: disclose before Q1, require explicit consent,
  invoke no external editor automatically, and document the limitation.
- **Oversized editor memory:** Pi's editor has no per-call byte cap and may hold
  a large paste before returning. Mitigation: validate immediately, never accept
  or re-prefill an oversized candidate over the prior bounded value, and add no
  extra storage; a custom editor is not justified by current evidence.
- **Terminal/control injection:** broadly permitting controls could disturb UI,
  framing, or downstream parsing. Mitigation: allow LF only for user answers;
  reject CR, tabs, all other controls, and keep model output control-free.
- **Transport amplification:** the assessment route accepts twice the current
  body size. Mitigation: only the authenticated route changes, the bound remains
  32 KiB before decoding, and all strict/security rules remain.
- **Mixed-version installation:** a new extension can meet an old daemon that
  rejects LF or bodies over 16 KiB. Mitigation: fail closed with an update
  action; no retry, flattening, alternate route, or capability guess.
- **Assessment expiry:** review/edit time consumes the existing thirty-minute
  volatile lifetime. Mitigation: do not hide or extend expiry; an unavailable
  assessment still requires a new explicit `/learn` flow.
- **Accidental answer disclosure:** rendering raw drafts in menus, notifications,
  errors, logs, history, or Session entries would expand recipients. Mitigation:
  show raw text only inside the selected editor and model-sharing path.
- **Validator coupling:** changing the existing shared helper could accidentally
  allow multiline model output. Mitigation: separate private answer/output
  policies and retain explicit negative output tests.
- **UI state regression:** cancellation or invalid editing could submit stale or
  partial answers. Mitigation: fixed IDs/order, bounded accepted-value state,
  explicit review actions, and command-level tests through the real interface.
- **Prompt injection in multiline answers:** added lines can contain instruction-
  like content. Mitigation: existing JSON serialization, untrusted-answer prompt
  delimiters, isolated no-tools evaluator, and strict output validation remain.

## 11. Implementation Phases

All implementation phases are high risk. ADR-0008 must be accepted first, each
phase requires explicit authorization, and work stops after each phase for
review, checkpoint, commit, and push only when the user requests them.

### Phase 1 — LF-only answer contract and assessment wire budget

Status: complete on 2026-09-03. See
`docs/checkpoints/reviewable-multiline-answers-phase-1.md`.

Contract:

- split user-answer and model-output text validation inside
  `internal/evaluator` without widening its interface;
- accept internal LF only for initial/F1 user answers under the existing 4-KiB
  UTF-8 bound and reject every other control;
- keep output question/feedback text control-free;
- raise only `maxAssessmentRequestBytes` to 32 KiB before strict decoding;
- verify an accepted >16-KiB canonical initial request, >32-KiB rejection,
  non-consumption on rejection, and initial/F1 LF behavior; and
- add no route, schema/prompt version, dependency, persistence, or extension
  behavior.

On completion, create the Phase 1 checkpoint, advance this plan to Phase 2 with
`phase_status: awaiting_approval`, and stop.

### Phase 2 — Pi editor, answer review, and stable documentation

Status: complete on 2026-09-03. See
`docs/checkpoints/reviewable-multiline-answers-phase-2.md`.

Contract:

- require Pi's existing `editor` method at the current command UI seam;
- add the pre-editor temporary-file/resource disclosure;
- collect Q1/Q2/Q3 and F1 through the bounded editor;
- add the ID-only initial-answer review/edit/cancel loop;
- preserve the existing post-review answer-sharing confirmation, request count,
  model binding, no-retry behavior, and result/history rendering;
- give a safe update action for an old-daemon `invalid_request` mismatch;
- test multiline preservation, editing, cancellation at every boundary, invalid
  draft recovery, answer non-disclosure, old-daemon failure, direct Git and
  Session-bound paths; and
- update `README.md` and `PROJECT.md` to replace the single-line limitation with
  the accepted editor and privacy/resource behavior.

On completion, create the Phase 2 checkpoint, mark this plan complete, and stop.

## 12. Acceptance Criteria

The complete task is accepted only when:

1. Q1/Q2/Q3 and F1 accept internal LF, remain nonblank valid UTF-8, remain at
   most 4,096 bytes each, and reject CR, tab, and every other control rune.
2. Model-produced F1 question and feedback text continue to reject LF and all
   other controls.
3. The assessment route accepts the official client's valid canonical request
   above 16 KiB, rejects a body above 32 KiB before state consumption, and no
   other endpoint limit changes.
4. The initial answers can be individually reopened with their accepted bounded
   text and reviewed through ID-only actions before the existing sharing gate.
5. Invalid drafts never reach the daemon, appear in errors/notifications, or
   replace a prior valid answer; cancel behavior is deterministic and tested.
6. F1 uses the same editor and still permits exactly one non-retried final call.
7. Before Q1, the user is told about Pi's external-editor temporary file,
   best-effort cleanup, external-process artifacts, and oversized-draft limit.
8. No answer is added to the Pi Session, history schema/output, logs, persisted
   files, evidence values, or Session provenance by LearnLoop.
9. Request/response shapes, IDs, prompt/schema versions, evaluator isolation,
   assessment lifecycle, labels, evidence, and source-free history are unchanged.
10. No dependency or Go/Node/Pi version changes.
11. Every changed file is in the phase allowlist and all required verification
    passes or any environment-specific exception is reported without concealment.

## 13. Verification

Draft-document verification for this request:

```text
scripts/test-agent-infra.sh
scripts/validate-agent-infra.sh
git diff --check
git status --short --branch
git diff --stat
git diff
```

Phase 1 focused verification:

```text
gofmt on touched Go files
go test -count=1 ./internal/evaluator ./internal/assessment ./internal/daemon
go test -p=1 -count=1 ./...
go test -race -p=1 -count=1 ./...
go vet ./...
go build ./...
Go 1.21.13 with CGO_ENABLED=0, isolated cache, and serial go test ./...
scripts/test-agent-infra.sh
scripts/validate-agent-infra.sh
git diff --check
complete status/stat/diff review
```

Phase 2 focused and final verification:

```text
npm run typecheck
npm test
npm pack --dry-run --json with an isolated cache
go test -p=1 -count=1 ./...
go test -race -p=1 -count=1 ./...
go vet ./...
go build ./...
Go 1.21.13 with CGO_ENABLED=0, isolated cache, and serial go test ./...
scripts/test-agent-infra.sh
scripts/validate-agent-infra.sh
git diff --check
complete status/stat/diff review
```

Automated tests use only synthetic answers and deterministic/fake evaluator
adapters. They must not invoke a live Pi evaluator, provider, real repository
source, real Session content, production history database, or network service.

## 14. Open Questions

No factual implementation question remains open. The user accepted ADR-0008 on
2026-09-03, including Pi 0.84.3's disclosed external-editor/privacy/resource
limits and the route-specific 32-KiB HTTP v1 relaxation with fail-closed
mixed-version behavior. Both separately authorized phases are complete.
