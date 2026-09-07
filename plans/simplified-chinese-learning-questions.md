---
id: simplified-chinese-learning-questions
status: complete
risk: high
current_phase: 2
phase_status: complete
updated: 2026-09-07
---

# Require Simplified-Chinese Learning Questions

## 1. Goal

Make every model-generated question shown by the current `/learn` flow use
Simplified Chinese: the initial Q1, Q2, and Q3, plus the optional F1 follow-up.
Keep evidence references, fixed IDs, JSON fields, model selection, assessment
verdicts, and all non-question user-interface text unchanged.

## 2. Background

The production `evaluator-question-generation` and
`evaluator-answer-assessment` v2.0.0 prompts constrain grounding, structure,
size, and safety but do not select a natural language. The Go output validators
accept any nonblank UTF-8 question text within the existing byte and control-
character limits. A provider may therefore return English Q1/Q2/Q3 or F1 while
fully satisfying the current contract.

The user confirmed on 2026-09-06 that only Q1/Q2/Q3 and F1 must use Simplified
Chinese. Preview, confirmation, error, feedback, result, and history UI text are
not part of this localization request.

Released prompt assets are immutable under `agent/prompts/README.md`. The
existing v2.0.0 files cannot be edited in place. A behavior instruction with
unchanged input/output schemas is a minor prompt version.

## 3. Current Behavior

- The updated Pi extension always uses the enriched v2 preview path for direct
  Git and Session-bound `/learn` selections.
- `internal/daemon/daemon.go` constructs one versioned question evaluator and
  one versioned assessment evaluator with the v1.0.0 and v2.0.0 prompt bodies.
- `internal/daemon/go_context_preview.go` independently selects the matching
  prompt metadata for source-free history provenance.
- `PiRPCEvaluator.Evaluate` selects the prompt from the validated evaluator
  input schema, runs one isolated model turn, then calls `ParseQuestionSet`.
- `PiRPCAssessmentEvaluator.EvaluateAssessment` similarly calls
  `ParseAssessmentTurn`; an initial assessment may return one F1 question.
- The strict runtime schemas contain free-form `text` fields and intentionally
  have no language or locale field.
- Current question and F1 validators enforce UTF-8, nonblank content, byte
  limits, control-character rules, IDs, ordering, and evidence references, but
  not natural language.
- History already records prompt identifier, version, and SHA-256. Its bounded
  generic prompt-provenance fields can store a new version without migration.

## 4. Relevant Call Chain

```text
/learn enriched preview
  -> daemon continuation contract v2
  -> evaluatorInputForContinuation
       -> v2 question/assessment prompt provenance
  -> PiRPCEvaluator.Evaluate
       -> v2 prompt body
       -> isolated Pi ModelRuntime worker
       -> ParseQuestionSet
       -> private v2 question-language guard
  -> Q1/Q2/Q3 rendering
  -> PiRPCAssessmentEvaluator.EvaluateAssessment
       -> v2 assessment prompt body
       -> isolated Pi ModelRuntime worker
       -> ParseAssessmentTurn
       -> private v2 F1-language guard when disposition=follow_up
  -> optional F1 rendering or complete feedback/result
  -> existing source-free history with new prompt versions/hashes
```

The evaluator remains the deep module. Callers continue to supply the same
validated inputs and receive the same question/assessment interfaces; prompt
selection and language enforcement stay inside its implementation and the
existing daemon composition seam.

## 5. Relevant Files

- `agent/prompts/README.md`
- `agent/prompts/assets.go`
- `agent/prompts/assets_test.go`
- `agent/prompts/evaluator-question-generation/v2.0.0.md`
- `agent/prompts/evaluator-answer-assessment/v2.0.0.md`
- `agent/evals/README.md`
- `agent/evals/cases/`
- `agent/README.md`
- `internal/evaluator/contract.go`
- `internal/evaluator/assessment_contract.go`
- `internal/evaluator/evaluator.go`
- `internal/evaluator/assessment_evaluator.go`
- `internal/evaluator/pi_rpc.go`
- `internal/evaluator/pi_rpc_test.go`
- `internal/evaluator/pi_rpc_assessment_test.go`
- `internal/evaluator/pi_rpc_version_test.go`
- `internal/daemon/daemon.go`
- `internal/daemon/go_context_preview.go`
- `internal/daemon/go_context_preview_test.go`
- `README.md`
- `PROJECT.md`
- `docs/decisions/ADR-0011-simplified-chinese-learning-questions.md`

