---
id: answer-assessment-workflow
status: complete
risk: high
current_phase: 3
phase_status: complete
updated: 2026-09-01
---

# Answer Assessment Workflow

## 1. Goal

Complete the first interactive learning loop after the existing three-question result: collect one answer for each fixed question, evaluate those answers against the exact retained evidence, optionally ask at most one targeted follow-up, and return a deterministic repository-scoped label of `understood`, `partial`, or `review_needed` with concise evidence-backed feedback.

This task deliberately stops before durable learning history. It proves the answer and assessment state machine without SQLite, background jobs, Session indexing, or automatic reminders.

## 2. Background

The completed `post-preview-evaluator-adapter` plan implements the safe flow from an explicit Git selection through preview, confirmation, isolated Pi RPC question generation, and three-question rendering. The user cannot yet submit answers or receive an assessment, so the product does not satisfy the core learning-loop goals recorded in `PROJECT.md`.

Adding answers crosses new high-risk seams:

- user-authored answers become model-visible untrusted data;
- the daemon must retain the exact evaluator input after question generation without asking the client to resubmit source;
- a new authenticated protocol operation can trigger one or two additional paid model calls;
- concurrent or retried submissions must not duplicate evaluation;
- follow-up eligibility and the final label must be deterministic enough to remain a compatibility commitment;
- the extension must collect answers without moving assessment rules into TypeScript;
- a model process must not remain alive while waiting for human input.

ADR-0004 was accepted on 2026-09-01. All three implementation phases are complete.

## 3. Current Behavior

Verified on 2026-09-01 from the repository, the synchronized CodeGraph index, and the locally installed Pi 0.84.3 declarations:

- `createLearnCommand` collects a Git selection, renders the bounded preview, obtains explicit confirmation, calls `LearnClient.questions`, renders the returned `QuestionSet`, and returns.
- Pi 0.84.3 exposes `ctx.ui.input(title, placeholder)` returning `Promise<string | undefined>`. The current public type guarantees a string result but does not promise a multiline editor.
- `DaemonEvidenceClient.questions` sends only one opaque continuation ID and validated non-secret model metadata to strict authenticated `POST /v1/question-sets`; it never retries the request.
- `handleQuestionSet` atomically consumes the five-minute preview continuation, builds `evaluator.Input` from that exact retained evidence, runs `QuestionEvaluator.Evaluate`, returns the strict question set, and then loses the input and question state.
- `evaluator.Input` owns the selected source-bearing EvidenceBundle without a repository root. `QuestionSet` owns fixed Q1/Q2/Q3 questions and evidence references.
- `PiRPCEvaluator.Evaluate` creates one no-session/no-tools Pi process, sends one runtime input, validates one final assistant JSON object, and always terminates and reaps the process.
- The daemon retains no value after a successful question response. There is no assessment identifier, answer schema, follow-up schema, assessment evaluator interface, label aggregation, or answer UI.
- The evaluator development schema already supports a synthetic `user_answer` field, and existing cases cover evidence fidelity, insufficiency, prompt injection, and strict output. They are development fixtures, not runtime product schemas.
- No database, SSE stream, durable job, or release migration exists.

## 4. Relevant Call Chain

Implemented flow:

```text
/learn
→ explicit Git selection
→ POST /v1/evidence-previews
→ exact bounded evidence retained for five minutes
→ visible preview and confirmation
→ POST /v1/question-sets
→ atomic continuation consume
→ BuildBundle → evaluator.NewInput
→ isolated Pi RPC question generation
→ strict QuestionSet
→ extension renders Q1/Q2/Q3 and returns
```

Proposed extension of that flow:

```text
successful QuestionSet
→ daemon assessment module retains owned evaluator input,
  question set, and fixed non-secret model selection
→ response includes an optional opaque assessment descriptor
→ extension collects Q1/Q2/Q3 answers locally
→ user confirms answer/evidence sharing and possible model cost
→ POST /v1/assessment-turns (initial_answers)
→ atomic awaiting_answers → evaluating transition
→ new isolated Pi RPC assessment process
→ strict complete result OR one targeted follow-up
→ if follow-up: retain the same assessment state and collect F1 answer
→ POST /v1/assessment-turns (follow_up_answer)
→ atomic awaiting_follow_up → evaluating transition
→ new isolated Pi RPC assessment process
→ strict complete result
→ Go derives understood / partial / review_needed
→ extension renders feedback; volatile assessment state is removed
```

