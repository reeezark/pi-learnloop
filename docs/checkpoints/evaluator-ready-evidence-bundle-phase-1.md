---
id: evaluator-ready-evidence-bundle-phase-1
plan: evaluator-ready-evidence-bundle
phase: 1
status: current
updated: 2026-08-31
---

# Evaluator-Ready Evidence Bundle Phase 1 Handoff

## Change Summary

Phase 1 adds a pure, in-memory transformation from `evidence.Result` to an evaluator-ready evidence bundle. The bundle preserves the effective preview limits, assigns stable evidence references, records content hashes and byte counts, and derives its bundle ID from a canonical metadata-only manifest. Validation is fail-closed. This phase intentionally does not connect the bundle to product output, evaluator transport, persistence, protocol selection, or adapter discovery.

## Goal

Transform one already selected and bounded `internal/evidence.Result` into a deterministic, citation-ready in-memory EvidenceBundle without reading additional repository content, changing the daemon protocol, calling a model, or persisting raw source.

## Current Phase

Phase 1 is implemented and verified. This is the only phase in the `evaluator-ready-evidence-bundle` plan. The internal bundle is intentionally not reachable from the daemon, extension, Pi SDK/RPC, filesystem, persistence, or any model entry point.

## Task Contract

Goal:

- preserve the exact preview budget and construct a deterministic evidence-sharing value from only the bytes already represented in the preview.

Scope and allowed files:

- `internal/evidence/evidence.go`
- `internal/evidence/evidence_test.go`
- `internal/evidence/bundle.go`
- `internal/evidence/bundle_test.go`
- `PROJECT.md`
- `plans/evaluator-ready-evidence-bundle.md`
- `docs/checkpoints/evaluator-ready-evidence-bundle-phase-1.md`

Forbidden changes:

- daemon or extension behavior and the accepted `/v1` protocol;
- evaluator development assets, production prompts, model calls, or Pi adapters;
- dependencies, `go.mod`, `go.sum`, npm manifests, persistence, jobs, SSE, telemetry, or automatic behavior;
- commits, tags, publication, or release automation.

Acceptance and verification are defined by the completed plan and recorded below.

## Handoff Snapshot

- Handoff date: 2026-08-31.
- Repository: `https://github.com/reeezark/pi-learnloop`.
- Local checkout: `/Users/bytedance/workspace/pi-learnloop`.
- Branch: `main`, tracking `origin/main`.
- Baseline commit remains `9ced24de5347c0a8ed3f8ada5aaaac6138a7a61e` (`feat: add local evidence preview workflow`).
- A pre-existing uncommitted update to `docs/checkpoints/changeset-evidence-preview-phase-3.md` was present before this task and was preserved without modification by this phase.
- No commit or remote operation was performed; the Phase 1 changes remain in the working tree.

## Resume Order for the Next Agent

1. Inspect the working tree and preserve the pre-existing change in `docs/checkpoints/changeset-evidence-preview-phase-3.md`; it is not part of this phase.
2. Read `AGENTS.md`, `PROJECT.md`, `plans/evaluator-ready-evidence-bundle.md`, this checkpoint, and ADR-0001/ADR-0002 before changing the design.
3. Review `internal/evidence/evidence.go`, `internal/evidence/bundle.go`, and their tests to understand the phase boundary and invariants.
4. Confirm that the current plan is complete and that `BuildBundle` remains deliberately unconnected to product paths.
5. Before starting the next slice, choose a concrete integration target and write a new plan/task contract; do not extend the completed Phase 1 plan in place.
6. If the next slice involves an evaluator, inspect the current Pi interfaces and `agent/README.md`, `agent/policy/`, `agent/schemas/`, and `agent/evals/`. Do not infer a transport protocol, runtime adapter, model, or dependency without repository evidence.

## Completed

- Added `AppliedLimits` to `internal/evidence.Result`; `Preview` now retains the exact validated limits used for files, declarations, and aggregate excerpt bytes.
- Added the pure `BuildBundle(Result)` interface. It has no repository, context, filesystem, Git, process, network, credential, persistence, or model input and therefore cannot expand evidence after preview.
- Added internal bundle format version 1 with a content-addressed `eb1-<manifest-sha256>` identifier.
- Added deterministic `E001`-style evidence references in existing preview file/declaration order.
- Added `code` and `_test.go`-derived `test` item classification using only the already previewed repository-relative path.
- Preserved resolved revisions, applied limits, file status and changed ranges, declaration kind/identity/location, declaration changed ranges, file omissions, per-excerpt truncation, and aggregate truncation.
- Added exact UTF-8 content byte counts and SHA-256 hashes for every non-empty evidence item.
- Added a canonical metadata-only manifest whose hash covers format version, revisions, budget, counts, file coverage, item locators/content hashes, omissions, and truncation. Raw excerpt content and the absolute repository root are excluded from the manifest.
- Added separate declaration and evidence-item counts so excerpts fully removed by the preview byte budget remain observable.
- Added fail-closed validation for missing/non-positive budgets, limit overflow, missing revisions, unsafe or duplicated paths, invalid enums, malformed or unordered ranges, inconsistent omissions/truncation, invalid UTF-8, deleted-file declarations, and unexplained empty excerpts.
- Added a stable `insufficient_evidence` path when no non-empty excerpt can be supplied, plus `invalid_result` and `unknown` internal error codes.
- Updated `PROJECT.md` to record the implemented but unconnected bundle seam and preserve the explicit-continuation target flow.

## Module Interface and Boundaries

The implemented deep module interface is:

```go
func BuildBundle(result Result) (Bundle, error)
```

The builder receives a value already produced and bounded by `Preview`. It never reopens the repository. The returned bundle contains the selected excerpts in memory because a later evaluator will need those bytes, but this phase neither serializes nor persists them and exposes no product route.

