---
id: changeset-evidence-preview-phase-3
plan: changeset-evidence-preview
phase: 3
status: current
updated: 2026-08-31
---

# Context

## Goal

Connect a thin, user-triggered Pi extension to the completed Phase 2 daemon so a trusted-project user can explicitly select a Git changeset and inspect its bounded changed-Go evidence before any evaluation.

## Current Phase

Phase 3 and the `changeset-evidence-preview` plan are complete. No later evaluator, persistence, SSE, automatic reminder, or learning workflow work is authorized by this checkpoint.

## Completed

- Added the `pi-learnloop` 0.1.0 Pi package manifest with Node.js `>=22.19.0`, Pi 0.84.x as the documented supported line, the required Pi core peer declaration, and no third-party runtime npm dependency.
- Locked the explicitly approved development dependencies: `@earendil-works/pi-coding-agent@0.84.3`, `@types/node@22.19.19`, and `typescript@5.9.3`.
- Added the minimal Pi adapter that registers only `/learn`; it subscribes to no events and displays no automatic reminder.
- Added explicit commit-range and working-tree selection dialogs using `ctx.cwd`, UI availability, and project-trust checks.
- Added preview rendering for file paths, statuses, mapped symbol identities, approximate UTF-8 excerpt bytes, empty Go changes, and truncation details.
- Added understandable recovery messages for daemon unavailability, persistent authentication failure, invalid revisions, unsupported selections, and missing revision input.
- Added a deep daemon-client module behind one `preview(repository, selection)` interface.
- Implemented exact macOS runtime-directory resolution, directory/file ownership and permission checks, symlink rejection, strict `http://127.0.0.1:<port>` validation, instance status verification before token access, direct Node HTTP requests that ignore environment proxies, custom-scheme authentication, one discovery-race retry, a 1 MiB response bound, timeouts, stable server-error propagation, and complete v1 success-payload validation.
- Added a public README with current behavior, local installation, manual usage, security/privacy limits, and authoritative development commands.
- Updated `PROJECT.md` to distinguish the completed evidence-preview slice from future evaluator and persistence work.

## Modified Files

- `.gitignore`
- `package.json`
- `package-lock.json`
- `tsconfig.json`
- `extensions/pi-learnloop.ts`
- `extensions/lib/daemon-client.ts`
- `extensions/lib/learn-command.ts`
- `tests/extension/daemon-client.test.ts`
- `tests/extension/extension-entry.test.ts`
- `tests/extension/learn-command.test.ts`
- `README.md`
- `PROJECT.md`
- `plans/changeset-evidence-preview.md`
- `docs/checkpoints/changeset-evidence-preview-phase-2.md`
- `docs/checkpoints/changeset-evidence-preview-phase-3.md`

No Go source, `go.mod`, accepted ADR, evaluator asset, dependency version outside the approved npm list, CI configuration, or Git history was changed in Phase 3.

## Important Decisions

- `pi-learnloop` is the package identity. A public npm lookup returned `404` on 2026-08-31, but the name is not reserved because this phase did not publish it.
- Pi 0.84.3 is the verified minimum and Pi 0.84.x is the initial compatibility claim. Later Pi versions require explicit verification before the claim changes.
- Official Pi guidance is followed by declaring `@earendil-works/pi-coding-agent: "*"` as a non-bundled peer. Version 0.84.3 is also an exact development dependency for interface type-checking.
- Node's built-in `node:test`, filesystem, and HTTP modules avoid runtime and test-framework dependencies.
- `skipLibCheck` is enabled because Pi 0.84.3's published declaration graph fails full library checking on missing optional declarations and JSON import attributes. Project TypeScript remains strict and is checked with `tsc --noEmit`.
- The command module owns only user interaction and rendering. The client module owns transport/discovery behavior, and the Go daemon/evidence modules remain the only owners of selection validation, Git analysis, Go parsing, evidence limits, and protocol behavior.
- `/learn` is preview-only. It does not call a model, start an Agent turn, append a Pi Session entry, save data, or upload telemetry.

## Tests / Verification

- Passed: `npm run typecheck` with TypeScript 5.9.3.
- Passed: `npm test`; all 12 command, entry, schema, security, retry, proxy-bypass, and real-loopback tests pass on Node 26.0.0.
- Passed: `npm install --ignore-scripts --registry=https://registry.npmjs.org`; 140 packages audited with zero reported vulnerabilities at install time.
- Passed: `npm pack --dry-run --json`; the package contains only `package.json`, `README.md`, the Pi entry, and its two internal TypeScript modules, with no bundled dependency.
- Passed: real manual smoke on Pi 0.84.3 using `pi -e`: `/learn` selected a temporary repository's working tree against `HEAD` and rendered one modified file, the `Answer` symbol, 32 excerpt bytes, no truncation, and the explicit no-model/no-save notice.
- Passed: daemon shutdown removed `daemon.json` and `daemon.token`; only the accepted protected empty lock file remained.
- Passed: `go vet ./...` with the default Go 1.21.13 toolchain.
- Failed only for the already-recorded host toolchain issue: default Go 1.21.13 `go test -count=1 ./...` and `go test -race -count=1 ./...` abort network-enabled test binaries with `missing LC_UUID`; `internal/evidence` still passes.
- Passed with the established host workaround: `CGO_ENABLED=0 go test -count=1 ./...`.
- Passed with the established host workaround: `go test -race -count=1 -tags netgo ./...` and `go vet -tags netgo ./...`.
- Passed: `scripts/validate-agent-infra.sh` and all positive/negative cases in `scripts/test-agent-infra.sh`.
- Reviewed: dependency versions and licenses, allowed-file scope, status, package contents, runtime cleanup, empty ordinary diff caused by the all-untracked baseline, and direct contents of all Phase 3 source, tests, manifest, and documentation.

## Known Issues

- The repository still has no initial Git commit; every file remains untracked and ordinary `git diff`, `git diff --stat`, and `git diff --check` cannot represent the full repository state.
- The default Go 1.21.13 external linker issue on this macOS 26 host remains unchanged; pure-Go/netgo verification passes, and Phase 3 changed no Go file.
- Full checking of Pi 0.84.3's transitive declaration files is not possible with the approved dependency graph, so `skipLibCheck` is required; the project's own TypeScript remains strictly checked.
- Runtime and manual Pi verification were performed only on macOS ARM64. macOS AMD64 remains an unverified release target.
- The package has not been published, the npm name is not reserved, and no Git remote or release ref exists in this local repository.
- The Instance Token does not protect against root, a malicious same-user process, or a compromised trusted Pi extension, as defined by ADR-0002.

## Remaining Work

- Create a new approved plan before adding evaluator questions, answers, scoring, persistence, SQLite, SSE, durable jobs, Session association, automatic behavior, release automation, or publication.
- Decide whether to create the initial Git baseline and configure the Git remote through a separately authorized repository-management task.
- Verify macOS AMD64 and later Pi versions before expanding compatibility claims.

## Next Step

Select the next product slice and investigate it in a new plan. Do not continue implementation under the completed `changeset-evidence-preview` plan.

## Do Not Change

- Do not weaken ADR-0002 loopback, discovery, authentication, permissions, fixed limits, error, or compatibility rules without a new accepted decision and explicit phase authorization.
- Do not make `/learn` automatic or add background Session indexing; learning remains user-triggered.
- Do not call a model or persist learning data until evaluator and storage contracts are separately designed and approved.
- Do not publish the npm package, create a Git commit, configure a remote, or add release automation without explicit authorization.