The proposed `internal/assessment` module is the deep module. Its small interface owns copying, TTL/capacity, lifecycle validation, atomic submission, evaluator invocation, and deterministic label aggregation. The daemon maps authenticated HTTP to that interface; the extension only collects and renders values.

## 5. Relevant Files

- `AGENTS.md`: high-risk authorization, phase gates, allowed-file scope, verification, and commit/checkpoint rules.
- `PROJECT.md`: learning-loop goals, thin-extension rule, evaluator isolation, local-data policy, and deferred persistence.
- `plans/post-preview-evaluator-adapter.md`: completed predecessor and explicit requirement for a new answer-workflow plan.
- `docs/checkpoints/post-preview-evaluator-adapter-phase-3.md`: current verified handoff and do-not-change constraints.
- `docs/decisions/ADR-0002-local-daemon-protocol-security.md`: authenticated loopback v1 transport and compatibility rules.
- `docs/decisions/ADR-0003-post-preview-evaluator-boundary.md`: exact retained evidence, single-use continuation, Pi isolation, strict schemas, and no-retry rules.
- `internal/evaluator/contract.go`: current source-bearing input and strict question-set contracts.
- `internal/evaluator/evaluator.go`: current `QuestionEvaluator` seam and deterministic adapter.
- `internal/evaluator/pi_rpc.go`: frozen executable, deny arguments, bounded JSONL process lifecycle, and strict output extraction.
- `internal/daemon/server.go`: current question endpoint and loss of state after its response.
- `internal/daemon/continuation.go`: existing owned-copy, TTL, capacity, and atomic-consume pattern.
- `internal/daemon/daemon.go`: current runtime composition and shutdown cleanup.
- `extensions/lib/learn-command.ts`: current terminal interaction and final question rendering.
- `extensions/lib/daemon-client.ts`: strict authenticated client and response validation.
- `agent/policies/evaluator-capabilities.json`: deny-by-default evaluator constraints.
- `agent/prompts/README.md`: immutable prompt version and untrusted-answer requirements.
- `agent/evals/README.md` and `agent/evals/cases/`: existing synthetic assessment-oriented fixtures.
- Pi 0.84.3 `dist/core/extensions/types.d.ts`: locally pinned `ui.input` interface verified during investigation.

## 6. Scope

- Add a separately versioned runtime input and output contract for answer assessment.
- Add a separate assessment-evaluator interface at the existing evaluator seam, with deterministic and Pi RPC adapters.
- Add one deep in-memory assessment module that owns exact state, bounds, atomic transitions, and deterministic label aggregation.
- Add an optional assessment descriptor to successful question-set responses and one strict authenticated assessment-turn route.
- Bind an assessment to the exact evaluator input, question set, model selection, daemon instance, and prompt/schema versions established when questions were generated.
- Collect exactly three non-empty bounded answers through the Pi extension.
- Permit at most one evidence-backed follow-up and one bounded answer to it.
- Derive the public final label in Go from three validated per-question verdicts rather than trusting a model-selected overall label.
- Preserve the existing no-tools, no-session, Pi-managed credential, no-product-retry, bounded-output, termination, and reaping guarantees for every assessment call.
- Add deterministic, adversarial, concurrency, expiry, cancellation, and fake-Pi tests without contacting a provider.
- Update stable documentation and create one checkpoint per completed phase.

## 7. Out of Scope

- SQLite, durable jobs, leases, crash recovery, event cursors, learning history, or migration formats.
- SSE, polling orchestration, background workers, daemon autostart, or automatic reminders.
- Resuming an assessment after daemon restart, lost HTTP response, expiry, or failed evaluator process.
- Pi Session selection or association, type/dependency enrichment, additional repository reads, or expanded evidence.
- More than three initial questions or more than one follow-up.
- Free-form chat with the evaluator, hints, coding exercises, spaced repetition, or automatic remediation content.
- Model/provider switching after question generation.
- Live provider calls in automated tests, product retries, or repair calls.
- Persisting source, answers, prompts, RPC streams, model output, credentials, or Session transcripts.
- New dependencies, package publication, CI/CD, release automation, or unrelated refactoring.

