---
id: post-preview-evaluator-adapter
status: draft
risk: high
current_phase: 1
phase_status: planned
updated: 2026-09-01
---

# Post-Preview Evaluator Adapter

## 1. Goal

Add the smallest safe product slice after the existing evidence preview: after the user explicitly confirms that the already displayed preview may be evaluated, generate one structured set of three evidence-backed learning questions through an isolated Pi evaluator.

The slice must send exactly the bytes retained by the preview, keep Pi credentials out of the Go daemon and model context, provide no evaluator tools, persist no raw source or Session transcript, and fail closed when evidence or evaluator output is invalid.

## 2. Background

The completed preview flow proves repository selection, bounded Go evidence extraction, local authenticated transport, and Pi UI rendering. The completed `internal/evidence.BuildBundle` seam can convert that exact bounded result into citation-ready evidence, but it has no product caller.

The next step crosses several compatibility and security boundaries at once:

- a second user action must prove preview-before-evaluation;
- the daemon must preserve the exact previewed value without silently rereading a changing working tree;
- a runtime evaluator input and output contract must be introduced without reusing development-only fixture schemas;
- a production prompt and model invocation will share selected source with the configured provider;
- the evaluator must be isolated from the development Session and have no tools;
- a new local daemon route will be able to trigger a potentially paid external model call.

These are high-risk changes under `AGENTS.md`. This plan therefore remains a draft until the proposed architecture decision is accepted and Phase 1 is explicitly authorized.

## 3. Current Behavior

Verified on 2026-09-01 from the repository, CodeGraph, and the locally installed Pi 0.84.3 interfaces:

- `extensions/pi-learnloop.ts` registers `/learn` with `createLearnCommand(new DaemonEvidenceClient())`.
- `createLearnCommand` requires an interactive, trusted Pi project; asks for a working-tree or commit-range selection; calls `client.preview`; renders the preview through `ui.notify`; and then returns.
- The extension has no confirm step, evaluator client, question renderer, answer collection, or model call.
- `DaemonEvidenceClient` performs protected daemon discovery and calls only `GET /v1/status` and authenticated `POST /v1/evidence-previews`.
- The daemon calls `internal/evidence.Preview` with fixed v1 limits and returns the mapped result. It does not retain the result after the response.
- `internal/evidence.BuildBundle(Result)` validates the applied budget and exact retained structure, excludes the absolute repository root, assigns stable evidence references and hashes, and fails closed without usable evidence.
- CodeGraph reports 13 callers of `BuildBundle`, all in `internal/evidence/bundle_test.go`; no product path can invoke it.
- `agent/` contains a deny-by-default evaluator policy, synthetic development cases, and a privacy-safe development run-record schema. These assets explicitly are not runtime product protocols.
- Pi 0.84.3 supports both in-process SDK sessions and JSONL RPC subprocesses. RPC supports `--no-session`, `--no-tools`, `--no-extensions`, `--no-skills`, `--no-prompt-templates`, `--no-themes`, `--no-context-files`, and `--no-approve`.
- Pi RPC emits `agent_settled` after retries, compaction retries, and queued continuations have stopped. It does not provide a repository-defined structured-output schema for this product; Pi LearnLoop must validate the assistant text itself.

No database, durable job, SSE stream, production prompt, runtime evaluator schema, evaluator adapter, answer workflow, follow-up, scoring, or assessment label currently exists.

## 4. Relevant Call Chain

Current:

```text
manual /learn
→ createLearnCommand
→ collect explicit Git selection
→ DaemonEvidenceClient.preview(ctx.cwd, selection)
→ protected local POST /v1/evidence-previews
→ daemon evidence.Preview
→ bounded preview response
→ extension formatPreview + ui.notify
→ stop
```

Proposed slice:

