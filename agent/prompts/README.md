---
asset_id: prompt-versioning-guide
version: 1.0.0
status: development-contract
---

# Prompt Versioning

No production evaluator prompt exists yet. This guide defines how prompt artifacts must be introduced when an approved evaluator implementation plan provides their content and runtime schemas.

## Layout

```text
agent/prompts/<prompt-id>/v<major>.<minor>.<patch>.md
```

A prompt file must begin with:

```yaml
---
id: evaluator-question-and-assessment
version: 1.0.0
status: draft
input_schema: TODO
output_schema: TODO
capability_policy: evaluator-capabilities@1.0.0
updated: YYYY-MM-DD
---
```

Allowed prompt states are `draft | released | deprecated`.

## Version Rules

- Patch: editorial clarification with no intended behavior or schema change.
- Minor: backward-compatible behavior, rubric, or instruction change.
- Major: input, output, safety invariant, or evaluation-contract change.
- Released versions are immutable. Add a new file instead of editing one.
- Every behavior-changing version must add or update eval cases before release.
- Every run record stores prompt identifier, version, and SHA-256 hash.

## Content Rules

- Delimit evidence and user answers as untrusted data.
- Never place credentials, repository source, user answers, or Session transcripts in prompt files.
- Require evidence references for code-specific claims.
- Require an explicit insufficient-evidence path.
- Do not select a model, score threshold, or runtime schema in this guide.

The capability policy is independent of prompt wording. A prompt cannot grant a denied capability.
