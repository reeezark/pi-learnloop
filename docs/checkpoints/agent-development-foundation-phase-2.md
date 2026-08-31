---
id: agent-development-foundation-phase-2
plan: agent-development-foundation
phase: 2
status: current
updated: 2026-08-31
---

# Context

## Goal

Complete the pre-development Agent foundation by adding versioned evaluator-development assets, deny-by-default capability constraints, synthetic eval coverage, and privacy-safe run provenance.

## Current Phase

Phase 2 is complete. The `agent-development-foundation` plan is complete.

## Completed

- Added one evaluator-development module interface under `agent/`.
- Defined immutable prompt-versioning rules without creating a production prompt.
- Added the `evaluator-capabilities@1.0.0` deny-by-default contract.
- Added versioned development schemas for eval cases and run provenance.
- Added synthetic cases for evidence fidelity, insufficient evidence, prompt injection, and malformed structured output.
- Added a run-record fixture that records adapter, prompt, policy, model, evidence, execution, result, and privacy provenance.
- Extended repository validation to enforce asset identity, versions, policy invariants, category coverage, hashes, and privacy flags.
- Extended validator self-tests to 16 positive and negative scenarios.
- Updated `AGENTS.md` and `PROJECT.md` with the implemented Agent-development facts.

## Modified Files

- `AGENTS.md`
- `PROJECT.md`
- `plans/agent-development-foundation.md`
- `agent/README.md`
- `agent/prompts/README.md`
- `agent/policies/evaluator-capabilities.json`
- `agent/schemas/eval-case.schema.json`
- `agent/schemas/run-record.schema.json`
- `agent/evals/README.md`
- `agent/evals/cases/evidence-fidelity-unsupported-claim.json`
- `agent/evals/cases/insufficient-evidence-abstain.json`
- `agent/evals/cases/prompt-injection-in-evidence.json`
- `agent/evals/cases/structured-result-malformed.json`
- `agent/fixtures/run-record/example-fixture-run.json`
- `scripts/validate-agent-infra.sh`
- `scripts/test-agent-infra.sh`
- `docs/checkpoints/agent-development-foundation-phase-2.md`

## Important Decisions

- These schemas are development contracts, not runtime product protocols.
- Evidence fixtures are synthetic and must be treated as untrusted data.
- Evaluator adapters receive no filesystem, edit, command, process, network, credential, or remote-control tools.
- A missing evidence budget fails closed.
- Run records persist identifiers, versions, hashes, counts, and decisions rather than raw source code or credentials.
- The run fixture's capability-policy hash must match the repository asset.

## Tests / Verification

- Passed: `sh -n scripts/validate-agent-infra.sh`.
- Passed: `sh -n scripts/test-agent-infra.sh`.
- Passed: `scripts/test-agent-infra.sh` with 16 scenarios.
- Passed: `scripts/validate-agent-infra.sh`.
- JSON syntax, stable identifiers, required eval categories, policy invariants, and run privacy are covered by the repository validator.
- Passed: repository-wide Git whitespace validation, final status inspection, policy-hash comparison, and complete content review after checkpoint creation.

## Known Issues

- No production prompt, model choice, scoring threshold, runtime structured-output schema, or live evaluator exists.
- JSON Schema meta-validation is not installed; the repository validator checks JSON syntax and the stable invariants required by this phase.
- CI provider and hosted enforcement remain `TODO / Need Confirmation`.
- ShellCheck is not installed.
- The repository has no initial commit, so Git reports every file as untracked and cannot produce a normal baseline diff.

## Remaining Work

No work remains in this plan. Runtime evaluator implementation requires a new investigated plan and explicit authorization.

## Next Step

Create the initial Git baseline only if the user authorizes a commit, or investigate the first runtime implementation task in a new plan.

## Do Not Change

- Do not broaden evaluator capabilities without a high-risk plan and explicit phase authorization.
- Do not treat development schemas as released runtime protocols.
- Do not add a production prompt or live model call without the evaluator implementation plan.
- Do not persist raw source, credentials, or telemetry by default.
