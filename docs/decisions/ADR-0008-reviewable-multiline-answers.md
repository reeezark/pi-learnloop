---
id: ADR-0008
status: accepted
date: 2026-09-03
supersedes: none
---

# ADR-0008: Reviewable Multiline Answer Editing

## Context

Pi LearnLoop currently collects exactly Q1/Q2/Q3 and optional F1 with Pi
0.84.3's single-line `ctx.ui.input` dialog. The extension trims and rejects an
empty, over-4-KiB, or control-bearing answer, then sends the accepted answers
only after an explicit model-sharing confirmation. Users cannot reopen an
accepted answer or inspect all three answer slots before that confirmation.

Pi 0.84.3 publicly declares `ctx.ui.editor(title, prefill?)` for multiline text
editing. Its TUI implementation returns an in-memory string without appending a
Session entry, supports cancellation and prefill, and exposes an external-editor
shortcut. RPC mode represents the editor and its optional prefill as a blocking
UI request to the controlling client.

The external-editor implementation creates an OS-temporary
`pi-editor-*/prompt.md`, starts the user's configured editor on that path, reads
the result, and attempts recursive removal in `finally`; cleanup is explicitly
best effort. The external editor may independently create swap, backup, recovery,
history, or telemetry artifacts. The built-in editor also exposes no per-call
input cap, so a large paste can be materialized in the extension process before
LearnLoop validates it.

ADRs 0003–0007 require user answers and source-bearing assessment state to
remain volatile, prevent Session or answer content from entering source-free
history, isolate every evaluator turn in a fresh no-session/no-tools Pi process,
and forbid product retry. Those guarantees must remain. ADR-0002 also selected
a 16-KiB request-body limit and stated that changing fixed limit semantics
normally requires a protocol major.

The current assessment route's 16-KiB limit is insufficient for its already
documented three answers of up to 4 KiB each after JSON escaping. The official
Node client's canonical request is about 12,486 bytes for three plain 4,096-byte
answers and 24,774 bytes for the same amount of quotes, backslashes, or LF.
Multiline support therefore requires an explicit wire-budget decision rather
than relying on decoded-content bounds alone.

## Decision

### 1. Use Pi's existing editor and keep review inside the command module

The `/learn` answer interaction will use Pi 0.84.3's public `ui.editor` for
Q1/Q2/Q3 and F1. The existing `learn-command` module will own accepted drafts,
validation feedback, initial-answer review, editing, cancellation, and the
transition to the existing model-sharing confirmation.

After three valid answers, the user can continue, edit Q1, Q2, or Q3, or cancel.
The review selector contains only those fixed actions and question IDs. It does
not display answer text. Editing one answer prefills only that accepted bounded
answer. Cancelling an edit preserves the previous value; cancelling the review
discards the collection and sends no assessment request.

F1 uses the same editor but no separate review selector. Submitting it still
authorizes at most one final evaluator call. No editor action starts an Agent
turn, changes the fixed model, retries a request, refreshes assessment expiry,
or adds another follow-up.

No custom TUI module or answer-editor interface will be introduced. Pi's TUI/RPC
editor implementations and the existing command test fakes remain the real
adapters at the already established `LearnCommandContext.ui` seam. The command
module stays deep: callers do not coordinate draft state, validation, review
order, or cancellation.

### 2. Permit LF only in bounded user answers

Q1/Q2/Q3 and F1 answers remain nonblank valid UTF-8 and at most 4,096 UTF-8
bytes each. After the extension's existing JavaScript `trim()` boundary, an
answer may contain LF (`U+000A`) as its only Unicode control rune. Internal LF
is retained exactly. CR, CRLF, tab, NUL, DEL, C0/C1, and every other Unicode
control rune remain invalid. LearnLoop does not silently flatten or normalize
line endings.

User-answer validation will be separated privately from model-output text
validation. Model-produced F1 questions and feedback remain control-free.
Invalid or oversized editor results are not sent, logged, shown in a
notification, or installed as accepted state; the editor reopens with only the
previous accepted bounded answer, if one exists.

This applies to both `evaluator-assessment-input@1` and `@2`. Their shapes,
schema versions, 4-KiB logical bound, answer IDs/order, and evaluator meaning do
not change. Existing assessment prompts already treat answers as bounded
untrusted data, while their no-control-character output rule governs generated
text. No prompt or output-contract version changes.

### 3. Raise only the assessment-turn wire limit to 32 KiB

`POST /v1/assessment-turns` will read at most 32 KiB before strict JSON decoding.
This covers the official Node client's canonical worst normal encoding of three
accepted 4-KiB answers plus the fixed envelope. Bodies above the limit retain
the existing `413 request_too_large` behavior and do not consume assessment
state.

Every other route remains at its existing body limit. Authentication, exact
Host/Origin/peer checks, no-CORS/no-cache rules, strict fields, response shapes,
timeouts, safe errors, and no request-body logging remain unchanged.

This is an explicit, route-specific exception to ADR-0002's initial rule that a
fixed-limit change requires protocol v2. It is accepted under HTTP v1 because it
only permits a bounded superset: no previously valid request becomes invalid,
no field or response meaning changes, and existing clients require no change.
A future reduction, required field, changed meaning, or incompatible response
still requires a versioned protocol decision.

An old extension works with the new daemon. A new extension using LF or a
larger escaped body can be rejected by an old daemon. It must fail closed, make
the update action explicit, and must not flatten, retry, fall back, or resubmit
the answer through another route.

### 4. Disclose Pi's external-editor and memory limits before Q1

