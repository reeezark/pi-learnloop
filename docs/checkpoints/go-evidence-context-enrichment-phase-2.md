---
id: go-evidence-context-enrichment-phase-2
plan: go-evidence-context-enrichment
phase: 2
status: current
updated: 2026-09-03
---

# Go Evidence Context Enrichment Phase 2

## Context

### Goal

Expose the exact retained enriched Go preview through additive authenticated
daemon routes and carry it through versioned bundle, evaluator-input, assessment
input, and immutable prompt contracts without changing v1, extension, database,
or dependency behavior.

### Current Phase

Phase 2 is complete. The active `go-evidence-context-enrichment` plan now points
to Phase 3 with `phase_status: awaiting_approval`. Phase 3 is not authorized.

Phase 1 was committed and pushed to `main` and `origin/main` as `477c1aa` before
Phase 2 began. The completed Phase 2 working tree has not been committed or
pushed.

## Completed

- Added strict authenticated `POST /v1/go-context-evidence-previews` and
  `POST /v1/pi-session-go-context-evidence-previews`. Their request shapes match
  the existing explicit Git selections, while their responses visibly include
  all context items, relations, build configuration, applied limits, analysis
  counts, completeness, omissions, and truncation.
- Preserved both existing preview routes and v1 JSON/model behavior. No optional
  Go-context field was added to an old response, and v1 evaluator JSON omits
  `go_context` exactly.
- Added a private daemon-owned evidence-contract marker to the existing
  five-minute, single-use continuation. The store deep-copies enriched values
  and accounts for all repository-derived context bytes under the existing
  8-entry/1-MiB cap.
- Added pure `evidence-bundle@2` construction and detached validation. The
  manifest hashes changed evidence, context evidence, relationships, build
  values, fixed limits, completeness, omissions, and truncation without a
  repository read, root, or Session identifier. A genuine import-only change
  can proceed using C-series evidence with no invented declaration item.
- Added `evaluator-input@2` and `evaluator-assessment-input@2`; both own their
  values, validate E- and C-series references, and preserve question-set and
  assessment-turn outputs at v1. The complete serialized v2 evaluator input is
  capped at the accepted 256 KiB. Explicit empty arrays remain arrays across
  bundle-to-input and assessment JSON round trips, including a genuine
  import-only input with no E-series item.
- Added and embedded immutable `evaluator-question-generation@2.0.0` and
  `evaluator-answer-assessment@2.0.0` prompts. Production Pi RPC adapters choose
  the v1 or v2 prompt from the validated schema version and otherwise preserve
  the existing fresh-process, no-tools isolation.
- Added strict runtime documentation schemas for the three v2 inputs, the
  supplemental `go-context-evidence@1.0.0` policy, and synthetic fixtures for
  import-only evidence, type-checked interface relations, partial/unavailable
  facts, instruction-like context, budget exhaustion, and Session isolation.
- Proved that the exact previewed result survives a working-tree mutation and
  flows through question generation and assessment without a reread. Completed
  history records only the v2 manifest and prompt provenance; it stores no
  source/context/input and needs no schema migration.
- Proved Pi Session ID isolation for successful preview, question, assessment,
  completion history, strict rejection, cancellation, expiration, and
  serialized error paths. The daemon has no request logging facility or request
  data logging calls, and Phase 2 added no logging path, so there is no log sink
  into which the identifier or enriched evidence can flow.

## Modified Files

Runtime implementation and tests:

- `internal/evidence/go_context.go`
- `internal/evidence/bundle_v2.go`
- `internal/evidence/bundle_v2_test.go`
- `internal/evaluator/contract.go`
- `internal/evaluator/contract_v2.go`
- `internal/evaluator/contract_v2_test.go`
- `internal/evaluator/assessment_contract.go`
- `internal/evaluator/evaluator.go`
- `internal/evaluator/evaluator_test.go`
- `internal/evaluator/pi_rpc.go`
- `internal/evaluator/pi_rpc_version_test.go`
- `internal/daemon/continuation.go`
- `internal/daemon/continuation_test.go`
- `internal/daemon/daemon.go`
- `internal/daemon/server.go`
- `internal/daemon/go_context_preview.go`
- `internal/daemon/go_context_preview_test.go`

Evaluator assets and documentation:

- `agent/README.md`
- `agent/prompts/README.md`
- `agent/prompts/assets.go`
- `agent/prompts/assets_test.go`
- `agent/prompts/evaluator-question-generation/v2.0.0.md`
- `agent/prompts/evaluator-answer-assessment/v2.0.0.md`
- `agent/policies/go-context-evidence.json`
- `agent/schemas/evidence-bundle-v2.schema.json`
- `agent/schemas/evaluator-input-v2.schema.json`
- `agent/schemas/evaluator-assessment-input-v2.schema.json`
- `agent/evals/README.md`
- `agent/evals/cases/go-context-*.json`

Stable and lifecycle documentation:

- `PROJECT.md`
- `plans/go-evidence-context-enrichment.md`
- `docs/decisions/ADR-0007-snapshot-consistent-go-context-evidence.md`
- `docs/checkpoints/go-evidence-context-enrichment-phase-1.md`
- `docs/checkpoints/go-evidence-context-enrichment-phase-2.md`

## Important Decisions

