---
id: release-ready-macos-distribution-phase-2
plan: release-ready-macos-distribution
phase: 2
status: current
updated: 2026-09-04
---

# Release-Ready macOS Distribution Phase 2: Verification Checkpoint

## Context

### Goal

Install and verify read-only native CI and an explicit reviewed-commit manual
bootstrap without changing product behavior or granting publication authority.

### Current Phase

Phase 1 was committed/pushed to main as
`d93dfa5805a7cc172ba2caecfc16d2356422a16c`
(`feat: add local macOS release artifacts`). The user authorized Phase 2 and then
confirmed native ARM64/Intel runner availability and the narrowly scoped
bootstrap plan/ADR amendment on 2026-09-04.

Phase 2 is active/in-progress. Its implementation passed local verification and
awaits the authorized initial commit/push; actual hosted CI and manual bootstrap have
not run yet. Do not infer completion from implemented workflow files.

## Completed

- Recovered governance, project, plan, checkpoints, applicable ADRs, manifests,
  command/daemon behavior, and all release scripts from the repository.
- Reverified remote main at the Phase 1 commit, with no tags. GitHub metadata
  confirms a public repository and authenticated maintainer repository access;
  this does not prove a successful runner execution.
- Approved/authorized and active/in-progress metadata validations passed.
- Updated the plan and ADR-0009 with the accepted read-only bootstrap exception.
  Successful signed-tag verification and privileged workflow implementation move
  to separately authorized Phase 3; every stable-publication gate remains.
- Added ordinary push/PR CI with exact event-commit checkout and a two-native-
  runner matrix. Both lanes run governance, extension/package, Go 1.21 pure-Go
  compatibility, Go 1.27.1 test/race/vet/build, and release-artifact self-tests.
- Added a manual main-only bootstrap accepting one exact reviewed commit equal
  to the event SHA and calling that same commit's CI workflow. It has no
  publication mode, release environment, signing job, secret, OIDC, write access,
  persisted checkout credential, cache upload, or artifact upload.
- Extended only the existing release self-test: optional exact clean commit
  validation and both binaries' clean embedded provenance, plus native packaged
  foreground-daemon smoke in a child-only temporary home. Fake Pi accepts only
  two version preflights; status/authentication and SIGTERM cleanup are checked.
  The existing artifact builder/verifier interfaces and implementations are unchanged.

## Modified Files

- `.github/workflows/ci.yml`
- `.github/workflows/release.yml`
- `scripts/test-release-artifacts.sh`
- `PROJECT.md`
- `plans/release-ready-macos-distribution.md`
- `docs/decisions/ADR-0009-release-ready-macos-distribution.md`
- `docs/checkpoints/release-ready-macos-distribution-phase-1.md` (superseded metadata)
- This Phase 2 checkpoint

All are Phase 2 allowed files. No Go, extension, dependency manifest, package
version/content, runtime protocol, history schema, or user data changed.

## Important Decisions

codebase-design keeps artifact policy in the existing builder/verifier modules;
the manual workflow reuses CI rather than duplicating its native verification.
Native smoke tests cross the actual packaged executable interface, without a
production adapter or test-only daemon flag.

Reviewed official action release notes, tag-to-commit identities, README, and
action inputs on 2026-09-04:

- checkout v7.0.1: `3d3c42e5aac5ba805825da76410c181273ba90b1`
- setup-node v7.0.0: `820762786026740c76f36085b0efc47a31fe5020`
- setup-go v7.0.0: `b7ad1dad31e06c5925ef5d2fc7ad053ef454303e`

The jobs pin Node 22.19.0 (the declared floor), Go 1.21.13 for source
compatibility, and Go 1.27.1 for release verification. Actions' internal Node 24
runtime is separate from the project Node runtime. Explicit runner labels are
`macos-15` ARM64 and `macos-15-intel`; neither proves execution on macOS 13.

## Tests / Verification

Passed so far:

- Release self-tests twice with exact Go 1.27.1 and the existing local Node
  v26.0.0: repeat-built binaries, static archive/build/dependency checks, all
  previous refusal/tamper/cleanup cases, new expected-commit refusal cases, and
  native ARM64 foreground smoke. Local uncommitted candidates honestly retain
  `vcs.modified=true`; exact clean positive execution awaits the installed commit.
- Ruby/Psych YAML parsing and bash syntax validation for every workflow run
  step; inspected triggers, job dependencies, read-only permissions, exact pins,
  disabled caches, non-persisted checkout credentials, and native matrix.
- Ten executions of the actual bootstrap gate with synthetic environment values:
  correct main/SHA passes; wrong event/ref/tag/SHA, empty/uppercase/newline input,
  and shell-substitution/injection-like input fail without interpreting the input.
- Extension typecheck, 58/58 provider-free tests, and isolated-cache
  `npm pack --dry-run --ignore-scripts --json`: exactly the six existing paths,
  no bundled dependency, no tarball or publication.
- Both Agent-infrastructure scripts and Git whitespace validation.
- Dependency-manifest diff is empty.
- Go 1.21.13 with `CGO_ENABLED=0 go test -count=1 ./...`: all seven packages pass.
- Go 1.27.1 `go test -count=1 ./...`, `go test -race -count=1 ./...`,
  `go vet ./...`, and `CGO_ENABLED=0 go build` to a temporary output: all pass.

Initial broad Go attempts inherited an unrelated local Go 1.26.4 `GOROOT`,
causing compiler-version mismatch before tests. Successful reruns used
`env -u GOROOT`, exact toolchain paths, isolated caches, and automatic switching
disabled. This changes only test-process environment, not system configuration.

No hosted CI/manual run, native Intel smoke, signed-tag, signing/notarization,
or publication verification has yet passed. No live model was called.

## Known Issues

- CodeGraph tools are unavailable in this session; focused source reads were
  used without changing the existing index.
- The GitHub connector exposes run inspection but no manual-dispatch operation;
  the available in-app browser is signed out. Manual dispatch may require the
  maintainer's authenticated browser action after installation. Do not extract
  credentials or add a privileged workflow trigger as a workaround.
- The first browser navigation timed out; recovery reached the public Actions
  page and confirmed the signed-out state. No account setting was changed.
- Exact release SemVer, protected-environment approver, trusted signer, Apple
  credentials, and npm ownership/trusted publisher remain Phase 3 prerequisites.

## Remaining Work

Finish the full allowed-file diff review; commit/push
the reviewed read-only implementation under the explicit bootstrap authorization.
Obtain actual ordinary CI and manual-bootstrap native execution evidence at
that exact SHA, including runner images/OS, binary provenance, run/job URLs,
and zero uploaded artifacts. Failed or unavailable execution blocks completion.

## Next Step

Continue Phase 2 only. After all agreed verification succeeds, record the
results and advance to Phase 3 awaiting explicit authorization. If manual
dispatch needs authenticated user action, record a blocked checkpoint with the
exact committed SHA and workflow URL rather than claiming Phase 2 complete.

## Do Not Change

No business code, Go 1.21 baseline, dependency, protocol, schema, model/evidence
behavior, daemon lifecycle, package version/contents, or user data. No tag,
release, npm publication, environment/secret mutation, signing/notarization,
OIDC, or artifact/cache upload. Phase 3 is not authorized.