## 8. Proposed Changes

### 8.1 Add independent assessment runtime contracts

Keep `evaluator-input@1` and `evaluator-question-set@1` unchanged. Add an `evaluator-assessment-input@1` value containing:

- the owned `evaluator.Input` produced from the exact preview;
- the validated original `QuestionSet`;
- a stage of `initial_answers` or `follow_up_answer`;
- exactly Q1/Q2/Q3 bounded answers for the initial stage;
- for the follow-up stage, the original answers, the validated F1 question, and one bounded F1 answer.

Answers are untrusted UTF-8 data, not prompt instructions. Each answer is non-empty after trimming, contains no forbidden control characters, and is limited to 4 KiB; total initial-answer content is therefore at most 12 KiB. The follow-up answer is also limited to 4 KiB.

Add an `evaluator-assessment-turn@1` result with exactly one of:

- `follow_up`: one F1 question targeted to one original question and grounded in valid bundle references;
- `complete`: exactly one evaluation for Q1, Q2, and Q3, each with `demonstrated`, `partial`, or `not_demonstrated`, concise feedback, and validated evidence references.

The follow-up stage accepts only `complete`. Strict parsing rejects duplicate/unknown fields, invalid UTF-8, oversized text, unknown references, wrong IDs/order, a second follow-up, free-form prose, or trailing content.

### 8.2 Keep the final label deterministic

The model does not select the public assessment label. After a strict complete turn, Go derives:

- `understood` when all three verdicts are `demonstrated`;
- `review_needed` when at least two verdicts are `not_demonstrated`;
- `partial` for every other valid combination.

This mapping is a compatibility rule accepted in ADR-0004. The daemon returns the derived label plus the validated per-question feedback. Changing the mapping later requires compatibility review.

### 8.3 Add one deep assessment module

Create `internal/assessment` with a small caller interface similar in responsibility to:

```text
Start(exact input, question set, fixed model selection) → descriptor
Submit(context, assessment ID, one typed submission) → follow-up or final result
Close()
```

The interface intentionally accepts no repository path, source supplied by the client, credential, prompt text, or executable path. Its implementation owns defensive copies, random IDs, stage transitions, evaluator invocation, expiry, capacity, cleanup, and label aggregation. Tests cross the same interface used by the daemon.

Proposed volatile limits are:

```text
assessment lifetime:                  30 minutes
maximum live assessments:             8
maximum retained evidence excerpts:   1 MiB aggregate
answer limit:                          4 KiB each
identifier:                            as1- + 32 random bytes as base64url
```

Expired entries are purged before insert and submit. An unexpired entry is never evicted to admit another. Capacity prevents answer continuation but does not discard or hide the already generated questions.

### 8.4 Extend authenticated v1 additively

A successful question-set response may add:

```json
{
  "assessment": {
    "available": true,
    "id": "as1-<43 base64url characters>",
    "expires_at": "2026-09-01T12:30:00Z"
  }
}
```

When questions are insufficient, volatile capacity is unavailable, or the production assessment evaluator is unavailable, no state is retained and the descriptor reports `insufficient_evidence`, `capacity`, or `evaluator_unavailable`. Existing clients can ignore this additive field.

Add strict authenticated `POST /v1/assessment-turns`. Initial submission carries only the opaque assessment ID, stage, and fixed Q1/Q2/Q3 answers. Follow-up submission carries only the same ID, stage, F1 ID, and F1 answer. It never accepts evidence, questions, model selection, prompt version, repository paths, or credentials from the client.

Unknown, expired, completed, failed, wrong-instance, malformed, and concurrently consumed IDs share `409 assessment_unavailable`. Existing safe evaluator failure codes remain reusable. Requests are never automatically retried by the extension or daemon.

### 8.5 Make every human-wait boundary process-free

Do not keep the question-generation Pi process or an assessment Pi process alive while waiting for user input. Each model turn starts a fresh isolated Pi 0.84.3 process with the already accepted deny flags and pre-prompt checks, sends one assessment envelope, validates one final assistant object, and terminates/reaps the process.

