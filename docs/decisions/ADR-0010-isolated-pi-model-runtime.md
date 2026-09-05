---
id: ADR-0010
status: accepted
date: 2026-09-05
supersedes: none
---

# ADR-0010: Isolated Pi Model Runtime Instead of CLI Agent Evaluation

## Context

Real Pi 0.84.3 verification on 2026-09-05 contradicted the CLI-isolation
assumptions behind ADR-0003/ADR-0004. The CLI's `main` always adds its built-in
llama extension, even with `--no-extensions`. A no-prompt RPC probe returned one
command, `llama`, source `extension`. LearnLoop's empty-command check therefore
rejects initialization before sending the selected evidence to a model.

The same CLI RPC setup also calls file-backed global-settings setters when
disabling Agent retry and auto-compaction. This violates the intended isolation
from the user's development environment. Original user setting values were not
recorded; an automatic restoration to defaults would be another unsafe change.

The existing tests use fake Pi processes and did not prove these real SDK/CLI
properties. Actual question generation failed after a successful enriched
preview. Generic UI error text concealed the stage and existing safe error code.

Pi 0.84.3 exports `ModelRuntime`, which owns model/provider configuration and
Pi-managed credentials independently of CLI main and AgentSession. A local
provider-free feasibility probe resolved `deepseek/deepseek-v4-pro` with an
injected empty credential store and in-memory model configuration, while
observing no extension provider, credential access, Session or config file.
That probe proves initialization feasibility, not successful model evaluation.

## Decision

Accepted on 2026-09-05. Acceptance amends the private evaluator mechanism selected in
ADR-0003 sections 5/6 and shared by ADR-0004 section 6. It does not supersede their
remaining contracts, nor any HTTP, evidence, assessment, history or prompt schema.

### One deep evaluator module, one process per turn

Keep the existing Go question and assessment interfaces. Their shared private
implementation starts a fresh Node process running an embedded LearnLoop worker
and imports the matching installed Pi SDK. It invokes Pi's `ModelRuntime` for
exact model selection and one provider stream, not Pi CLI, RPC Agent main or an
AgentSession. No extension factories, resource loader, project discovery,
SessionManager, automatic compaction or Agent retry loop is instantiated.

The worker has a narrow private pipe contract, not a public endpoint or general
process-control interface. Node, Pi installation and SDK paths are resolved,
validated and frozen by the daemon, never selected by an HTTP client, reviewed
repository, prompt or model. Worker code is embedded and executed from memory;
source-bearing inputs and results are never written to files.

### Preserve Pi-owned transport, remove ambient capabilities

Pi remains the only owner of provider/model configuration and auth resolution,
including its normal credential refresh when required. LearnLoop does not copy
auth files, export credentials, implement a provider HTTP client or feed secrets
through Go, argv, HTTP, prompts, logs or history.

Each call provides the exact released system prompt, one validated user input,
the selected model/thinking options and `tools: []`. There is no Agent prompt
decoration, working-directory annotation, development Session or Session ID.
Provider retries are explicitly zero for this call without changing settings.

The worker snapshots global settings once through a storage adapter that can
only return bytes and rejects writes, then uses Pi's exported `SettingsManager`
with project trust disabled. It fails on every load/validation error. Pi's
`FileSettingsStorage` and every settings setter are forbidden. The worker uses
the pinned 0.84.3 HTTP proxy/dispatcher and provider-attribution helpers, with no
Session ID, and forwards the effective transport, timeouts, thinking budgets and
retry-delay cap to `ModelRuntime`. `maxRetries` is always overridden to zero and
`toolChoice` to `none`. Settings, proxy values and headers remain child-local and
never appear in Go, pipes, HTTP, prompts, history, logs or errors. Unsupported
configuration fails before provider dispatch rather than being silently ignored.

A tool call, failed/aborted/partial completion, unexpected stream structure or
invalid output is a failure, not an instruction to execute tools or repair the
answer. Go retains strict schema, size, duplicate-key and citation validation,
the existing 120-second deadline and stream/output limits, cancellation and
unconditional process reaping. No retry, alternate model or CLI fallback exists.

### Version and local installation compatibility

Require exact Pi 0.84.3 and the already required Node >=22.19.0. Both CLI and
owning importable package must match. Discovery is bounded and confined to that
installation, with no npm/network lookup or untrusted working-directory module
resolution. A standalone Pi executable without its matching SDK is unsupported
and leaves evaluation unavailable. No dependency or installed-Pi modification is
part of this decision; Go remains at language baseline 1.21.

### Safe failure visibility and real verification

Use existing HTTP error codes with stage-specific UI explanations; never expose
raw SDK/provider responses. A confirmation is not evidence that the provider was
contacted. Preserve explicit preview/answer confirmations and single-use state.

Real SDK isolation and settings-preservation checks with a fake provider become
mandatory alongside process/protocol tests. The final local-use gate is actual
Pi question generation, editable answers, assessment and history under an
explicitly bounded live-call authorization. No public release is required.

## Alternatives

- **Ignore or allowlist `llama`:** this treats an unexpected capability as safe
  without removing its registration and does not fix global-settings writes.
- **Reuse the current Pi Session or an in-process AgentSession:** couples the
  evaluator to development context, tool/resource lifecycle and settings.
- **Build a custom AgentSession with in-memory settings:** feasible, but still
  introduces Agent/tool registries, prompt decoration, session state and RPC
  lifecycle that a single model turn does not need. ModelRuntime is narrower.
- **Patch/upgrade installed Pi or wait for upstream CLI flags:** changes the
  supported dependency or workstation installation and does not repair the
  currently pinned, verified local use case within this scope.
- **Copy settings/auth into a temporary agent home:** introduces credential
  copies, refresh synchronization and cleanup obligations; no such mirror is
  allowed. Unknown prior preferences must not be guessed during restoration.
- **Call DeepSeek or another provider directly:** duplicates credential and
  transport logic, abandons Pi model configuration and adds another adapter.
- **Retry, return canned questions or loosen JSON validation:** conceals failure
  without delivering the evidence-backed learning contract.

## Consequences

The existing evaluator module gains a private embedded JavaScript worker and
trusted SDK/Node discovery, but callers and data formats stay stable. Both
question and assessment paths receive the same fix. Node/SDK layout becomes an
explicit evaluator requirement, and real SDK tests become necessary to avoid
another false confidence gap from fake CLI fixtures.

SDK catalog/auth processing has its own in-process allocations and Pi-managed
I/O; source-stream budgets are not a claim of a universal worker memory limit.
Only model transport, pinned provider-attribution headers and Pi's required
configuration/auth handling are allowed. There is no startup catalog network,
user extension, analytics/telemetry request or source discovery. Preserve the
existing threat-model limitation for a malicious same-user process or
compromised installed Pi.

No database migration, released prompt edit, new provider dependency, automatic
startup, public release, or automatic global-settings restoration is authorized.
The plan is `plans/isolated-pi-model-runtime.md`; every high-risk phase still
requires explicit authorization.
