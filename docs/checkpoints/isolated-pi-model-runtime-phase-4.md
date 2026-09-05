---
id: isolated-pi-model-runtime-phase-4
plan: isolated-pi-model-runtime
phase: 4
status: current
updated: 2026-09-05
---

# Context

## Goal

Correct the private Pi worker's nonlinear stream budget, prove the resource
contract with the actual installed Pi 0.84.3 SDK without network access, run the
complete established verification, and repeat the explicitly bounded actual-Pi
acceptance flow.

## Current Phase

Phase 4 is complete. The accepted personal local-use objective in
`plans/isolated-pi-model-runtime.md` is satisfied; no further phase in that plan
is authorized or required.

## Completed

- Preserved `main` at
  `dd91d494f2fbf96dd0745f36150ef92c02466fec`, equal to the recorded
  `origin/main`, together with all authorized uncommitted Phase 1/2 and
  release-closeout work.
- Replaced cumulative-event serialization charging with linear accounting for
  newly emitted text/thinking delta bytes plus fixed per-event overhead.
- Added an independent 32,768-event limit, exact contiguous content-block index
  checks, and equality checks across deltas, content-end values, and the final
  assistant message. Cumulative Pi `partial` copies are ignored for charging;
  repeated content-end/final copies are validated without being charged as new
  provider output.
- Added provider-free actual-SDK regressions proving equal unique content has
  the same content charge regardless of chunking, while true content overflow,
  event exhaustion, and invalid event ordering fail closed.
- Kept the correction inside the private `internal/evaluator` module. No caller,
  HTTP route, public contract, storage schema, prompt, dependency, model
  selection, retry/fallback policy, setting, credential, or installed Pi file
  changed.
- Started the current working-tree daemon in the foreground and ran the exact
  authorized Pi 0.84.3 flow with `deepseek/deepseek-v4-pro`, high thinking, and
  `d93dfa5^..d93dfa5`.
- Verified the canonical range
  `2fac6e6c1d2c44e7817105f7eb55729948f20fc6..`
  `d93dfa5805a7cc172ba2caecfc16d2356422a16c`, two files, six declarations,
  2,197 changed-excerpt bytes, 3,151 repository-derived evidence bytes, partial
  Go context, and no truncation before confirmation.
- Exercised three multiline answers, the fixed-ID review menu, one accepted
  answer edit, the initial sharing confirmation, complete assessment rendering,
  and `/learn-history` without storing any source, question, answer, feedback,
  reasoning, transcript, or raw provider output in this checkpoint.
- The result was `understood` with all three verdicts `demonstrated`. The call
  audit was one question generation, one initial assessment, no F1, no retry,
  and no fallback, within the authorized maximum of one question and two
  assessment calls.
- Retained the explicitly authorized source-free diagnostic history record
  `lr1-obhkoAutwwUe8t_0OW1gr_Uyn1dsJs__d1UXOGF0Dlc`.
- Exited Pi with EOF, stopped the matching foreground daemon, and verified its
  runtime descriptor/token were removed. The protected lock file remains as
  designed.
- Verified the Pi settings sentinel was unchanged in SHA-256, mode, owner,
  size, mtime, and inode before and after the live flow.

## Modified Files

Phase 4 changed `internal/evaluator/pi_model_worker.mjs`,
`tests/evaluator/pi-model-worker.test.mjs`, this checkpoint, the Phase 3
checkpoint status, `plans/isolated-pi-model-runtime.md`, `PROJECT.md`, and
`README.md`. Other existing uncommitted Phase 1/2 and release-closeout changes
remain preserved.

## Important Decisions

The logical stream budget charges each newly emitted text/thinking delta once
and a small fixed amount for every event. The separate event cap prevents
empty-event resource exhaustion. Repeated cumulative snapshots do not affect
the budget; content-end and final-message copies must exactly match the joined
deltas. This preserves the 2-MiB logical stream-content bound and the existing
64-KiB assistant-text bound without coupling acceptance to a provider's
fragmentation pattern.

The deep-module boundary remains unchanged: stream parsing and resource
accounting are private worker responsibilities. No complexity or compatibility
field leaked into the Go adapters, daemon routes, extension client, evaluator
schemas, prompts, or durable history.

The live run was acceptance evidence only. Its retained history row is the
already approved source-free diagnostic record, not evidence of the user's own
understanding, and no raw model content was copied into repository artifacts.

## Tests / Verification

- Focused Phase 4 worker regressions: 4 passed; complete worker suite: 21 passed.
- `go test ./internal/evaluator`: passed.
- `go test ./internal/daemon`: passed.
- `go test ./...`: passed.
- `go test -race ./...`: passed.
- `go vet ./...`: passed.
- `env -u GOROOT GOTOOLCHAIN=local CGO_ENABLED=0 go test -count=1 ./...`:
  passed, preserving the Go 1.21 module baseline with a compatible toolchain.
- `npm run typecheck`: passed.
- `npm test`: the sandbox run reached the listener tests and failed only because
  the sandbox denied `listen(127.0.0.1)` with `EPERM`; the approved normal-local
  rerun passed all 98 tests.
- `scripts/test-release-artifacts.sh` passed with checksum-verified Go 1.27.1,
  including deterministic ARM64/AMD64 builds and native ARM64 foreground smoke.
- Initial long-running Go invocations interrupted during diagnosis are not
  counted as verification; every listed Go command was rerun cleanly to exit 0.
- `scripts/test-agent-infra.sh` and `scripts/validate-agent-infra.sh` passed.
  Tracked and untracked whitespace checks passed, and final status/stat/complete
  diff review found only the preserved authorized Phase 1/2, release-closeout,
  and Phase 4 changes.

## Known Issues

No remaining defect blocks the accepted personal local `/learn` use case. Public
signing, notarization, package publication, SSE, and durable background worker
coordination remain explicitly separate work, not Phase 4 incompleteness.

## Remaining Work

None in `isolated-pi-model-runtime`. The working tree remains uncommitted until
the user separately authorizes commit and push.

## Next Step

Run the final governance and diff gates, report Phase 4 completion, and stop.
Commit or push only after separate explicit authorization.

## Do Not Change

Do not make another model request, delete or rewrite the authorized diagnostic
history row, expose raw provider/model content, add retries or fallback, change
dependencies, protocols, persistence, prompts, credentials, settings, installed
Pi, or commit/push without separate authorization.
