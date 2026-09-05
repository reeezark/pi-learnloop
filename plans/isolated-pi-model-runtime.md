---
id: isolated-pi-model-runtime
status: complete
risk: high
current_phase: 4
phase_status: complete
updated: 2026-09-05
---

# Restore Local LearnLoop with an Isolated Pi Model Runtime

## 1. Goal

Make the existing local Pi extension usable end to end: explicit Git or Session
selection, exact evidence preview, confirmed question generation, reviewable
multiline answers, optional F1, final assessment, and source-free local history.
Passing a bootstrap probe or fake-process suite is not completion. The final gate
includes the actual Pi UI and a controlled real-model learning flow.

The user's current objective is personal development-environment use, not a
public macOS/npm release. Preserve the separate manual foreground Go daemon.

## 2. Background

On 2026-09-05 an actual Pi 0.84.3 `/learn` invocation in this repository,
using `d93dfa5^` to `d93dfa5` and `deepseek/deepseek-v4-pro` with high thinking,
successfully previewed two Go files and six declarations. Confirming question
generation produced the reported generic error in less than one second.

The production adapter then failed a no-provider diagnostic: its fixed CLI deny
arguments still yield `get_commands` containing `llama`, source `extension`.
Installed Pi's `main` unconditionally adds its hidden built-in llama extension.
The Go adapter correctly rejects a nonempty command list before sending prompt
content. Merely permitting this command would not meet the no-extension design.

Pi's `set_auto_retry` and `set_auto_compaction` RPC handlers also call setters
that save global settings. The current values are false; their pre-investigation
values were not captured and must not be guessed or automatically restored.

## 3. Current Behavior

- The Phase 1 working tree replaces the private CLI/RPC lifecycle with one
  shared exact-installation preflight and a fresh embedded `ModelRuntime` worker
  for each question, assessment, or possible F1 turn. It is not yet committed.
- Actual-SDK tests with intercepted transport prove the bounded no-tools turn,
  read-only settings projection, zero provider retry, settings/Session sentinel
  preservation, and safe failure handling without contacting a real provider.
- Mandatory listener-based daemon, extension-client, and packaged foreground
  smoke suites cannot run past `listen(127.0.0.1)` in the current sandbox.
- The Phase 2 working tree maps safe runtime, evaluator, local-connection,
  compatibility, and expired-state errors to the actual question-generation or
  answer-assessment stage without exposing raw errors or retrying a request.
- Existing fake-worker tests prove the private process/framing contract; they
  are supplemented by the actual-SDK suite rather than treated as SDK proof.
- The installed package exports `ModelRuntime`. A no-provider probe imported
  exact Pi 0.84.3, resolved the selected DeepSeek model with an injected empty
  credential store and in-memory model configuration, and observed zero
  extension-provider registrations, credential reads/writes, Session creation,
  or files in its fresh temporary config directory. No provider method ran.
- `ModelRuntime.streamSimple` supplies Pi-owned model/auth transport without an
  AgentSession, resource loader, CLI main, or SettingsManager. Its preparation
  path resolves authentication inside Pi and forwards request options.
- Pi's CLI Agent wrapper, not `ModelRuntime`, additionally applies global HTTP
  proxy/dispatcher settings and forwards transport, HTTP/WebSocket timeouts,
  thinking budgets, retry delay and provider-attribution headers. A direct SDK
  adapter must reproduce that bounded subset explicitly while forcing provider
  retries to zero; it must not inherit Agent retry, Session or extension hooks.
- A read-only audit of the current Pi setup found no `httpProxy`, transport,
  HTTP/WebSocket timeout, provider-retry or thinking-budget override, no proxy
  environment variable and no `models.json`. Effective values are transport
  `auto`, HTTP idle timeout 300,000 ms and retry-delay cap 60,000 ms. The
  settings file retained the same size, mtime and inode after inspection.
