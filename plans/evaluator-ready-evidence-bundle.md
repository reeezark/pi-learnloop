---
id: evaluator-ready-evidence-bundle
status: complete
risk: high
current_phase: 1
phase_status: complete
updated: 2026-08-31
---

# Evaluator-Ready Evidence Bundle

## 1. Goal

Create the smallest evaluator-ready evidence boundary after the completed changeset preview: transform one already selected and bounded `internal/evidence.Result` into a deterministic, citation-ready in-memory EvidenceBundle without reading any additional repository content, calling a model, changing the daemon protocol, or persisting raw source.

The bundle must make its evidence budget, content volume, omissions, truncation, and integrity manifest explicit so a later isolated evaluator adapter can receive exactly the bytes the user previewed and cite stable evidence references.

## 2. Background

The completed `changeset-evidence-preview` plan lets a trusted Pi user manually select a Git changeset and inspect changed Go files, mapped declarations, approximate excerpt bytes, omissions, and truncation. The daemon applies fixed v1 limits before the TypeScript extension renders that preview.

The evaluator development contract requires selected-bundle-only evidence, a mandatory budget, user preview before evaluation, untrusted-content handling, structured evidence references, and a fail-closed path when evidence is insufficient. The development run-record schema already reserves an `evidence_bundle_id`, an `evidence_manifest_sha256`, a file count, and approximate bytes, but it explicitly is not a runtime product schema.

No runtime EvidenceBundle type, production evaluator prompt, evaluator adapter, or question/result schema exists. This task introduces only the internal bundle boundary required before those later slices can be designed safely.

## 3. Current Behavior

- `internal/evidence.Preview` accepts an explicit Git selection and positive limits, reads only changed Go source, and returns deterministic files, declarations, excerpts, omissions, and truncation.
- The returned `Result` does not retain the limits that produced it.
- Declarations have repository-relative paths through their containing file, source locations, changed line ranges, identities, bounded UTF-8 excerpts, and per-excerpt truncation state.
- The daemon applies 20-file, 100-declaration, and 128-KiB excerpt caps and exposes the result through authenticated `/v1/evidence-previews`.
- The extension renders file paths, symbols, aggregate excerpt bytes, and truncation. It does not ask the user to continue into evaluation.
- `agent/` contains development-only policy, schemas, and synthetic failure fixtures. It defines no production bundle JSON, prompt, model, or evaluator runtime.
- Pi 0.84.3 exposes both an SDK session API and JSONL RPC mode. Its SDK can suppress all tools, but selecting and enforcing the future evaluator adapter is outside this task.

## 4. Relevant Call Chain

Implemented preview flow:

```text
manual /learn selection
-> authenticated daemon preview request
-> internal/evidence.Preview with explicit limits
-> bounded Result
-> daemon v1 response
-> user-visible preview
```

Proposed internal bundle seam:

```text
already bounded internal/evidence.Result
-> validate retained budget and structural invariants
-> project only existing preview excerpts and metadata
-> assign deterministic evidence references
-> hash item content and a canonical manifest
-> in-memory EvidenceBundle
```

This task does not connect the proposed seam to the daemon, extension, Pi SDK/RPC, persistence, or a model. A later explicitly authorized orchestration slice must require user continuation after the preview before calling the bundle builder or evaluator.

## 5. Relevant Files

- `AGENTS.md`: lifecycle, high-risk authorization, scope, and verification rules.
- `PROJECT.md`: evaluator isolation, selected-evidence-only, preview, privacy, and target-flow constraints.
- `docs/checkpoints/changeset-evidence-preview-phase-3.md`: completed baseline and handoff constraints.
- `docs/decisions/ADR-0001-agent-development-lifecycle.md`: phase authorization rules.
- `docs/decisions/ADR-0002-local-daemon-protocol-security.md`: compatibility-sensitive v1 preview protocol that this task must not change.
- `internal/evidence/evidence.go`: current `Result`, limits, ordering, excerpts, omissions, and truncation.
- `internal/evidence/evidence_test.go`: real-Git behavior and evidence-limit coverage.
- `internal/daemon/server.go`: confirms that v1 limits are currently applied before preview serialization.
- `extensions/lib/learn-command.ts`: confirms exactly what the user currently previews.
- `agent/README.md`, `agent/policies/evaluator-capabilities.json`, and `agent/prompts/README.md`: evaluator capability and evidence invariants.
- `agent/schemas/run-record.schema.json`: development-only names for future bundle provenance, not a runtime schema to reuse.
- `agent/evals/`: synthetic citation, insufficiency, prompt-injection, and structured-output failure cases.
- Pi 0.84.3 local declarations under `node_modules/@earendil-works/pi-coding-agent`: current SDK and RPC capabilities investigated only to keep this bundle independent from an adapter choice.

## 6. Scope

