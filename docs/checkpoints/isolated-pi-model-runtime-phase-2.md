---
id: isolated-pi-model-runtime-phase-2
plan: isolated-pi-model-runtime
phase: 2
status: superseded
updated: 2026-09-05
---

# Context

## Goal

Continue fixing until LearnLoop is usable as a local Pi extension. Phase 2 makes
post-confirmation failures actionable without changing any daemon protocol,
model input, persistence, dependency, selection, confirmation, or retry contract.

## Current Phase

ADR-0010 is accepted. Phase 2 was explicitly authorized on 2026-09-05 and is
complete in the uncommitted working tree. The plan is now paused at the high-risk
Phase 3 authorization gate. No real provider call was made.

## Completed

- Recovered the repository on `main` at
  `dd91d494f2fbf96dd0745f36150ef92c02466fec`, equal to the recorded
  `origin/main`, and preserved all pre-existing Phase 1 and release-closeout
  changes.
- Re-ran the exact Phase 1 daemon preflight/listener test in the sandbox and
  reproduced only `listen tcp4 127.0.0.1:0: bind: operation not permitted`.
  Multiple sandbox-exempt launches stalled before producing test output and
  were terminated; no product failure was inferred from that tool limitation.
- Followed the TypeScript unit-test workflow against the public `/learn` command.
  The initial assessment-timeout test failed with the old question-generation
  message, proving the stage-collapse defect, and passed after the implementation.
- Kept `extensions/lib/learn-command.ts` as the UI boundary. It records whether
  the current remote turn is question generation or answer assessment and maps
  only existing allowlisted error codes to safe messages. The already-correct
  `extensions/lib/daemon-client.ts` propagation was left unchanged.
- Both stages now distinguish unavailable initialization, evaluator failure,
  timeout, invalid evaluator output, lost/changed local daemon connection,
  incompatible response, and expired/consumed capability. Raw daemon/provider
  messages are never rendered. Lost-connection outcomes are explicitly unknown.
- Runtime-unavailable guidance states that initialization can fail before a
  provider request and names the exact Pi 0.84.3 and Node >=22.19.0 prerequisites.
  Compatibility errors require updating the daemon and extension together.
- Question and assessment confirmation text now accurately states that the
  isolated runtime configures zero model retries. All existing selection,
  preview, confirmation, answer-review, cancellation, F1, and one-shot request
  behavior remains unchanged.
- Added local-use guidance requiring the foreground daemon and extension to use
  the same checkout, with a restart after either side changes, plus concise
  troubleshooting for every Phase 2 failure category.
- Advanced the plan to Phase 3 awaiting approval. No dependency, lockfile,
  protocol, route, schema, database, prompt, evaluator input, model output,
  Session provenance, installed Pi, credential, or user setting changed.

## Modified Files

Phase 2 changed `extensions/lib/learn-command.ts`,
`tests/extension/learn-command.test.ts`, `README.md`, `PROJECT.md`, this plan,
and the Phase 1/Phase 2 checkpoint metadata. `extensions/lib/daemon-client.ts`
was inspected and intentionally left unchanged.

All Phase 1 implementation files and the pre-existing dirty release plan and
checkpoints remain preserved. No commit or push occurred.

## Important Decisions

The UI owns only stage-aware rendering of existing safe codes. Node/Pi discovery,
SDK initialization, model execution, cancellation, and raw failure isolation
remain inside the deep `internal/evaluator` module. No new protocol field or
client-side error taxonomy was needed.

Question/assessment calls are not retried after any ambiguous failure because
their daemon-owned capabilities are single-use and a provider may already have
received the request. Starting a fresh `/learn` flow is deliberate user action,
not automatic recovery.

## Tests / Verification

- Passed the red/green focused assessment-timeout test. The red result contained
  the old `could not generate learning questions` text; the green result named
  answer assessment, disclosed no raw detail, and made one request.
- Passed all 47 reported tests/subtests in
  `tests/extension/learn-command.test.ts`, including 16
  table-driven question/assessment safe-error cases, zero-retry confirmation,
  all existing confirmations, cancellation paths, answer review, F1, and history
  behavior.
- Passed `npm run typecheck`.
- Full `npm test`: 75/94 passed. All 19 failures were existing
  `tests/extension/daemon-client.test.ts` fixtures denied while creating
  `127.0.0.1` listeners; all command, Session, registration, and actual-SDK
  tests passed.
- Full serial `go test -p=1 ./...`: all non-daemon packages passed, including
  `internal/evaluator`; `internal/daemon` failed only its existing listener-based
  cases at the same operating-system sandbox denial. No Go source changed in
  Phase 2; the prior Phase 1 race and Go 1.21 pure-Go results remain applicable.
- Passed `go vet ./...` with a writable isolated Go cache, `scripts/test-agent-infra.sh`,
  `scripts/validate-agent-infra.sh`, and `git diff --check`.
- Coverage collection was skipped because neither the user, repository policy,
  nor the local execution source defines a coverage gate for this task.
- The Phase 1 release self-test was not rerun after this TypeScript/documentation-
  only phase; its prior result remains blocked only at the final foreground
  loopback smoke after all build/static/negative checks passed.
- No live provider call, paid request, or actual Pi acceptance flow was run.

## Known Issues

This execution environment still prevents completing the listener-based Phase 1
verification. Passing deterministic command and actual-SDK tests does not prove
end-to-end local usability. The working tree is uncommitted.

## Remaining Work

Phase 3 must run one controlled actual Pi `/learn` flow against an explicitly
approved changeset and real model, then verify Q1/Q2/Q3, one optional F1, final
label, `/learn-history`, settings preservation, and normal foreground daemon
operation. It may make one question call and at most two assessment calls.

Under the repository's high-risk gate, Phase 3 still requires explicit approval
of the model, evidence/changeset, paid-call bound, foreground daemon restart, and
the disclosed source-free diagnostic history record. A broad request to continue
does not substitute for these concrete approvals.

## Next Step

After those Phase 3 details are explicitly authorized, recheck daemon ownership,
restart only the matching foreground daemon, and execute the bounded real flow.
If it reveals another product defect, update the plan and obtain scoped authority
before changing implementation.

## Do Not Change

Do not auto-start or kill an unrelated daemon, retry a consumed model request,
change the approved evidence, copy credentials, patch installed Pi, alter public
protocols/persistence/prompts/dependencies, delete diagnostic history, or claim
normal usability before the actual acceptance flow passes.
