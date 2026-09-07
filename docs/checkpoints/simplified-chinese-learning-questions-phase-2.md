---
id: simplified-chinese-learning-questions-phase-2
plan: simplified-chinese-learning-questions
phase: 2
status: current
updated: 2026-09-07
---

# Simplified-Chinese Learning Questions Phase 2

## Context

### Goal

Complete one explicitly authorized actual-Pi acceptance flow proving that the
current enriched `/learn` path produces human-readable Simplified-Chinese
Q1/Q2/Q3 and, if naturally returned, F1 without changing the established
evidence, model, retry, settings, or source-free-history boundaries.

### Current Phase

Phase 2 is complete. The user explicitly approved sending the confirmed
current-working-tree Go evidence to DeepSeek with possible provider charges,
restarting the matching foreground daemon, at most one question-generation and
two assessment calls, and one retained source-free diagnostic history record.

The run started from `main` at
`da0f396228ed640f984b6e9c54aba71aab78a1ea`, equal to `origin/main`, with the
authorized uncommitted Phase 1 working tree. Nothing has been committed or
pushed.

## Completed

- Started the daemon from the exact current working tree and an ephemeral Pi
  0.84.3 interaction that explicitly loaded only this checkout's extension.
- Fixed the active model to `deepseek/deepseek-v4-pro` with high thinking and
  selected the working tree against `HEAD`, resolved to
  `da0f396228ed640f984b6e9c54aba71aab78a1ea..WORKTREE`.
- Reviewed the enriched preview before confirmation: 8 changed Go files, 27
  declarations, 27,046 changed-excerpt bytes, and 35,917 repository-derived
  evidence bytes. Context was visibly partial with 115
  `external_type_unavailable` and 6 `type_incomplete` omissions; neither
  changed nor context evidence was truncated.
- Confirmed exactly one question-generation call. Q1/Q2/Q3 were natural,
  human-readable Simplified Chinese while Go identifiers, fixed IDs, and
  evidence references remained intact and relevant.
- Completed the multiline answer and fixed-ID review flow, then confirmed one
  initial assessment call. It completed immediately with label `partial`, Q1
  `partial`, Q2 `demonstrated`, and Q3 `demonstrated`.
- No F1 was returned. In accordance with the plan, no second assessment call
  was made solely to elicit one; deterministic and actual-SDK no-network tests
  remain the required F1 gate.
- The exact call audit was one question generation and one assessment, with no
  retry, fallback, repair, translation, Agent turn, or tool call.
- `/learn-history`, which makes no model call, reported source-free complete
  record `lr1-hM9gY5j4ZNb7gQeWoVq1Cr7slpb3eV2uVQL4okuUs3o` for
  `da0f396228ed..WORKTREE`, evidence hash prefix `339bb17b0d20`, and the exact
  current provenance:
  `evaluator-question-generation@2.1.0#260c5a4febec` and
  `evaluator-answer-assessment@2.1.0#046c37a9f523`.
- Exited the ephemeral Pi interaction and stopped the matching foreground
  daemon. Its descriptor and token were removed; only the designed lock file
  remains.
- Verified the global Pi settings file stayed identical before and after: SHA-
  256 `cbc0f76efb3c17b69f683cc92431177d29dc31dec93cddd03a3ac203987368e5`,
  mode `-rw-r--r--`, uid 501, gid 20, size 267, mtime 1788543608, and inode
  28954275.
- Copied no raw question, answer, feedback, source, transcript, reasoning,
  private worker output, credential, or model response into repository
  artifacts.

## Modified Files

Phase 2 changes only acceptance and stable-status documentation:

- `plans/simplified-chinese-learning-questions.md`
- `docs/checkpoints/simplified-chinese-learning-questions-phase-1.md`
- `docs/checkpoints/simplified-chinese-learning-questions-phase-2.md`
- `PROJECT.md`
- `README.md`

All Phase 1 source, tests, prompt assets, evaluator fixtures, governance
association, and documentation remain preserved as reviewed. No Phase 2
business code, dependency, protocol, schema, database, settings, installed-Pi,
or extension change occurred.

## Important Decisions

- The acceptance used the current working tree because it contains the
  uncommitted v2.1.0 implementation under review. `HEAD` and the Go-only diff
  SHA-256 `39818fe2137f1bfefabd8bda374a08fb92c924a77af5452a7d3d90815a5877ad`
  make the selected state recoverable without copying source.
- A complete assessment without F1 satisfies the plan. Forcing a follow-up
  would spend an unnecessary real call and contradict the approved acceptance
  protocol.
- The diagnostic outcome tests the product path, not the user's understanding.
  Its source-free record is retained only because the user explicitly allowed
  it.

## Tests / Verification

Completed live verification:

- matching current-working-tree foreground daemon startup
- ephemeral Pi 0.84.3 with `deepseek/deepseek-v4-pro`, high thinking, and only
  the explicit LearnLoop extension
- enriched working-tree preview and confirmation
- one real question-generation call with visible Simplified-Chinese Q1/Q2/Q3
- multiline answers, fixed-ID review, one real assessment call, and complete
  result rendering
- `/learn-history` prompt-provenance and source-free-record check
- settings fingerprint preservation and Pi/daemon cleanup

`scripts/test-agent-infra.sh`, `scripts/validate-agent-infra.sh`, and
`git diff --check` passed after the lifecycle update. Final status, stat, and
complete tracked plus untracked diff review found only the authorized Phase 1
implementation and Phase 2 acceptance/status documentation. Phase 1's complete
automated, race, vet, Go 1.21 baseline, TypeScript, actual-SDK no-network, and
exact Go 1.27.1 release-artifact verification remains recorded in the
superseded Phase 1 checkpoint; no business code changed in Phase 2.

## Known Issues

- Unicode Han presence remains the accepted deterministic floor; it cannot by
  itself distinguish Simplified Chinese from every other Han-script language.
  The live questions were manually verified as Simplified Chinese.
- The live assessment returned no F1, so F1 remains covered by the Phase 1
  deterministic and actual-SDK no-network tests rather than a paid live sample.

## Remaining Work

None in `simplified-chinese-learning-questions`. Both authorized phases are
complete.

## Next Step

Run the final governance and diff gates, report Phase 2 completion, and stop.
Commit or push only after separate explicit authorization.

## Do Not Change

Do not make another model request, delete or rewrite the authorized diagnostic
history row, copy model-visible or model-returned content into repository
artifacts, broaden localization beyond Q1/Q2/Q3/F1, edit immutable prompts,
change dependencies/protocols/storage/settings/installed Pi, or commit/push
without separate authorization.