## 6. Scope

- Add immutable released v2.1.0 question-generation and answer-assessment
  prompt assets for the existing v2 input schemas and v1 output schemas.
- Require Simplified-Chinese natural-language prose for Q1/Q2/Q3 in the new
  question prompt and for F1 in the new assessment prompt.
- Preserve English JSON keys, Q1/Q2/Q3/F1 IDs, kinds, verdicts, evidence
  references, Go syntax, and code identifiers.
- Add a private dependency-free v2 production guard that requires at least one
  Unicode Han code point in each Q1/Q2/Q3 and any returned F1.
- Fail an all-English v2 question or F1 as existing `evaluator_invalid_output`,
  without repair, translation, fallback, or retry.
- Wire only the enriched v2 production path to the new prompt bodies and exact
  v2.1.0 provenance; preserve old prompt access for immutable-asset tests.
- Add synthetic prompt-injection/language cases and focused regression tests.
- Update stable project/evaluator/user documentation and phase checkpoints.

## 7. Out of Scope

- Localizing preview, confirmation, editor disclosure, errors, feedback,
  verdicts, labels, history, command names, or installation documentation.
- Requiring the user to answer in Chinese.
- Adding a locale field, CLI flag, environment variable, HTTP field, schema
  version, route, database column, or stored language preference.
- Translating evidence, source excerpts, package/type identities, or code.
- Editing or deleting the released v1.0.0 or v2.0.0 prompt files.
- Changing the legacy v1 evidence path's language behavior.
- Adding a language-detection/conversion dependency, a second model call, output
  repair, retry, fallback model, or client-side translation.
- Changing Pi, Node, Go, existing dependencies, credentials, settings, Session
  behavior, evidence budgets, or model-call limits.
- Public release, signing, notarization, npm publication, SSE, or background
  work.

## 8. Proposed Changes

### 8.1 Add immutable v2.1.0 prompts

Create new prompt files rather than altering v2.0.0. The question prompt must
say that Q1/Q2/Q3 natural-language prose is Simplified Chinese and that English
is retained only where needed for code identifiers and established technical
terms. Evidence instructions cannot override the language rule.

The assessment prompt must apply the same rule only to F1 question text. Final
feedback remains governed by its existing concise evidence-backed contract and
gains no deterministic language requirement.

Both assets retain their current input/output schema identifiers and capability
policy. Version `2.1.0` reflects a backward-compatible behavior instruction.
Embed them behind explicit new accessors and metadata while retaining the old
v2.0.0 values and tests unchanged.

### 8.2 Keep language enforcement inside the evaluator deep module

After the existing strict structural parser succeeds, the production Pi adapter
must apply the v2 language guard before returning a result. Every initial
question and a returned F1 must contain at least one rune in Unicode's Han
script. This is a narrow deterministic floor: it rejects all-English output but
does not replace the prompt or attempt general language classification.

Do not add language fields to `QuestionSet`, `AssessmentTurn`, HTTP values, or
the evaluator interfaces. The generic parsers remain language-neutral so old
fixtures, stored values, and the legacy v1 contract are not retroactively
reinterpreted. Complete assessment feedback is not checked by the language
guard.

Update deterministic evaluator question/F1 fixtures to Simplified Chinese where
they represent the current user-visible behavior, without changing their IDs,
kinds, references, verdicts, or labels.

### 8.3 Keep prompt body and provenance synchronized

Production daemon construction must receive the new v2.1.0 prompt bodies, and
v2 continuation/history provenance must use the exact matching metadata.
Focused tests must compare version and SHA-256 so body/provenance drift cannot
silently occur. Existing history rows and schema v2 remain byte-for-byte
unchanged; only new completed attempts record v2.1.0 prompt provenance.

### 8.4 Test adversarial and compatibility cases

Add synthetic cases in which evidence or an answer requests English output,
while the expected Q/F1 remains Chinese. Test Chinese questions containing Go
identifiers, all-English rejection, F1 rejection, no language check for complete
feedback, unchanged v1 behavior, immutable old prompt hashes, no retry, and
correct new history provenance.

The actual-SDK tests continue to intercept transport and use no provider. A
separately authorized live phase validates the human-visible language with the
chosen real model and reviewed evidence.