- A synthetic request through the actual built-in DeepSeek provider and
  `ModelRuntime` produced one intercepted POST with the exact selected model,
  `tools: []`, `tool_choice: none`, high reasoning, an AbortSignal and one
  credential read. It made no real network request, credential write, Session,
  extension-provider registration or user-file access.

## 4. Relevant Call Chain

Original failing path:

```text
/learn -> retained preview -> confirmation -> question evaluator
-> Pi CLI main -> built-in llama registration -> get_commands not empty -> stop
```

Implemented Phase 1 working-tree path, shared privately by both adapters:

```text
existing evaluator interface -> bounded fresh Node child
-> trusted installed Pi 0.84.3 ModelRuntime -> one model stream, tools=[]
-> bounded assistant text -> existing Go schema/reference validation
-> existing assessment/history lifecycle
```

No Pi AgentSession is created. Agent retries and auto-compaction therefore do
not exist in this execution path; provider retries are explicitly zero per call.

## 5. Relevant Files

- `internal/evaluator/pi_rpc.go`, `pi_contract.go`, their question/assessment and
  version tests, and the existing strict runtime contracts.
- `internal/daemon/daemon.go`, question/assessment routes and integration tests.
- `extensions/lib/learn-command.ts`, `daemon-client.ts`, and extension tests.
- `agent/README.md`, released prompt assets, capability/context policies,
  evaluator cases, and development run-record schema.
- Installed Pi 0.84.3 `dist/main.js`, `dist/extensions/index.js`,
  `dist/extensions/llama/index.js`, `dist/modes/rpc/rpc-mode.js`,
  `dist/core/settings-manager.js`, `dist/core/model-runtime.js` and declarations,
  `dist/index.js`, and package manifest. Inspect these in the installation owned
  by the selected `pi`, not an unrelated checkout's package.
- ADR-0002 through ADR-0008; ADR-0010 proposes only the evaluator mechanism change.

## 6. Scope

Replace the private production evaluator mechanism, prove its isolation against
the real installed SDK, make existing safe error codes actionable in the UI, and
verify normal local usage. Keep the evaluator's caller-facing Go interfaces and
all HTTP, evidence, assessment, history, and released prompt contracts unchanged.

Design-only allowed files: this plan, ADR-0010, its Phase 1 investigation
checkpoint, `PROJECT.md`, and the superseded release plan/checkpoint.
The existing uncommitted release Phase 2 evidence must be preserved.

## 7. Out of Scope / Forbidden Changes

- No dependency addition, removal or upgrade; Go language baseline stays 1.21.
- No modification of installed Pi, credentials, auth files, user settings,
  Session files, trust decisions, or shell configuration by LearnLoop code.
- No credential copying, temporary auth-file mirror, auth token on argv, or
  credentials in Go, HTTP, model input, errors, logs, or diagnostic output.
- No relaxed tool/extension permissions, `llama` allowlist, CLI fallback, prompt
  repair, provider/model substitution, or automatic model retry.
- No changed database/schema, prompt version/content, verdict or label rules,
  evidence scope/budgets, Session provenance, or public request/response shape.
- No automatic daemon start, lifecycle hook, Session indexing, SSE, durable jobs,
  signing, npm publication, tag, dependency download, or public release work.
- No automatic restoration of the two pre-existing global setting values: the
  original values are unknown. A user-requested restoration is a separate action.
- No commit or push without separate direction.

## 8. Proposed Changes

### Deep module and runtime ownership

Keep `internal/evaluator` as the deep module. It owns discovery, private child
framing, source bounds, SDK invocation, cancellation, safe failures, and child
reaping behind the existing question/assessment interfaces. The extension and
daemon routes must not learn Node/SDK startup details.

Embed a small repository-owned JavaScript worker in the Go executable; execute
it from memory using a frozen Node executable, without writing script, source,
answers or output to disk. The worker imports the installed Pi SDK by a
server-resolved absolute entry path. It must not invoke Pi CLI `main`,
`runRpcMode`, `createAgentSession`, extensions, or a default resource loader.

