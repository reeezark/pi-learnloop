---
id: ADR-0003
status: proposed
date: 2026-09-01
supersedes: none
---

# ADR-0003: Post-Preview Continuation and Isolated Evaluator Boundary

## Context

Pi LearnLoop currently ends after displaying a bounded evidence preview. The pure `internal/evidence.BuildBundle` function can transform the exact preview result into citation-ready evaluator evidence, but it has no product caller. The daemon does not retain preview results, and the extension has no continuation action or evaluator adapter.

The next product slice must generate three learning questions only after the user has inspected and explicitly approved the preview. It also introduces the first external model call and therefore crosses protocol, cost, credential, privacy, prompt-injection, and compatibility boundaries.

The existing contracts require:

- the evaluator receives only the selected, budgeted EvidenceBundle;
- the preview occurs before evaluation;
- evidence content is untrusted data;
- the evaluator is isolated from the development Session;
- evaluator tools default to deny and none are allowed;
- API credentials remain Pi-managed and are not persisted or exposed to the evaluator;
- raw source and Session transcripts are not persisted;
- output is structured, evidence-backed, and has an insufficient-evidence path;
- the Pi extension remains thin while business rules and evidence ownership remain in the Go daemon.

Verified Pi 0.84.3 capabilities relevant to this decision are:

- JSONL RPC mode is intended for cross-language and process-isolated integration.
- `--no-session` disables Session persistence.
- `--no-tools` disables built-in, extension, and custom tools.
- `--no-extensions`, `--no-skills`, `--no-prompt-templates`, `--no-themes`, and `--no-context-files` disable resource discovery.
- `--no-approve` prevents project-local trust for the run.
- `--system-prompt`, `--provider`, `--model`, and `--thinking` select the prompt and non-secret model identity.
- `agent_settled` marks completion after automatic retries, compaction retries, and queued continuations have stopped.

The current daemon protocol in ADR-0002 is authenticated loopback HTTP v1. It exposes only status and preview routes, owns fixed evidence caps, permits additive response fields within v1, and requires a new decision before evaluator behavior.

### Threat model extension

In addition to ADR-0002, this boundary must address:

- a working tree changing between preview and confirmation;
- a client substituting unpreviewed evidence;
- accidental duplicate paid model calls through retry or concurrent consume;
- prompt injection embedded in selected source;
- Pi default tools or discovered resources broadening evaluator access;
- credentials or raw evidence leaking through HTTP, argv, logs, errors, Session files, or run records;
- malformed, oversized, stalled, or adversarial RPC output;
- a same-user local client with the Instance Token triggering evaluator cost without a visible Pi confirmation.

As in ADR-0002, this design does not claim protection from root, same-user malware, or a compromised trusted extension. Explicit confirmation protects the intended Pi interaction, not a hostile process already holding the Instance Token.

## Decision

This ADR is proposed, not accepted. It does not authorize implementation or a model call.

### 1. Separate preview from continuation

Preview and evaluation are separate authenticated operations. A successful preview may return an optional opaque continuation descriptor. The extension must display the preview before asking the user to continue.

The extension uses `ctx.ui.confirm` with language that selected excerpts will be sent to the configured model and may incur provider cost. Decline or dismissal performs no continuation request and no model call.

### 2. Retain the exact preview in daemon memory

When a preview produces usable evidence, the daemon retains the exact bounded `evidence.Result` in memory behind a cryptographically random continuation ID. It does not reread the repository after confirmation and does not accept a client-built bundle or resubmitted source.

Continuation entries are:

- bound to the current daemon instance;
- single-use and atomically consumed before evaluator execution;
- short-lived;
- bounded by both entry count and aggregate retained excerpt bytes;
- removed on expiry and daemon shutdown;
- never serialized, logged, uploaded as telemetry, or recovered after restart.

`TODO / Need Confirmation`: Phase 1 must fix the TTL, entry count, aggregate byte cap, and overload behavior before this ADR can be accepted.

### 3. Add one protected continuation capability

The continuation request carries only the opaque ID plus validated non-secret model identifiers. It cannot alter repository selection, evidence limits, source content, bundle metadata, or prompt version.

Unknown, expired, already consumed, malformed, wrong-instance, and concurrently consumed IDs fail with safe stable errors. The product does not automatically retry a continuation. A user who wants another attempt starts a new `/learn` flow and reviews a new preview.

`TODO / Need Confirmation`: Phase 1 must fix the endpoint path, v1/v2 compatibility choice, JSON fields, bounds, deadlines, and error codes before acceptance.

### 4. Build the bundle only after atomic consume

The daemon atomically consumes the retained result and passes that value directly to `internal/evidence.BuildBundle`. Build failure stops before any evaluator process starts. The builder remains pure and must not gain repository, filesystem, Git, network, credential, persistence, or model inputs.

The internal Go bundle remains a domain value. A dedicated adapter maps it to a versioned runtime evaluator input; the Go type does not gain JSON tags merely to publish an accidental wire protocol.

### 5. Use a separate Pi RPC process

The production adapter uses Pi RPC rather than the development Session or an in-process nested SDK Session.