```text
manual /learn
→ create bounded preview and an opaque, expiring continuation ID
→ show the preview
→ explicit ui.confirm
→ authenticated continuation request carrying only the opaque ID
→ consume the exact in-memory evidence.Result once
→ evidence.BuildBundle(result)
→ isolated evaluator adapter
   → deterministic fixture in tests
   → Pi RPC in production
→ strict runtime question-set validation
→ show three questions
→ stop
```

The continuation request must not resubmit a repository path, Git revision, source excerpt, or client-built bundle. That would either permit client substitution or rerun analysis against a working tree that may have changed since the preview.

## 5. Relevant Files

- `AGENTS.md`: high-risk authorization, scope, verification, and stop rules.
- `PROJECT.md`: target question flow, thin-extension boundary, Pi-managed credentials, and evaluator isolation constraints.
- `docs/decisions/ADR-0001-agent-development-lifecycle.md`: lifecycle and authorization decision.
- `docs/decisions/ADR-0002-local-daemon-protocol-security.md`: current local transport, authentication, v1 compatibility, bounds, and threat model.
- `docs/decisions/ADR-0003-post-preview-evaluator-boundary.md`: proposed continuation, data-sharing, and evaluator-isolation decision for this task.
- `docs/checkpoints/evaluator-ready-evidence-bundle-phase-1.md`: current evidence seam and next-step constraints.
- `internal/evidence/evidence.go`: bounded preview source value.
- `internal/evidence/bundle.go`: pure evaluator-ready bundle boundary.
- `internal/daemon/server.go`: current protected v1 route and evidence adapter.
- `extensions/lib/daemon-client.ts`: current daemon discovery and protocol client.
- `extensions/lib/learn-command.ts`: current `/learn` UI flow.
- `agent/README.md`: evaluator-development module interface.
- `agent/policies/evaluator-capabilities.json`: deny-by-default runtime ceiling.
- `agent/prompts/README.md`: immutable production prompt lifecycle.
- `agent/evals/README.md` and `agent/evals/cases/`: required behavior and failure categories.
- `agent/schemas/run-record.schema.json`: development-only provenance expectations, not a product schema.
- `package.json` and `package-lock.json`: current Pi 0.84.3 development/peer dependency contract.
- Installed Pi 0.84.3 `docs/rpc.md`, `docs/sdk.md`, `docs/usage.md`, and TypeScript declarations: authoritative local interface evidence for this plan.

## 6. Scope

This plan may, phase by phase and only after explicit authorization:

- accept and implement ADR-0003;
- introduce production runtime schemas for the post-preview continuation request and the three-question result;
- add one versioned production question-generation prompt and corresponding synthetic eval coverage;
- retain bounded preview results in daemon memory behind opaque, single-use, short-lived continuation IDs;
- add a protected local continuation endpoint that consumes one continuation ID;
- connect the retained result to `evidence.BuildBundle` without rereading the repository;
- define an internal evaluator interface with deterministic fixture and Pi RPC implementations;
- invoke Pi RPC with no session persistence, no tools, no discovered extensions/skills/prompts/themes/context files, and no project approval;
- pass the active Pi model's non-secret provider/model/thinking identifiers when the compatibility investigation proves their mapping;
- render exactly three validated questions in the existing manual `/learn` interaction;
- add focused Go and TypeScript tests, adversarial protocol tests, deterministic adapter tests, and no-model integration coverage;
- update stable project documentation and create a checkpoint for each completed phase.

## 7. Out of Scope

- Collecting or persisting user answers.
- Asking a targeted follow-up question.
- Scoring, weakness labels, concept tracking, mastery, recommendations, or learning reminders.
- SQLite, migrations, durable jobs, leases, queues, SSE, WebSocket, polling, or daemon recovery of an in-flight evaluation.
- Reusing the development Session, its transcript, tools, context, extensions, skills, or prompt templates.
- Giving the evaluator filesystem, edit, command, process, credential, remote-control, or network tools. Pi-managed provider transport is the sole allowed network path.
- Sending an absolute repository root, unselected source, Git stderr, runtime tokens, Pi credentials, or Session data to the model.
- Persisting raw source, prompts containing source, model responses containing source, or credentials.
- Live model calls in normal automated tests.
- Automatic `/learn` execution, reminders, mobile/QQ/WeChat integration, telemetry, remote access, autostart, or release publication.
- Adding a second model/provider SDK or bypassing Pi credential management.
- Dependency additions or upgrades unless a later phase identifies and receives separate explicit approval for an exact version.