Resolve and freeze Node and `pi` from the daemon startup PATH. Verify Node's
existing >=22.19.0 requirement, CLI version, owning package identity/version,
canonical SDK entry, and required exported capabilities. Restrict discovery to
the actual Pi installation, with bounded parent traversal and metadata reads.
No npm resolution from the reviewed repository, automatic installation, network
lookup, or fallback SDK path is allowed. A compiled standalone Pi without its
matching importable SDK is unavailable, not silently supported.

### One turn and no ambient learning context

Use Pi's `ModelRuntime` for provider/model resolution and Pi-managed auth. The
worker receives only the exact immutable selected prompt, fixed model selection,
and validated runtime input through private pipes. It submits one user message
with `tools: []`, no developer Session, no extra context or working-directory
annotation, and no Agent loop. Reject missing/exact-model mismatches before the
provider call; never let an SDK default pick another model.

Use explicit `maxRetries: 0`, `toolChoice: "none"` and the existing deadline.
Pass the validated selected thinking level as `reasoning`; pass global
`thinkingBudgets` and `transport`; derive `timeoutMs` exactly as Pi does from
provider timeout or the effective HTTP idle timeout; and pass the configured
WebSocket timeout and retry-delay cap. The outer 120-second deadline remains
authoritative. Capture all options in deterministic transport tests. No
tool-call content, failed/aborted completion, unknown completion shape, or
partial output may become a successful result. Go remains the final authority
for JSON, duplicate keys, UTF-8, size, question shape and E/C citations.

Pi owns auth resolution and any necessary OAuth refresh in its normal store;
LearnLoop must not inspect, export, clone, or log credential values. Retain
Pi-supported global model configuration and environment-based transport. Inside
the child only, snapshot the global `settings.json` bytes once into a custom
read-only `SettingsStorage`, construct Pi's exported `SettingsManager` with
`projectTrusted: false`, reject any drained load error, and forbid every storage
callback that returns replacement bytes. Never use Pi's `FileSettingsStorage`,
which creates a lock-file side effect, and never invoke a setting setter.

Apply Pi 0.84.3's pinned `applyHttpProxySettings` and
`configureHttpDispatcher` helpers to that child-local snapshot, and use its
`mergeProviderAttributionHeaders` helper without a Session ID. This preserves
normal Pi provider attribution where enabled but creates no LearnLoop analytics
or separate telemetry request. Proxy values, settings bytes and derived headers
remain inside the worker and never cross Go, HTTP, logs or errors. Missing,
unreadable, malformed or unsupported transport settings fail before model
dispatch with the existing safe unavailable/failure categories. Exact internal
helper paths are part of the pinned 0.84.3 installation preflight.

### Bounds and diagnostics

Keep existing input-schema budgets, 120-second evaluation deadline, 2-MiB stream
budget, 64-KiB stderr and final-text ceilings, and narrower schema-specific
output limits. Bound private request framing including prompt/metadata overhead
separately from the unchanged evaluator-input limit. Reject extra frames,
trailing content, duplicate fields, overflow, child exit and cancellation.
Enforce stream limits while consuming SDK events, not only after materializing
the final response. SDK internal catalog/auth allocations are not covered by
these source-stream bounds and must be documented honestly.

Use only existing safe error codes across the HTTP seam. UI messages distinguish
unavailable runtime, evaluator failure, timeout, invalid output, lost local
connection, incompatible response and expired continuation. Never include raw
provider errors or claim a model was called solely because confirmation occurred.
Question and assessment failures must be named for the stage that actually ran.

## 9. Compatibility

