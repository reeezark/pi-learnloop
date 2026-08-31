# Architecture Decision Records

Use ADRs only for important decisions intended to remain valid across multiple tasks, such as:

- System architecture or module boundaries.
- Public protocols and compatibility guarantees.
- Persistent data models and migration strategy.
- Security and privacy boundaries.
- Core dependency or storage-engine selection.

Do not create an ADR for routine implementation details, local refactors, formatting, or decisions that belong only to one task plan.

Use sequential names such as:

```text
docs/decisions/ADR-0001-local-transport.md
```

Every ADR except this guide begins with:

```yaml
---
id: ADR-0001
status: proposed
date: YYYY-MM-DD
supersedes: none
---
```

Allowed ADR states are `proposed | accepted | superseded | deprecated`. Use a real ADR identifier in `supersedes` when replacing a decision; otherwise use `none`.

Before adding an ADR, inspect existing decisions and link superseded records instead of rewriting history.

## ADR Template

```markdown
---
id: ADR-XXXX
status: proposed
date: YYYY-MM-DD
supersedes: none
---

# ADR-XXX: Decision Title

## Context

What verified forces and constraints require a decision?

## Decision

What is being decided?

## Alternatives

What credible alternatives were considered, and why were they not selected?

## Consequences

What positive, negative, compatibility, migration, and operational effects follow?
```

An ADR should state facts and trade-offs concisely. Implementation details should remain in the corresponding plan unless they are themselves part of the long-lived decision. Run `scripts/validate-agent-infra.sh` after changing ADR metadata.
