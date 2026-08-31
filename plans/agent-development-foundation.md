---
id: agent-development-foundation
status: complete
risk: high
current_phase: 2
phase_status: complete
updated: 2026-08-31
---

# Agent Development Foundation

## 1. Goal

Establish the minimum development infrastructure required to maintain Pi LearnLoop as a long-lived Agent project before business implementation begins.

The result must make Agent work recoverable, machine-checkable, evaluable, security-conscious, and traceable without defining unapproved product behavior.

## 2. Background

The repository already has a strong documentation-first governance skeleton:

- `AGENTS.md` defines startup, scope, planning, authorization, verification, and recovery rules.
- `PROJECT.md` separates planned architecture from implemented facts.
- `plans/`, `docs/checkpoints/`, and `docs/decisions/` have distinct intended responsibilities.

The current structure still depends on natural-language interpretation. It has no machine-readable task state, no validator, no versioned evaluator-development assets, no executable evaluator capability policy, and no run-provenance contract.

## 3. Current Behavior

- The repository contains five documentation files and no business code, manifests, tests, scripts, CI, or dependencies.
- The `main` branch has no commits; every current file is untracked.
- Agents are instructed to read an “active plan” and “latest related checkpoint,” but no metadata or deterministic resolution rule identifies either.
- Phase-authorization rules are repeated in `AGENTS.md` and `plans/README.md`.
- Evaluator isolation, read-only behavior, evidence minimization, structured results, and repeatable fixtures are planned in `PROJECT.md`, but no corresponding development assets exist.
- Verification is limited to manual diff review and Git whitespace checking.

## 4. Relevant Call Chain

There is no executable call chain yet. The verified Agent-development flow is:

```text
User request
→ AGENTS.md startup gate
→ PROJECT.md facts and constraints
→ active plan
→ latest related checkpoint
→ applicable ADRs
→ authorized edits
→ verification and complete diff review
→ checkpoint or completion
```

Current friction occurs at the plan/checkpoint resolution seam and at the verification seam because neither has an executable implementation.

## 5. Relevant Files

- `AGENTS.md`: current authoritative Agent instructions and repeated workflow rules.
- `PROJECT.md`: current facts, target evaluator architecture, privacy constraints, and testing goals.
- `plans/README.md`: plan lifecycle and template.
- `docs/checkpoints/README.md`: recovery-state template.
- `docs/decisions/README.md`: ADR lifecycle and template.

No source, test, manifest, prompt, schema, policy, or CI file currently exists.

## 6. Scope

### Agent governance

- Establish one authoritative task lifecycle with explicit risk and status states.
- Make plans, checkpoints, and ADRs deterministically discoverable and linkable.
- Add dependency-free local validation for Agent-development artifacts.
- Reduce duplicated workflow rules while retaining conservative scope control.

### Evaluator development

- Add a cohesive `agent/` module for evaluator-development assets.
- Define prompt/versioning conventions without writing the production prompt.
- Define development-time eval-case and run-provenance schemas without freezing the runtime product protocol.
- Define an explicit evaluator capability policy that encodes existing read-only and evidence-minimization constraints.
- Add representative fixtures for evidence fidelity, insufficient evidence, prompt injection, and structured-result validation.

### Documentation

- Update repository facts and navigation after new infrastructure exists.
- Record the long-lived lifecycle decision in an ADR.

## 7. Out of Scope

- Go daemon, TypeScript extension, SQLite schema, HTTP/SSE protocol, Pi RPC implementation, or any business behavior.
- Production evaluator prompts, model selection, scoring thresholds, or runtime assessment schemas.
- Dependency manifests or third-party validation libraries.
- GitHub Actions or another CI provider until the hosting and CI choice is confirmed.
- Creating a Git commit or repository baseline.
- Changing product commands, assessment labels, evidence-sharing defaults, or compatibility guarantees.

## 8. Proposed Changes

### 8.1 Deepen the task-control module

Keep `AGENTS.md` as the Agent-facing interface. Concentrate lifecycle semantics there and make the other guides describe their own artifact formats rather than repeat authorization policy.

Add structured metadata to plans, checkpoints, and ADRs:

- stable identifier;
- status;
- risk;
- current or completed phase;
- related plan;
- last-updated date;
- supersession where applicable.

Add a local validator that checks lifecycle invariants and artifact links. This produces locality: workflow changes and verification concentrate in one module instead of leaking across several documents.

### 8.2 Add the evaluator-development module

Create a top-level `agent/` module with a small interface:

- an index describing authoritative assets;
- prompt versioning rules;
- evaluator capability policy;
- eval-case schema and fixtures;
- privacy-safe run-provenance schema.

The implementation may contain multiple files, but callers and tests should enter through the module index and validator. The production Pi evaluator and a deterministic fixture evaluator will eventually be two adapters at the evaluator seam; this task defines only their shared development contract.

### 8.3 Add executable verification

Create dependency-free shell validation plus self-tests using temporary fixtures. The validator will check:

- allowed lifecycle statuses and risk values;
- zero or one active plan;
- checkpoint-to-plan linkage and phase consistency;
- allowed ADR statuses and supersession links;
- JSON syntax for policy, schemas, and fixtures;
- required evaluator asset versions and identifiers;
- absence of raw source-code fields in run-provenance fixtures.