HTTP/provenance/storage/prompt versions remain unchanged, as do all legacy v1
and enriched v2 paths. Pi remains exactly 0.84.3. Node becomes an explicit daemon
evaluator prerequisite, already required by the extension. Importable npm-style
Pi installations are the initially verified layout; standalone-only executables
fail closed. No dependency graph change or installed-Pi patch is permitted.

The former CLI/RPC setup sequence and `agent_settled` check are replaced by a
one-stream provider completion contract, not merely removed. Both model paths
must receive equivalent cancellation, completion, tool-rejection and byte-bound
coverage. Generic history and Session completion-only semantics are unchanged.

## 10. Risks

- CLI and SDK can differ even at the same version; verify the installation and
  add an actual-SDK, provider-free test instead of relying solely on a fake CLI.
- SDK dependency import or model catalog handling may perform ambient I/O;
  isolate tests with synthetic stores and prove no startup network/discovery.
- Node/SDK package resolution and private framing become security-sensitive.
- Global provider configuration, proxies, thinking levels and auth refresh may
  differ from CLI behavior. Resolve mismatches explicitly, not by credential
  extraction or silent fallback.
- Passing isolation tests does not prove model output validity or UI usability.
  Live completion is a separate final gate; failures leave the goal incomplete.
- A diagnostic assessment is not proof of the user's understanding. Identify
  agent-authored test answers and any resulting local history record explicitly.

## 11. Implementation Phases

### Phase 1: Replace the isolated evaluator mechanism

Requires acceptance of ADR-0010 and explicit Phase 1 authorization. Implement
the resolved read-only global-settings projection above. Add regression coverage
first for real CLI built-in registration and settings mutation, then the new
provider-free SDK initialization and one-turn transport behavior.

Allowed: `internal/evaluator/` production helpers, embedded `.mjs` worker and
focused tests/fixtures; `internal/daemon/daemon.go` composition and affected
daemon tests; `tests/evaluator/` Node built-in-runner tests; `package.json` only
to include those tests in the existing test script, without manifest dependency,
version, engine, peer or pack-allowlist changes; `scripts/test-release-artifacts.sh`
only if its fake-runtime smoke assumptions need a scoped adjustment; `README.md`,
`PROJECT.md`, `agent/README.md`, this plan/ADR and phase checkpoints.

No UI behavior change, live provider call, public protocol/schema change, or
unrelated cleanup. Replace obsolete private RPC mechanics in place; do not add
a second production fallback. Retain equivalent negative tests. Run focused and
full verification below, record a checkpoint, advance and stop.

### Phase 2: Actionable UI errors and local-use guidance

Requires explicit Phase 2 authorization. Allowed: `extensions/lib/learn-command.ts`,
`extensions/lib/daemon-client.ts` only where existing error propagation requires
it, their tests, `README.md`, `PROJECT.md`, this plan and checkpoints.

Preserve selections, mandatory confirmations, answer review and cancellation,
and no-retry behavior. Explain the real failing stage with allowlisted codes,
including question-versus-assessment failure, and disclose that initialization
failure may precede a provider request. Document matching local daemon/extension
startup and exact Pi/Node requirements. Do not add request fields, default Git
revisions, new daemon commands or diagnostic logs. Verify, checkpoint and stop.

### Phase 3: Actual local Pi acceptance

Requires explicit Phase 3 authorization including the chosen real model,
approved evidence and bounded paid-call scope. Recheck current daemon ownership
and coordinate a normal foreground restart; do not terminate an unrelated or
user-active process based on a stale descriptor. No auto-start service is added.

Use actual Pi `/learn` on the approved Go changeset. Inspect the exact preview,
confirm once, require Q1/Q2/Q3, enter clearly identified diagnostic answers,
exercise one edit, confirm assessment, answer F1 only if requested, and require
the final label plus a successful `/learn-history` lookup. A single successful
flow uses one question call and at most two assessment calls. Failed calls are
not automatically repeated; further attempts require an explicitly agreed scope.
Any saved diagnostic record is disclosed; no history deletion is authorized.

