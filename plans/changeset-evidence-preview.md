---
id: changeset-evidence-preview
status: complete
risk: high
current_phase: 3
phase_status: complete
updated: 2026-08-31
---

# Changeset Evidence Preview

## 1. Goal

Implement the first end-to-end product increment for Pi LearnLoop: after the user explicitly invokes `/learn`, the Pi extension submits an explicit Git changeset selection to the local Go daemon, and the user can inspect a bounded preview of the changed Go symbols before any source is sent to an evaluator.

This task proves the Pi-to-Go integration and the code-evidence path. It does not yet ask learning questions or call a model.

## 2. Background

The Agent-development foundation is complete, and Phase 1 has added the internal evidence-preview module without introducing a released command or protocol. `PROJECT.md` defines the intended product and architecture; `plans/agent-development-foundation.md` explicitly excluded runtime implementation from the earlier foundation task.

The evidence preview is the narrowest useful product slice because later assessment quality and privacy both depend on selecting the correct changed symbols. Building it first avoids coupling the evaluator to an unverified changeset model.

Official Pi documentation checked on 2026-08-31 confirms that:

- project-local TypeScript extensions can register `/learn` with `pi.registerCommand()`;
- extension command contexts expose `ctx.cwd` and read-only Session state;
- RPC mode is JSONL over stdin/stdout and is intended for non-Node integrations;
- `--tools` is a strict tool allowlist, but Pi itself is not an operating-system sandbox.

References:

- <https://pi.dev/docs/latest/extensions>
- <https://pi.dev/docs/latest/rpc>
- <https://pi.dev/docs/latest/usage>
- <https://pi.dev/docs/latest/security>

## 3. Current Behavior

- Phase 1 is complete: `go.mod` uses `github.com/reeezark/pi-learnloop`, and `internal/evidence` implements and tests the bounded changed-Go-declaration preview.
- `docs/checkpoints/changeset-evidence-preview-phase-1.md` records the verified Phase 1 handoff.
- Phase 2 is complete: `pi-learnloop daemon` provides the accepted authenticated `/v1` loopback protocol through `internal/daemon`.
- Runtime discovery, per-start token authentication, single-instance locking, strict request bounds, safe error mapping, cancellation, and graceful shutdown are implemented and tested.
- Phase 3 is complete: `package.json` defines the `pi-learnloop` Pi package, and the TypeScript extension registers the manual `/learn` evidence preview.
- The extension implements protected discovery, status-before-token validation, proxy-independent authenticated requests, at most one discovery retry, strict v1 response validation, explicit selection UI, preview rendering, and recoverable errors.
- There is no database, SSE, evaluator, model call, persistence, automatic reminder, or broader learning workflow implementation.
- ADR-0002 is accepted and implemented for the Phase 2 scope.
- The `main` branch has no commits and all current files are untracked.
- No Git remote is configured; the canonical module path is recorded in `go.mod`.
- The local development machine currently provides default Go 1.21.13, installed Go 1.26.4, Node 26.0.0, npm 11.17.0, and Pi 0.84.3.
- `scripts/test-agent-infra.sh` and `scripts/validate-agent-infra.sh` pass.

## 4. Relevant Call Chain

The implemented runtime call chain is:

```text
Operator starts pi-learnloop daemon
→ daemon publishes protected local discovery state
→ authenticated local client submits an explicit Git selection
→ authenticated loopback request to the Go daemon
→ evidence-preview module validates the repository and selection
→ Git diff identifies changed Go files and line ranges
→ Go parser maps changed lines to declarations
→ evidence budget bounds files, symbols, and excerpts
→ daemon returns a versioned inspectable preview
```

The Phase 3 runtime call chain is now:

```text
User manually invokes /learn in a trusted Pi project
→ extension collects one explicit working-tree or commit-range selection
→ client validates protected discovery state and exact loopback URL
→ client verifies the current instance through GET /v1/status
→ client reads the Instance Token and sends the authenticated preview request
→ daemon returns the bounded Phase 1 evidence result
→ extension renders files, symbols, approximate excerpt bytes, and truncation
```

The evidence-preview module is the deep module. Its interface accepts one validated selection and explicit limits, then returns one preview result. Git invocation and Go parsing remain implementation details. The HTTP server and Pi extension are adapters at later seams; they must not duplicate selection or evidence rules.

