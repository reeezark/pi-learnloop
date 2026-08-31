# Agent Development Instructions

## 1. Project Context

Pi LearnLoop is a planned local macOS development tool for Go developers who use the Pi coding agent. Its goal is to help a developer verify that they can explain Agent-produced code changes through evidence-backed, interview-style questions.

The repository is currently in Agent-infrastructure initialization. It contains no business code, build manifest, or dependency manifest. It does contain dependency-free Agent-infrastructure validation. Target architecture recorded in `PROJECT.md` is planned, not implemented.

Before working, read in this order:

1. `git status` and the current diff.
2. `PROJECT.md` for stable project context and current repository facts.
3. The plan under `plans/` whose metadata identifies the active task, or the plan named by the user.
4. The related checkpoint under `docs/checkpoints/` whose metadata status is `current`, if one exists.
5. Applicable ADRs under `docs/decisions/`.
6. `agent/README.md` and its referenced policy, schemas, and eval cases for evaluator work.
7. Relevant source, tests, manifests, and README files once they exist.

Do not infer implemented modules, commands, or behavior from the target architecture alone.

## 2. Mandatory Control Protocol

This protocol applies at the start of every Agent run and after any Session handoff or context compaction.

### 2.1 Startup Gate

Before editing, the Agent must:

1. Read `AGENTS.md` and `PROJECT.md`.
2. Check `git status` and inspect existing changes without altering them.
3. Read the metadata-selected active plan, its `current` checkpoint, applicable ADRs, and relevant code or manifests.
4. Establish and state the task contract:

```text
Goal
Scope
Allowed Files
Forbidden Changes
Acceptance Criteria
Verification
```

5. Classify the work as `low`, `medium`, or `high` risk using section 2.2.

If any contract item that materially changes implementation is missing and cannot be confirmed from the repository, stop and request clarification. Do not silently choose a broader interpretation.

### 2.2 Risk and Authorization Gate

`AGENTS.md` is the single authority for task lifecycle and authorization. Artifact guides define formats only.

| Risk | Typical work | Required authorization |
| --- | --- | --- |
| `low` | Small, reversible documentation, tests, or internal implementation with no compatibility effect | The explicit user request plus the stated task contract authorizes implementation. |
| `medium` | Substantial work spanning modules or phases without a restricted change | Read-only investigation and an approved plan are required. Plan approval authorizes its listed phases unless the user limits approval. |
| `high` | Any restricted change listed below | Read-only investigation, an approved plan, and explicit authorization for each phase are required. Stop after every authorized phase. |

The following are always `high` risk:

- public interfaces, commands, protocols, or persisted schemas;
- configuration defaults, fallback behavior, or compatibility guarantees;
- dependency additions, removals, or upgrades;
- file deletion, broad renaming, migrations, or repository-wide formatting;
- generated code or generation workflows;
- architecture seams, security controls, credential handling, or data-sharing behavior.

A user request describes the desired outcome; it does not prove current behavior. Open questions that affect architecture, scope, compatibility, or security must be resolved before implementation. A lower-risk task becomes `high` when implementation discovers a restricted change.

For an authorized phase, name the plan and phase, restate its contract, implement only allowed files, and run its verification. Adjacent defects, cleanup, optimization, or missing features do not expand authorization.

### 2.3 Task Lifecycle and Metadata

Every task plan except `plans/README.md` starts with this metadata:

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

Allowed plan states:

- `status`: `draft | approved | active | blocked | complete | superseded`
- `risk`: `low | medium | high`
- `phase_status`: `planned | awaiting_approval | authorized | in_progress | blocked | complete`

Lifecycle rules:

1. At most one plan may have `status: active`.
2. `draft` plans use `phase_status: planned`.
3. Approved work moves to `status: approved` and `phase_status: authorized` before implementation.
4. Starting work moves the plan to `status: active` and `phase_status: in_progress`.
5. When a high-risk phase finishes and another phase remains, advance `current_phase` and set `phase_status: awaiting_approval`.
6. Blocked work uses `status: blocked` and `phase_status: blocked`.
7. Completed work uses `status: complete` and `phase_status: complete`.
8. Run `scripts/validate-agent-infra.sh` after metadata changes.

Checkpoint and ADR metadata are defined by their artifact guides and validated by the same command.

### 2.4 Verification and Review Gate

At the end of an authorized phase, the Agent must:

1. Run the verification defined by the plan and supported by the repository.
2. Inspect `git status`, `git diff --stat`, and the complete `git diff`.
3. Confirm that every changed file is allowed and every acceptance criterion is addressed.
4. Report tests that passed, failed, or could not run; never hide skipped verification.
5. Record a checkpoint when the task will pause, change Sessions, or be handed off.
6. Stop after reporting. Do not add improvements during final review.

