---
asset_id: prompt-versioning-guide
version: 1.0.0
status: development-contract
---

# Prompt Versioning

Production evaluator prompts are immutable versioned assets. This guide defines their layout and lifecycle.

## Layout

```text
agent/prompts/<prompt-id>/v<major>.<minor>.<patch>.md
```

A prompt file must begin with:

```yaml
---
id: evaluator-question-generation
version: 1.0.0
status: draft
input_schema: evaluator-input@1
output_schema: evaluator-question-set@1
capability_policy: evaluator-capabilities@1.0.0
updated: YYYY-MM-DD
---
```

Allowed prompt states are `draft | released | deprecated`.

## Current Released Prompt

| Prompt | Version | Input | Output |
| --- | --- | --- | --- |
| `evaluator-question-generation` | `1.0.0` | `evaluator-input@1` | `evaluator-question-set@1` |
| `evaluator-answer-assessment` | `1.0.0` | `evaluator-assessment-input@1` | `evaluator-assessment-turn@1` |

## Version Rules

- Patch: editorial clarification with no intended behavior or schema change.
- Minor: backward-compatible behavior, rubric, or instruction change.
- Major: input, output, safety invariant, or evaluation-contract change.
- Released versions are immutable. Add a new file instead of editing one.
- Every behavior-changing version must add or update eval cases before release.
- Every run record stores prompt identifier, version, and SHA-256 hash.
- Draft prompts are review assets only. Production embedding or invocation requires an explicitly authorized release phase.

## Content Rules

- Delimit evidence and user answers as untrusted data.
- Never place credentials, repository source, user answers, or Session transcripts in prompt files.
- Require evidence references for code-specific claims.
- Require an explicit insufficient-evidence path.
- Do not select a model or score threshold in this guide.

The capability policy is independent of prompt wording. A prompt cannot grant a denied capability.
