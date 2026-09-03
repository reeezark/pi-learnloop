---
id: go-evidence-context-enrichment-phase-1
plan: go-evidence-context-enrichment
phase: 1
status: superseded
updated: 2026-09-03
---

# Go Evidence Context Enrichment Phase 1

## Context

### Goal

Implement and verify the accepted internal snapshot-consistent, bounded Go
context model without exposing it through daemon routes, evaluator schemas,
prompts, the Pi extension, history, or other user-visible behavior.

### Current Phase

Phase 1 is complete. The active `go-evidence-context-enrichment` plan now points
to Phase 2 with `phase_status: awaiting_approval`. Phase 2 is not authorized.

The implementation started from clean `main` at
`d4ce92319019f42ef85674c82fa7de80fccf3193`, equal to `origin/main`. The two
draft design documents were untracked at that baseline. The completed Phase 1
working tree has not been committed or pushed.

## Completed

- Recorded the user's acceptance of ADR-0007 and explicit authorization for
  Phase 1 plus direct `golang.org/x/mod v0.19.0` before implementation.
- Added exactly that direct dependency. `go 1.21`, `go.sum`, and every other
  selected module version remain unchanged.
- Kept `internal/evidence.Preview` as the deep module seam and added an opt-in
  internal Go-context mode. The zero-value mode produces the unchanged v1
  result; the v1 bundle builder rejects enriched results pending Phase 2.
- Added private commit and working-tree snapshot adapters. Commit reads use
  streamed bounded `git ls-tree -z`, verified object sizes, and bounded
  `git cat-file`; working-tree reads enumerate tracked plus nonignored untracked
  files, require contained regular files, detect read-time change, and reject
  outside symlinks. Both paths honor cancellation and create no source copy.
  Every context-mode Git read disables promised-object lazy fetch, terminal
  prompting, and optional locks; the context-disabled v1 path remains unchanged.
- Added one shared in-memory analyzer using `x/mod/modfile` plus
  `x/mod/module` path validation, a custom `go/build.Context`, `go/parser`, and
  `go/types`. It reads only the selected snapshot, changed packages, and direct
  repository-local imports; it invokes no Go package driver and reads no vendor,
  module-cache, GOROOT, compiled-export, or repository-external source.
- Fixed the build policy to runtime GOOS/GOARCH/tool/release tags, CGO disabled,
  no custom tags, a required selected-module `go` directive, contained
  workspaces/local replacements, and test files only for selected `_test.go`
  changes. Missing or newer language versions fail closed instead of being
  guessed.
- Made workspace replacement precedence explicit. An outside workspace override
  removes a prior local mapping and retains only the replaced module identity.
  Version-specific replacements and workspaces that do not list the changed
  module fail closed as `unsupported_module_layout`.
- Added deterministic `changed_import` and `context_declaration` items,
  `imports`, `references`, and `implements` relations, fixed build/limit data,
  complete/partial/unavailable states, the accepted closed omission taxonomy,
  and exact aggregate truncation. Type-checked relations require complete local
  packages; syntactic import relations remain separately labeled.
- Enforced every accepted input and output limit before or during work. Input
  overrun makes context unavailable; deterministic output overrun keeps only a
  bounded prefix and reports omitted files/items/relations/bytes.
- Added real temporary-repository and narrow private-adapter coverage for commit
  versus working-tree parity, historical isolation, additions/deletions,
  import-only evidence, direct references/implementations, module layouts,
  workspaces/replacements, build constraints, tests, CGO, external imports,
  malformed packages, cycles, unsupported versions, symlinks, vendor exclusion,
  cancellation, limits, ordering, serialization size, and v1 compatibility.

## Modified Files

Implementation and tests:

- `go.mod`
- `internal/evidence/evidence.go`
- `internal/evidence/bundle.go`
- `internal/evidence/snapshot.go`
- `internal/evidence/snapshot_test.go`
- `internal/evidence/go_context.go`
- `internal/evidence/go_context_test.go`
- `internal/evidence/go_context_internal_test.go`

Stable and lifecycle documentation:

- `PROJECT.md`
- `plans/go-evidence-context-enrichment.md`
- `docs/decisions/ADR-0007-snapshot-consistent-go-context-evidence.md`
- `docs/checkpoints/go-evidence-context-enrichment-phase-1.md`

## Important Decisions

- The `snapshot` interface is private and has exactly two real adapters. Callers
  cannot coordinate Git enumeration, module parsing, type checking, budgets, or
  omissions.
- Phase 1 exposes no HTTP or JSON contract. The internal result shape is ready
  for Phase 2 to map into additive strict routes and versioned bundle/input
  schemas without changing v1.
- Standard-library and external imports provide bounded syntax identity only.
  Their source and compiled type facts are never consulted. Missing facts make
  affected packages partial and cannot produce a type-checked relation.
- Workspace and replacement paths are interpreted with snapshot-relative slash
  semantics. Absolute and escaping paths are never echoed or followed.
- The accepted fixed values are encoded once in the evidence module and covered
  by an exact equality test. Later changes require compatibility review.
- Input limits are fail-closed because a discovered prefix cannot prove complete
  coverage. Output limits run only after stable ordering and retain exact
  aggregate omission counters.

