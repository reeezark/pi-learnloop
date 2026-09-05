---
id: isolated-pi-model-runtime-phase-1
plan: isolated-pi-model-runtime
phase: 1
status: superseded
updated: 2026-09-05
---

# Context

## Goal

Continue fixing until LearnLoop is usable as a local Pi extension, including
actual questions, answers, assessment and history. Public distribution is not
the user's completion criterion.

## Current Phase

ADR-0010 is accepted and Phase 1 was explicitly authorized on 2026-09-05. The
Phase 1 implementation and all non-listener verification are complete. The plan
is blocked at Phase 1 because this execution environment rejects every test
listener on `127.0.0.1`; mandatory daemon, extension-client, and packaged
foreground-smoke verification cannot advance far enough to exercise requests.
The active thread goal remains incomplete.

## Completed

- Rechecked main at `dd91d494f2fbf96dd0745f36150ef92c02466fec`, tracking
  origin/main. No fresh remote check or push occurred.
- Preserved the existing four dirty release-closeout documentation paths.
- Actual Pi 0.84.3 TUI verification used a no-session, no-tools, offline-startup
  invocation loading only the local LearnLoop extension. `/learn` selected
  `d93dfa5^` to `d93dfa5`, rendered the same two files/six declarations/3,151
  estimated evidence bytes and showed the active DeepSeek model confirmation.
  One confirmed question attempt reproduced the generic error within one second.
  No answers were supplied; the temporary TUI was exited. The user-owned daemon
  was neither stopped nor restarted.
- Two no-prompt real CLI probes confirmed successful setup replies but a
  nonempty command list: `[{name: "llama", source: "extension"}]`. The minimized
  assertion exited 1 with `FAIL: LearnLoop requires an empty command list`.
  They sent no prompt, printed no raw stderr or credential data and reaped their
  child processes. Source inspection confirms the production check precedes
  the prompt send and the CLI adds the hidden built-in factory unconditionally.
- Inspected Pi's RPC setters and file-backed SettingsManager saves. Current
  global Agent retry and auto-compaction are false; provider retries are zero.
  Values before the original user attempts were not recorded. No restoration
  was attempted. Do not claim the old evaluator has no settings side effects.
- Investigated the exported Pi ModelRuntime rather than weakening the command
  gate. A Node stdin probe against the installed SDK, with a fresh temporary
  config directory, in-memory models and injected empty credential store,
  resolved the DeepSeek model and passed: zero extension providers, zero
  credential reads/writes, zero created files, zero model calls, no Session.
- Completed the non-secret transport/configuration audit without printing raw
  settings or proxy values. The current global settings have no proxy,
  transport, HTTP/WebSocket timeout, provider-retry or thinking-budget override;
  no proxy environment variable or `models.json` exists. Effective settings are
  transport `auto`, 300,000-ms HTTP idle timeout and 60,000-ms retry-delay cap.
  The inspected settings file's size, mtime and inode remained unchanged.
- Proved Pi's `SettingsManager.fromStorage` can consume exactly one synthetic or
  actual global snapshot with project trust disabled and zero storage writes.
  A malformed snapshot produced one drainable error and stopped before any
  model/network action. Synthetic proxy application and dispatcher setup stayed
  inside the child and exposed no proxy value.
- Exercised the actual built-in DeepSeek provider through `ModelRuntime` with an
  injected credential and intercepted fetch. It produced one synthetic success,
  one credential read, zero writes, no extension providers, no Session and no
  real network/user-file access. The captured safe request shape had the exact
  model, no tools, `tool_choice: none`, `reasoning_effort: high`, an AbortSignal
  and one POST target host. Current Pi attribution for DeepSeek adds no header.
- Source inspection located the missing CLI wrapper semantics: global proxy and
  HTTP dispatcher setup plus transport, HTTP/WebSocket timeout, thinking budget,
  retry delay and provider-attribution projection. The Plan/ADR now preserve
  that subset via pinned Pi helpers and read-only settings storage while forcing
  zero provider retries. Synthetic provider registration is rejected as a test
  strategy because it triggered a broad availability refresh (201 credential
  reads with the injected store); the built-in-provider intercepted-fetch seam
  is narrower and passed with one read.
- Drafted the bounded three-phase repair plan and ADR, and recorded the user's
  local-use direction in stable/release-scope documentation.
- Replaced the production Pi CLI/RPC lifecycle with an embedded private
  `internal/evaluator/pi_model_worker.mjs` worker. Go resolves and freezes Node,
  the selected `pi`, its owning exact Pi 0.84.3 package, SDK entry, and pinned
  settings/HTTP/attribution helpers at startup with bounded, confined reads.
- Both established evaluator adapters now use one shared startup preflight and
  a fresh Node child for each question, initial assessment, or F1 turn. The
  worker runs from the embedded source in memory, receives runtime values only
  through one strict LF-framed stdin request, and returns one bounded response.
- The worker creates `ModelRuntime` without network catalog refresh, resolves
  the exact selected model, sends one user message with `tools: []`, forces
  `toolChoice: none` and `maxRetries: 0`, and rejects tool calls, error/abort/
  length outcomes, unknown events, incomplete streams, overflow, and invalid
  assistant identity/content. There is no CLI main, AgentSession, resource
  loader, Session, extension registry, model fallback, repair, or product retry.
- Global Pi settings are read at most once into a write-forbidden storage
  adapter with project trust disabled. The worker validates and projects Pi's
  proxy, HTTP dispatcher, transport, timeouts, thinking budgets, retry-delay,
  and attribution behavior inside the child. Settings, proxy values, headers,
  credentials, and raw provider errors do not cross the private boundary.
