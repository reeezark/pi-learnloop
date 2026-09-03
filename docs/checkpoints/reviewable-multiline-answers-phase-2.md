---
id: reviewable-multiline-answers-phase-2
plan: reviewable-multiline-answers
phase: 2
status: current
updated: 2026-09-03
---

# Reviewable Multiline Answers Phase 2

## Context

### Goal

Use Pi 0.84.3's existing multiline editor for Q1/Q2/Q3 and F1, let the
user review and revise the three initial answers before the unchanged sharing
confirmation, and preserve all evidence, model-isolation, no-retry, Session,
and source-free-history guarantees.

### Current Phase

Phase 2 and the two-phase `reviewable-multiline-answers` plan are complete.
The user explicitly authorized Phase 2 on 2026-09-03.

The Phase 2 baseline is the committed and pushed Phase 1 commit `1e8d793`,
which was verified equal to `origin/main` before implementation began. The
completed Phase 2 changes are in the working tree and have not been committed
or pushed.

## Completed

- Added exactly Pi's public `editor(title, prefill?)` method to the command's
  existing injected UI seam; no custom editor interface, Pi TUI import, or
  second owner for answer rules was introduced.
- Added one confirmation after validated questions and before Q1. It states
  that LearnLoop does not save drafts, the explicitly invoked Pi external
  editor writes a temporary `prompt.md` and starts the configured editor,
  cleanup is best effort, editor/environment artifacts may remain, Pi may
  materialize an oversized draft before validation, and declining sends no
  answer.
- Q1, Q2, and Q3 now open in fixed order with no first-visit prefill. The
  already rendered question set remains authoritative and question text is
  never copied into the answer draft.
- The extension trims once and accepts only nonblank well-formed Unicode text
  of at most 4,096 UTF-8 bytes whose only control character is LF. CR, tabs,
  C0/C1 controls, lone surrogates, whitespace-only values, and oversized
  drafts are rejected locally.
- Invalid candidates are never installed as accepted state, submitted, echoed
  in notifications, or reused as prefill. A generic warning reopens the editor
  with the previous accepted bounded answer, or no prefill when none exists.
- The initial review selector contains exactly `Continue to sharing
  confirmation`, `Edit Q1`, `Edit Q2`, `Edit Q3`, and `Cancel`. It contains no
  answer text. Valid edits replace one answer accepted value; cancelled edits
  preserve it and return to review; cancelling or dismissing review sends no
  assessment request.
- F1 uses the same bounded multiline editor without a second review selector.
  Invalid F1 candidates recover generically and cancellation sends no final
  request, preserving the existing at-most-one non-retried follow-up.
- An old daemon's `invalid_request` now produces a safe matched-update action.
  The extension does not flatten, alter, retry, fall back, or echo the answer.
- Direct Git and explicit Session-bound paths share the same private editor
  state machine. Tests prove Session ID remains outside question and assessment
  inputs and answer text remains outside notifications.
- Updated stable public/project documentation for the editor, review flow,
  32-KiB assessment wire limit, external-editor privacy/resource limits, and
  completed ADR-0008 compatibility contract.
- Added no Go or daemon change, dependency, route, request/response field,
  schema/prompt version, persisted value, Session entry, log, retry, fallback,
  background process, or new command.

## Modified Files

Implementation:

- `extensions/lib/learn-command.ts`

Focused verification:

- `tests/extension/learn-command.test.ts`
- `tests/extension/pi-session-review.test.ts`

Stable and lifecycle documentation:

- `README.md`
- `PROJECT.md`
- `plans/reviewable-multiline-answers.md`
- `docs/decisions/ADR-0008-reviewable-multiline-answers.md`
- `docs/checkpoints/reviewable-multiline-answers-phase-1.md`
- `docs/checkpoints/reviewable-multiline-answers-phase-2.md`

`tests/extension/extension-entry.test.ts` was not changed because its runtime
fake exits before answer collection and is intentionally cast at the Pi
adapter boundary; it did not need to satisfy `LearnCommandContext` directly.

## Important Decisions

