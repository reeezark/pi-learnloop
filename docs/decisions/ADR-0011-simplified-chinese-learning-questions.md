---
id: ADR-0011
status: accepted
date: 2026-09-06
supersedes: none
---

# ADR-0011: Simplified-Chinese Questions on the Enriched Learning Path

## Context

The current `/learn` extension always uses the enriched v2 evaluator path. Its
released v2.0.0 question and assessment prompts define evidence, safety,
structure, and size rules but do not define a natural language. The runtime
contracts accept any bounded nonblank UTF-8 question text, so English Q1/Q2/Q3
and F1 outputs are valid today.

The user requires only the initial Q1/Q2/Q3 and optional F1 question prose to be
Simplified Chinese. Static UI, confirmations, errors, assessment feedback,
verdicts, labels, history, user answers, schemas, and identifiers are not being
localized.

Released prompt files are immutable. Prompt versioning defines a
backward-compatible behavior instruction as a minor version. History already
stores prompt identifier, version, and hash without constraining the version to
a fixed release.

Natural-language identity cannot be proven perfectly with the Go standard
library. Unicode's Han script is shared across Simplified Chinese, Traditional
Chinese, Japanese, and Korean, while valid questions may also contain English
Go identifiers and technical terms. A strict ASCII ban or character ratio would
reject legitimate technical questions without guaranteeing Simplified Chinese.

## Decision

Add immutable released v2.1.0 assets for
`evaluator-question-generation` and `evaluator-answer-assessment`, retaining the
existing v2 input schemas, v1 output schemas, capability policy, and all old
prompt bytes.

The v2.1.0 question prompt requires Q1/Q2/Q3 natural-language prose in
Simplified Chinese. The v2.1.0 assessment prompt applies the same requirement to
F1 only. JSON keys, Q/F IDs, kinds, verdicts, evidence references, Go syntax,
and code identifiers remain unchanged. Evidence or answer text asking for
English is untrusted and cannot override this instruction. Final assessment
feedback has no new language requirement.

Only the current enriched v2 production path selects v2.1.0. The legacy v1 path
continues selecting v1.0.0, and v2.0.0 remains an immutable released historical
asset. New history records from the enriched path store the exact v2.1.0
versions and SHA-256 hashes; existing rows and the database schema are unchanged.

Keep enforcement inside the evaluator deep module. After existing structural
validation, production v2 question output must contain at least one Unicode Han
rune in each Q1/Q2/Q3. A returned v2 F1 must meet the same floor. Zero-Han output
uses the existing safe invalid-output failure, consumes no repair or fallback
call, and is never retried. Generic parsers and public runtime values remain
language-neutral, so no caller learns a locale option and no legacy value is
retroactively invalidated.

The Han check is a deterministic minimum, not a claim of perfect language
detection. Simplified-Chinese semantics are enforced by the versioned system
prompts, adversarial synthetic cases, and controlled human-visible acceptance.
No language detector, Chinese-conversion dependency, client translator, or
second model call is introduced.

## Alternatives

- **Edit v2.0.0 in place:** rejected because released prompt bytes and recorded
  hashes are immutable provenance.
- **Rely only on a prompt sentence:** rejected because an all-English provider
  response would still cross the product seam as valid.
- **Add a client-supplied locale or configuration default:** rejected because
  the user selected one fixed product behavior; a new interface, compatibility
  matrix, and persistence/configuration surface add no current leverage.
- **Translate in the extension after validation:** rejected because it moves
  model-output policy across the evaluator seam, can damage identifiers and
  evidence meaning, and makes displayed text differ from validated output.
- **Use a second model call to translate or repair:** rejected because it adds
  source transmission, latency, cost, nondeterminism, and retry-like behavior.
- **Add a Simplified/Traditional conversion or language-classifier dependency:**
  rejected for this scope because it changes dependencies, still requires
  identifier-aware rewriting, and does not improve the narrow evaluator
  interface.
- **Reject all ASCII or require a Han-character ratio:** rejected because Go,
  HTTP, JSON, package/type names, and code identifiers legitimately use Latin
  characters; these heuristics create false rejection without proving language.
- **Localize all `/learn` UI and assessment feedback:** rejected because the
  user explicitly limited scope to Q1/Q2/Q3 and F1.
- **Change legacy v1 output too:** rejected because the current extension uses
  v2 and the requested behavior does not justify altering older client behavior.

## Consequences

Current `/learn` questions become Simplified Chinese without changing commands,
schemas, HTTP, storage, model selection, evidence, answers, or non-question UI.
The evaluator retains a small interface while hiding prompt selection and the
language floor in one implementation, preserving locality across direct-Git and
Session-bound callers.

Prompt v2.1.0 identities and hashes become durable history provenance. Old
records remain readable and truthful. Downgrades can create v2.0.0 records again
without migration; history reports the actual prompt used.

All-English provider output now fails closed after a possibly billable call and
cannot reuse the single-use continuation. This is preferable to displaying a
question that violates the requirement, and it does not authorize retry,
translation, fallback, or output repair.

The implementation can prove Han presence deterministically but must describe
its Simplified-Chinese semantic guarantee honestly. Actual quality and script
choice remain model behavior constrained by the released prompt and verified in
a separately authorized live acceptance phase.

The implementation plan is
`plans/simplified-chinese-learning-questions.md`. This decision does not amend
ADR-0002 transport, ADR-0003 evidence/isolation/no-retry rules, ADR-0004 answer
lifecycle, ADR-0005/ADR-0006 history privacy, or ADR-0010's private model
runtime. Each high-risk phase remains separately authorized.