## 5. Relevant Files

- `AGENTS.md`: risk, authorization, scope, verification, and checkpoint rules.
- `PROJECT.md`: approved target behavior, architecture, security defaults, and non-goals.
- `plans/agent-development-foundation.md`: completed foundation and runtime exclusions.
- `docs/checkpoints/agent-development-foundation-phase-2.md`: verified handoff state.
- `docs/decisions/ADR-0001-agent-development-lifecycle.md`: governing lifecycle decision.
- `docs/decisions/ADR-0002-local-daemon-protocol-security.md`: accepted Phase 2 protocol and security contract.
- `docs/checkpoints/changeset-evidence-preview-phase-1.md`: completed Phase 1 evidence and remaining boundary.
- `internal/evidence/`: implemented Phase 1 deep module to be called by, not duplicated in, the daemon adapter.
- `internal/daemon/`: implemented Phase 2 runtime, discovery, authentication, protocol, and lifecycle module.
- `cmd/pi-learnloop/`: implemented foreground daemon command adapter.
- `docs/checkpoints/changeset-evidence-preview-phase-2.md`: verified Phase 2 handoff and remaining boundary.
- `agent/policies/evaluator-capabilities.json`: future evaluator capability ceiling; no runtime enforcement exists yet.
- `scripts/test-agent-infra.sh` and `scripts/validate-agent-infra.sh`: current authoritative validation.

## 6. Scope

- Establish the Go module after confirming its canonical module path.
- Implement explicit Git commit-range and working-tree selection for Go repositories.
- Map changed diff lines to Go functions, methods, types, interfaces, variables, constants, and related declaration spans.
- Produce a deterministic, bounded evidence preview with truncation metadata and no model call.
- Add a loopback-only daemon adapter with local client authentication.
- Add a thin project-distributable Pi extension that registers `/learn`, collects the selection, and renders the preview.
- Add focused unit, race, integration, and fixture-based verification supported by the introduced manifests.
- Update stable project facts and record long-lived protocol/security decisions when those phases are authorized.

## 7. Out of Scope

- Question generation, answer collection, follow-up questions, scoring, or assessment labels.
- Production evaluator prompt, live Pi RPC evaluator, or model/provider selection.
- SQLite, durable jobs, leases, retry state, event cursors, or learning history.
- Automatic reminders, hooks, background Session indexing, or automatic `/learn` triggering.
- Selecting multiple historical Pi Sessions in this first slice.
- Web UI, mobile integration, multi-Agent orchestration, remote control, or non-Go language support.
- Release automation, CI provider setup, code signing, notarization, or cross-platform packaging.

## 8. Proposed Changes

### 8.1 Evidence-preview module

Phase 1 created one internal Go module whose interface takes:

- canonical repository root;
- one explicit changeset selection: commit range or working tree against a base revision;
- explicit evidence limits supplied by the caller.

It returns:

- resolved revisions and repository identity;
- changed Go files and diff ranges;
- mapped declaration identity, kind, location, and bounded excerpts;
- omissions and truncation reasons;
- deterministic errors for invalid selections, non-Go changes, parse failures, and exceeded limits.

The module uses real temporary Git repositories in tests. Git is a local-substitutable dependency, so no public Git port was added merely for mocking.

### 8.2 Loopback daemon adapter

Add the smallest daemon interface needed to request an evidence preview. The adapter must:

- bind only to `127.0.0.1`;
- authenticate every product request with a locally stored secret;
- reject malformed repositories and any evidence path that escapes the resolved repository root;
- enforce request size, evidence limits, timeouts, and cancellation;
- expose structured error codes without leaking unrelated local paths or source.

ADR-0002 defines the accepted endpoint, payload schema, local state directory, and port-selection behavior. Phase 2 implementation was separately authorized on 2026-08-31.

### 8.3 Thin Pi extension adapter

Add a TypeScript Pi package that:

- registers `/learn` only;
- uses `ctx.cwd` as the candidate repository and verifies project trust before reading project-local configuration;
- asks the user to choose a supported explicit Git selection;
- calls the daemon and renders the returned preview;
- does not implement Git parsing, Go analysis, evidence limits, storage, or assessment rules.

