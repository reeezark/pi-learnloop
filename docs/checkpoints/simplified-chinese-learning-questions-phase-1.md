---
id: simplified-chinese-learning-questions-phase-1
plan: simplified-chinese-learning-questions
phase: 1
status: superseded
updated: 2026-09-07
---

# Simplified-Chinese Learning Questions Phase 1

## Context

### Goal

Make Q1/Q2/Q3 and optional F1 use Simplified-Chinese natural-language prose on
the current enriched `/learn` path, while preserving v1, identifiers, schemas,
routes, persistence, model selection, answers, final feedback, and all other UI.

### Current Phase

Phase 1 is complete. The user accepted ADR-0011, including the honest Unicode
Han limitation, and explicitly authorized this phase on 2026-09-06.

The implementation started from `main` at
`da0f396228ed640f984b6e9c54aba71aab78a1ea`, equal to `origin/main`. The draft
plan and ADR were the only untracked baseline files. The completed Phase 1
working tree has not been committed or pushed.

The plan has advanced to Phase 2 with `phase_status: awaiting_approval`. No real
provider call, live language acceptance, daemon restart, or Phase 2 edit was
performed.

## Completed

- Accepted ADR-0011 and released new immutable
  `evaluator-question-generation@2.1.0` and
  `evaluator-answer-assessment@2.1.0` prompt assets. Their SHA-256 values are
  `260c5a4febecbd8af86474c7ad642417abc15c8a47f8c211924b219d15947191`
  and `046c37a9f5239f852f072ccb6c78c960b398cee5a0240d58c2027800b618e390`.
- Required natural-language Q1/Q2/Q3 prose in Simplified Chinese in the new
  question prompt and applied the same rule only to F1 in the new assessment
  prompt. JSON fields, fixed IDs, kinds, verdicts, references, Go syntax, code
  identifiers, and established technical terms remain unchanged.
- Added a private dependency-free production v2 guard after strict structural
  parsing. Every Q1/Q2/Q3 and returned F1 must contain at least one Unicode Han
  rune; zero-Han output uses the existing `invalid_output` contract without
  repair, translation, fallback, or retry.
- Kept `ParseQuestionSet`, `ParseAssessmentTurn`, v1 production behavior, and
  complete assessment feedback language-neutral. The v1.0.0 and v2.0.0 prompt
  files were not edited; tests pin their prior exact SHA-256 values.
- Wired daemon production v2 prompt bodies and daemon-owned history provenance
  to the matching v2.1.0 assets. Direct-Git and Session-bound enriched flows
  share this existing private v2 marker; no client language field was added.
- Added interface-level fake-worker tests for accepted Chinese questions/F1,
  zero-Han rejection in each individual Q position and F1, exactly one worker
  start on rejection, unchanged v1 English acceptance, and unrestricted English
  complete feedback.
- Added synthetic adversarial cases proving English-only instructions in
  evidence or answers cannot override the Chinese Q/F1 prompt rule.
- Updated stable evaluator, prompt, project, and user documentation. Static UI,
  answers, final feedback, protocols, storage, dependencies, credentials,
  settings, installed Pi, and the extension source remain unchanged.
- Extended the governance validator self-test fixture to remove the two new
  prompt-injection cases when simulating a missing category. This two-line test
  association was required because the previous negative fixture removed only
  the two older cases and therefore stopped exercising its intended failure;
  validator production behavior was not changed.

## Modified Files

Prompt assets and evaluator development fixtures:

- `agent/prompts/evaluator-question-generation/v2.1.0.md`
- `agent/prompts/evaluator-answer-assessment/v2.1.0.md`
- `agent/prompts/assets.go`
- `agent/prompts/assets_test.go`
- `agent/prompts/README.md`
- `agent/evals/cases/question-generation-simplified-chinese.json`
- `agent/evals/cases/assessment-follow-up-simplified-chinese.json`
- `agent/evals/README.md`
- `agent/README.md`

Production implementation and tests:

- `internal/evaluator/pi_rpc.go`
- `internal/evaluator/pi_rpc_test.go`
- `internal/evaluator/pi_rpc_version_test.go`
- `internal/daemon/daemon.go`
- `internal/daemon/go_context_preview.go`
- `internal/daemon/go_context_preview_test.go`

Governance, stable documentation, and lifecycle:

- `scripts/test-agent-infra.sh`
- `README.md`
- `PROJECT.md`
- `plans/simplified-chinese-learning-questions.md`
- `docs/decisions/ADR-0011-simplified-chinese-learning-questions.md`
- `docs/checkpoints/simplified-chinese-learning-questions-phase-1.md`

No dependency manifest, runtime schema, HTTP contract, database schema,
extension source, or released v1.0.0/v2.0.0 prompt changed.

## Important Decisions

