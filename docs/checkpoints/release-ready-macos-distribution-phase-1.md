---
id: release-ready-macos-distribution-phase-1
plan: release-ready-macos-distribution
phase: 1
status: current
updated: 2026-09-04
---

# Release-Ready macOS Distribution Phase 1

## Context

### Goal

Provide the additive local version diagnostic and a closed, independently
verifiable unsigned macOS artifact contract without changing runtime behavior,
the Go 1.21 source baseline, or any dependency.

### Current Phase

The user accepted ADR-0009, authorized Phase 1, and permitted obtaining Go
1.27.1 against the official go.dev checksum on 2026-09-04. Phase 1 is complete;
the plan is active at Phase 2 `awaiting_approval`. Phase 2 is not authorized.

The starting checkout was `main` at `2fac6e6c1d2c44e7817105f7eb55729948f20fc6`,
matching the local `origin/main` tracking reference. Only the prior draft Plan
and ADR-0009 were untracked; no tracked change existed. This phase's changes
remain uncommitted and unpushed. No remote tag or publication was created.

## Completed

- Added exactly `pi-learnloop version`: stdout is `pi-learnloop <version>` plus
  LF, stderr is empty, and exit status is zero. Source builds default to `dev`;
  the builder injects the package SemVer into the private `main.version` string.
- Preserved the daemon invocation and error path. Unsupported arguments still
  exit 2 with the updated closed usage string; no weakening flags were added.
- Added one local builder accepting only version and output directory. It checks
  canonical package-version equality, exact Go 1.27.1, fixed architecture/CGO/
  build flags, and no automatic toolchain switching. It creates private staging
  beside an absent/empty outside-checkout output, independently verifies all
  artifacts, then renames the complete set into place without overwriting a
  nonempty directory. Normal failures and handled signals clean staging;
  cancellation terminates/reaps the active child process group.
- Added a separate static verifier accepting only version and artifact directory.
  It checks an exact two-ZIP-plus-`SHA256SUMS` set, fixed entries and modes,
  archive checksums/CRC, exact license and README, thin Mach-O architecture,
  the actual file-backed version string, exact Go/dependency/build metadata,
  valid VCS fields, and cross-architecture provenance equality. Inspection uses
  private copies of the same bytes checked by SHA-256, never executes candidates,
  and extracts only explicitly named streams into non-executable temporary files.
- Added self-tests for real builds, repeated binary digests, native diagnostic,
  invalid/mismatched versions, compiler rejection, safe output refusal, signal
  cleanup, second-architecture failure, and adversarial artifacts/provenance.
- Kept npm's exact six-file package, all dependency manifests, Go 1.21 baseline,
  HTTP/descriptors, schema/prompt versions, data, model isolation, and foreground
  daemon lifecycle unchanged.

## Modified Files

- `cmd/pi-learnloop/main.go`
- `cmd/pi-learnloop/main_test.go`
- `scripts/build-release-artifacts.sh`
- `scripts/verify-release-artifacts.sh`
- `scripts/test-release-artifacts.sh`
- `PROJECT.md`
- `plans/release-ready-macos-distribution.md`
- `docs/decisions/ADR-0009-release-ready-macos-distribution.md`
- `docs/checkpoints/release-ready-macos-distribution-phase-1.md`

## Important Decisions

- The release modules use existing Node built-ins and native macOS `zip`, `unzip`,
  and `nm`, plus the exact Go compiler. No package or build-framework dependency
  was introduced. Reusable build policy stays in the builder/verifier, not in a
  future workflow's duplicated steps (codebase-design deep-module discipline).
- Go 1.27.1's `-trimpath` build metadata does not record the linker version value.
  The verifier therefore inspects the `main.version` symbol's string header and
  file-backed Mach-O segments; an unrelated matching string is not accepted.
- Local uncommitted candidates truthfully retain `vcs.modified=true`. Integrity
  verification does not grant release eligibility; clean signed-tag checks,
  native Intel execution, and Apple trust gates remain separate future work.
- “Unsigned candidate” means no Developer ID signing/notarization was performed;
  the Go toolchain's platform-required ad-hoc signature is not publisher trust.
- ZIP bytes and signed bytes are not claimed reproducible. Only the pre-Developer-
  ID executable hashes are compared.

## Tests / Verification

Passed on 2026-09-04:

