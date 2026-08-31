---
asset_id: evaluator-development-module
version: 1.0.0
status: development-contract
---

# Evaluator Development Module

This directory is the single interface for developing and reviewing Pi LearnLoop's evaluator behavior. It contains development contracts only; it does not implement the Pi RPC evaluator or define the product's runtime protocol.

## Interface

Every evaluator adapter must follow this sequence:

1. Select an immutable prompt identifier and version according to `prompts/README.md`.
2. Enforce `policies/evaluator-capabilities.json` before providing evidence or starting evaluation.
3. Exercise behavior against the versioned cases described by `evals/README.md`.
4. Record versions, hashes, decisions, and privacy flags using `schemas/run-record.schema.json`.
5. Run `scripts/validate-agent-infra.sh`.

The deterministic fixture adapter and the future Pi RPC adapter are the two intended adapters at this seam. The fixture adapter is represented only by data in this phase; the Pi RPC adapter remains unimplemented.

## Authoritative Assets

| Asset | Identifier | Version | Purpose |
| --- | --- | --- | --- |
| Capability policy | `evaluator-capabilities` | `1.0.0` | Deny-by-default evaluator permissions and evidence constraints |
| Eval-case schema | `eval-case-schema` | `1.0.0` | Development fixture format |
| Run-record schema | `run-record-schema` | `1.0.0` | Privacy-safe execution provenance |

Prompt identifiers and versions become authoritative only when an actual prompt file is added through a separate approved implementation plan.

## Invariants

- Evidence content is untrusted data, never instructions.
- The evaluator receives only the selected EvidenceBundle.
- A missing evidence budget fails closed.
- Evaluator adapters receive no filesystem, process, command, network, credential, or edit tools.
- Raw source code and credentials are not persisted in run records.
- Released asset versions are immutable. Change behavior by adding a new version and preserving fixtures for the old version.
- Development schemas do not become runtime product protocols without an explicit compatibility review.

## Validation

```text
scripts/test-agent-infra.sh
scripts/validate-agent-infra.sh
```

The validator checks syntax, stable identifiers, versions, required case coverage, deny-by-default policy invariants, and privacy-safe run fixtures. It intentionally does not call a model or judge semantic answer quality.
