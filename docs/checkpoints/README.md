# Checkpoint Guide

Checkpoints preserve resumable task state after an important phase, before changing Agent Sessions, before a handoff, or when context may be compacted. They prevent completed work from being repeated and make remaining scope explicit.

Use this naming convention:

```text
docs/checkpoints/<task>-phase-<n>.md
```

Every checkpoint except this guide begins with:

```yaml
---
id: <task>-phase-<n>
plan: <task>
phase: <n>
status: current
updated: YYYY-MM-DD
---
```

Allowed checkpoint states are `current | superseded`. A plan may have at most one `current` checkpoint. Before adding a newer checkpoint, change the previous checkpoint to `status: superseded`. Metadata, not modification time or filename sorting, identifies resumable state.

A checkpoint is not a substitute for a task plan, Git history, or stable project documentation. Do not create one for every small edit. Update `PROJECT.md` or an ADR when information is long-lived rather than phase-specific.

## Checkpoint Template

```markdown
---
id: task-phase-n
plan: task
phase: n
status: current
updated: YYYY-MM-DD
---

# Context

## Goal

## Current Phase

## Completed

## Modified Files

## Important Decisions

## Tests / Verification

## Known Issues

## Remaining Work

## Next Step

## Do Not Change
```

Checkpoint entries must be factual and recoverable:

- List exact file paths and verified commands.
- Distinguish completed work from proposed work.
- Record failed or skipped verification explicitly.
- Preserve unresolved questions as `TODO / Need Confirmation`.
- Check Git status and diff before trusting a checkpoint after resuming.
- Run `scripts/validate-agent-infra.sh` after adding or superseding a checkpoint.
