---
id: reviewable-multiline-answers-phase-1
plan: reviewable-multiline-answers
phase: 1
status: superseded
updated: 2026-09-03
---

# Reviewable Multiline Answers Phase 1

## Context

### Goal

Allow bounded LF-only multiline text in initial and F1 user answers while
keeping model-produced question and feedback text control-free, and make the
existing assessment route large enough for every canonically encoded valid
three-answer request.

### Current Phase

Phase 1 is complete. The user accepted ADR-0008 and explicitly authorized
Phase 1 on 2026-09-03.

The implementation started from commit `d63a39e`, which was on `main` and equal
to `origin/main`. The completed Phase 1 changes remain in the working tree and
have not been committed or pushed.

The plan has advanced to Phase 2 with `phase_status: awaiting_approval`. No
Phase 2 extension or stable-documentation work has started.

This checkpoint was superseded after Phase 1 was committed and pushed as
`1e8d793`, and the separately authorized Phase 2 completed on 2026-09-03.

## Completed

- Split private bounded-text validation into a user-answer path and the
  existing strict model-output path without exporting a new interface.
- Initial Q1/Q2/Q3 and F1 answers now accept and preserve U+000A LF under the
  existing nonblank, valid-UTF-8, 4,096-byte per-answer bound.
- User answers still reject CR, CRLF, tab, NUL, DEL, C1 controls, invalid UTF-8,
  whitespace-only multiline text, and over-limit content.
- Model-produced F1 question and evaluation feedback validation remains
  control-free and continues to reject decoded LF.
- Raised only the authenticated assessment-turn request-body limit from 16 KiB
  to 32 KiB before strict JSON decoding. Other endpoint limits are unchanged.
- Added route coverage for initial and F1 multiline submissions and a
  canonically encoded valid request above 16 KiB.
- Preserved the existing above-limit rejection and subsequent successful
  submission test, proving a body above 32 KiB is rejected before assessment
  state is consumed.
- Added no route, request/response field, schema or prompt version, dependency,
  persisted value, history output, extension behavior, retry, or fallback.

## Modified Files

Implementation:

- `internal/evaluator/assessment_contract.go`
- `internal/daemon/server.go`

Verification:

- `internal/evaluator/assessment_contract_test.go`
- `internal/daemon/assessment_test.go`

Lifecycle and decision records:

- `plans/reviewable-multiline-answers.md`
- `docs/decisions/ADR-0008-reviewable-multiline-answers.md`
- `docs/checkpoints/reviewable-multiline-answers-phase-1.md`

`internal/assessment/service_test.go` was not changed because evaluator and
authenticated-route tests exercise the existing service seam, including its
non-consumption behavior, without requiring another duplicate test.

## Important Decisions

- LF allowance belongs to the private answer validator, not to the generic
  assessment-text validator. This keeps user-input policy from weakening the
  model-output contract.
- The new bounded-text helper retains the existing blank, UTF-8, byte-length,
  and Unicode-control checks. It exempts exactly U+000A only when validating a
  user answer.
- The 32-KiB limit remains local to `POST /v1/assessment-turns`. It covers the
  official client's worst valid JSON escaping while retaining a finite
  pre-decode transport bound.
- Assessment lifecycle ownership remains in `internal/assessment`; invalid or
  oversized input does not create a model call or consume the assessment.
- No extension declaration or behavior was changed in Phase 1. Pi editor,
  review actions, disclosure, and stable user documentation remain Phase 2.

## Tests / Verification

Passed on 2026-09-03:

- `gofmt` on all four touched Go files.
- Focused new and changed evaluator/daemon tests for v1/v2 initial LF answers,
  F1 LF answers, rejected controls, strict model-output text, the 32-KiB bound,
  a canonical request above 16 KiB, and non-consumption after rejection.
- `go test -count=1 ./internal/evaluator ./internal/assessment ./internal/daemon`
  with Go 1.26.4 and an isolated cache: evaluator 27.391 seconds, assessment
  0.909 seconds, and daemon 131.630 seconds.
- `go test -p=1 -count=1 ./...` with Go 1.26.4: all packages passed; daemon
  130.528 seconds, evaluator 23.962 seconds, evidence 167.991 seconds, and
  history 1.753 seconds.
- `go test -race -p=1 -count=1 ./...` with Go 1.26.4: all packages passed;
  daemon 133.103 seconds, evaluator 25.476 seconds, evidence 171.306 seconds,
  and history 3.593 seconds.
- `go vet ./...` and `go build ./...` with Go 1.26.4. The successful build
  emitted only a sandbox denial while trying to update a user-level Go module
  stat-cache entry; it did not affect the build or repository.
- installed Go 1.21.13 with `GOROOT` cleared, `CGO_ENABLED=0`, isolated cache,
  and serial package scheduling: all packages passed; daemon 134.990 seconds,
  evaluator 26.375 seconds, evidence 169.201 seconds, and history 2.089 seconds.
- `scripts/test-agent-infra.sh` and `scripts/validate-agent-infra.sh`.
- Git whitespace, status, stat, and complete-diff review, including all three
  newly added lifecycle/decision documents.

The first sandboxed affected-package attempt could not write the default Go
build cache, and a sandboxed full daemon-package attempt could not bind IPv4
loopback. Both were environment denials. The same affected packages passed
with an isolated cache and permitted temporary loopback listeners.

No verification invoked a live provider/model, external network, real Pi
Session, production source copy, or production history database.

## Known Issues

- The installed Pi 0.84.3 editor's external-editor path may create a temporary
  file and invoke an external process; Phase 2 must disclose its best-effort
  cleanup and possible editor-created artifacts before Q1.
- The built-in editor can materialize an oversized paste before LearnLoop
  validates the 4-KiB bound. Phase 2 must keep invalid content local and allow
  recovery without replacing a previous valid answer.
- Old daemons retain the 16-KiB assessment limit. Phase 2 must fail closed with
  the planned update guidance and must not retry automatically.
- macOS AMD64 remains an intended but unverified target.
- The configured CodeGraph MCP capability was unavailable during this run, so
  focused source inspection used bounded direct reads after confirming the
  repository index directory exists.

## Remaining Work

Phase 2 remains unauthorized. It covers Pi's existing multiline editor, the
one-time external-editor disclosure, bounded answer recovery, ID-only review
and editing, F1 editor reuse, old-daemon guidance, extension regression tests,
and stable README/PROJECT updates.

## Next Step

Review and commit/push Phase 1 only if explicitly requested, then explicitly
authorize `reviewable-multiline-answers` Phase 2 before implementation begins.

## Do Not Change

- Do not start Phase 2 without explicit authorization.
- Do not broaden LF acceptance to questions, feedback, other model output, or
  another endpoint.
- Do not normalize CR/CRLF, add retries or fallback, or change assessment
  request/response shapes, schema/prompt versions, evidence, labels, history,
  persistence, or model isolation.
- Do not add a dependency, editor framework, answer store, Session entry,
  autosave, draft recovery, telemetry, or background work.