The extension must use the current official `@earendil-works/pi-coding-agent` extension interface and must not read or export Pi credentials.

## 9. Compatibility

- No runtime callers, stored product data, released commands, or protocols currently exist.
- The Go module path, `/learn` behavior, daemon protocol, authentication file, and evidence limits become compatibility-sensitive once introduced.
- The daemon/extension protocol must be explicitly versioned from its first implementation.
- Runtime support will initially be verified only on macOS ARM64 with the local toolchain. macOS AMD64 support remains a release acceptance requirement but cannot be claimed from this machine alone.
- Pi 0.84.3 is the observed local baseline, not yet the declared minimum supported version.

## 10. Risks

- Diff ranges may map incorrectly to declarations containing comments, generics, generated files, build tags, or syntax errors.
- Rename, deletion, and uncommitted-file semantics can make selections ambiguous.
- Large or adversarial repositories can exceed memory, process, or source-sharing budgets.
- A loopback listener without strong authentication can expose local source to another local process.
- Pi extensions execute with the user's permissions; project trust and tool allowlists are not an OS sandbox.
- Freezing a protocol or module path too early creates avoidable compatibility cost.
- The current all-untracked repository makes normal diff review and rollback weaker until an initial baseline commit exists.

## 11. Implementation Phases

### Phase 1 — Go evidence-preview core

Goal: implement and verify changed-Go-symbol mapping without networking, persistence, TypeScript, or external Go dependencies.

Prerequisites:

- use the confirmed canonical Go module path `github.com/reeezark/pi-learnloop`;
- Phase 1 was explicitly authorized on 2026-08-31;
- decide whether to create an initial Git baseline commit before product code begins.

Allowed files:

- `go.mod`
- `internal/changeset/*.go`
- `internal/evidence/*.go`
- `internal/evidence/testdata/**`
- `PROJECT.md`
- `plans/changeset-evidence-preview.md`
- `docs/checkpoints/changeset-evidence-preview-phase-1.md`

Forbidden changes:

- external Go dependencies or `go.sum`;
- public executable commands, HTTP, SQLite, TypeScript, Pi RPC, production prompts, or model calls;
- generated-code handling beyond explicit detection and reporting;
- default evidence-sharing behavior;
- Git commits unless separately authorized.

Acceptance criteria:

- commit-range and working-tree selections resolve deterministically;
- added and modified Go lines map to enclosing declarations with stable ordering;
- deleted-only, renamed, non-Go, malformed, and out-of-repository inputs have tested outcomes;
- caller-provided limits produce explicit truncation metadata rather than silent omission;
- tests use real temporary Git repositories and assert only through the evidence-preview interface;
- no source leaves the process and no network listener is created.

Verification:

- `gofmt` only on new Go files;
- `go test ./...`;
- `go test -race ./...`;
- `scripts/test-agent-infra.sh`;
- `scripts/validate-agent-infra.sh`;
- `git diff --check`, status, diff statistics, and complete diff review.

### Phase 2 — Authenticated loopback daemon

Goal: expose the Phase 1 module through the smallest versioned local daemon interface.

Design status:

- `docs/decisions/ADR-0002-local-daemon-protocol-security.md` was accepted on 2026-08-31.
- The design uses a foreground, single-instance process; `127.0.0.1:0`; a protected Runtime Descriptor and per-start Instance Token; `GET /v1/status`; and authenticated `POST /v1/evidence-previews`.
- The adapter owns transport validation and fixed public limits, then delegates selection and evidence behavior to `internal/evidence`.
- Phase 2 implementation was explicitly authorized and completed on 2026-08-31.

Prerequisites:

- Phase 1 checkpoint is complete;
- ADR-0002 was reviewed and accepted, including endpoint versioning, authentication, state directory, port discovery, exact limits, threat boundary, and error semantics;
- Phase 2 implementation was explicitly authorized after ADR acceptance;
- no external dependency is proposed. Any later dependency requires separate approval.

Allowed files after both prerequisites are satisfied:

- `cmd/pi-learnloop/*.go`
- `internal/daemon/*.go`
- `internal/daemon/testdata/**`
- `PROJECT.md`
- `plans/changeset-evidence-preview.md`
- `docs/decisions/ADR-0002-local-daemon-protocol-security.md`
- `docs/checkpoints/changeset-evidence-preview-phase-1.md` for the required `current` to `superseded` lifecycle transition
- `docs/checkpoints/changeset-evidence-preview-phase-2.md`