## Tests / Verification

Passed before this checkpoint was finalized:

- `go test -count=1 ./internal/evidence` using the documented writable temporary
  Go build cache: final post-review run 165.706 seconds.
- `go test -run '^$' -bench '^BenchmarkPreviewGoContextAtOutputBudget$'
  -benchtime=3x -benchmem ./internal/evidence`: 107226805 ns/op, 517989 B/op,
  3337 allocs/op on darwin/arm64 Apple M4 Pro.
- `/usr/bin/time -p go test -count=1 -run
  '^TestPreviewGoContextInputLimitsFailClosed$' ./internal/evidence`: 24.28
  seconds real, 4.18 user, 5.12 sys, below the fixed 30-second analysis
  deadline for the complete adversarial limit fixture group.
- standard full `go test -count=1 ./...` with a writable temporary Go build cache
  and permitted loopback tests.
- installed Go 1.21.13 with `GOROOT` cleared, `CGO_ENABLED=0`, a writable
  temporary cache, permitted loopback, and serial package scheduling:
  `go test -p=1 -count=1 ./...`; every package passed, including the final
  evidence run in 162.364 seconds.
- `go test -race -count=1 ./...` with a writable temporary Go build cache and
  permitted loopback tests.
- `go vet ./...` with a writable temporary Go build cache.
- `go build ./...`; a second `GOFLAGS=-buildvcs=false go build ./...` run was
  clean. The first successful build emitted a sandbox-only inability to update a
  read-only module stat-cache entry.
- `npm run typecheck`.
- `npm test`: 45/45 tests after permitting temporary IPv4 loopback listeners.
- `npm pack --dry-run --json`: six expected package entries, 18615-byte archive,
  74457-byte unpacked content.
- `go list -mod=readonly -m all`: `x/mod` remains v0.19.0, `x/tools` remains
  indirect v0.23.0, and all other selected versions match the baseline.
- `go.sum` has no diff.
- `scripts/test-agent-infra.sh`.
- `scripts/validate-agent-infra.sh`.
- `git diff --check`.

Expected environment-only first attempts were retained in the review record:

- the default Go build cache under `~/Library/Caches` is read-only in the
  workspace sandbox, so the supported temporary-cache path was used;
- sandboxed loopback binding returned `EPERM` for 18 extension client cases and
  daemon integration cases; both suites passed after loopback permission;
- the first full race run timed out one existing fake-Pi evaluator startup under
  concurrent package load; the isolated evaluator race suite passed immediately,
  and a second unchanged full race run passed every package;
- the first Go 1.21 invocation inherited mise's Go 1.26 `GOROOT` and therefore
  rejected mismatched standard-library tools before compiling the repository;
  clearing `GOROOT` selected the correct Homebrew 1.21 tree. A parallel-package
  1.21 run then passed every other package but reproduced the existing fake-Pi
  startup timeout; its evaluator suite passed alone, and the serial full run
  passed every package;
- the default npm cache was read-only during the final packaging check, so the
  unchanged dry run passed with a writable temporary cache;
- `/usr/bin/time -l` completed its selected tests but returned 1 because sandbox
  `sysctl kern.clockrate` was denied, so the successful portable `-p` timing and
  committed allocation benchmark are the recorded measurements.

No verification invoked Pi, contacted a provider or network service, created a
production source checkout/archive, or wrote a production history database.

Final Git status, stat, and complete-diff review are performed after this
checkpoint is finalized.

## Known Issues

- No enriched daemon route, evidence-bundle@2, evaluator-input@2, prompt v2,
  continuation accounting, or extension preview exists. Current product
  behavior remains v1.
- Version-specific replacements are intentionally unsupported by the initial
  loader because applying them requires additional selected-version reasoning.
- A discovered in-repository workspace that does not list the changed module is
  unavailable rather than silently ignored.
- Standard-library and external type facts are intentionally unavailable, so
  otherwise valid packages that depend on them can be partial.
- Deletion-side reconstruction remains outside scope; deleted-file and
  deletion-only-hunk omissions continue to describe that gap.

## Remaining Work

- Phase 2: add the two authenticated enriched-preview routes, exact retained
  context accounting, evidence-bundle@2, evaluator input/assessment input v2,
  immutable prompt v2 assets, and source-free history/provenance proofs.
- Phase 3: render and explicitly confirm every enriched category and
  completeness state in the Pi extension before provider invocation.

## Next Step

Review and commit the completed Phase 1 working tree if requested. Do not begin
Phase 2 until the user explicitly authorizes `go-evidence-context-enrichment`
Phase 2.

## Do Not Change

- Do not add or modify daemon routes, evaluator contracts, prompts, extension
  behavior, persistence, or v1 model-visible content before Phase 2 approval.
- Do not let external, vendor, module-cache, GOROOT, compiled-export, or
  repository-external source enter evidence.
- Do not weaken snapshot consistency, fixed limits, cancellation, explicit
  omission states, or type-checked relation requirements.
- Do not add `go/packages`, `x/tools`, another dependency, another selected
  module version, a Go baseline upgrade, network access, source cache, checkout,
  archive, background index, hook, marker, Session content, or Session ID.
