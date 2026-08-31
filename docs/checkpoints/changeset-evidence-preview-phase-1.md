---
id: changeset-evidence-preview-phase-1
plan: changeset-evidence-preview
phase: 1
status: superseded
updated: 2026-08-31
---

# Context

## Goal

Implement and verify the Go evidence-preview core without networking, persistence, TypeScript, Pi RPC, model calls, or third-party Go dependencies.

## Current Phase

Phase 1 is complete. The active plan is at Phase 2 `awaiting_approval` and no Phase 2 implementation is authorized.

## Completed

- Created the Go 1.21 module `github.com/reeezark/pi-learnloop`.
- Added the `internal/evidence` deep module with one caller-facing `Preview` function.
- Implemented canonical Git-root and commit resolution with option-safe revision verification.
- Implemented explicit commit-range and working-tree selections, including staged, unstaged, and untracked non-ignored Go files.
- Mapped changed lines to functions, methods, types, interfaces, variables, and constants with stable source ordering.
- Added method receiver and declaration identity fields.
- Added caller-provided file, declaration, and aggregate excerpt limits with UTF-8-safe truncation metadata.
- Added explicit omission reasons for deleted files, deletion-only hunks, and changed ranges outside declarations.
- Added stable error codes for invalid requests, non-repositories, invalid revisions, Git failures, source-read failures, parse failures, and repository escape attempts.
- Rejected working-tree symlinks that resolve outside the canonical repository root.

## Modified Files

- `go.mod`
- `internal/evidence/evidence.go`
- `internal/evidence/evidence_test.go`
- `PROJECT.md`
- `plans/changeset-evidence-preview.md`
- `docs/checkpoints/changeset-evidence-preview-phase-1.md`

## Important Decisions

- The canonical Go module path is `github.com/reeezark/pi-learnloop`.
- The evidence-preview module owns Git invocation, diff parsing, Go syntax parsing, ordering, limits, and error semantics behind one interface.
- Git is treated as a local-substitutable dependency; tests use real temporary repositories, so no Git port or mock adapter was introduced.
- Rename detection is disabled for this phase so renames have the deterministic outcome `deleted old path + added new path`.
- Evidence limits have no silent defaults in the core; every caller must supply positive limits.
- The core creates no listener, performs no model call, persists nothing, and sends no source outside the process.
- No Git commit is authorized or created in this phase.

## Tests / Verification

- Passed: `go test ./...`.
- Passed: `go test -race ./...`.
- Informational: `go test -cover ./...` reported 89.7% statement coverage.
- Passed: `scripts/test-agent-infra.sh` with all 16 positive and negative scenarios.
- Passed: `scripts/validate-agent-infra.sh` after checkpoint and lifecycle updates.
- Passed: formatting of all new Go files and repository metadata validation.
- Reviewed: Git status and direct contents of all Phase 1 files. Ordinary `git diff`, `git diff --stat`, and `git diff --check` cannot represent untracked files before the initial commit; this limitation remains explicit rather than being reported as a clean baseline diff.

## Known Issues

- The module is syntax-level only; it does not yet use `go/types` or `golang.org/x/tools/go/packages`.
- Deletion-only hunks are explicitly reported as omissions rather than mapped through base-side source.
- Generated Go files are not specially classified.
- No daemon or caller exists yet, so evidence limits are explicit inputs rather than released defaults.
- The repository has no initial commit and every file remains untracked; normal `git diff` and `git diff --stat` are empty.

## Remaining Work

- Phase 2: approve the local protocol/security ADR and implement the authenticated loopback daemon adapter.
- Phase 3: approve dependencies and implement the thin Pi `/learn` extension.
- Later plans: type/dependency enrichment, evaluator, persistence, and durable jobs.

## Next Step

Investigate and approve the Phase 2 protocol/security ADR, then explicitly authorize Phase 2 before implementing any daemon interface.

## Do Not Change

- Do not add HTTP, SQLite, TypeScript, Pi RPC, prompts, model calls, or external Go dependencies under Phase 1 authorization.
- Do not introduce default evidence-sharing limits before the daemon protocol/security decision.
- Do not create a Git commit without separate user authorization.