Forbidden changes:

- changes to `internal/evidence`, `go.mod`, `go.sum`, or dependency versions;
- SSE, WebSocket, SQLite, durable jobs, background workers, evaluators, TypeScript, or a Pi extension;
- autostart, `launchd`, remote/non-loopback binding, TLS configuration, telemetry, or public configuration flags;
- Git commits unless separately authorized.

Acceptance criteria:

- only `tcp4` on `127.0.0.1:0` is used, and authentication plus origin, host, and peer validation are enforced by integration tests;
- protected discovery files, token rotation, single-instance locking, stale-state handling, and graceful shutdown follow accepted ADR-0002 semantics;
- strict request decoding, versioned success/error schemas, fixed evidence limits, invalid paths, oversized requests, deadlines, and cancellation are tested;
- the adapter calls the Phase 1 module instead of reimplementing its rules;
- no database, evaluator, background worker, or telemetry is introduced.

Verification:

- `gofmt` only on new Phase 2 Go files;
- focused daemon and command tests introduced by the phase;
- `go test ./...`;
- `go test -race ./...`;
- `scripts/test-agent-infra.sh`;
- `scripts/validate-agent-infra.sh`;
- `git diff --check`, status, diff statistics, and complete diff review.

Completion:

- All Phase 2 acceptance criteria are implemented and covered by focused integration tests.
- No forbidden Phase 2 file or dependency was changed.
- The phase stops at `docs/checkpoints/changeset-evidence-preview-phase-2.md`; Phase 3 remains unauthorized.

### Phase 3 — Pi `/learn` evidence preview

Goal: connect a thin Pi extension to the Phase 2 daemon and render the first end-to-end evidence preview.

Entry investigation completed on 2026-08-31:

- the local executable is `@earendil-works/pi-coding-agent` 0.84.3 under the MIT license, with Node.js `>=22.19.0`;
- the observed extension contract provides `pi.registerCommand()`, `ctx.cwd`, `ctx.hasUI`, `ctx.isProjectTrusted()`, and `ctx.ui.select/input/notify` required by this phase;
- official Pi package guidance requires imported Pi core packages to be declared as `peerDependencies` with a `"*"` range and not bundled;
- a public npm registry lookup returned `404` for `pi-learnloop` on 2026-08-31; this is only an availability observation and does not reserve the name;
- the intended local smoke path is `pi -e ./extensions/pi-learnloop.ts`; the later install paths are `pi install git:github.com/reeezark/pi-learnloop@<ref>` and, only after a separately authorized publish, `pi install npm:pi-learnloop`.

Proposed package and compatibility contract:

- package identity: `pi-learnloop`;
- license metadata: `MIT`, matching the stable project goal in `PROJECT.md`;
- supported baseline: Pi 0.84.3 on Node.js `>=22.19.0`;
- compatibility claim: Pi 0.84.x only until a later version is explicitly verified;
- runtime npm dependencies: none;
- peer dependency: `@earendil-works/pi-coding-agent: "*"` as required by official Pi package guidance;
- development dependencies, all exact: `@earendil-works/pi-coding-agent@0.84.3`, `@types/node@22.19.19`, and `typescript@5.9.3`;
- tests use the built-in `node:test` runner, so no test-framework dependency is proposed.

Prerequisites:

- Phase 2 checkpoint is complete;
- approve or revise the investigated package identity, supported Pi version range, and installation path above;
- explicitly authorize Phase 3 and the exact peer/development dependency declarations above.

Proposed allowed files after both prerequisites are satisfied:

- `.gitignore`
- `package.json`
- `package-lock.json`
- `tsconfig.json`
- `extensions/pi-learnloop.ts`
- `extensions/lib/*.ts`
- `tests/extension/*.test.ts`
- `tests/extension/fixtures/**`
- `README.md`
- `PROJECT.md`
- `plans/changeset-evidence-preview.md`
- `docs/checkpoints/changeset-evidence-preview-phase-2.md` for the required `current` to `superseded` lifecycle transition
- `docs/checkpoints/changeset-evidence-preview-phase-3.md`

