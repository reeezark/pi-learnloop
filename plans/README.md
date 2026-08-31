# Development Plan Guide

Use a separate plan for every medium- or high-risk task. `AGENTS.md` is the single authority for risk, lifecycle, and authorization; this guide defines the plan artifact.

## Workflow

```text
Requirement
→ Code Investigation
→ Draft Plan
→ Review Open Questions
→ Approved Scope
→ Phased Implementation
→ Verification
→ Checkpoint / Completion
```

A plan must be based on repository investigation. Do not turn a user request directly into an implementation design without verifying current behavior, call chains, affected files, tests, and compatibility constraints.

Use short, descriptive kebab-case names such as:

```text
plans/add-callback-support.md
plans/mq-migration.md
plans/user-auth-refactor.md
```

Update a plan when investigation changes its assumptions or scope. Do not use plans as a chronological work log; use checkpoints for resumable phase state.

## Metadata

Every task plan except this guide begins with:

```yaml
---
id: kebab-case-task-id
status: draft
risk: medium
current_phase: 1
phase_status: planned
updated: YYYY-MM-DD
---
```

Use only the states defined in `AGENTS.md`. The file name must be `plans/<id>.md`. Run `scripts/validate-agent-infra.sh` after changing metadata.

## Plan Template

```markdown
---
id: task-id
status: draft
risk: medium
current_phase: 1
phase_status: planned
updated: YYYY-MM-DD
---

# Task

## 1. Goal

What problem must this task solve?

## 2. Background

Why is the change needed?

## 3. Current Behavior

How does the verified current implementation behave?

## 4. Relevant Call Chain

What entry points, calls, state transitions, and external interactions are involved?

## 5. Relevant Files

Which existing files provide the core evidence for the plan?

## 6. Scope

What files, modules, and behavior may change?

## 7. Out of Scope

What is explicitly excluded?

## 8. Proposed Changes

What changes are proposed, and why do they fit existing repository patterns?

## 9. Compatibility

What callers, stored data, configuration, commands, or protocols may be affected?

## 10. Risks

What can fail, regress, become incompatible, or expose data?

## 11. Implementation Phases

### Phase 1

### Phase 2

### Phase 3

## 12. Acceptance Criteria

What observable conditions define completion?

## 13. Verification

What existing tests, builds, static checks, manual checks, or recovery scenarios must run?

## 14. Open Questions

What cannot yet be confirmed from code or authoritative documentation?
```

If a section is not applicable, explain why. Mark unresolved facts as `TODO / Need Confirmation`; do not replace missing evidence with assumptions.