The assessment evaluator is a separate interface from `QuestionEvaluator`, but the production Pi adapter should share the existing private process-isolation implementation instead of publishing a broader process interface. Deterministic and fake-process adapters provide the real testing seam.

### 8.6 Keep the Pi extension thin and explicit

After rendering Q1/Q2/Q3, the extension collects one concise answer per question with `ctx.ui.input`. Cancellation or an empty answer stops locally without a submission. Before the initial assessment request, confirmation states that the same selected excerpts and the entered answers will be sent to the fixed configured model, one evaluation will occur, and an answered follow-up may cause one additional evaluation and provider cost.

If the first result is a follow-up, the extension renders F1, collects one answer, and submits it once. A complete result renders the derived label and per-question feedback. The extension does not calculate labels, decide follow-up eligibility, retain source, or start an Agent turn.

The initial UI may be single-line because Pi 0.84.3 only guarantees a string-returning `ui.input` dialog. Multiline/custom-editor work is deferred unless manual Phase 2 smoke proves it is required for usable answers.

### 8.7 Preserve privacy and fail closed

- Treat both source evidence and user answers as independently delimited untrusted data.
- Send only the exact retained evaluator input, questions, answers, and non-secret provenance required by the runtime schema.
- Keep credentials Pi-managed and out of HTTP, argv, prompts, errors, logs, fixtures, and persistence.
- Never log or persist raw assessment input/output.
- On evaluator failure after an atomic stage transition, invalidate the volatile assessment rather than silently repeat a potentially paid call.
- No automatic repair call is permitted for invalid model output.

## 9. Compatibility

- `/learn`, existing selection behavior, preview fields, `/v1/question-sets` request semantics, `evaluator-input@1`, `evaluator-question-set@1`, and the released question-generation prompt remain unchanged.
- The optional question-response assessment descriptor and new authenticated route are additive v1 capabilities under ADR-0002/0003. Existing clients must continue to accept question results while ignoring the new field.
- Assessment IDs are opaque, instance-bound, volatile capabilities and are not persisted identifiers.
- Assessment schema version 1, turn ordering, answer limits, verdict values, follow-up maximum, and final-label mapping become compatibility-sensitive once accepted and implemented.
- The model selection captured during question generation remains fixed for every later turn. The client cannot switch provider, model, thinking level, Pi version, or prompt.
- There is no stored-data migration in this plan. A later persistence plan must introduce a separate durable schema and may not serialize internal Go structs by accident.

## 10. Risks

- **Duplicate cost:** concurrent or retried answer submissions could start multiple model calls. Mitigation: atomic stage transition before spawn and no retry.
- **State loss:** a daemon restart, expiry, or lost response destroys the assessment. This is accepted only for the pre-persistence slice and must be disclosed as a known limitation.
- **Source retention:** evidence remains in memory up to thirty minutes. Mitigation: fixed count/byte caps, owned values, no eviction, expiry purge, and shutdown clearing.
- **Prompt injection:** source and answers can contain instructions. Mitigation: independent untrusted-data delimiters, empty tools, released prompt rules, eval fixtures, and strict output validation.
- **Self-grading bias:** a model may over-credit plausible wording. Mitigation: per-question verdicts, evidence references, synthetic adversarial fixtures, concise feedback, and deterministic Go aggregation.
- **Follow-up inflation:** the model may request unnecessary follow-ups. Mitigation: at most one F1, strict eligibility prompt, fixed schema, and no follow-up during the final stage.
- **Provider cost:** completing one `/learn` can use one question call, one initial assessment call, and one optional follow-up assessment call. Confirmation must disclose this upper bound and external provider retry behavior.
- **Single-line UX:** Pi's guaranteed input dialog may constrain detailed answers. Mitigation: bounded concise answers first; custom UI is a separate UX decision.
- **Protocol complexity:** adding stateful turns expands the authenticated route surface. Mitigation: one route, explicit stages, strict schemas, opaque IDs, and daemon-owned context.
- **Premature persistence coupling:** designing around future SQLite could distort the minimal state machine. Mitigation: keep durable records outside this task and define them from the validated completed workflow later.

## 11. Implementation Phases

All phases are high risk. Each requires explicit authorization, its own checkpoint, complete verification, a separate commit, and a push to `origin/main` under the user's standing workflow instruction.