## 8. Proposed Changes

### 8.1 Freeze the boundary before implementation

Accept ADR-0003 before changing product behavior. It records these proposed long-lived rules:

- preview and evaluation are two distinct authenticated operations;
- the daemon issues an opaque, single-use, expiring continuation ID while retaining the exact bounded `evidence.Result` only in memory;
- continuation consumes that value and calls `BuildBundle` without repository access;
- production evaluation uses a separate Pi RPC process, not the development Session;
- all Pi tools and project/user resource discovery are disabled explicitly;
- credentials remain loaded and used by Pi, never supplied in HTTP JSON, command arguments, prompts, logs, or persisted records;
- runtime question output is strict JSON validated by Pi LearnLoop and fails closed on any mismatch.

ADR acceptance does not authorize implementation.

### 8.2 Add a bounded, single-use continuation store

The preview handler should create a continuation only when `Preview` succeeded and the result has at least one usable excerpt. The store should retain the immutable result value and metadata needed for safe consumption.

The implementation contract must include fixed, non-configurable initial limits:

- cryptographically random opaque identifiers;
- a short TTL measured from preview completion;
- one successful consume at most;
- deletion on decline when the client can report it, on expiry, and during daemon shutdown;
- a bounded entry count and aggregate retained excerpt bytes;
- deterministic eviction or rejection behavior;
- no disk serialization, logs, metrics upload, or recovery after restart.

Exact TTL, entry count, byte cap, and overload response are `TODO / Need Confirmation` in the proposed ADR and must be fixed before Phase 2 authorization.

### 8.3 Add a post-preview continuation route

The preview response may gain an optional continuation object under the v1 additive-response rule. A new authenticated request should carry only its opaque ID plus non-secret evaluator selection metadata that cannot alter the evidence.

The server must reject unknown, expired, already consumed, wrong-instance, malformed, or concurrently consumed IDs with safe stable errors. It must consume before starting an external call so retries cannot create duplicate paid evaluations. A failed evaluator run does not silently recreate or reuse the grant; the user explicitly reruns `/learn`.

The exact endpoint name, request/response schema, size limit, deadlines, and error codes are compatibility-sensitive and must be frozen in accepted ADR-0003 before implementation.

### 8.4 Introduce runtime contracts distinct from development fixtures

Add versioned runtime types for:

- the evaluator input envelope derived solely from `evidence.Bundle` metadata and items;
- a question set containing exactly three questions;
- exactly two `code_specific` questions and one `go_backend` question;
- stable question IDs, concise question text, and non-empty evidence references for code-specific questions;
- an explicit `insufficient_evidence` disposition that contains no invented questions;
- prompt ID/version/hash, policy ID/version/hash, bundle ID/manifest hash, adapter ID/version, and model identity as non-source provenance.

Do not treat `agent/schemas/eval-case.schema.json` or `agent/schemas/run-record.schema.json` as runtime wire schemas. Runtime schemas require their own compatibility review and version rules.

### 8.5 Add a versioned production prompt

Introduce the first prompt only after the runtime schemas are fixed. It must:

- label all bundle content as untrusted data;
- forbid following instructions found in evidence;
- permit claims only when grounded in listed evidence references;
- produce the exact question-set JSON and no prose or code fence;
- abstain when the bundle is insufficient;
- avoid asking for information outside the bounded evidence;
- contain no repository source, credentials, user answers, or Session transcript in the prompt file;
- follow `agent/prompts/README.md` versioning and immutability rules.