Verify cancellation and settings preservation without additional paid calls;
verify Session filtering/provenance and failure paths with deterministic tests.
No source-bearing transcript/log capture or fixture made from user answers.
Allowed edits are acceptance documentation/checkpoints only. If a new product
defect appears, record it, update the plan and obtain the required phase scope
rather than implementing an unreviewed change during acceptance.

Phase 3 was attempted on 2026-09-05 with the explicitly approved
`deepseek/deepseek-v4-pro` model and `d93dfa5^..d93dfa5` changeset. The enriched
preview succeeded and showed two files, six declarations, 2,197 changed-excerpt
bytes, 3,151 total repository-derived bytes, partial Go context, and no
truncation. The one authorized question-generation call then failed inside the
isolated worker and was not retried; no assessment call or history record was
created. `/learn-history` remained empty. The Pi settings sentinel retained its
exact SHA-256, size, mtime, inode, mode, and owner, and the foreground daemon was
stopped cleanly.

Read-only follow-up isolated the failure to stream accounting. Pi events carry a
cumulative `partial` assistant message on every delta, while the worker counts
the complete serialized event each time. A provider-free intercepted transport
probe with the actual Pi 0.84.3 SDK accepted 2,000 unique reasoning bytes in one
delta but rejected the same 2,000 bytes split across 2,000 deltas. The current
bound therefore grows quadratically with provider chunking rather than with
unique streamed content. Fine-grained DeepSeek high-reasoning output can fail
despite remaining far below the intended 2-MiB content limit. Phase 3 acceptance
criterion 7 remains unmet.

### Phase 4: Correct stream accounting and repeat actual acceptance

Requires explicit Phase 4 authorization, including a renewed real-model call
allowance for the final acceptance retry. Keep `internal/evaluator` as the deep
module and preserve the existing 2-MiB logical stream-content bound, 64-KiB
assistant-text bound, strict event ordering, tool rejection, zero retry, process
deadline, private framing, safe public errors, and final Go schema validation.

Replace cumulative-event serialization accounting with deterministic linear
accounting over newly emitted text/thinking deltas plus bounded per-event
overhead. Validate end/final content consistently without charging repeated Pi
`partial`, `content`, and final-message copies as new provider output. Retain an
independent event-count bound so a stream of empty events cannot evade resource
limits. Do not expose or persist raw provider events, reasoning, model output,
source, credentials, or error details.

Add actual-SDK intercepted-transport regressions proving that equivalent unique
content is accepted independently of provider chunking, while genuinely
oversized unique content, excessive event count, tool calls, invalid ordering,
and error/abort/length outcomes still fail closed. Run the full Phase 1/2
verification before restarting the matching foreground daemon. The live retry
must use only the separately approved model, changeset, question/assessment call
limits, and diagnostic-history scope, then repeat every Phase 3 UI, history, and
settings-preservation check. No dependency, public protocol, database, prompt,
credential, setting, installed-Pi, or unrelated behavior change is allowed.

Phase 4 completed on 2026-09-05. The private worker now accounts only newly
emitted text/thinking delta bytes plus fixed per-event overhead and independently
caps the stream at 32,768 events. It validates exact contiguous content-block
ordering and equality among joined deltas, content-end values, and the final
assistant message without charging Pi's cumulative copies as new output.
Actual-SDK intercepted-transport regressions cover chunking invariance, genuine
content overflow, event-count exhaustion, and invalid event ordering; the
pre-existing tool, completion, framing, deadline, cancellation, and final-output
tests remain green.

The renewed controlled flow also completed with Pi 0.84.3,
`deepseek/deepseek-v4-pro` at high thinking, and
`d93dfa5^..d93dfa5`. One question call and one initial assessment call ran, with
no F1, retry, or fallback. The UI exercised Q1/Q2/Q3 collection, one accepted
answer edit, final `understood` rendering, and `/learn-history`; one authorized
source-free diagnostic record remains. The Pi settings sentinel was unchanged,
and the foreground daemon's descriptor and token were cleaned up after exit.