- Retain the exact positive `Limits` applied by `Preview` on its returned `Result`.
- Add an internal, versioned Go EvidenceBundle model and a pure builder from one `Result`.
- Include only declaration excerpts and metadata already present in that result; perform no filesystem, Git, process, or network access while building.
- Produce deterministic evidence references, item content hashes, exact UTF-8 content byte counts, a canonical manifest hash, and a content-addressed bundle identifier.
- Distinguish evidence from `_test.go` paths as test evidence using only the already previewed path.
- Preserve selection revisions, declaration identity/location, changed line ranges, excerpt truncation, file omissions, and aggregate truncation in the manifest.
- Exclude the absolute repository root and any unselected source from evaluator-facing bundle data.
- Fail closed for a missing/invalid retained budget, structurally invalid preview data, limit overflow, duplicate references, or a result with no non-empty evidence content.
- Add focused deterministic and adversarial tests for the bundle boundary.
- Update stable project documentation and record a resumable checkpoint when the phase completes.

## 7. Out of Scope

- Reading imports, package context, dependencies, base-side deletions, callers, tests outside the selected preview, or any other repository content not included in the current preview.
- `go/types`, `go/packages`, a new Go module dependency, `go.sum`, or invoking the Go toolchain at runtime.
- A daemon endpoint, `/v2`, changes to `/v1`, new JSON fields, SSE, or extension behavior.
- User continuation UI, answer collection, questions, follow-ups, scoring, assessment labels, or learning records.
- A production prompt, runtime evaluator input/output schema, Pi RPC/SDK adapter, model/provider selection, credentials, or model calls.
- SQLite, durable jobs, persistence, retries, leases, event cursors, telemetry, or source logging.
- Editing released evaluator development assets or treating their fixture schemas as product protocols.
- Automatic `/learn` behavior, Session indexing/association, package publication, dependency changes, or release automation.

Type/dependency enrichment is deliberately excluded because the current user preview does not disclose or account for those additional bytes. It requires a later plan that first expands the preview and reviews the new data-sharing behavior.

## 8. Proposed Changes

### 8.1 Retain the applied preview budget

Add `AppliedLimits Limits` to `internal/evidence.Result` and set it from the validated request before analysis begins. Existing preview semantics and daemon JSON remain unchanged. Tests will prove that direct callers receive the authoritative budget that bounded the result.

The bundle builder will trust neither zero values nor caller-supplied replacement limits. It validates the retained limits and verifies file, declaration, and aggregate excerpt counts do not exceed them.

### 8.2 Add a pure, versioned bundle builder

Add `internal/evidence/bundle.go` with a narrow constructor similar to:

```go
func BuildBundle(result Result) (Bundle, error)
```

The constructor must use only its value argument. It must not accept a repository path or context, because those would permit new source access after the user-visible preview.

The in-memory bundle will carry:

- an internal format version;
- a content-addressed bundle ID and SHA-256 manifest hash;
- resolved base/head revision identifiers, but no absolute repository root;
- the applied file, declaration, and excerpt-byte budget;
- exact included file, item, and content-byte counts;
- deterministic items in existing file/declaration order;
- explicit file omissions and aggregate truncation copied into the canonical manifest.

Each non-empty excerpt becomes one evidence item with:

- a deterministic ordinal reference such as `E001`;
- kind `code` or `test`, where `_test.go` is classified as `test`;
- repository-relative path, declaration kind and identity, declaration span, and changed ranges;
- the exact bounded excerpt, its UTF-8 byte count, SHA-256 hash, and truncation flag.

Ordinal references are scoped to the immutable bundle manifest. The manifest hash covers the version, revisions, budget, counts, item locators/hashes, omissions, and truncation using structs and arrays only, never map iteration. The bundle ID is derived from that hash with a fixed version prefix. Raw content is not duplicated into the manifest and is not persisted by this task.

### 8.3 Fail closed at the evidence-sharing boundary

Return typed bundle errors with stable internal codes for invalid input and insufficient evidence. Validation includes:

- positive retained limits;
- safe repository-relative slash paths;
- valid and ordered line ranges and declaration spans;
- valid UTF-8 excerpts and exact byte accounting;
- unique, deterministic references;
- actual counts within the retained budget;
- at least one non-empty excerpt.

Empty/truncated-away declaration excerpts do not become evaluator evidence. Their absence remains visible through counts and the copied preview truncation metadata. If no usable item remains, construction fails as insufficient evidence so a later caller cannot invoke an evaluator with an empty bundle.

### 8.4 Preserve adapter independence

Do not add JSON tags or publish a daemon schema in this phase. The Go bundle is an internal domain value, not the `agent/` eval-case schema, the development run-record schema, or a new product protocol. A later adapter plan must define serialization, capability enforcement, explicit user continuation, and Pi process/session isolation independently.

## 9. Compatibility

- `internal/evidence.Result` gains one additive internal field. The repository has no external Go consumer because the package is under `internal/`.
- The existing `Preview` call, daemon routes, v1 request/response fields, fixed public limits, TypeScript types, `/learn` behavior, and package dependencies remain unchanged.
- Bundle format version 1 is internal and unreleased. Any future runtime serialization or persisted representation requires explicit compatibility review and must not be inferred from these Go structs.
- Bundle determinism depends on the current deterministic preview ordering. Tests freeze the new reference and manifest behavior for this internal version.
- No ADR is proposed because this phase does not select a runtime protocol, persistence format, evaluator adapter, model, or dependency. If implementation reveals a long-lived public or persisted contract, stop and draft an ADR before continuing.