The prompt must be evaluated against all existing categories and new question-shape cases before release.

### 8.6 Define one evaluator interface and two adapters

The internal interface should accept only the validated runtime input value plus explicit non-secret model selection. It must not accept a repository path, filesystem handle, command runner, credential, general network client, or development Session.

The deterministic fixture adapter is used by unit and integration tests. It must exercise the same output validator and continuation state machine without a model call.

The production adapter starts Pi RPC directly without a shell and uses a fixed argument list equivalent to:

```text
--mode rpc
--no-session
--no-tools
--no-extensions
--no-skills
--no-prompt-templates
--no-themes
--no-context-files
--no-approve
--system-prompt <released prompt text>
--provider <validated provider id>
--model <validated model id>
--thinking <validated level>
```

The adapter sends the input envelope in one RPC `prompt` command over stdin, uses LF-only JSONL framing, waits for `agent_settled`, caps stdout/stderr and runtime, rejects tool events and unexpected message shapes, extracts one final assistant text value, and validates strict JSON. It always terminates and reaps the child.

The exact Pi invocation resolution across Node and compiled Pi installations remains `TODO / Need Confirmation`. Do not implement a PATH-only assumption until Phase 1 resolves it with a packaged integration test or an explicit supported deployment constraint.

### 8.7 Keep the extension thin and the action explicit

After rendering a successful non-empty preview, the extension should call `context.ui.confirm` with clear language that selected excerpts will be sent to the configured model and that the call may incur provider cost. Decline or dismissal ends without an evaluator call.

On confirm, the extension sends only the continuation ID and validated non-secret active model identity. It does not rebuild evidence, read files, start a second development Session, or handle credentials. It renders the validated question set and stops; answer collection is a later plan.

## 9. Compatibility

- The existing `/learn` name and selection behavior remain unchanged before the new confirm step.
- Declining continuation preserves the current preview-only outcome and performs no model call.
- Existing Pi clients must continue to parse preview responses; any new preview response field must be optional under ADR-0002.
- The continuation route is a new authenticated v1 capability. Its route, fields, errors, size/time bounds, and single-use behavior become compatibility commitments once accepted and implemented.
- Requests remain strict. Adding required fields or changing field meaning requires the versioning rule selected in ADR-0003.
- The internal Go `Bundle` remains a domain value. JSON mapping belongs at the evaluator boundary and must not add JSON tags that accidentally publish the internal type.
- Development fixture and run-record schemas remain development-only and unchanged unless their own versioned contract requires an update.
- Pi 0.84.3 is the only locally verified evaluator interface. Supporting other Pi versions is `TODO / Need Confirmation`; do not claim a compatibility range from the peer dependency wildcard alone.
- No durable data migration exists in this plan.

## 10. Risks

- **Preview/evaluation mismatch:** rerunning evidence after confirmation can send bytes the user did not inspect. Mitigation: retain and consume the exact in-memory result.
- **Duplicate paid calls:** request retry or concurrent consume could start multiple evaluators. Mitigation: atomic single-use consume before spawn and no automatic product retry.
- **Prompt injection:** source comments or strings may instruct the evaluator. Mitigation: untrusted-data delimiters, tool isolation, strict prompt rules, and required eval coverage.
- **Tool/resource escape:** Pi defaults enable tools and can discover extensions, skills, prompts, and context files. Mitigation: every deny flag is explicit and adapter tests inspect active behavior/events.
- **Credential exposure:** passing API keys through the daemon, protocol, argv, prompt, or logs would violate policy. Mitigation: Pi-managed auth only and redaction/adversarial tests.
- **Unbounded memory:** transient previews contain source excerpts. Mitigation: fixed TTL, count/byte caps, bounded v1 evidence, and shutdown cleanup.
- **Subprocess hang or output flood:** an RPC process can stall or emit unexpected output. Mitigation: context deadline, capped streams, strict JSONL parser, termination, and reaping.
- **Malformed model output:** text generation does not guarantee schema conformance. Mitigation: strict one-object parsing, schema validation, no repair call, and fail-closed user error.
- **Model mismatch:** a daemon default may differ from the active Pi model. Mitigation: explicitly propagate validated provider/model/thinking identifiers after compatibility is proved.
- **Pi invocation mismatch:** npm CLI, global CLI, and compiled binary layouts differ. Mitigation: resolve and test the exact invocation before production authorization.
- **Privacy through diagnostics:** stdout, stderr, errors, or tests may contain evidence. Mitigation: bounded redacted errors and no raw-output logging or persistence.
- **Daemon availability during model call:** the current foreground daemon can exit. Mitigation in this slice is a clear failure and no recovery; durable jobs are out of scope.

