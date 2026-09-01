---
id: ADR-0004
status: accepted
date: 2026-09-01
supersedes: none
---

# ADR-0004: Volatile Answer Assessment Lifecycle

## Context

Pi LearnLoop currently ends after an isolated evaluator returns three evidence-backed questions. The stable project goal requires the user to answer those questions, permits at most one targeted follow-up, and requires a repository-scoped result of `understood`, `partial`, or `review_needed`.

The existing safe boundary cannot simply be replayed:

- `POST /v1/question-sets` atomically consumes and removes the exact evidence continuation;
- the daemon discards the evaluator input and question set after returning the response;
- the extension is not an authority for rebuilding or resubmitting evidence;
- the question-generation Pi process is intentionally terminated before control returns to the user;
- answers and source can both contain prompt injection;
- every answer-evaluation request can transmit source again and incur provider cost;
- a retry, concurrent submission, or repaired model output could silently create duplicate paid calls;
- persistence and crash recovery do not yet exist.

ADR-0002 continues to govern authenticated IPv4-loopback transport. ADR-0003 continues to govern exact retained evidence, Pi-managed credentials, isolated no-tools processes, strict JSONL/output, and no product retry. This ADR extends those rules to the human answer lifecycle; it does not supersede them.

## Decision

This ADR was accepted on 2026-09-01. Implementation remains subject to the separate phase gates in `plans/answer-assessment-workflow.md`.

### 1. Retain one daemon-owned assessment value after question generation

After a successful question set, the daemon may retain an owned copy of:

- the exact validated `evaluator.Input` built from the preview continuation;
- the exact validated Q1/Q2/Q3 `QuestionSet`;
- the validated non-secret Pi version, provider, model ID, and thinking level used for question generation;
- the assessment stage and, after submission, bounded user answers and an optional validated F1 question.

The retained value contains no repository root, credential, Instance Token, executable path, development Session, or unselected source. The client cannot create, replace, or augment it.

Assessment entries are volatile, daemon-instance-bound, and identified by `as1-` plus 32 random bytes encoded as unpadded base64url. They expire after thirty minutes. At most eight live entries and 1 MiB of aggregate retained evidence excerpts are allowed. Expired entries are purged before insert and submission; a live entry is never evicted to admit another; all entries are cleared on daemon shutdown.

### 2. Extend authenticated HTTP v1 with one assessment-turn capability

The successful `/v1/question-sets` response gains an optional assessment descriptor. Existing clients may ignore it:

```json
{
  "assessment": {
    "available": true,
    "id": "as1-<43 base64url characters>",
    "expires_at": "2026-09-01T12:30:00Z"
  }
}
```

If the question set is insufficient, volatile assessment capacity is unavailable, or the production assessment evaluator is unavailable, no state is retained and the descriptor is unavailable with stable reason `insufficient_evidence`, `capacity`, or `evaluator_unavailable`.

One new strict authenticated route, `POST /v1/assessment-turns`, accepts either the initial answers:

```json
{
  "assessment_id": "as1-<43 base64url characters>",
  "stage": "initial_answers",
  "answers": [
    {"question_id":"Q1","text":"<answer>"},
    {"question_id":"Q2","text":"<answer>"},
    {"question_id":"Q3","text":"<answer>"}
  ]
}
```

or one follow-up answer:

```json
{
  "assessment_id": "as1-<43 base64url characters>",
  "stage": "follow_up_answer",
  "follow_up_id": "F1",
  "answer": "<answer>"
}
```

Each answer is non-empty bounded UTF-8 with a 4-KiB maximum. The route never accepts evidence, questions, model selection, prompt version, credentials, repository paths, or an overall label from the client.

Unknown, expired, completed, failed, wrong-instance, malformed, and concurrently consumed IDs deliberately share `409 assessment_unavailable`. Evaluator failures reuse ADR-0003's safe bounded error categories. The daemon and extension never automatically retry this route.

### 3. Use an atomic two-stage state machine

The allowed state transitions are:

```text
awaiting_answers
→ evaluating_initial
→ complete
   or awaiting_follow_up
→ evaluating_follow_up
→ complete
```

Submitting a stage atomically leaves its awaiting state before an evaluator process starts. Concurrent or replayed submissions cannot start a second evaluator. A failure after that transition invalidates the assessment; it does not become retryable. A final result removes the volatile entry after constructing the response.

No transition returns from `evaluating_*` to an awaiting state. No completed, failed, or expired assessment is resumable in this pre-persistence design.

### 4. Keep assessment schemas independent and strict

`evaluator-input@1` and `evaluator-question-set@1` remain unchanged. Answer evaluation introduces independent `evaluator-assessment-input@1` and `evaluator-assessment-turn@1` runtime values.

Source evidence and user answers are separately delimited untrusted data. The first evaluator turn may return either:

- exactly one targeted follow-up `F1`, bound to one original question and valid evidence references; or
- a complete result with exactly Q1/Q2/Q3 evaluations in order.