## 10. Risks

- A bundle could accidentally include absolute workstation paths or source that was not represented in the user preview.
- A mismatched or missing budget could make the later evaluator receive more evidence than the user expected.
- Unstable canonicalization could make bundle IDs and run provenance non-repeatable.
- Empty or truncated excerpts could be mistaken for sufficient evidence.
- Evidence content may contain prompt injection; this phase can label and bound it but cannot enforce future prompt or adapter behavior.
- Calling the builder directly does not prove user continuation. This task intentionally leaves it unreachable from product entry points; later orchestration must enforce the preview/continue transition.
- The current preview can omit imports, deletion-only content, dependency resolution, or declarations beyond limits. The bundle must report that incompleteness rather than infer missing behavior.

## 11. Implementation Phases

### Phase 1 — Internal bounded EvidenceBundle

Goal: implement and verify the pure in-memory bundle boundary without any evaluator, protocol, persistence, or dependency work.

Risk: high. This phase defines the bytes and provenance a future evaluator may receive, so it changes an architecture and data-sharing seam.

Prerequisites:

- this investigated plan is reviewed and approved;
- Phase 1 is explicitly authorized;
- any request to expose the bundle through HTTP, Pi, files, or a model requires a revised plan and separate authorization.

Allowed files:

- `internal/evidence/evidence.go`
- `internal/evidence/evidence_test.go`
- `internal/evidence/bundle.go`
- `internal/evidence/bundle_test.go`
- `PROJECT.md`
- `plans/evaluator-ready-evidence-bundle.md`
- `docs/checkpoints/evaluator-ready-evidence-bundle-phase-1.md`

Forbidden changes:

- `AGENTS.md`, `README.md`, `go.mod`, `go.sum`, `package.json`, or dependency versions;
- `cmd/**`, `internal/daemon/**`, `extensions/**`, or `tests/extension/**`;
- `agent/**` development contracts, schemas, prompts, policies, evals, or fixtures;
- existing ADRs or a new runtime protocol/schema;
- filesystem, Git, subprocess, network, credential, persistence, telemetry, or model access from the bundle builder;
- commits, tags, publication, or release automation without separate authorization.

Acceptance criteria:

- `Preview` retains and returns the exact validated limits used to bound its result.
- `BuildBundle` is deterministic and performs no I/O.
- the bundle contains only non-empty excerpts and metadata already present in the supplied result.
- every item has a unique stable reference, safe relative locator, exact byte count, content hash, and explicit truncation state.
- test-file evidence is classified from the already previewed `_test.go` path; no unseen test file is read.
- the canonical manifest includes revisions, applied budget, counts, item metadata/hashes, omissions, and truncation, while excluding raw content and the absolute repository root.
- repeated construction from the same result yields the same manifest hash and bundle ID; any covered content or metadata change changes the hash.
- invalid budgets, unsafe paths, malformed ranges, invalid UTF-8, limit overflow, and no usable evidence fail closed with tested internal error codes.
- existing preview, daemon, and extension behavior remains unchanged.
- no model is called, no source is persisted, and no product endpoint can obtain the bundle in this phase.

Verification:

- `gofmt` only on changed Go files;
- focused: `go test ./internal/evidence`;
- canonical: `go test ./...`;
- canonical: `go test -race ./...`;
- canonical: `go vet ./...`;
- established host workarounds, reported separately if canonical Go 1.21 commands hit the recorded linker issue: `CGO_ENABLED=0 go test -count=1 ./...`, `go test -race -count=1 -tags netgo ./...`, and `go vet -tags netgo ./...`;
- `npm run typecheck` and `npm test` to prove unchanged extension compatibility;
- `scripts/test-agent-infra.sh` and `scripts/validate-agent-infra.sh`;
- `git diff --check`, `git status`, `git diff --stat`, and complete diff review.

The phase stops after its checkpoint and report. Later evaluator or orchestration work requires another approved phase or plan.

## 12. Acceptance Criteria

The task is complete when Phase 1 satisfies every acceptance criterion above, the bundle remains unreachable from product/model entry points, verification results are recorded accurately, and the phase checkpoint captures the exact repository state and remaining boundary.

## 13. Verification

The authoritative verification set is the Phase 1 list above. Focused tests must run before broader checks. Canonical Go commands and host workarounds must be reported separately; a workaround pass does not convert a canonical failure into a pass.

Review must confirm that every changed file is in the allowed list and that the pre-existing uncommitted Phase 3 handoff change remains preserved and distinguishable from this task.

## 14. Open Questions

No open question blocks Phase 1 after approval.

The following are deliberately deferred and must not be answered implicitly during implementation:

- the runtime JSON/input schema for a production evaluator;
- whether the future adapter uses Pi SDK or RPC and how it proves full tool isolation;
- model/provider selection and credential flow;
- explicit continuation UI and daemon orchestration after preview;
- type/dependency enrichment and how the expanded evidence is previewed to the user;
- persistence and lifecycle semantics for bundle manifests or assessment runs.
