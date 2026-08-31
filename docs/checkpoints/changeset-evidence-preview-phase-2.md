---
id: changeset-evidence-preview-phase-2
plan: changeset-evidence-preview
phase: 2
status: superseded
updated: 2026-08-31
---

# Context

## Goal

Expose the completed Phase 1 evidence-preview module through the smallest versioned, authenticated, loopback-only daemon defined by accepted ADR-0002, without adding the Pi extension or later business behavior.

## Current Phase

Phase 2 is complete. The active plan is at Phase 3 `awaiting_approval`; no TypeScript, Pi extension, package manifest, or Phase 3 implementation is authorized.

## Completed

- Accepted `docs/decisions/ADR-0002-local-daemon-protocol-security.md` after explicit user approval.
- Added the foreground `pi-learnloop daemon` command with `SIGINT` and `SIGTERM` cancellation and no security-weakening flags.
- Added the `internal/daemon` deep module with one `Run` interface and an internal state-directory injection used by integration tests.
- Bound one `tcp4` listener to `127.0.0.1:0` and published the assigned address through a versioned Runtime Descriptor.
- Added a 32-byte per-start Instance Token, constant-time custom-scheme authentication, protected runtime files, owner and mode checks, symlink rejection, token rotation, single-instance advisory locking, stale-state replacement, and instance-aware cleanup.
- Added unauthenticated `GET /v1/status` and authenticated `POST /v1/evidence-previews` with exact Host, IPv4-loopback peer, Origin, no-CORS, no-cache, method, media type, 16 KiB body, strict JSON, selection-length, timeout, and cancellation controls.
- Added fixed public evidence caps of 20 files, 100 declarations, and 131072 aggregate excerpt bytes.
- Added stable versioned success and safe error payloads that translate, but do not modify, Phase 1 evidence error codes.
- Added real-Git integration coverage for commit ranges and working trees, plus adversarial protocol, fixed 30-second deadline, lifecycle, stale-state, and cancellation cases.

## Modified Files

- `cmd/pi-learnloop/main.go`
- `cmd/pi-learnloop/main_test.go`
- `internal/daemon/daemon.go`
- `internal/daemon/daemon_test.go`
- `internal/daemon/runtime.go`
- `internal/daemon/server.go`
- `PROJECT.md`
- `plans/changeset-evidence-preview.md`
- `docs/decisions/ADR-0002-local-daemon-protocol-security.md`
- `docs/checkpoints/changeset-evidence-preview-phase-1.md`
- `docs/checkpoints/changeset-evidence-preview-phase-2.md`

No change was made to `internal/evidence`, `go.mod`, dependencies, or any Phase 3 file.

## Important Decisions

- ADR-0002 is accepted and its `/v1` protocol, fixed limits, security defaults, discovery fields, token behavior, and error codes are compatibility-sensitive.
- HTTP remains a thin adapter. `internal/evidence` still owns Git invocation, changeset semantics, Go parsing, declaration mapping, ordering, omissions, and truncation.
- The daemon is foreground-only and single-instance. It does not install itself, daemonize, start at login, or expose a non-loopback address.
- Runtime state uses `os.UserConfigDir()/pi-learnloop/runtime`, mode `0700`, with `0600` descriptor, token, and lock files.
- `Config.StateDir` is an internal Go test seam, not a CLI flag or released protocol setting.
- The implementation uses only the Go standard library and the existing local `git` executable; no dependency or `go.sum` was added.

## Tests / Verification

- Passed with already-installed Go 1.26.4: `go test -count=1 ./...`.
- Passed with already-installed Go 1.26.4: `go test -race -count=1 ./...`.
- Passed with already-installed Go 1.26.4: `go vet ./...`.
- Passed with default Go 1.21.13: `CGO_ENABLED=0 go test -count=1 ./...`.
- Passed with default Go 1.21.13: `go test -count=1 -tags netgo ./...`.
- Passed with default Go 1.21.13: `go test -race -count=1 -tags netgo ./...` and `go vet -tags netgo ./...`.
- Passed: command build to a temporary path; the artifact was removed after verification.
- Failed only with the default Homebrew Go 1.21.13 external network linker on this macOS 26 machine: untagged `go test -count=1 ./...` aborts daemon and command test binaries with `missing LC_UUID`; the evidence-only package passes. This toolchain limitation is recorded in `PROJECT.md` and does not reproduce with the installed Go 1.26.4 or pure-Go networking.
- Passed: `scripts/validate-agent-infra.sh` and all positive/negative cases in `scripts/test-agent-infra.sh`.
- Passed: `gofmt -l cmd/pi-learnloop internal/daemon` returned no files.
- Reviewed: allowed-file scope, status, empty ordinary diff caused by the all-untracked baseline, direct contents of every Phase 2 source/document, and absence of `go.sum`, TypeScript, Pi extension files, and repository build artifacts.

## Known Issues

- The repository still has no initial Git commit; every file remains untracked and ordinary `git diff`, `git diff --stat`, and `git diff --check` cannot represent the full change.
- The default Homebrew Go 1.21.13 toolchain on this macOS 26 host needs `CGO_ENABLED=0` or `-tags netgo` for network-enabled binaries; the installed Go 1.26.4 runs the unmodified commands.
- Runtime support has been exercised only on the current macOS ARM64 machine. macOS AMD64 remains an unverified release target.
- The Instance Token does not protect against root, a malicious same-user process, or a compromised trusted Pi extension, as defined by ADR-0002.
- No packaged client consumes the descriptor yet; the Phase 3 Pi extension must implement exact loopback validation, one status check, token read, proxy bypass, and at most one discovery retry.

## Remaining Work

- Phase 3: approve the Pi package identity, supported Pi version range, installation path, npm dependencies, and exact extension test commands.
- Implement the thin, user-triggered `/learn` extension only after separate Phase 3 authorization.
- Later plans: SSE and durable jobs, SQLite persistence, isolated evaluator, assessment behavior, packaging, and cross-architecture verification.

## Next Step

Review this checkpoint and the accepted daemon contract. If desired, investigate the Phase 3 Pi extension prerequisites without implementation, then request explicit Phase 3 authorization.

## Do Not Change

- Do not begin Phase 3, add `package.json`, TypeScript, Pi APIs, npm dependencies, or `/learn` behavior without explicit Phase 3 authorization.
- Do not weaken ADR-0002 binding, discovery, authentication, permissions, limits, error, or compatibility rules silently.
- Do not add SSE, SQLite, durable jobs, evaluators, model calls, telemetry, remote access, autostart, or configuration flags under the completed Phase 2 authorization.
- Do not modify `internal/evidence`, `go.mod`, dependency versions, or create a Git commit without separate authorization.