Forbidden changes:

- changes to `cmd/`, `internal/`, `go.mod`, or the accepted `/v1` daemon protocol;
- runtime npm dependencies other than the required Pi peer declaration;
- model calls, evaluator behavior, session writes, SQLite, SSE, background work, automatic reminders, or automatic `/learn` invocation;
- package publication, Git commits, release automation, CI configuration, or dependency upgrades.

Acceptance criteria:

- `/learn` is user-triggered and never displays an automatic reminder;
- only explicit supported selections can be submitted;
- the user sees the files, symbols, approximate excerpt volume, and truncation before any evaluation;
- daemon-unavailable, unauthorized, invalid-selection, and empty-Go-change states are understandable and recoverable;
- no model is called and no learning record is persisted.

Verification will include the extension's manifest-derived type/test commands, the Go suite, Agent-infrastructure checks, and a documented manual Pi 0.84.3 smoke test.

Proposed authoritative extension commands after dependency approval:

```text
npm run typecheck
npm test
```

`npm test` will use Node's built-in test runner. The loopback integration coverage will use a temporary protected runtime directory and a real local HTTP server, without contacting an external service.

Completion:

- Package identity, Pi/Node compatibility, installation paths, the required peer declaration, and all three exact development dependencies were explicitly approved.
- The package manifest, lockfile, TypeScript configuration, thin Pi entry, command module, deep daemon-client module, behavior tests, public README, and stable project facts are implemented.
- All Phase 3 acceptance criteria are covered by automated tests and a real Pi 0.84.3 → TypeScript extension → Go daemon manual smoke test.
- No Go source, accepted `/v1` protocol, runtime dependency, model behavior, persistence, automation, publication, CI, or Git history was changed.
- The phase stops at `docs/checkpoints/changeset-evidence-preview-phase-3.md`; this plan is complete.

## 12. Acceptance Criteria

- A trusted project user can invoke `/learn`, explicitly choose a supported Git changeset, and inspect the changed-Go-symbol preview.
- Evidence mapping is deterministic, bounded, and covered by real-Git fixture tests.
- The Pi extension contains presentation and transport concerns only; evidence rules remain local to the Go module.
- The daemon is loopback-only, authenticated, versioned, bounded, and free of model/storage behavior.
- No automatic reminder, evaluator, SQLite storage, telemetry, or unsupported language behavior is added.
- Documentation distinguishes implemented behavior from later target architecture.
- Each high-risk phase stops with a checkpoint and complete verification before the next phase is authorized.

## 13. Verification

The task will progressively add authoritative commands only when its manifests exist. The final task verification must include:

```text
go test ./...
go test -race ./...
scripts/test-agent-infra.sh
scripts/validate-agent-infra.sh
git diff --check
```

Phase 3 must additionally run the exact extension type/test commands defined in its approved `package.json`, a loopback integration test, and a manual Pi smoke test. No unconfigured lint, CI, packaging, or cross-architecture result will be claimed.

## 14. Open Questions

- `Resolved 2026-08-31` — The canonical Go module path is `github.com/reeezark/pi-learnloop`.
- `Not authorized in Phase 1` — No Git commit will be created. All files remain untracked, so final review must include direct content inspection in addition to Git status.
- `Resolved by plan approval 2026-08-31` — The first slice supports commit range plus working tree against a base revision; historical multi-Session selection remains deferred.
- `Resolved 2026-08-31` — ADR-0002 was accepted with exact Phase 2 evidence caps, authentication-file location and permissions, discovery flow, protocol schema, error mapping, and threat boundary; Phase 2 implementation was explicitly authorized.
- `Resolved 2026-08-31` — Phase 2 implemented the accepted ADR without external dependencies, SSE, persistence, evaluators, or Pi extension code.
- `Resolved 2026-08-31` — The user explicitly approved package identity `pi-learnloop`, Pi 0.84.3 as the minimum and 0.84.x as the initially supported line, no runtime third-party dependency, the required `@earendil-works/pi-coding-agent: "*"` peer declaration, and exact development dependencies `@earendil-works/pi-coding-agent@0.84.3`, `@types/node@22.19.19`, and `typescript@5.9.3`. Phase 3 implementation is authorized.