The fixed security argument set includes:

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
```

The adapter starts the process directly without a shell. It provides the released system prompt and validated provider/model/thinking identifiers as separate arguments, then sends exactly one runtime input envelope in one RPC prompt command.

The adapter must not pass API keys, Instance Tokens, repository paths, source excerpts, or Session identifiers in argv. Pi resolves and uses its own credentials for provider transport. The Go daemon must neither read nor persist Pi auth files.

`TODO / Need Confirmation`: Phase 1 must prove how the daemon launches the same supported Pi installation across npm CLI and compiled-binary layouts. A bare `pi` PATH assumption is not accepted by this proposal.

### 6. Deny all evaluator capabilities except Pi-managed model transport

The evaluator receives no tools. Project/user extensions, skills, prompt templates, themes, AGENTS/CLAUDE context files, and project trust are disabled independently of tool suppression.

Runtime tests must treat any tool registration/execution event, discovered resource behavior, unexpected extension event, or Session file as a policy violation. Prompt text cannot grant a denied capability.

The adapter may spawn the Pi process as orchestration infrastructure; the model itself has no process tool. The sole allowed network activity is Pi's provider transport. No general network tool is exposed to the evaluator.

### 7. Introduce independent runtime schemas and a released prompt

Development eval-case and run-record schemas remain non-runtime assets. This slice introduces separately versioned runtime input and question-set schemas.

The successful result contains exactly:

- two `code_specific` questions;
- one `go_backend` question;
- stable question IDs and concise text;
- evidence references required for code-specific questions;
- no answer, score, assessment label, follow-up, or persistence fields.

The alternate disposition is `insufficient_evidence` and contains no invented questions.

The production prompt is a versioned immutable asset. It delimits evidence as untrusted data, forbids following evidence instructions, requires evidence references, requires strict JSON with no surrounding prose, and requires abstention when the bundle cannot support the question contract.

### 8. Validate strict output and fail closed

The adapter uses LF-only JSONL framing, bounded stdout/stderr buffers, a fixed deadline, cancellation, and guaranteed child termination/reaping. It waits for `agent_settled`, extracts one final assistant text value, and accepts exactly one JSON object with no code fence or trailing content.

Pi LearnLoop validates the complete runtime schema and evidence references before returning questions. It does not make an automatic repair or retry model call. Invalid, missing, tool-using, timed-out, oversized, or otherwise unexpected output returns a safe user-facing failure and no questions.

`TODO / Need Confirmation`: Phase 1 must fix the evaluator deadline and output caps and determine how Pi retry/compaction settings affect provider call count and the user's cost expectation.

### 9. No persistence in this slice

Continuation state, prompts containing runtime evidence, RPC streams, model output, questions, and errors containing source are not written to disk. No SQLite, durable job, Session file, transcript, or raw run record is introduced.

Tests may retain synthetic fixtures committed under `agent/`, but they must contain no real repository source, credentials, or workstation-specific paths.

## Alternatives

### Rerun preview after confirmation

Rejected because a working tree or revision selection can resolve to different bytes after the user inspected the preview. The evaluator must receive the exact retained value.

### Return the preview to the daemon in the continuation request

Rejected because the client could substitute or mutate evidence and the server would no longer be the authority for bounds and provenance. It also retransmits source unnecessarily.

### Build and evaluate during the original preview request

Rejected because the user cannot inspect the result before data sharing. A confirmation shown before preview does not satisfy the established preview-before-evaluation contract.

### Persist continuation bundles to disk

Rejected for the first slice because it expands the privacy, schema, migration, cleanup, and recovery surface. Short-lived bounded memory is sufficient for a foreground daemon and manual flow.

### Reuse the active development Session

Rejected because it contains unrelated transcript/context and may have filesystem, command, edit, network, extension, or custom tools. It violates the required evaluator Session isolation.

### Create a nested Pi SDK Session inside the extension

Credible but not selected. Pi 0.84.3 can create an in-memory no-tools Session with a fully custom empty resource loader, and the extension already has Pi as a peer dependency. However it shares the development process, moves evaluator orchestration and bundle handling into the thin extension, and requires exposing the bundle to TypeScript before the daemon-owned state transition completes. RPC better preserves process isolation, cross-language ownership, and the recorded target architecture.

This alternative should be reconsidered if exact production Pi invocation from the daemon cannot be made portable and testable without passing executable paths through the public protocol.

### Give the model a submission tool for structured output

Rejected because the capability policy allows no evaluator tools. Strict assistant-text JSON plus fail-closed validation preserves the empty tool set.

### Add durable jobs before evaluator integration

Rejected as broader than the smallest manual product slice. Failure after atomic consume is reported; recovery and retry belong to a later persistence plan.

## Consequences

- The user gains an explicit, visible privacy/cost gate after preview.
- The daemon becomes temporarily stateful in memory even though durable persistence remains absent.
- Exact preview fidelity is preserved across a changing working tree.
- A new authenticated endpoint can trigger paid provider traffic, so single-use semantics, bounds, confirmation wording, and safe errors become security and compatibility commitments.
- The Pi evaluator is isolated at both Session and process boundaries and has no tools or discovered project/user resources.
- Credentials remain Pi-managed, but the daemon now orchestrates a process that performs Pi-managed network transport.
- Runtime schemas and the production prompt become versioned compatibility assets independent from development fixtures.
- Strict text-to-JSON validation can reject otherwise useful model prose; failure is preferred to accepting ambiguous or ungrounded output.
- Foreground daemon exit loses continuations and active evaluation state. This is an explicit limitation until durable jobs are separately designed.
- Supporting multiple Pi packaging layouts and versions requires a proved invocation contract; it cannot be inferred from the current peer dependency wildcard.
- Accepting this ADR still does not authorize code changes, dependency changes, model calls, answer collection, scoring, persistence, or any implementation phase.