- Downloaded `go1.27.1.darwin-arm64.tar.gz` via HTTPS from go.dev into an isolated
  temporary directory, verified size 68,100,347 and official SHA-256
  `ee215d57e0ec269c60cc9ceca68e6bda321ba9ee5afe24f4b0988703c2d87d12`,
  then confirmed `go version go1.27.1 darwin/arm64` with `GOROOT` cleared and
  `GOTOOLCHAIN=local`. No system toolchain installation was replaced.
- `gofmt` on the two command files and focused `go test -count=1 ./cmd/pi-learnloop`.
- `scripts/test-release-artifacts.sh` with exact Go 1.27.1 on PATH and an isolated
  build cache: both architectures build, verify, and repeat with identical bytes.
  ARM64 binary SHA-256:
  `19efd1a2b116a5c3618383cb07a2cc3daed3d8bac46609a680e6f1da4f94a8c3`.
  AMD64 binary SHA-256:
  `9bb801ac9f169dc6a93077e7f2e5fcb40125a5ceefa772f8d8beb8abb7246e95`.
  These identify local dirty candidates, not a published release.
- Native ARM64 `version` and unsupported-invocation smoke checks; static Mach-O,
  symbol, Go `version -m -json`, ZIP mode/content, and checksum checks for both
  architectures. The development build separately reports `pi-learnloop dev`.
- Go 1.27.1 `go test -p=1 -count=1 ./...`: all seven packages passed.
- Go 1.27.1 `go test -race -p=1 -count=1 ./...`: all seven packages passed.
- Go 1.27.1 `go vet ./...` and `CGO_ENABLED=0 go build -mod=readonly` for the
  command, with executable output directed outside the repository.
- Installed Go 1.21.13 `CGO_ENABLED=0 go test -p=1 -count=1 ./...`: all seven
  packages passed, using a separate cache and no inherited `GOROOT`.
- `npm run typecheck`; `npm test` (58/58); isolated-cache
  `npm pack --dry-run --json` (exact original six entries, no bundled dependency,
  25,604-byte archive, 103,586-byte unpacked content; no tarball written).
- `scripts/test-agent-infra.sh`, `scripts/validate-agent-infra.sh`, shell syntax
  checking, Git whitespace checking, and allowed-file/full-diff review.
- `git diff --exit-code -- go.mod go.sum package.json package-lock.json` confirms
  no baseline, dependency, or package change.

The first sandboxed download failed DNS resolution; the permitted HTTPS retry
succeeded. Preliminary sandboxed builds emitted a non-fatal Go module stat-cache
write warning; the final permitted build/self-test completed cleanly. TDD first
observed the missing command/scripts, then found and fixed interrupted-build
staging retention and acceptance of extra README text. A metadata-tampering test
initially modified only Go's duplicate runtime string rather than the inspected
build-info copy; the fixture was corrected to alter both copies. One preliminary
self-test run overlapped a script edit and printed a shell EOF warning; it was
discarded and the final unchanged script completed successfully.

No verification called a live model or used production history or Pi Sessions.

## Known Issues

- Native AMD64 execution, minimum-macOS execution, CI, signing, notarization,
  protected environments, and public publication are not verified or implemented
  by Phase 1. The local host is ARM64; cross-compilation is not Intel execution.
- A SIGKILL or host failure can leave private temporary directories; it cannot
  turn an incomplete staging set into published output. No durable build-recovery
  or background cleanup process is introduced.
- The native static version inspection is intentionally tied to the exact
  approved Go toolchain. A compiler/layout change requires new verification.
- CodeGraph MCP tools were unavailable despite the existing `.codegraph` index;
  focused direct file reads were used instead. The unit-test generation skill's
  extra external bootstrap/telemetry was not run; repository-native tests and TDD
  remained within the authorized no-new-dependency/file scope.

## Remaining Work

Phase 2: confirm hosted ARM64/Intel runner and account prerequisites, then add
least-privilege native CI and the default-non-publishing signing-ready workflow.
Phase 3: separately authorize exact public version, credentials/environments,
documentation, signed tag, and GitHub/npm mutations after their prerequisites.

## Next Step

Review and commit/push Phase 1 only upon an explicit user request. Start Phase 2
only after its separate authorization and prerequisite checks. Re-verify any
temporary toolchain before reuse; the user/system installation was not changed.

## Do Not Change

Do not change Go 1.21 or dependencies, public protocol, descriptor, database,
evidence/evaluator schemas, prompts, history, model behavior, retry/fallback,
extension contents, daemon lifecycle, or user data. Do not create workflows,
tags, release assets, registry versions, signing state, or external publication
without the later phase's explicit authority.
