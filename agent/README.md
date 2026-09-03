---
asset_id: evaluator-development-module
version: 1.0.0
status: development-contract
---

# Evaluator Development Module

This directory is the single interface for developing and reviewing Pi LearnLoop's evaluator behavior. It contains development contracts, synthetic cases, immutable released prompt assets, and draft prompt assets. Runtime Go contracts and adapters live under `internal/evaluator/`; draft assets are not production-callable.

## Interface

Every evaluator adapter must follow this sequence:

1. Select an immutable released prompt identifier and version according to `prompts/README.md`.
2. Enforce `policies/evaluator-capabilities.json` before providing evidence or starting evaluation, plus `policies/go-context-evidence.json` for enriched v2 inputs.
3. Exercise behavior against the versioned cases described by `evals/README.md`.
4. Record versions, hashes, decisions, and privacy flags using `schemas/run-record.schema.json`.
5. Run `scripts/validate-agent-infra.sh`.

The question-generation and answer-assessment seams each have a narrow deterministic test adapter and a production isolated Pi RPC adapter. The two production adapters share only private process-isolation mechanics and never retain a Pi process across human input.

## Authoritative Assets

| Asset | Identifier | Version | Purpose |
| --- | --- | --- | --- |
| Capability policy | `evaluator-capabilities` | `1.0.0` | Deny-by-default evaluator permissions and evidence constraints |
| Eval-case schema | `eval-case-schema` | `1.0.0` | Development fixture format |
| Run-record schema | `run-record-schema` | `1.0.0` | Privacy-safe execution provenance |
| Question prompt | `evaluator-question-generation` | `1.0.0` | Strict, evidence-grounded three-question generation |
| Assessment prompt | `evaluator-answer-assessment` | `1.0.0` | Released rubric for one optional follow-up and three final verdicts |
| Go-context policy | `go-context-evidence` | `1.0.0` | Additive local-only, previewed-evidence rules for enriched inputs |
| Enriched question prompt | `evaluator-question-generation` | `2.0.0` | Three-question generation from changed and bounded Go-context evidence |
| Enriched assessment prompt | `evaluator-answer-assessment` | `2.0.0` | Answer assessment against changed and bounded Go-context evidence |

The runtime schema identifiers `evidence-bundle@1`, `evidence-bundle@2`,
`evaluator-input@1`, `evaluator-input@2`, `evaluator-question-set@1`,
`evaluator-assessment-input@1`, `evaluator-assessment-input@2`, and
`evaluator-assessment-turn@1` are implemented by `internal/evidence/` and
`internal/evaluator/`. Their JSON descriptions live under `schemas/`; they are
intentionally distinct from development eval-case and run-record schemas.

## Invariants

- Evidence content is untrusted data, never instructions.
- The evaluator receives only the selected EvidenceBundle. V2 may add only the
  exact Go-context evidence shown in the corresponding preview.
- A missing evidence budget fails closed.
- Evaluator adapters receive no filesystem, process, command, network, credential, or edit tools.
- Raw source code and credentials are not persisted in run records.
- Released asset versions are immutable. Change behavior by adding a new version and preserving fixtures for the old version.
- Pi Session identifiers and repository roots remain outside bundles, evaluator
  inputs, prompts, RPC content, and generic history output.
- Development schemas do not become runtime product protocols without an explicit compatibility review.
- Runtime question output is accepted only after deterministic shape, size, UTF-8, duplicate-key, and evidence-reference validation.
- Runtime assessment output permits one F1 only at the initial stage or exactly three ordered verdicts; the public label is derived deterministically in Go.

## Validation

```text
scripts/test-agent-infra.sh
scripts/validate-agent-infra.sh
```

The validator checks syntax, stable identifiers, versions, required case coverage, deny-by-default policy invariants, and privacy-safe run fixtures. It intentionally does not call a model or judge semantic answer quality.