## 9. Compatibility

No runtime JSON schema, HTTP protocol, route, command, storage schema, or public
label changes. Existing clients continue to parse the same fields. Existing
history records retain their old prompt versions and hashes.

The current extension's enriched v2 path changes observable question language
and will fail closed if the provider returns no Han text. This is the requested
behavior change. The legacy v1 production path and immutable v1.0.0/v2.0.0
assets remain available and unchanged.

The new v2.1.0 prompt versions are durable provenance. Downgrading the daemon
later may produce v2.0.0 records again, but no migration or reinterpretation of
either record is allowed.

## 10. Risks

- A prompt alone cannot guarantee provider compliance; the host guard prevents
  wholly non-Chinese questions from reaching the user.
- Unicode Han includes characters shared by Simplified Chinese, Traditional
  Chinese, Japanese, and Korean. With no language dependency, the deterministic
  guard cannot prove linguistic Simplified Chinese. Prompt wording, adversarial
  fixtures, and live review provide the semantic assurance.
- A ratio or ASCII ban would reject legitimate Go identifiers and technical
  terms, so the guard deliberately remains a minimum rather than a translator.
- Updating prompt bodies without matching metadata would corrupt history
  provenance; tests must bind both exact values.
- Applying the guard to generic parsers would silently break old fixtures and
  v1 compatibility; enforcement must stay on the current production v2 path.
- A provider language failure consumes the single-use request and may incur
  cost; the existing no-retry rule remains necessary.
- Prompt changes can alter question quality even with unchanged schemas; focused
  cases and a controlled real flow remain required.

## 11. Implementation Phases

### Phase 1: Version, enforce, and wire Simplified-Chinese questions

Requires acceptance of ADR-0011 and explicit Phase 1 authorization.

Allowed: new v2.1.0 prompt assets; prompt asset accessors/metadata/tests;
synthetic evaluator cases; private evaluator language validation and focused
tests; deterministic question/F1 fixture wording; v2 daemon prompt composition
and provenance tests; `agent/README.md`, `agent/prompts/README.md`, `README.md`,
`PROJECT.md`, this plan/ADR, and one phase checkpoint.

Forbidden: released-prompt edits, extension/UI localization, dependency or
schema changes, client language fields, database migrations, model calls,
translation, repair, retry, fallback, unrelated cleanup, commit, or push without
separate authorization.

Implement tests at the evaluator interface and daemon composition seam. Run the
complete automated verification in section 13, checkpoint, advance to Phase 2,
and stop.

Completed on 2026-09-06. The enriched production path now uses immutable
v2.1.0 question and assessment prompts with matching source-free provenance.
The private production v2 adapters require Han text in every Q1/Q2/Q3 and any
F1 after strict parsing, while v1 and complete feedback remain language-neutral.
Focused, compatibility, adversarial, governance, full repository, race,
toolchain-baseline, TypeScript, actual-SDK no-network, and exact Go 1.27.1
release-artifact checks passed. No real provider call was made.

### Phase 2: Controlled actual-Pi language acceptance

Requires separate Phase 2 authorization naming the real model, exact reviewed
Git evidence, daemon restart permission, call limits, and whether a source-free
diagnostic history record may remain.

Authorized on 2026-09-07 with the user delegating the exact acceptance choices.
This phase uses `deepseek/deepseek-v4-pro`, reviews the current working tree
against `HEAD` at `da0f396228ed640f984b6e9c54aba71aab78a1ea`, may restart the
matching repository foreground daemon, permits at most one question-generation
call and two assessment calls, and may retain one source-free diagnostic
history record.

The environment initially required explicit informed approval rather than
delegated choice for the concrete external data transfer. The user supplied
that approval on 2026-09-07, including DeepSeek as the recipient and possible
provider charges. The earlier rejected launch made no model call; its matching
daemon was stopped and cleaned up before Phase 2 resumed.

Run `/learn` with the current extension and matching foreground daemon. Verify
Q1/Q2/Q3 are human-readable Simplified Chinese while identifiers/references
remain accurate. Complete the answer workflow; if F1 is naturally returned,
verify it is also Simplified Chinese. Do not force extra calls solely to elicit
F1: the intercepted actual-SDK and deterministic tests remain the required F1
gate when the live initial assessment completes immediately.