- Preserved the public evaluator interfaces, HTTP routes and payloads, evidence
  and assessment schemas, released prompts, history schema, Session provenance,
  limits, label rules, Go 1.21 baseline, and dependency graph. Removed only the
  obsolete internal CLI-argument and RPC framing mechanics; no UI behavior was
  changed.
- Added fake-worker coverage for path/version/package/helper preflight, shared
  startup, both input versions, exact request fidelity, bounds, malformed and
  duplicate framing, child failure, cancellation, timeout, and reaping.
- Added 17 actual Pi 0.84.3 SDK tests using built-in providers, synthetic
  credentials, and intercepted transport. They prove one exact no-tools call,
  zero extension providers, zero provider retry, no real network, read-only
  settings, unchanged Session sentinels after success/failure/timeout/cancel,
  child-local proxy projection, Session-free attribution, and stream/output
  failure behavior.

## Modified Files

Phase 1 changed `internal/evaluator/pi_rpc.go`, `pi_contract.go`,
`pi_model_worker.mjs` and focused evaluator tests; `internal/daemon/daemon.go`
and its fake-runtime test fixture; `tests/evaluator/pi-model-worker.test.mjs`;
the existing npm test command; the release self-test's fake-runtime fixture;
`README.md`, `PROJECT.md`, `agent/README.md`, this plan, ADR and checkpoint.

The pre-existing dirty release-plan, release Phase 2 checkpoint and release
Phase 3 checkpoint remain separate preserved work. No dependency, prompt,
HTTP/schema, database, installed Pi, credential, Session, or user configuration
was changed. No commit or push occurred.

## Important Decisions

Codebase-design's deep-module boundary remains `internal/evaluator`: discovery,
settings projection, private framing, SDK invocation, bounds, cancellation, safe
errors, and reaping do not leak into daemon routes or the extension. TDD first
captured that the old implementation entered Pi's CLI path, then the same focused
test passed through the ModelRuntime worker. Actual-SDK transport tests supplement
fake-worker protocol tests; neither is represented as a live-model acceptance.

## Tests / Verification

- Passed: 17/17 `node --test tests/evaluator/pi-model-worker.test.mjs` actual-SDK
  tests, using synthetic credentials and intercepted fetch with no real provider.
- Passed: full `internal/evaluator` tests, including a ten-run repeat of the one
  preflight case that timed out once only while two full Go suites were
  intentionally run concurrently. The subsequent ordinary full-suite run passed
  evaluator in 39.719 seconds; the race full-suite run also passed evaluator.
- Passed: Go 1.21.13 pure-Go compile of every package and full tests for every
  non-daemon package; current-toolchain `go vet ./...`; `npm run typecheck`.
- Passed: the selected non-listener daemon unit/handler suite in ordinary and
  race mode, including continuation, assessment, enriched preview, Session
  provenance, history query, strict HTTP-handler, and protected-path behavior.
- Passed outside the final smoke: exact Go 1.27.1 release self-test cross-build,
  independent verification, repeat-build hashes, native version invocation, and
  all malformed-artifact/metadata/argument negative checks. The final packaged
  foreground daemon check failed only because loopback bind was denied.
- Passed: isolated-cache `npm pack --dry-run --json` retained the exact six-file
  package with no bundled dependency; JavaScript syntax checks; `git diff --check`.
- Blocked: `go test ./...` and `go test -race ./...` reached only daemon listener
  failures (`listen tcp4 127.0.0.1:0: bind: operation not permitted`); every
  non-daemon package passed. An exact sandbox-exempt daemon-test request did not
  begin execution and was terminated after waiting, so it supplied no result.
- Blocked: `npm test` passed 56/75 tests; all 19 failures occurred while each
  daemon-client fixture tried to call `listen(127.0.0.1)`, before request logic.
- Blocked: `scripts/test-release-artifacts.sh` passed all build/static/negative
  gates and then reported `release smoke: foreground verification failed` for
  the same listener restriction. Its first invocation used the default compiler
  and correctly refused it; the rerun used the retained, previously verified
  exact Go 1.27.1 toolchain without a download.
- Passed after the blocked-state update: `scripts/test-agent-infra.sh`,
  `scripts/validate-agent-infra.sh`, and `git diff --check`.
- Final `git status`, tracked `git diff --stat`, and segmented complete tracked/
  untracked diff review confirmed that Phase 1 files match the allowlist, the
  three pre-existing release-closeout documents remain distinct, and there is
  no dependency, extension UI, prompt, protocol, schema, database, or generated
  file change.
- No live model call was made.

## Known Issues

The production mechanism is implemented but not committed. Mandatory listener-
based verification remains blocked by this execution environment, so Phase 1
cannot yet be marked complete or advanced. Phase 2 UI error work and Phase 3
actual-model acceptance remain unauthorized. CodeGraph tools were unavailable;
specific installed and repository files were read without altering the index.

## Remaining Work

Run the mandatory daemon, full npm, and packaged foreground smoke in an
environment that permits random IPv4 loopback listeners. If they pass, change
the plan to `status: active`, `current_phase: 2`, and
`phase_status: awaiting_approval`, preserve this checkpoint, and stop for
separate Phase 2 authorization. Phase 3 still owns paid real-model acceptance.

## Next Step

Resolve only the verification-environment gate and rerun the existing checks;
do not change product behavior merely to accommodate a sandbox that forbids
loopback sockets. No Phase 2 implementation, live provider call, commit, or push
is authorized.

## Do Not Change

Preserve all unrelated dirty files. Do not weaken isolation, fake successful
questions, retry consumed calls, patch installed Pi, change public schemas or
released prompts, or claim complete local usability before the final live gate.