- Answer validation, accepted-value ownership, review transitions, and
  cancellation stay private to `learn-command`. Callers see only the existing
  command function and injected Pi UI/client capabilities.
- Only a previously accepted, already bounded answer can become editor
  prefill. First visits and invalid first candidates always reopen without
  prefill, limiting extra copies and preventing rejected text from becoming
  state.
- Review labels use fixed question IDs only. Raw answers appear only in the
  selected editor and the existing confirmed assessment request path.
- The extension explicitly checks for well-formed surrogate pairs before UTF-8
  byte counting so a JavaScript string cannot be silently replaced while
  crossing JSON into Go.
- `invalid_request` remains terminal and non-retryable. Matched daemon and
  extension updates are the only supported recovery for a mixed-version
  multiline or larger escaped request.

## Tests / Verification

Passed on 2026-09-03:

- focused command and Session tests: 35/35.
- `npm run typecheck`.
- `npm test`: 58/58 extension tests with permitted temporary IPv4 loopback
  listeners.
- `npm pack --dry-run --json` with an isolated cache: six package entries,
  25,604-byte archive, 103,586-byte unpacked content, no bundled dependency,
  and no created tarball.
- `go test -p=1 -count=1 ./...` with Go 1.26.4 and an isolated cache: all
  packages passed; daemon 129.222 seconds, evaluator 23.758 seconds, evidence
  169.242 seconds, and history 1.003 seconds.
- `go test -race -p=1 -count=1 ./...` with Go 1.26.4 and an isolated cache:
  all packages passed; daemon 133.554 seconds, evaluator 24.660 seconds,
  evidence 172.436 seconds, and history 3.417 seconds.
- `go vet ./...` and `go build ./...` with Go 1.26.4 and an isolated cache.
- installed Go 1.21.13 with inherited `GOROOT` cleared, `CGO_ENABLED=0`, an
  isolated cache, and serial package scheduling: all packages passed; daemon
  129.205 seconds, evaluator 24.689 seconds, evidence 171.263 seconds, and
  history 0.993 seconds.
- `scripts/test-agent-infra.sh` and `scripts/validate-agent-infra.sh`.
- Git whitespace and complete-diff review.

The first full npm run and two preliminary Go runs were intentionally allowed
to finish and recorded only sandbox failures: local `127.0.0.1` binds and the
user Go build cache were not writable. The same suites passed with permitted
temporary loopback listeners and isolated caches. The first sandboxed
`go build` exited zero but printed a non-fatal user module stat-cache warning;
the final permitted build passed cleanly.

No verification invoked a live provider/model, real Pi Session content,
production repository source, production history database, or external
network service.

## Known Issues

- Explicitly invoking Pi's external editor can write a draft to temporary disk
  and expose it to editor/environment artifacts; cleanup and secure memory
  erasure are not guaranteed.
- Pi may materialize an oversized draft before LearnLoop rejects it. A custom
  editor would require a separately approved design if that limit becomes
  unacceptable.
- A new extension can be rejected by an old daemon for LF or a larger escaped
  body. The failure is intentionally terminal and requires a matched update.
- Editing consumes the existing thirty-minute volatile assessment lifetime;
  expiry still requires a new manual `/learn` flow.
- Pi 0.84.3 still materializes candidate Session messages and metadata during
  manual Session listing, as accepted separately in ADR-0006.
- macOS AMD64 remains an intended but unverified target.
- The configured CodeGraph MCP capability was unavailable during this run, so
  focused inspection used bounded direct source and declaration reads after
  confirming the repository index directory exists.

## Remaining Work

No work remains in this plan. SSE and durable worker coordination remain
separate future capabilities.

## Next Step

Review and commit/push the completed Phase 2 working tree only if explicitly
requested. No additional implementation phase is authorized or required by
this plan.

## Do Not Change

- Do not persist, log, summarize, or append answer drafts to Pi Sessions.
- Do not expose answers in review actions, notifications, errors, evidence,
  Session provenance, or generic history.
- Do not flatten invalid multiline input or retry/fallback after daemon
  rejection.
- Do not change routes, request/response shapes, schemas, prompts, labels,
  model isolation, assessment lifecycle, dependencies, or stored data.
