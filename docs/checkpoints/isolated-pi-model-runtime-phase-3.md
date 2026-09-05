---
id: isolated-pi-model-runtime-phase-3
plan: isolated-pi-model-runtime
phase: 3
status: superseded
updated: 2026-09-05
---

# Context

## Goal

Run the explicitly bounded actual-Pi acceptance flow for ADR-0010 without
changing product code, dependencies, protocols, persistence, prompts, settings,
credentials, or installed Pi state.

## Current Phase

Phase 3 was authorized and attempted on 2026-09-05. It exposed a private worker
stream-accounting defect before questions were returned, so the end-to-end
acceptance criterion remains unmet. The plan now awaits explicit authorization
for Phase 4 remediation and a separately bounded live retry.

## Completed

- Recovered `main` at
  `dd91d494f2fbf96dd0745f36150ef92c02466fec`, equal to the recorded
  `origin/main`, and preserved the existing uncommitted Phase 1/2 and
  release-closeout work.
- Verified Pi `0.84.3`, Node `v26.0.0`, and Go `go1.26.4 darwin/arm64`.
- Verified that the previous daemon descriptor belonged to this exact
  repository, stopped it, and started the current working-tree daemon in the
  foreground. The extension and daemon came from the same checkout.
- Started Pi with only the explicit LearnLoop extension, no Session, tools,
  other extensions, skills, prompt templates, themes, context files, or online
  discovery, using `deepseek/deepseek-v4-pro` with high thinking.
- Ran `/learn`, selected commit range `d93dfa5^..d93dfa5`, and verified the
  canonical preview range `2fac6e6c1d2c44e7817105f7eb55729948f20fc6..`
  `d93dfa5805a7cc172ba2caecfc16d2356422a16c`.
- Verified the preview contained two files, six declarations, 2,197 changed
  excerpt bytes, 3,151 total repository-derived evidence bytes, partial Go
  context, and no truncation. The displayed model, high thinking, 262,144-byte
  evaluator-input cap, possible provider cost, and zero retries were correct.
- Confirmed exactly once. The single authorized question-generation worker call
  failed and was not retried. No answer assessment call ran and no diagnostic
  history row was created; `/learn-history` reported no records for the
  repository.
- Verified after the failed worker call that the Pi settings sentinel remained
  byte-for-byte unchanged: SHA-256
  `cbc0f76efb3c17b69f683cc92431177d29dc31dec93cddd03a3ac203987368e5`,
  mode `0644`, owner UID `501`, size `267`, mtime `1788543608`, and inode
  `28954275`.
- Stopped Pi and the matching foreground daemon. The runtime descriptor/token
  were removed and nothing remained listening on its former port.

## Modified Files

Phase 3 changed only this checkpoint, the Phase 2 checkpoint status, and
`plans/isolated-pi-model-runtime.md`. No business code or dependency changed.
All existing uncommitted Phase 1/2 and release-closeout files remain preserved.

## Important Decisions

The failed question call consumed its one-shot continuation and the authorized
question-call allowance. It must not be retried under the Phase 3 authorization.
No assessment allowance was consumed.

The worker intentionally returned only the existing safe failure category, so
no raw provider event, response, reasoning, output, credential, source, or error
was captured. Read-only code inspection showed that Pi delta events include the
entire cumulative assistant `partial`, which the worker serializes and charges
again for every event. A no-network probe through the actual installed SDK then
proved the defect: one 2,000-byte reasoning delta passed, while 2,000 one-byte
deltas representing the same unique content failed the 2-MiB stream bound.
Accounting is therefore quadratic in provider fragmentation.

While trying to exit Pi, `/exit` was entered as text; Pi has no such local
command in this configuration, so it caused one unintended direct Pi chat model
call outside LearnLoop. The request contained only that literal text and did not
include LearnLoop evidence or answers. It completed successfully, independently
confirming that the selected provider/model credential path was available. No
additional LearnLoop question or assessment request was made. Pi was then exited
correctly with EOF.

## Tests / Verification

- Passed `scripts/validate-agent-infra.sh` after moving Phase 3 metadata to
  `in_progress` before the live run.
- The actual UI preview and stage-specific failure behavior passed their
  observable checks; complete question/answer/history acceptance did not run
  because question generation failed.
- The provider-free actual-SDK intercepted probe returned `accepted` for one
  delta and `rejected` for 2,000 deltas with identical 2,000-byte unique
  reasoning content, reproducing the resource-accounting defect without a
  provider request.
- Settings preservation and foreground-daemon cleanup passed as recorded above.
- No business-code test suite was rerun during the documentation-only Phase 3
  acceptance attempt. Phase 1/2 verification remains recorded in their
  checkpoints.

## Known Issues

`internal/evaluator/pi_model_worker.mjs` counts `JSON.stringify(event)` for each
Pi stream event. Because `event.partial` is cumulative, legitimate fine-grained
high-reasoning streams can exceed the 2-MiB accounting threshold while their
unique content remains small. The complete actual-Pi flow is not yet usable or
accepted.

The direct `/exit` chat invocation was an operator error and an extra model call
outside the authorized LearnLoop call categories. It cannot be undone and must
not be omitted from the call audit.

## Remaining Work

Phase 4 must implement linear unique-content plus bounded-event accounting in
the private worker, add chunking-invariance and adversarial resource regressions,
run the complete established verification, and repeat the actual `/learn` flow
under a renewed explicit model/evidence/call authorization. Q1/Q2/Q3, one answer
edit, initial assessment, optional single F1, final label, `/learn-history`, and
settings preservation all remain required.

## Next Step

Obtain explicit authorization for `isolated-pi-model-runtime` Phase 4 and a new
bounded live-call allowance. Then change only the evaluator worker/tests and the
listed acceptance documentation, verify, restart the matching foreground daemon,
and perform one final controlled flow.

## Do Not Change

Do not retry the consumed Phase 3 question request, reuse its continuation,
silently broaden model-call limits, expose raw worker/provider data, weaken the
2-MiB logical content limit, add retries or fallback, change dependencies,
protocols, persistence, prompts, credentials, settings, installed Pi, or commit
and push without separate authorization.