## 12. Acceptance Criteria

1. Both evaluator adapters use a fresh bounded child and the exact installed Pi
   runtime, with no CLI-built-in commands, extensions, AgentSession or tools.
2. Global/project settings and Pi Session files remain unchanged after success,
   failure, timeout and cancellation; credential handling stays Pi-owned.
3. Exactly one selected model stream runs per user-confirmed turn, with zero
   configured provider retries and no output-repair/model fallback.
4. Prompt/input fidelity, budgets, single-use capabilities, Session isolation,
   source-free history, one-F1 limit and label mapping continue to pass.
5. Errors identify the failing stage safely and require deliberate new work
   rather than suggesting that repeatedly submitting the same request is safe.
6. Existing suites and the actual-SDK no-provider regression gate pass.
7. The complete controlled actual-Pi flow succeeds and preserves settings. A
   passing probe, generated questions alone, or fake assessment is insufficient.
8. Stream resource accounting is linear in unique model content and bounded
   event overhead, not cumulative Pi `partial` snapshots or provider chunking.

## 13. Verification

Design-only: `scripts/test-agent-infra.sh`, `scripts/validate-agent-infra.sh`,
`git diff --check`, full status/stat/diff and new-file review.

Implementation: focused `go test ./internal/evaluator ./internal/daemon`, then
`go test ./...`, `go test -race ./...`, `go vet ./...`, `npm run typecheck`,
`npm test`, both governance checks and whitespace/full-diff review. Preserve
Go 1.21 compatibility with the established `CGO_ENABLED=0` suite and verify a
current compatible installed Go toolchain without inherited-GOROOT mismatch.
Use the existing release self-test if embedded-worker/runtime-smoke changes
affect executable packaging; no signing, publication or hosted dispatch required.

New tests must exercise the actual SDK in synthetic/no-network conditions and
capture request shape/options through a fake provider transport, not a mocked
success that skips runtime creation. Include settings/Session sentinel bytes,
read-only settings storage with malformed-input refusal, child-only proxy setup,
zero discovery, absent/invalid SDK and Node, exact model selection, startup
failure, all stream limits, tool-call rejection, error/abort/length completions,
malformed JSON, duplicate frames, cancellation/reaping and both input versions.
Use a built-in provider with injected credentials and intercepted fetch; do not
register a synthetic provider on the production runtime because registration
triggers a broad availability refresh. Validate proxy/header presence and
option values without exposing their contents.
Mandatory acceptance checks cannot be silently skipped when SDK/Node is missing.

Live verification remains manual, explicitly scoped and separate from automated
tests. Record versions, selection, safe stage/result metadata, call counts and
settings-preservation results, never raw provider output or source/answer logs.

## 14. Resolved Gates

- ADR-0010 and each of Phases 1 through 4 were explicitly authorized on
  2026-09-05. All four phases are complete in the working tree.
- The global transport audit is resolved for the current setup and the SDK seam:
  production uses Pi's read-only settings semantics and pinned transport helpers
  as specified above, and actual-SDK tests verify that malformed or unsupported
  settings fail closed before provider dispatch.
- Exact private framing and installation validation are implementation details
  locked down in Phase 1 tests; public contracts remain unchanged.
- The sandbox rejects listener tests with `EPERM`, but the same complete npm
  suite passed in the approved normal local environment. The exact Go 1.27.1
  release self-test also passed, including the native ARM64 foreground smoke.
- The first Phase 3 question allowance remains consumed by its failed attempt
  and was not retried. Phase 4 used its separately authorized allowance: one
  question call, one assessment call, no F1, and no retries.
- The accepted personal local-use goal is complete. Commit/push and any future
  public signing, notarization, publication, SSE, or durable worker work remain
  separate user-authorized tasks.