Before the first answer editor opens, the extension will state that LearnLoop
does not save drafts, but invoking Pi's external-editor shortcut writes the
current draft to a temporary `prompt.md`, starts the configured external
process, and relies on best-effort cleanup. It will also state that the editor
or operating environment may retain other artifacts outside LearnLoop's
control. Declining stops answer collection and sends no answer.

LearnLoop never invokes that shortcut automatically and adds no file, Session
entry, clipboard operation, draft store, recovery file, background process, or
dependency. It cannot promise secure memory erasure or prevent Pi from holding
an oversized draft before the editor returns. It validates immediately, retains
only accepted bounded strings, and makes no additional copy beyond the existing
UI/request/evaluator path.

In RPC mode, Pi forwards the editor title and optional prefill to the controlling
RPC client. That client already supplies the dialog response; this decision adds
no network endpoint or new principal. Answer text remains absent from evidence,
Pi Session provenance, notifications, errors, logs, generic history, and the
SQLite schema.

### 5. Preserve assessment and model isolation

Accepted answers continue through the authenticated loopback route into the
daemon-owned volatile assessment and one fresh isolated Pi evaluator process.
The evidence, prompt version, fixed model selection, provider-cost confirmation,
atomic stage transition, thirty-minute expiry, at-most-one F1, deterministic
label, source-free history, and no-retry behavior remain unchanged.

Multiline content is untrusted answer data, not instructions. It grants no tool,
filesystem, Session, process, network, credential, repository, or evidence
capability. The existing strict model-output parser remains the only result
authority.

## Alternatives

### Keep the one-shot single-line input

Rejected because it leaves the documented richer-editing gap and makes it hard
to explain multi-step Go behavior or revise earlier answers before sharing.

### Build a custom editor with `ctx.ui.custom`

Rejected. It would require LearnLoop to own keyboard, focus, rendering, IME,
external-editor, cancellation, accessibility, and RPC differences. Custom UI is
not supported in Pi RPC mode and would create a large shallow interface for a
capability Pi already provides. It is worth reconsidering only if the disclosed
temporary-file or pre-validation memory limits become unacceptable.

### Import `@earendil-works/pi-tui` and disable external editing

Rejected because it adds a direct dependency and couples LearnLoop to Pi's TUI
implementation solely to remove one explicitly user-invoked option. It would
also lose the real RPC adapter and expand compatibility testing.

### Add multiline editing without a review step

Rejected because it solves line breaks but not the user's inability to revisit
Q1 after answering Q2/Q3. A fixed ID-only review loop adds useful leverage
without exposing answer text or changing daemon state.

### Display answer excerpts in the review menu or Session transcript

Rejected because answer text would be duplicated into a UI/history surface not
required to edit it. The selected editor is the only local raw-draft display.

### Persist or autosave answer drafts

Rejected. It requires a new sensitive schema, retention, cleanup, migration,
recovery, and encryption decision and contradicts the accepted source-free
history design. Assessment expiry and uncertain provider calls also prevent
safe automatic resume.

### Allow all whitespace or control characters

Rejected. Tabs, CR, C0/C1, and other controls add terminal, line-ending, framing,
and ambiguity risks without being necessary for explanatory prose. LF alone is
the smallest useful multiline expansion.

### Flatten newlines before submission

Rejected because it changes user-authored structure without consent and would
make the editor's multiline affordance dishonest.

### Keep 16 KiB and add an aggregate client-only limit

Rejected because it leaves an existing mismatch between three valid logical
answers and the official client's escaped wire representation. Server and
client acceptance would diverge, and failures would depend on answer characters
rather than the documented 4-KiB per-answer contract.

### Add a new route or protocol v2

Rejected for this bounded relaxation. The request/response shape and meaning do
not change, old clients remain valid, and a separate route would duplicate the
same assessment state machine. A new protocol major would force unrelated
preview, question, history, and Session clients to migrate.

### Implement SSE or durable workers first

Rejected for this capability. Streaming would add progress/disconnect/cancel
interfaces, while durable resume would require persisting source-bearing
assessment inputs and resolving uncertain provider-call outcomes. Neither is
needed to keep local answer editing behind the current sharing gate.

## Consequences

- Users can write structured multiline explanations and revise Q1/Q2/Q3 before
  any answer is shared with the configured model.
- The command module gains leverage while preserving its small caller interface;
  no new answer subsystem or custom TUI implementation is introduced.
- LF becomes an additive accepted input value for existing assessment schema
  versions, while model-produced output remains control-free.
- The assessment route's authenticated pre-decode exposure increases from 16 to
  32 KiB; all other local HTTP bounds and security controls remain unchanged.
- Mixed extension/daemon versions can fail closed for multiline or large escaped
  requests and require an explicit matched update; no compatibility fallback is
  provided.
- Drafts remain volatile in LearnLoop, but explicitly invoking Pi's external
  editor can write them to an OS temporary file and expose them to the configured
  editor. Cleanup and editor-created artifacts cannot be guaranteed by
  LearnLoop.
- Pi may materialize an oversized paste before LearnLoop can reject it, and
  process memory is not securely erased. LearnLoop retains only accepted
  bounded values after the dialog returns.
- Longer editing may consume the existing thirty-minute assessment lifetime;
  expiry still requires a new manual `/learn` flow.
- No dependency, database migration, prompt/schema asset, evidence behavior,
  Session provenance, label rule, background worker, retry, or model capability
  is added.
- Accepting this ADR does not authorize implementation. Each high-risk phase in
  `plans/reviewable-multiline-answers.md` requires separate explicit
  authorization.