## 11. Implementation Phases

### Phase 1 — Runtime contract and isolation proof

Goal: accept the architecture boundary and add no live model behavior yet.

Risk: high.

Allowed changes after separate explicit authorization:

- accept ADR-0003 after resolving every blocking `TODO / Need Confirmation`;
- add versioned runtime input and question-set schemas/types;
- add the draft/released question-generation prompt following repository version rules;
- extend synthetic evaluator cases for question count/kinds, evidence references, strict output, prompt injection, and insufficient evidence;
- add deterministic validators and tests that do not invoke Pi or change `/learn` behavior;
- document and test the exact Pi invocation/model-mapping contract without calling a provider.

Forbidden in Phase 1:

- daemon routes or preview retention;
- extension UI changes;
- subprocess spawn in product code;
- credentials or live/paid model calls;
- dependencies, persistence, or business behavior.

Stop gate: create `docs/checkpoints/post-preview-evaluator-adapter-phase-1.md`, report the accepted schemas/prompt/isolation proof, and wait for explicit Phase 2 authorization.

### Phase 2 — Explicit continuation with deterministic adapter

Goal: implement and verify the preview/confirm/consume state machine end to end without a live model call.

Risk: high.

Expected scope after separate explicit authorization:

- bounded in-memory continuation store;
- additive preview response metadata and the protected continuation route;
- atomic single-use consume and exact `BuildBundle` connection;
- internal evaluator interface plus deterministic fixture adapter;
- explicit Pi UI confirmation and deterministic three-question rendering in tests;
- adversarial protocol, expiry, concurrency, capacity, decline, and compatibility tests.

Forbidden in Phase 2:

- production Pi RPC spawn or live model calls;
- answers, follow-up, scoring, persistence, SSE, background jobs, or dependency changes.

Stop gate: create `docs/checkpoints/post-preview-evaluator-adapter-phase-2.md`, report the exact bytes/state transition proved by tests, and wait for explicit Phase 3 authorization.

### Phase 3 — Isolated Pi RPC adapter

Goal: replace the production fixture seam with the bounded Pi RPC adapter while retaining deterministic automated tests.

Risk: high.

Expected scope after separate explicit authorization:

- exact Pi invocation resolution approved in Phase 1;
- fixed deny flags, Pi-managed model/auth, process deadline, cancellation, output caps, termination, and JSONL handling;
- strict extraction and validation of one question-set result;
- safe user-facing errors for missing Pi, missing model/auth, timeout, invalid output, and process failure;
- integration tests with a fake Pi RPC executable and an opt-in manual smoke procedure that is disabled by default.

Forbidden in Phase 3:

- live provider calls in automated tests;
- retries that can silently create a second paid call;
- raw source/output logging or persistence;
- answers, follow-up, scoring, database, durable execution, or unrelated cleanup.

Stop gate: create `docs/checkpoints/post-preview-evaluator-adapter-phase-3.md`, update stable documentation, run full supported verification, and stop. Any answer workflow requires a new investigated plan.

