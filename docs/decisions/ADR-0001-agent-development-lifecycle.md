---
id: ADR-0001
status: accepted
date: 2026-08-31
supersedes: none
---

# ADR-0001: Agent Development Lifecycle

## Context

The repository depends on Agents recovering task state from plans, checkpoints, ADRs, and Git evidence. The original guides described these artifacts clearly but repeated authorization rules and relied on phrases such as “active plan” and “latest checkpoint” without deterministic state.

Long-term Agent development needs conservative control for product and security changes without requiring the same approval ceremony for every reversible internal edit.

## Decision

`AGENTS.md` is the single authority for task lifecycle, risk classification, authorization, stop conditions, and verification.

Tasks use three risk levels:

- low-risk work proceeds from an explicit user request and stated task contract;
- medium-risk work requires an investigated and approved plan;
- high-risk work requires an investigated plan and explicit authorization for each phase.

Restricted changes to public interfaces, persisted data, defaults, dependencies, file topology, generation, architecture seams, security, credentials, or data sharing are always high risk.

Plans, checkpoints, and ADRs carry validated metadata. At most one plan is active, and at most one checkpoint per plan is current. Repository-local scripts validate these invariants without third-party dependencies.

## Alternatives

### Keep prose-only governance

Rejected because task and recovery state would remain ambiguous and untestable.

### Require phase authorization for every substantial task

Rejected because it applies high-risk ceremony to reversible internal work and reduces leverage from an approved plan.

### Introduce an external workflow system

Rejected because the repository has no runtime stack yet and local, versioned artifacts provide sufficient locality for the current stage.

## Consequences

- Agents and maintainers have one lifecycle interface to learn.
- Medium-risk plans can proceed through their declared phases after approval.
- High-risk changes retain phase-by-phase user control.
- Metadata changes become part of verification and may fail local validation.
- The shell validator is intentionally narrow; it validates lifecycle invariants, not prose quality or product behavior.