The final evaluator turn may return only a complete result. It cannot request F2 or another follow-up.

Each complete question evaluation contains one verdict from `demonstrated`, `partial`, or `not_demonstrated`, concise feedback, and validated evidence references. Code-specific feedback requires evidence; output with unknown references, extra fields, invalid order, unbounded text, duplicate keys, free-form prose, or trailing content fails closed. No repair call is made.

### 5. Derive the public label in Go

The evaluator does not choose the public overall label. Go derives it exhaustively from the three validated verdicts:

```text
all three demonstrated                 → understood
at least two not_demonstrated          → review_needed
every other valid combination          → partial
```

This prevents prompt wording or an untrusted answer from changing the aggregation rule. The mapping becomes compatibility-sensitive when implemented.

### 6. Start a new isolated Pi process for every model turn

No Pi process remains alive while the user reads or enters answers. Initial answer assessment and optional follow-up assessment each start a new Pi 0.84.3 process with ADR-0003's frozen executable, no-session/no-tools deny arguments, disabled retry/compaction, empty command discovery, stream bounds, deadline, cancellation, and guaranteed termination/reaping.

Every turn uses the fixed model selection captured at question generation. The client cannot switch model settings between turns. Pi continues to own credentials; credentials never enter daemon HTTP, argv, prompts, logs, persisted files, or model-visible content.

The assessment evaluator has its own narrow interface. The production Pi adapter may share private RPC mechanics with question generation, but no general process-control interface is published.

### 7. Require explicit answer-sharing interaction

The extension first renders the three questions and collects all three answers locally. Cancellation or an empty answer sends nothing. Before the initial assessment request, it discloses that the same selected excerpts and the entered answers will reach the fixed configured model, that the request may incur provider cost, and that answering an optional follow-up can cause one additional evaluation. External Pi/provider transport retry behavior remains disclosed as in ADR-0003.

If F1 is returned, displaying it and explicitly submitting its answer authorizes the one final assessment call. The extension renders the daemon-provided validated feedback and Go-derived label; it does not implement the rubric or state machine.

### 8. Do not persist or recover this slice

Assessment state, source, questions, answers, prompts, RPC streams, model output, and feedback remain in memory and are not written to SQLite, files, Session transcripts, logs, telemetry, or development run records. A daemon restart, expiry, lost HTTP response, or evaluator failure requires a new `/learn` flow.

Durable recovery and learning history require a later storage ADR and plan. That work must define an explicit persisted schema rather than serializing these internal runtime values.

## Alternatives

### Keep one Pi RPC process alive across user input

Rejected because human response time would create a long-lived child with provider/session state, complicate cancellation and shutdown, consume resources, and weaken the existing guarantee that every evaluator process is bounded and reaped.

### Let the extension resubmit the preview, bundle, or questions

Rejected because the client could substitute evidence, exceed server-owned bounds, or diverge from the exact bytes previously reviewed. It would also retransmit source through the local protocol unnecessarily.

### Reuse the active development Session as the interview

Rejected because it contains unrelated transcript/context and may expose tools, resources, credentials, or project state. User answers do not justify weakening evaluator isolation.

### Let the model emit the final overall label directly

Rejected because the same three per-question verdicts could produce inconsistent labels across prompts or providers. A small deterministic aggregation rule provides a stable product contract while retaining model judgment only where semantic assessment is unavoidable.

### Add SQLite and durable jobs before answer collection

Rejected for this task because storage schema, migrations, cleanup, crash recovery, retry policy, and compatibility would obscure the smallest useful answer loop. The volatile lifecycle intentionally exposes where durability is later required.

### Allow retry after timeout or invalid model output

Rejected because the original call may have reached the provider even when its local result is lost. Automatic retry could duplicate cost and return inconsistent feedback. The user must start a new visible `/learn` flow.

### Ask all possible follow-ups in the first question set

Rejected because a targeted follow-up is valuable only after observing an ambiguous answer. Expanding the initial fixed question count would also break ADR-0003's question-set contract.

## Consequences

- Pi LearnLoop gains a complete volatile learning interaction without waiting for database design.
- The daemon, not TypeScript or the model, owns state transitions, bounds, model binding, and the final label.
- One completed `/learn` can cause one question-generation call, one initial assessment call, and at most one follow-up assessment call, plus any external provider transport retry outside Pi LearnLoop's control.
- Selected evidence remains in daemon memory longer than the current five-minute preview continuation, bounded to thirty minutes and 1 MiB aggregate.
- A restart, expiry, network response loss, or evaluator failure loses the assessment and cannot be recovered until a later persistence plan.
- The first UI uses Pi 0.84.3's string input dialog; richer multiline editing is not guaranteed.
- The new v1 fields, route, answer bounds, turn ordering, verdicts, follow-up maximum, and label mapping become compatibility commitments after implementation.
- Future SQLite work has a concrete validated state machine to persist, but must define its own durable schema and migration policy.