- The preview route, not a client-supplied field on question or assessment
  requests, selects v1 versus v2. This preserves the strict existing request
  contracts and keeps compatibility state daemon-owned.
- `BuildBundleV2` is a separate pure constructor rather than broadening v1.
  Its detached validator is reused at the evaluator boundary, so model input
  cannot silently diverge from the content-addressed preview result.
- The runtime `Input` representation is a private superset with an omitted
  `go_context` field for v1. This keeps v1 serialized JSON exact while avoiding
  a parallel evaluator execution stack.
- Question-set and assessment-turn outputs stay at schema v1 because their
  semantics, verdicts, ordering, and field shapes did not change. Only their
  accepted input reference set expands for validated v2 inputs.
- History needs no schema change: existing manifest and prompt provenance
  distinguish v2, while source, context, input, and Session provenance remain
  outside generic history responses.
- The six new eval fixtures retain observable prompt-injection defenses; the
  instruction-like Go-context case uses the broader `evidence_fidelity`
  category because the existing governance self-test intentionally removes all
  fixtures in the dedicated prompt-injection category.
- Continuation byte capacity uses the enriched result's repository-derived
  context total, so retained build strings, paths, identities, relations, and
  content all count toward the existing 1-MiB boundary.

## Tests / Verification

Passed on 2026-09-03:

- `go test -count=1 ./internal/evidence` with the writable temporary Go cache:
  167.621 seconds.
- `go test -count=1 ./internal/daemon` with permitted local IPv4 loopback:
  130.721 seconds; the final post-review run after exact context-byte accounting
  passed in 130.879 seconds.
- `go test -count=1 ./internal/evaluator ./agent/prompts`: evaluator 22.010
  seconds and prompts 0.959 seconds; the final evaluator run after import-only
  JSON round-trip coverage passed in 22.625 seconds.
- focused strict-route, cancellation, expiry, versioned-prompt, bundle, and v2
  contract tests after their final edits.
- full `go test -count=1 ./...` with writable temporary cache and local loopback:
  all packages passed; evidence 169.459 seconds and daemon 127.881 seconds.
- final post-checkpoint `go test -p=1 -count=1 ./...` with the same cache and
  loopback permissions: every package passed; daemon 132.243 seconds, evaluator
  21.927 seconds, evidence 165.634 seconds, and history 1.143 seconds.
- final post-review serial full run after the import-only JSON round-trip and
  complete context-byte accounting fixes also passed every package: daemon
  133.198 seconds, evaluator 23.717 seconds, evidence 167.624 seconds, and
  history 1.052 seconds.
- full `go test -race -count=1 ./...` under the same local conditions: all
  packages passed; evidence 169.967 seconds and daemon 132.126 seconds.
- final focused daemon race run covering continuation stores and Go-context
  continuation expiry/accounting: 2.617 seconds.
- `go vet ./...` and `GOFLAGS=-buildvcs=false go build ./...` with writable
  temporary cache.
- installed Go 1.21.13, `GOROOT` cleared, `CGO_ENABLED=0`, writable temporary
  cache, local loopback, and serial package scheduling:
  `go test -p=1 -count=1 ./...`; every package passed, including evidence in
  170.921 seconds and daemon in 129.098 seconds.
- `npm run typecheck`.
- `npm test`: 45/45 existing extension tests with permitted temporary IPv4
  loopback listeners.
- `npm pack --dry-run --json` with a writable temporary npm cache: six unchanged
  package entries, 18615-byte archive, 74457-byte unpacked content.
- `scripts/test-agent-infra.sh`.
- `scripts/validate-agent-infra.sh`.

One expected environment-sensitive retry is retained in the review record: the
first final parallel full-suite rerun passed every package except one existing
Pi assessment-constructor preflight, whose fake executable exceeded the fixed
two-second startup deadline under concurrent package load. The unchanged full
evaluator package passed immediately afterward, and the final serial full suite
passed every package. The earlier parallel full and full race suites had also
passed.

Final Git whitespace, status, stat, and complete-diff review are performed after
this checkpoint and lifecycle metadata are finalized.

No verification invoked a live Pi process, model/provider, external network,
production source checkout/archive, or production history database.

## Known Issues

- The Pi extension does not yet request or render the enriched routes. Current
  `/learn` users remain on the unchanged v1 path until Phase 3.
- Standard-library and external type facts remain intentionally unavailable;
  partial context continues to report that omission rather than infer facts.
- Version-specific replacements and workspaces that do not list the changed
  module remain intentionally unavailable as decided in Phase 1.
- The daemon has no request log sink. If logging is added later, ADR-0006 and
  ADR-0007 require a separate privacy review and explicit Session/source
  redaction tests.

## Remaining Work

- Phase 3 only: update the Pi extension to call the additive routes and visibly
  render and confirm every enriched evidence category and incomplete state
  before provider invocation.

## Next Step

Review the completed Phase 2 working tree. Do not commit or push it unless the
user explicitly requests that action, and do not begin Phase 3 until the user
explicitly authorizes `go-evidence-context-enrichment` Phase 3.

## Do Not Change

- Do not modify extension behavior before Phase 3 authorization.
- Do not alter v1 routes, JSON, bundles, evaluator inputs, released prompts, or
  output protocols.
- Do not add a dependency, Go upgrade, database migration, external/GOROOT
  source, network access, source cache, background index, Session content, or
  model-visible Session identifier.