- Language enforcement stays inside the existing evaluator deep module and is
  selected only by the daemon-owned v2 input version. Callers learn no locale
  option and both enriched selection paths receive identical behavior.
- Structural parsing remains the first authority. The Han check runs only on a
  validated question/F1, so it does not weaken IDs, kinds, references, byte
  limits, strict JSON, duplicate-key rejection, or follow-up lifecycle rules.
- Unicode Han presence is deliberately a minimum. It reliably rejects
  all-English output but cannot distinguish Simplified Chinese from Traditional
  Chinese, Japanese, or Korean; the versioned prompt and live Phase 2 review
  supply the semantic assurance.
- A language violation consumes the existing single model call and fails
  closed. It never triggers a repair call, translation, retry, fallback, or
  client-side rewriting.
- Prompt body and source-free provenance use the same explicit v2.1.0 asset
  accessors. Existing rows remain truthful and require no migration.

## Tests / Verification

Passed on 2026-09-06:

- Test-driven red/green slices for all-English v2 questions, all-English v2 F1,
  accepted Chinese Q1/Q2/Q3/F1, every individual zero-Han Q position, exactly
  one worker start, v1 compatibility, and English complete feedback.
- `go test ./agent/prompts ./internal/evaluator ./internal/daemon -count=1`:
  prompt and evaluator packages passed; the first daemon run failed only because
  the workspace sandbox denied `listen(127.0.0.1)` with `EPERM`. The unchanged
  daemon suite passed with local loopback permission in 135.640 seconds.
- `go test ./... -count=1`: all packages passed, including daemon 135.345
  seconds, evaluator 48.697 seconds, evidence 166.548 seconds, and history 3.652
  seconds.
- `go test -race ./... -count=1`: all packages passed, including daemon 171.307
  seconds, evaluator 121.806 seconds, evidence 168.182 seconds, and history
  5.002 seconds.
- `go vet ./...`: passed.
- `env -u GOROOT GOTOOLCHAIN=local CGO_ENABLED=0 go test -count=1 ./...`: all
  packages passed, including daemon 136.033 seconds, evaluator 46.877 seconds,
  evidence 165.645 seconds, and history 3.501 seconds.
- `npm run typecheck`: passed.
- `npm test`: the sandbox run passed 79 tests and failed 19 listener tests only
  because `listen(127.0.0.1)` returned `EPERM`; the permitted local rerun passed
  all 98 tests.
- `scripts/test-agent-infra.sh` and `scripts/validate-agent-infra.sh`: passed.
  The first infrastructure self-test exposed the stale missing-category fixture;
  after the minimal two-line association update, the complete self-test and
  production validator passed.
- `scripts/test-release-artifacts.sh` with checksum-verified Go 1.27.1: passed
  deterministic ARM64/AMD64 builds, independent verification, negative cases,
  native ARM64 diagnostic, foreground daemon, protected discovery/auth/status,
  fake Pi, and SIGTERM cleanup. No artifact was published or retained.
- `git diff --check` and review of tracked plus untracked files passed before
  checkpoint finalization; final status/stat/complete-diff review follows this
  lifecycle update.

No verification contacted a real provider, made a paid model call, read a real
Pi Session, changed Pi settings, wrote production history, or persisted raw
questions, answers, feedback, source, reasoning, or worker output.

## Known Issues

- Han presence cannot prove linguistic Simplified Chinese. This accepted limit
  remains explicit and requires human-visible Phase 2 acceptance.
- A daemon already running before these working-tree changes does not contain
  v2.1.0 and must be deliberately restarted from the matching checkout in a
  separately authorized live phase.
- The configured CodeGraph index directory exists, but CodeGraph tools were not
  exposed in this run. Focused source inspection used bounded direct reads.

## Remaining Work

Phase 2 remains unauthorized. It requires a named real model, exact reviewed Git
evidence, daemon restart permission, bounded question/assessment call counts,
and an explicit decision on retaining a source-free diagnostic history record.
It must verify natural Simplified-Chinese Q1/Q2/Q3 and, if naturally returned,
F1 without forcing an extra call solely to elicit a follow-up.

## Next Step

Review and commit/push Phase 1 only if explicitly requested. Then explicitly
authorize `simplified-chinese-learning-questions` Phase 2 with its model,
evidence, restart, call-budget, and diagnostic-history scope.

## Do Not Change

- Do not run Phase 2 or make a real provider call without its exact separate
  authorization.
- Do not broaden localization beyond Q1/Q2/Q3 and F1, require Chinese answers or
  feedback, or change the extension UI.
- Do not edit released v1.0.0/v2.0.0 prompts, generic parsers, protocols,
  schemas, storage, dependencies, credentials, settings, installed Pi, model
  selection, evidence limits, retry/fallback policy, or persisted allowlists.
- Do not commit or push without separate explicit authorization.