One complete flow may use one question-generation call and at most two
assessment calls. Preserve settings, zero retry, source-free history, and
cleanup checks. Acceptance-only edits are limited to the plan/checkpoint and
stable status documentation. A newly discovered defect requires a new approved
phase rather than an unplanned fix.

Completed on 2026-09-07. The matching working-tree daemon and an ephemeral Pi
0.84.3 interaction used `deepseek/deepseek-v4-pro` with high thinking and the
current working tree against `da0f396228ed640f984b6e9c54aba71aab78a1ea`.
Q1/Q2/Q3 were human-readable Simplified Chinese with intact identifiers and
valid evidence references. One initial assessment completed without F1, so the
call audit was one question generation and one assessment with no retry or
fallback. Source-free history record
`lr1-hM9gY5j4ZNb7gQeWoVq1Cr7slpb3eV2uVQL4okuUs3o` reports prompt versions
2.1.0 and their expected hashes. Pi settings were byte- and metadata-identical
before and after; Pi and the matching daemon exited, descriptor/token cleanup
completed, and only the designed lock file remains. No raw questions, answers,
feedback, reasoning, source, transcript, or model output was copied into
repository artifacts.

## 12. Acceptance Criteria

1. Current direct-Git and Session-bound `/learn` Q1/Q2/Q3 prose is Simplified
   Chinese under the new v2.1.0 question prompt.
2. Any v2 F1 prose is Simplified Chinese under the new v2.1.0 assessment prompt.
3. Each production v2 Q/F1 contains at least one Unicode Han rune; zero-Han
   output fails as `evaluator_invalid_output` without retry or repair.
4. JSON keys, IDs, kinds, references, schemas, routes, labels, byte limits, and
   model-call counts remain unchanged.
5. Preview, confirmations, errors, feedback, results, history UI, and answer
   language remain outside the localization scope.
6. Existing v1 behavior and released v1.0.0/v2.0.0 prompt bytes/hashes remain
   unchanged.
7. New source-free records store exact v2.1.0 prompt versions and hashes; old
   rows and schema v2 require no migration.
8. Evidence/answer instructions requesting English cannot override the prompt,
   and all-English synthetic model output fails closed.
9. No dependency, settings, credential, installed-Pi, protocol, or persistence
   change occurs.
10. Automated suites pass, followed by a separately authorized successful live
    Q1/Q2/Q3 flow with no raw source, answers, or model output persisted in
    repository artifacts.

## 13. Verification

Design-only:

- `scripts/test-agent-infra.sh`
- `scripts/validate-agent-infra.sh`
- `git diff --check`
- final status/stat/complete-diff and new-file review

Phase 1 implementation:

- focused prompt asset, evaluator question/F1, Pi adapter, and daemon provenance
  tests
- `go test ./agent/prompts ./internal/evaluator ./internal/daemon`
- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- `env -u GOROOT GOTOOLCHAIN=local CGO_ENABLED=0 go test -count=1 ./...`
- `npm run typecheck`
- `npm test`
- `scripts/test-agent-infra.sh`
- `scripts/validate-agent-infra.sh`
- `scripts/test-release-artifacts.sh` with the established exact release
  toolchain, because prompt assets are embedded in the daemon binary
- `git diff --check` plus complete scope/diff review

No automated verification may contact a real provider. Phase 2 records the
separately authorized model/evidence/call scope, visible question-language
result, prompt provenance, settings preservation, history result, and daemon
cleanup without copying raw questions, answers, feedback, reasoning, or source.

## 14. Open Questions / Gates

- Resolved: only Q1/Q2/Q3 and F1 require Simplified Chinese. All other UI and
  feedback remain outside scope.
- Resolved: the user accepted ADR-0011, including the dependency-free Unicode
  Han limitation, and authorized Phase 1 on 2026-09-06. Phase 1 is complete.
- Resolved: on 2026-09-07 the user delegated the Phase 2 acceptance choices.
  The bounded model, evidence, restart, call, and diagnostic-history scope is
  recorded in section 11.
- Resolved: on 2026-09-07 the user explicitly approved transmitting the
  confirmed preview's current-working-tree Go evidence to DeepSeek with
  possible provider charges. Phase 2 completed within the recorded bounds.
- Resolved: the live initial assessment completed without F1. No extra call was
  made solely to elicit one; deterministic and actual-SDK no-network tests
  remain the required F1 validation gate.