## 9. Compatibility

- No runtime callers or persisted product data exist.
- Existing documentation remains readable, but its workflow wording and templates will change.
- Existing future plans will need the new metadata when created; there are no current task plans besides this plan.
- The capability policy records constraints already present in `PROJECT.md`; it does not broaden evaluator access.
- Runtime product schemas remain explicitly uncommitted.

## 10. Risks

- Over-engineering before business code exists. Mitigation: keep the module dependency-free and limit schemas to development metadata.
- Duplicating authority between `AGENTS.md` and `agent/README.md`. Mitigation: `AGENTS.md` owns task control; `agent/README.md` owns evaluator-development assets.
- Treating policy documents as runtime enforcement. Mitigation: clearly label the policy as the required contract and require implementation tests once runtime adapters exist.
- A validator may become a shallow collection of text checks. Mitigation: test only stable lifecycle invariants and avoid parsing prose.
- Changing authorization semantics without a durable record. Mitigation: add an ADR and require explicit Phase 1 approval.

## 11. Implementation Phases

### Phase 1 — Task control and executable validation

Goal: make Agent task state deterministic and remove duplicated lifecycle authority.

Allowed files:

- `AGENTS.md`
- `PROJECT.md`
- `plans/README.md`
- `plans/agent-development-foundation.md`
- `docs/checkpoints/README.md`
- `docs/decisions/README.md`
- `docs/decisions/ADR-0001-agent-development-lifecycle.md`
- `scripts/validate-agent-infra.sh`
- `scripts/test-agent-infra.sh`

Forbidden changes:

- product architecture, product security defaults, runtime protocols, dependencies, and business code;
- deletion or renaming of existing files;
- Git commit creation.

Acceptance criteria:

- one authoritative lifecycle and risk-tier model exists;
- task, checkpoint, and ADR metadata are deterministic;
- validation and validator self-tests pass;
- existing project facts remain accurate;
- complete diff contains only allowed files.

Verification:

- `scripts/test-agent-infra.sh`
- `scripts/validate-agent-infra.sh`
- `git diff --check`
- `git status --short`
- complete `git diff` review

### Phase 2 — Evaluator development, safety, and provenance

Goal: create a versioned, testable evaluator-development module without implementing the evaluator.

Allowed files:

- `AGENTS.md`
- `PROJECT.md`
- `plans/agent-development-foundation.md`
- `agent/README.md`
- `agent/prompts/README.md`
- `agent/policies/evaluator-capabilities.json`
- `agent/schemas/eval-case.schema.json`
- `agent/schemas/run-record.schema.json`
- `agent/evals/README.md`
- `agent/evals/cases/*.json`
- `agent/fixtures/run-record/*.json`
- `scripts/validate-agent-infra.sh`
- `scripts/test-agent-infra.sh`
- `docs/checkpoints/agent-development-foundation-phase-2.md`

Forbidden changes:

- production prompts or live model calls;
- runtime Pi RPC, daemon, extension, storage, or transport schemas;
- additional evaluator capabilities beyond current `PROJECT.md` constraints;
- dependencies, CI configuration, or Git commits.

Acceptance criteria:

- every evaluator-development asset has a stable identifier and version;
- capability policy is explicit, deny-by-default, read-only, and evidence-bounded;
- eval fixtures cover fidelity, insufficient evidence, prompt injection, and malformed structured output;
- run provenance captures versions and hashes without storing raw source code by default;
- validation and self-tests pass;
- a recovery checkpoint records exact completion state.

Verification:

- `scripts/test-agent-infra.sh`
- `scripts/validate-agent-infra.sh`
- JSON syntax checks through the repository validator
- `git diff --check`
- `git status --short`
- complete `git diff` review

## 12. Acceptance Criteria

- Agent work can deterministically identify task status, risk, phase, and related checkpoint.
- Governance rules have one authority and are not copied across guides.
- Agent-development artifacts are locally validated without third-party dependencies.
- Evaluator prompts, policies, eval cases, and run records have explicit versioning conventions.
- Read-only and evidence-minimization requirements are represented as a deny-by-default contract.
- Eval fixtures exercise both normal and adversarial paths.
- Run provenance supports comparison and recovery without persisting raw source code by default.
- No product behavior, dependency, runtime protocol, or compatibility promise changes.

## 13. Verification

The repository will gain two authoritative commands:

```text
scripts/test-agent-infra.sh
scripts/validate-agent-infra.sh
```

Each phase must also run Git whitespace validation, inspect status, inspect diff statistics, and review the complete diff. No build, lint, Go test, TypeScript test, integration test, or race test will be claimed because those runtimes remain unconfigured.

## 14. Open Questions

- CI provider and hosted enforcement remain `TODO / Need Confirmation`; local validation is sufficient for this task.
- Production evaluator prompt content, model choice, rubric thresholds, and runtime structured-output schema remain `TODO / Need Confirmation` for the evaluator implementation plan.
- The initial Git baseline and commit strategy remain outside scope and require separate user authorization.

No open question blocks Phase 1. Phase 2 deliberately creates development contracts only and does not decide the deferred runtime questions.