The absolute `RepositoryRoot` is intentionally absent from the bundle and its canonical manifest. Base/head revisions and repository-relative paths remain available for citations and provenance.

The bundle Go structs are an internal domain value, not a runtime JSON protocol, persisted schema, released evaluator input schema, or reuse of the development-only schemas under `agent/`.

## Modified Files

Phase-owned files:

- `internal/evidence/evidence.go`
- `internal/evidence/evidence_test.go`
- `internal/evidence/bundle.go`
- `internal/evidence/bundle_test.go`
- `PROJECT.md`
- `plans/evaluator-ready-evidence-bundle.md`
- `docs/checkpoints/evaluator-ready-evidence-bundle-phase-1.md`

Pre-existing uncommitted file preserved separately:

- `docs/checkpoints/changeset-evidence-preview-phase-3.md`

No file under `cmd/`, `internal/daemon/`, `extensions/`, `tests/extension/`, or `agent/` was changed. No manifest, dependency, ADR, generated file, or Git history was changed.

## Tests / Verification

Environment:

- Go: `go1.21.13 darwin/arm64`.
- Go build cache was redirected to `/private/tmp/pi-learnloop-go-cache` because the managed sandbox cannot write the user's default Go cache. This changes no source or test behavior.

Passed:

- focused `go test ./internal/evidence`; all existing preview tests and new bundle tests pass.
- `go vet ./...` with the default Go 1.21.13 toolchain.
- established host workaround `CGO_ENABLED=0 go test -count=1 ./...` when rerun outside the sandbox so daemon tests could bind `127.0.0.1`.
- established host workaround `go test -race -count=1 -tags netgo ./...` outside the sandbox.
- established host workaround `go vet -tags netgo ./...`.
- `npm run typecheck`.
- `npm test` outside the sandbox; all 12 extension tests pass.
- `scripts/validate-agent-infra.sh`.
- all positive and negative cases in `scripts/test-agent-infra.sh`.

Canonical host failures, recorded separately:

- `go test -count=1 ./...` fails for `cmd/pi-learnloop` and `internal/daemon` when the default Go 1.21.13 linker aborts network-enabled test binaries with `missing LC_UUID`; `internal/evidence` passes in the same run.
- `go test -race -count=1 ./...` fails for the same `missing LC_UUID` host issue; `internal/evidence` passes in the same run.

Sandbox-only observations, not product failures:

- the first sandboxed pure-Go full-suite run could not bind `127.0.0.1` and therefore failed daemon startup tests; the exact command passed outside the sandbox.
- the first sandboxed `npm test` run passed 8 tests and failed 4 loopback tests with `listen EPERM`; the exact command passed all 12 tests outside the sandbox.

Final review passed:

- `git diff --check` reported no tracked whitespace errors; no-index whitespace checks for each new file reported no errors.
- `git status --short --branch` shows only the seven Phase-owned files plus the preserved pre-existing Phase 3 checkpoint update.
- `git diff` and direct review of all new files confirmed the allowed-file scope.
- `BuildBundle` references occur only in `internal/evidence`, its tests, and task documentation; no daemon, extension, product protocol, or evaluator adapter can call it.
- diffs for `go.mod`, `go.sum`, npm manifests, `cmd/`, `internal/daemon/`, `extensions/`, `tests/extension/`, and `agent/` are empty.

## Important Decisions

- The bundle is projected from the existing bounded `Result`; it does not accept selection or repository inputs. This makes the interface itself enforce the no-re-read privacy property.
- The retained `AppliedLimits` are authoritative. A later caller cannot substitute a different budget while building.
- Content hashes, rather than raw content, enter the canonical manifest. The bundle still carries one copy of the selected excerpt in memory.
- Repository root is excluded from both the evaluator-facing value and its manifest. Relative paths and resolved revision identifiers are retained.
- Empty excerpts do not become evidence items. Declaration count, evidence count, and truncation metadata make a preview-budget removal visible; an entirely empty bundle fails as insufficient evidence.
- `_test.go` classification is syntactic and uses no additional source. Type/dependency enrichment remains excluded until its extra bytes can be included in a user-visible preview.
- No runtime serialization or ADR was introduced. A future product protocol, persisted format, evaluator adapter, or data-sharing expansion requires its own compatibility review and authorization.

## Known Issues / Remaining Boundary

- No current product entry point calls `BuildBundle`; the user cannot start an evaluation yet.
- The module cannot itself prove that the user clicked an explicit continuation action. Future orchestration must enforce preview-before-continue before invoking it.
- No production evaluator input/output schema, prompt, Pi SDK/RPC adapter, model/provider selection, question workflow, scoring, or result label exists.
- No type/dependency context, base-side deletion source, imports, callers, or unseen tests are included. The bundle faithfully reports only current preview evidence and its omissions.
- No bundle, manifest, or run is persisted. SQLite and durable execution remain unimplemented.
- The default Go 1.21.13 `missing LC_UUID` linker issue remains unchanged; pure-Go/netgo verification passes.

## Next Step

Choose and investigate the next explicit slice. The closest product step is an explicit post-preview continuation and isolated evaluator adapter contract, but it must define its runtime schema, prompt/output behavior, full tool isolation, and Pi SDK/RPC choice in a new high-risk plan before implementation.

## Do Not Change

- Do not connect the bundle to HTTP, Pi, persistence, or a model without an approved plan and explicit phase authorization.
- Do not add evidence bytes that were not represented in the user's preview.
- Do not treat the internal Go structs or `agent/` development fixtures as a released runtime protocol.
- Do not weaken ADR-0002 or make `/learn` automatic.
- Do not add dependencies, persist raw source, expose credentials, or upload telemetry without separately approved scope.