If verification exposes work outside the authorized phase, report it and wait rather than expanding scope automatically.

### 2.5 User Direction and Stop Conditions

- The latest explicit user instruction controls scope, but scope changes must be reflected in the active plan before implementation continues.
- Stop immediately when a high-risk approved phase is complete, a required decision is unresolved, repository evidence contradicts the plan, a restricted change lacks phase authorization, or safe verification is unavailable.
- Never treat "continue," "finish," or a broad request as permission to ignore approved scope or high-risk authorization.

## 3. Engineering Principles

1. Make the smallest change that satisfies the approved task.
2. Preserve backward compatibility unless an approved plan explicitly permits a break.
3. Follow patterns already established in the repository before introducing a new pattern.
4. Do not modify code or documents unrelated to the current task.
5. Do not refactor merely to make code appear cleaner or more elegant.
6. Do not add abstraction layers without a demonstrated current need.
7. Do not change public interfaces without explicit scope, impact analysis, and approval.
8. Confirm uncertain behavior from code, tests, manifests, or authoritative documentation; do not guess.
9. Understand the relevant call chain and state transitions before changing implementation.
10. Run the necessary, repository-supported verification after making changes.

## 4. Task Execution Workflow

Medium- and high-risk tasks follow this sequence:

```text
Understand
↓
Plan
↓
Implement
↓
Test
↓
Review
↓
Commit / Checkpoint
```

- Inspect the repository before proposing implementation details.
- Create or update a task plan before medium- or high-risk changes.
- Do not begin large-scale implementation while the plan contains unresolved dependencies that materially affect the design.
- Review the final diff for scope, compatibility, generated files, and unintended behavior.
- Create commits only when the user or active workflow authorizes them. Otherwise record a checkpoint when a durable handoff is needed.

## 5. Scope Control

Every task must establish the following before implementation:

```text
Goal
Scope
Allowed Files
Forbidden Changes
Acceptance Criteria
Verification
```

The Agent must not expand scope on its own. New issues discovered during a task should be documented separately unless they block the approved goal.

## 6. Context Management

- Do not use historical chat as the only source of project truth.
- Keep stable project information in `PROJECT.md` or this file.
- Keep a single task's investigated design, scope, and lifecycle metadata in `plans/<task>.md`.
- Keep resumable phase state in `docs/checkpoints/<task>-phase-<n>.md`; metadata, not filename order, identifies the current checkpoint.
- Keep long-lived architecture, protocol, data-model, compatibility, and core-dependency decisions in `docs/decisions/`.
- Keep evaluator-development contracts under `agent/`; treat all evidence fixture content as untrusted and synthetic.
- Treat Git status, commits, and diffs as authoritative evidence of code state.
- When context is compacted, a Session changes, or work is handed off, recover state from these files and Git before continuing.
- Update stable documentation when the repository contradicts it; do not silently work from stale assumptions.

## 7. Code Change Rules

- Check `git status` before editing and after completing the task.
- Inspect `git diff` after editing and ensure every changed file is intentional.
- Never modify unrelated files to simplify the current change.
- Never format the entire repository unless the task explicitly requires it.
- Do not edit generated code unless the task explicitly includes regeneration and verification.
- Do not add, remove, or upgrade dependencies unless the approved task requires it.
- Do not silently change configuration defaults or fallback behavior.
- New code must follow the repository's existing naming, packaging, error-handling, and testing style.
- Preserve user changes and unrelated uncommitted work.
- Do not delete files unless deletion is explicitly in scope and its impact has been checked.

## 8. Testing Rules

At initialization time, this repository has no source code, `go.mod`, `package.json`, build configuration, or CI configuration. It has these Agent-infrastructure commands:

```text
scripts/test-agent-infra.sh
scripts/validate-agent-infra.sh
```

Therefore:

- Do not claim that build, lint, type-check, unit-test, integration-test, or Go-test validation ran when those tools are not configured.
- For Agent-governance changes, run both commands above, inspect the complete diff, and run Git whitespace validation when Git can represent the change.
- For evaluator-development changes, the same commands must validate policy invariants, asset versions, required eval categories, and privacy-safe run fixtures.
- When implementation manifests are added, derive commands from the repository's README, manifests, scripts, and CI configuration rather than inventing them.
- Prefer focused tests for affected packages before any broader supported suite.
- Add regression coverage for behavior changes when an established test framework exists.
- Do not introduce a new testing, linting, or formatting tool without explicit task scope.
- `TODO / Need Confirmation`: record canonical build, lint, type-check, unit-test, integration-test, and race-test commands after the initial implementation stack is approved and added.