### Phase 1 — Assessment contracts, rubric, and deterministic evaluator

Status: complete on 2026-09-01. See `docs/checkpoints/answer-assessment-workflow-phase-1.md`.

Goal: accept ADR-0004 and implement the provider-independent runtime schemas, deterministic label rule, draft assessment prompt, and synthetic validation without daemon or extension behavior.

Prerequisites:

- review and accept or revise ADR-0004;
- explicitly authorize Phase 1;
- resolve every blocking open question in section 14.

Allowed files:

- `internal/evaluator/assessment_contract.go`
- `internal/evaluator/assessment_contract_test.go`
- `internal/evaluator/assessment_evaluator.go`
- `internal/evaluator/assessment_evaluator_test.go`
- `agent/prompts/evaluator-answer-assessment/v1.0.0.md`
- `agent/prompts/README.md`
- `agent/evals/README.md`
- new assessment-specific files under `agent/evals/cases/`
- `agent/README.md` only if its authoritative asset table must describe the new draft
- `PROJECT.md`
- `plans/answer-assessment-workflow.md`
- `docs/decisions/ADR-0004-answer-assessment-lifecycle.md`
- `docs/checkpoints/answer-assessment-workflow-phase-1.md`

Forbidden changes:

- daemon routes, stores, runtime composition, or HTTP fields;
- extension UI/client behavior;
- Pi process spawning or production prompt embedding;
- released question prompt edits, dependencies, persistence, SQLite, SSE, or live model calls.

Acceptance criteria:

- inputs own copies of validated evidence, questions, and answers without repository roots or credentials;
- initial and follow-up stages have strict distinct invariants;
- output accepts exactly one follow-up or exactly three ordered per-question verdicts;
- the final stage cannot return a second follow-up;
- every evidence reference is validated against the retained input;
- answer, question, feedback, and result bounds are deterministic and tested;
- Go derives the stable final label from verdicts with exhaustive table tests;
- synthetic cases cover unsupported answers, answer prompt injection, over-crediting, necessary/unnecessary follow-up, malformed output, and final-stage follow-up rejection;
- no product entry point can invoke the new evaluator.

### Phase 2 — Volatile assessment lifecycle and deterministic end-to-end flow

Status: complete on 2026-09-01. See `docs/checkpoints/answer-assessment-workflow-phase-2.md`.

Goal: implement the in-memory assessment module, additive authenticated protocol, answer UI, and deterministic end-to-end tests while production assessment remains unavailable until Phase 3.

Prerequisites:

- Phase 1 checkpoint complete;
- explicit Phase 2 authorization.

Expected allowed files:

- new files under `internal/assessment/`
- focused changes and tests under `internal/daemon/`
- focused changes under `extensions/lib/learn-command.ts` and `extensions/lib/daemon-client.ts`
- focused tests under `tests/extension/`
- `PROJECT.md`, `README.md`, this plan, ADR-0004, and phase checkpoints

Forbidden changes:

- production Pi assessment calls or live providers;
- dependencies, SQLite, durable jobs, SSE, background work, Session selection, or source expansion;
- changes to existing preview/question request meaning or security defaults.

Acceptance criteria:

- successful questions can start one bounded owned assessment state without client-supplied evidence;
- state is instance-bound, expires, respects count/byte caps, never evicts live entries, and clears on shutdown;
- initial and follow-up submissions transition atomically and concurrent/replayed calls start at most one deterministic evaluation;
- invalid requests do not mutate state; post-transition failure makes the assessment unavailable rather than retryable;
- existing clients still parse question responses without the new field;
- the extension collects exactly three answers, confirms sharing/cost, handles cancellation locally, renders one F1 when present, and displays the final derived result;
- production daemon behavior never returns a fabricated deterministic assessment when the production adapter is absent.

### Phase 3 — Isolated Pi RPC assessment adapter

Status: complete on 2026-09-01. See `docs/checkpoints/answer-assessment-workflow-phase-3.md`.

Goal: release and embed the assessment prompt, wire the production Pi adapter through the assessment module, and verify every model turn with a fake executable while keeping live smoke opt-in.

Prerequisites:

- Phase 2 checkpoint complete;
- explicit Phase 3 authorization;
- approve any exact prompt, schema, timeout, or Pi invocation detail that changed during implementation investigation.

Expected allowed files:

- focused evaluator and fake-process test files under `internal/evaluator/`
- `agent/prompts/assets.go`
- the finalized `agent/prompts/evaluator-answer-assessment/v1.0.0.md`
- focused daemon wiring/tests
- `README.md`, `PROJECT.md`, this plan, ADR-0004, and phase checkpoints

Forbidden changes:

- live provider calls in automated tests;
- process/session reuse across human input;
- retries, repair calls, raw logs, persistence, SQLite, SSE, dependencies, or unrelated cleanup.

Acceptance criteria:

- each assessment turn uses one new frozen Pi 0.84.3 no-session/no-tools process with the accepted deny arguments and pre-prompt checks;
- the shared private RPC implementation preserves current question-generation behavior and does not publish a general process interface;
- fake-Pi tests cover exact input, stage handling, output correlation, follow-up, final result, invalid JSON/schema/references, tool/unknown events, timeout, cancellation, caps, exit, and reaping;
- source and answers never appear in argv, safe errors, logs, Session files, or persisted files;
- one initial assessment and at most one answered follow-up are possible, with zero product retries;
- full supported verification passes and an explicit opt-in live smoke procedure documents that it can resend selected source and incur up to two additional calls.

## 12. Acceptance Criteria

- The completed flow remains manual and begins only through `/learn`.
- The evaluator receives the exact previously previewed evidence; no repository reread or client-built evidence is accepted.
- The user can answer Q1/Q2/Q3 and receive either one F1 or a complete assessment.
- F1 can occur at most once and only one answer can be submitted for it.
- Final output contains three strict per-question verdicts and a deterministic Go-derived label.
- Code-specific feedback is grounded in valid retained evidence references.
- User answers are treated as untrusted, bounded, not logged, and not persisted.
- Every paid-call boundary is explicit, single-consume, non-retrying, deadline-bounded, and process-reaped.
- Existing preview and question-only clients remain compatible.
- No SQLite, learning history, SSE, background worker, Session selection, reminder, dependency, or release work enters this task.
- Every phase passes its verification, records a checkpoint, commits, pushes, and stops at the next authorization gate.

## 13. Verification

Phase-specific focused tests must run before broader checks. The supported full set is:

```text
CGO_ENABLED=0 go test -count=1 ./internal/evaluator
CGO_ENABLED=0 go test -count=1 ./internal/assessment
CGO_ENABLED=0 go test -count=1 ./internal/daemon
CGO_ENABLED=0 go test -count=1 ./...
go test -count=1 -race -tags netgo ./...
go vet ./...
npm run typecheck
npm test
scripts/test-agent-infra.sh
scripts/validate-agent-infra.sh
git diff --check
git status --short
git diff --stat
git diff
```

Do not run or claim live provider verification unless the user separately opts in after seeing the source-sharing and cost warning. Phase 2 must include race tests for concurrent state submission. Phase 3 must use the fake Pi executable for automated process coverage.

## 14. Open Questions

ADR-0004 accepted the following answers on 2026-09-01. They are fixed constraints for later authorized phases rather than open Phase 1 questions:

1. Accept a 30-minute, eight-entry, 1-MiB aggregate evidence limit for volatile assessments.
2. Accept additive question-response assessment descriptors and strict `POST /v1/assessment-turns` under protocol v1.
3. Accept 4-KiB non-empty answers, exactly three initial answers, and at most one 4-KiB follow-up answer.
4. Accept per-question model verdicts with deterministic Go aggregation: all demonstrated → `understood`; at least two not demonstrated → `review_needed`; otherwise `partial`.
5. Accept a new isolated Pi process per assessment turn, meaning one `/learn` can make one question call plus one assessment call and at most one follow-up assessment call.
6. Accept no recovery or retry before the later SQLite plan: restart, expiry, evaluator failure, or lost response requires a new `/learn` flow.
7. Accept Pi 0.84.3's string input dialog for the first answer UI; custom multiline UI remains out of scope.

Phases 1, 2, and 3 are complete. Durable storage and every other deferred capability require a new plan and authorization.