## 12. Acceptance Criteria

- The user sees the bounded preview before any evaluator data sharing.
- Decline/dismiss performs no model call and preserves preview-only behavior.
- Confirmation consumes one opaque continuation exactly once.
- The evaluated bundle is built from the exact retained preview value with no repository reread.
- Only the bundle's selected excerpts and non-secret provenance reach Pi RPC.
- The evaluator runs in an ephemeral Session with all tools and discovered resources disabled.
- Pi credentials never enter daemon HTTP payloads, command arguments, prompts, logs, persisted files, or model-visible content.
- Successful output contains exactly two code-specific and one Go/backend question, with required evidence references.
- Insufficient evidence and malformed output fail closed without invented questions.
- No raw source, model response containing source, credentials, or Session transcript is persisted.
- Existing preview-only clients remain compatible with the additive preview response.
- Deterministic automated verification uses no paid model call.
- Each phase changes only its approved files, passes its verification, records a checkpoint, and stops at its authorization gate.

## 13. Verification

Investigation/draft verification:

```text
codegraph status
codegraph explore createLearnCommand Preview BuildBundle DaemonEvidenceClient
codegraph callers BuildBundle
scripts/test-agent-infra.sh
scripts/validate-agent-infra.sh
git diff --check
git status --short
git diff --stat
git diff
```

Future implementation verification, only when the relevant phase is authorized:

```text
go test ./internal/evidence
go test ./internal/daemon
go test ./...
npm run typecheck
npm test
scripts/test-agent-infra.sh
scripts/validate-agent-infra.sh
git diff --check
```

Phase 2 must add focused tests for continuation expiry, capacity, byte bounds, atomic consume, concurrent consume, wrong/expired identifiers, decline, daemon restart, BuildBundle failure, strict request decoding, old-client response parsing, and deterministic question rendering.

Phase 3 must add a fake executable/RPC harness covering LF framing, response correlation, `agent_settled`, invalid JSON, Unicode separators inside JSON strings, tool events, missing model/auth, timeout, cancellation, stdout/stderr caps, child exit, and process reaping. A live smoke test must be explicit, opt-in, and reported separately because it can transmit source and incur cost.

## 14. Open Questions

The following block implementation and must be resolved in proposed ADR-0003 or Phase 1 evidence before any business-code authorization:

1. `TODO / Need Confirmation`: exact continuation endpoint name and v1 schema, including whether a new route remains v1-additive under ADR-0002 or requires `/v2`.
2. `TODO / Need Confirmation`: fixed continuation TTL, maximum entry count, aggregate excerpt-byte cap, eviction/rejection semantics, and stable overload/expiry errors.
3. `TODO / Need Confirmation`: the supported way to launch the same Pi 0.84.3 installation from the Go daemon across npm CLI and compiled-binary layouts. The locally installed Pi example resolves this inside Node, not from an unrelated Go process.
4. `TODO / Need Confirmation`: exact mapping of `ExtensionCommandContext.model` and `thinkingLevel` to RPC `--provider`, `--model`, and `--thinking`, including behavior when no model is selected.
5. `TODO / Need Confirmation`: whether the initial runtime contract should produce only the three question prompts or include additional display metadata. It must not include answer/scoring fields under this plan.
6. `TODO / Need Confirmation`: exact evaluator and request deadlines and stdout/stderr caps, chosen against the existing 35-second preview client timeout without conflating the two operations.
7. `TODO / Need Confirmation`: whether Pi's default retry/compaction settings can cause more than one provider request and how the adapter makes potential cost explicit. Product-level retries remain forbidden.
8. `TODO / Need Confirmation`: the initially supported Pi version range. Only 0.84.3 has been inspected; the current peer dependency wildcard is not evidence of runtime compatibility.

No implementation phase is authorized by this draft. The next user decision is whether to accept ADR-0003 after the open questions are resolved, then explicitly authorize Phase 1.
