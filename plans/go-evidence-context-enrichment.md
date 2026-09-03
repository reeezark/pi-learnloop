---
id: go-evidence-context-enrichment
status: active
risk: high
current_phase: 3
phase_status: awaiting_approval
updated: 2026-09-03
---

# Go Evidence Context Enrichment

## Goal

Improve the questions produced from a selected Git changeset by adding bounded,
previewed Go package, type, and direct-dependency context from the same selected
repository snapshot.

The enriched context is evidence, not hidden metadata. Every model-visible fact
or excerpt must be shown to the user, counted against fixed budgets, retained by
the daemon after preview, and passed unchanged through a versioned evaluator
input. Existing changed-declaration-only review remains compatible and keeps its
current routes, evidence format, prompts, and behavior.

## Why This Is the Next Capability

`PROJECT.md` records three remaining product candidates: richer answer editing,
durable asynchronous execution, and Go evidence enrichment. The first is a
localized interaction improvement, while the second would add lifecycle
infrastructure before there is evidence that the current foreground operation is
insufficient. Go context directly addresses the current product limitation that
questions can inspect changed declaration excerpts but cannot reliably relate
them to package declarations, referenced types, interfaces, or direct
repository-local dependencies.

The current parser also reports import-only changes as outside-declaration
omissions. Deleted files and deletion-only hunks remain intentionally out of
scope for this plan; this capability must not imply that those changes have been
explained when no selected-snapshot source exists.

## Status and Authorization

This is a high-risk plan because implementation will affect a public command's
data-sharing behavior, HTTP routes, evidence and evaluator schemas, released
prompts, and the dependency manifest. ADR-0007 and Phase 1, including the exact
dependency addition below, were explicitly accepted and authorized on
2026-09-02.

Before Phase 1 begins:

- ADR-0007 must be explicitly accepted;
- the user must explicitly authorize adding direct
  `golang.org/x/mod v0.19.0` while preserving the Go 1.21 baseline; and
- the user must explicitly authorize Phase 1.

The loader, build-configuration, external-fact, dependency, budget, and omission
choices are resolved below. Phase 1 completed and validated the selected
internal design without any route, evaluator, prompt, extension, database, or
user-visible behavior change. Phase 1 was committed and pushed as `477c1aa` on
2026-09-03. Phase 2 was explicitly authorized and completed on 2026-09-03.
Phase 3 is not authorized.

Each later phase also requires explicit authorization after the previous phase
has completed and been verified.

## Task Contract

### Goal

Design and, only after phased authorization, implement bounded Go context as
visible evidence for the selected Git snapshot.

### Scope

- The current repository's Go evidence extraction module.
- A snapshot-consistent, local-only package/type analysis mechanism.
- Bounded direct context and explicit omissions.
- Additive authenticated daemon routes for enriched previews.
- Versioned evidence bundles, evaluator inputs, and immutable prompts.
- Pi extension rendering and confirmation of all enriched evidence.
- Compatibility tests for the unchanged v1 path.

### Allowed Changes

The files allowed in each implementation phase are listed under that phase.
Across the plan, changes may cover only:

- `internal/evidence/` and its tests;
- `internal/daemon/` and its tests;
- `internal/evaluator/` and its tests;
- new versioned evaluator assets under `agent/`;
- the Pi extension, package-local extension tests, and packaging metadata when
  required by the approved design;
- `go.mod` and `go.sum` only if a specific dependency decision is accepted and
  separately authorized as part of Phase 1; and
- this plan, ADR-0007, relevant checkpoints, `PROJECT.md`, and user-facing
  documentation needed to record verified behavior.

### Forbidden Changes

- Guessing Git relationships from Pi Session timestamps, messages, prompts,
  answers, tool calls, summaries, or transcripts.
- Sending any Pi Session content or identifier into evidence bundles, evaluator
  inputs, prompts, RPC, model content, logs, errors, or generic history output.
- Reading or sending unselected repository content without displaying it in the
  preview and charging it to a fixed budget.
- Model filesystem, process, Git, or network access.
- Automatic module downloads or other network access during evidence analysis.
- Reading source outside the canonical repository root for model evidence.
- Sending third-party module-cache source or transitive dependency source.
- Temporary production checkouts, archives, worktrees, or other raw-source
  copies used to analyze historical revisions.
- Persisting source, excerpts, type facts, dependency facts, answers, prompts,
  transcripts, or enriched evaluator inputs.
- Background indexing, lifecycle hooks, reminders, Git snapshots, Session
  markers, or extension-owned review state.
- Automatic or hidden fallback from inconsistent context to a different Git
  snapshot.
- Call-graph inference, runtime-behavior inference, whole-repository uploads,
  transitive dependency traversal, generated-code workflows, or deletion-side
  reconstruction.
- Database schema changes unless a newly discovered requirement is documented,
  planned, and explicitly authorized before implementation.
- Unrelated refactoring, formatting, dependency upgrades, or protocol changes.

### Acceptance Criteria

- All enriched evidence comes from the selected working tree or the selected
  commit, never a mixture with the current checkout.
- The preview identifies the analysis configuration, fixed limits, exact context
  items and relations, context completeness, omissions, and truncation.
- Every model-visible byte or fact is present in the preview and retained without
  rereading the repository after confirmation.
- Context remains bounded under adversarial repository layouts and input sizes.
- Package/type loading performs no network access, does not write raw source, and
  cannot follow workspace or replacement paths outside the canonical repository.
- Existing v1 routes, bundles, evaluator inputs, prompts, and extension behavior
  remain supported and unchanged for existing clients.
- The enriched path has additive routes and new bundle/input/prompt versions.
- Pi Session ID provenance remains isolated exactly as required by ADR-0006.
- No source or enriched model input is added to durable history.
- Partial or unavailable context is explicit in the preview and bundle; it is
  never silently represented as complete.
- Focused, compatibility, adversarial-budget, privacy, and full supported tests
  pass.

### Verification

Every phase must run its focused tests plus the repository-supported full Go,
extension, governance, packaging, and Git checks listed in **Complete
Verification**. Tests that cannot run must be reported rather than substituted
with unsupported claims.

## Verified Current State

The investigation was performed on clean `main` at `d4ce923`, equal to
`origin/main` at the start of the investigation.

### Current evidence path

`internal/evidence.Preview` is the current deep module interface. It:

1. validates an explicit repository, Git selection, and positive fixed limits;
2. canonicalizes the repository root and resolves the selected base and head;
3. obtains changed Go files and changed new-side lines;
4. reads source only for changed files at the selected head or working tree;
5. parses those files with the standard library `go/parser`; and
6. maps changed lines to top-level declaration excerpts.

The returned result contains repository/revision provenance, applied limits,
files, declarations, omissions, and truncation. Existing omissions cover deleted
files, deletion-only hunks, and changed lines outside declarations. The current
fixed server limits are 20 files, 100 declarations, and 131072 excerpt bytes.

`internal/evidence.BuildBundle` is intentionally pure. It accepts the already
previewed result, performs no Git or filesystem read, and produces
`evidence-bundle@1`. `internal/evaluator.NewInput` then produces
`evaluator-input@1`. The released v1 question prompt instructs the model to use
only selected bundle evidence.

The daemon retains the exact preview result in a single-use, expiring
continuation. A question request consumes that result; it does not rebuild the
evidence from a potentially changed repository.

### Current authenticated routes

- `POST /v1/evidence-previews`
- `POST /v1/pi-session-evidence-previews`
- `POST /v1/question-sets`
- `POST /v1/assessment-turns`
- dedicated and generic history routes

The two preview routes share Git evidence semantics. The Session route carries a
validated Pi Session ID only as daemon-owned provenance. ADR-0006 prohibits that
identifier from entering evidence, evaluator content, prompts, RPC, errors,
logs, or generic history responses.

### Current evaluator and history guarantees

- The evaluator receives only the selected, previewed, budgeted evidence bundle.
- The user previews evidence before any provider invocation and confirms the
  displayed model, scope, and estimated cost.
- The evaluator has no filesystem, process, Git, or network tools.
- Released prompt assets are immutable; an evaluator input or safety-contract
  change requires a new version.
- Durable history stores source-free manifest hashes and prompt provenance, not
  source, excerpts, evaluator inputs, or answers.
- History schema v2 already supports the optional Pi Session ID. This capability
  has no known need for a schema v3 migration.

### Current implementation gap

`PROJECT.md` correctly records that syntax parsing does not provide type or
dependency information. The module currently has no `go/types` integration and
no package-loading abstraction. `go.mod` declares Go 1.21 and has no direct
`golang.org/x/mod` or `golang.org/x/tools` dependency.

Before Phase 1, `BuildBundle` also required at least one changed-declaration excerpt. Therefore
an import-only changeset can currently preview an `outside_declaration` omission
but cannot create evaluator evidence. The enriched v2 path must represent a
changed import as its own evidence item instead of pretending it is a declaration
or silently dropping it.

Phase 1 now provides an internal, opt-in `ContextGo` evidence mode behind
`Preview`, backed by private commit and working-tree snapshots, selected-snapshot
module/workspace parsing, build selection, and bounded repository-local
`go/types` analysis. It is not reachable from the daemon. Existing callers leave
the mode disabled, and `BuildBundle` rejects an enriched result so the v1 bundle
cannot accidentally carry unversioned context.

### Authoritative loader and dependency findings

The installed module graph already selects `golang.org/x/mod v0.19.0` and
`golang.org/x/tools v0.23.0` indirectly through the SQLite driver's transitive
test/tooling graph, and `go.sum` already contains both versions. The selected
versions declare Go 1.18 and Go 1.19 respectively. An actual production import
of `golang.org/x/mod/modfile` still requires a direct `go.mod` requirement; a
read-only `go list` check rejected the import until `go mod tidy` would update
the manifest.

The selected design adds only direct `golang.org/x/mod v0.19.0` in an authorized
Phase 1. Its in-memory `modfile.Parse`, `ParseLax`, and `ParseWork` support the
needed `go.mod`, `go.work`, `use`, `replace`, `toolchain`, and `godebug` syntax;
`module.CheckPath` and `CheckImportPath` reject malformed module and import paths
before repository discovery. These packages stay within the `x/mod` module,
preserve the Go 1.21 baseline, and add no second module dependency. Version
v0.20.0 also declares Go 1.18 but would unnecessarily upgrade the already
selected graph. `x/mod` v0.21.0 and later require Go 1.22 and are not compatible
with the current baseline.

`golang.org/x/tools/go/packages` is not selected. Its default driver writes every
overlay value to temporary backing files plus an overlay JSON file before
running `go list -overlay`; that violates the production no-raw-source-copy
rule. A custom `GOPACKAGESDRIVER` would add a subprocess protocol seam and still
causes v0.23.0 to invalidate export data when an overlay is present, which can
expand source/type loading beyond the selected scope. The latest inspected
`x/tools` versions are also unsuitable for Go 1.21: v0.25.0 requires Go 1.22 and
v0.49.0 requires Go 1.25.

The standard `go/importer` path is also not selected for external facts. Its
documentation limits reliable use outside small standard-library cases, and its
compiler-data path may invoke `go list -export`, read GOROOT source, and mutate
the Go build cache. The initial loader therefore parses and type-checks only
repository-local snapshot source with a private importer. External and standard
library imports contribute syntactic import identities only; their source and
compiled type facts are not read.

The repository observation used to size the initial limits contains 52 tracked
Go files, 14782 lines, and 541908 source bytes. Its largest package contains 14
files and about 188 KiB, and the largest file is about 43 KiB. These are
one-repository observations, not performance guarantees; Phase 1 still must test
adversarial fixtures and measure CPU and memory under the fixed limits.

References:

- <https://pkg.go.dev/golang.org/x/tools/go/packages>
- <https://pkg.go.dev/golang.org/x/mod/modfile>
- <https://pkg.go.dev/go/build#Context>
- <https://pkg.go.dev/go/importer>
- <https://pkg.go.dev/go/types#Config>
- <https://git-scm.com/docs/git-cat-file>
- <https://git-scm.com/docs/git-ls-files>
- <https://git-scm.com/docs/git-ls-tree>
- <https://go.dev/doc/modules/gomod-ref>
- <https://github.com/golang/tools/blob/v0.23.0/internal/gocommand/invoke.go#L487-L555>
- <https://raw.githubusercontent.com/golang/tools/v0.49.0/go.mod>
- <https://raw.githubusercontent.com/golang/tools/v0.25.0/go.mod>
- <https://raw.githubusercontent.com/golang/tools/v0.23.0/go.mod>
- <https://raw.githubusercontent.com/golang/mod/v0.19.0/go.mod>

## Design Principles

### Deepen the evidence module

The public implementation seam remains `internal/evidence.Preview(ctx,
Request) (Result, error)`. Snapshot materialization, package loading, type
resolution, context selection, budgeting, and omission classification stay
behind that interface. Callers request a policy and receive one complete,
previewable result; they do not orchestrate Git and Go-tool details themselves.

Commit-backed and working-tree-backed source access are two real implementation
variations. A small private `snapshot` seam inside `internal/evidence` will have
exactly those two adapters and will hide bounded enumeration, file metadata,
content reads, and containment checks. It is not exported and is not a general
loader/plugin interface. Module parsing, build selection, syntax parsing, type
checking, context selection, and omission policy remain evidence implementation,
not caller orchestration. Tests primarily exercise `Preview` with temporary Git
repositories; narrow adapter tests cover streaming, cancellation, and size
enforcement that cannot be observed reliably at the external seam.

### Context is evidence

Package paths, type signatures, declared relationships, source locations,
repository-local excerpts, completeness indicators, and omission reasons can
influence model output. They are therefore evidence and must obey the same
preview, retention, budget, hashing, and untrusted-content rules as changed code.

No model-visible context may be attached after confirmation. Bundle construction
remains pure and receives only the retained preview result.

### Snapshot consistency is mandatory

For a commit range, every file, module/workspace rule, declaration, and derived
relationship used by the enriched result must describe the selected head commit.
The current checkout must not fill gaps. For working-tree review, analysis must
reflect the current index/worktree semantics already defined by the existing
evidence module, including uncommitted content.

A historical commit cannot be analyzed by loading its changed files over the
current checkout unless the implementation can prove that all build inputs are
the selected snapshot. A production temporary worktree or source archive is not
an acceptable workaround because it writes raw source, may leave residue after a
crash, and can mutate Git administrative state.

The commit adapter will enumerate bounded tree entries with streamed,
NUL-delimited Git output and will inspect object type and size before reading a
blob, using `git ls-tree` and `git cat-file` rather than a checkout. The
working-tree adapter will enumerate Git-tracked and nonignored untracked files,
then enforce canonical-root and symlink containment before a bounded regular-file
read. Existing v1 Git results remain unchanged, but new context discovery must
not use the current `gitOutput` full-buffer helper for potentially large output.

### Direct, repository-local scope

The initial enriched result is limited to changed packages plus their direct
repository-local dependencies and the smallest context needed to explain
selected changes:

- package identity and selected build configuration;
- repository-local declarations directly referenced by changed declarations;
- types, methods, interfaces, and implementation relationships proven by the
  accepted type-analysis mechanism;
- direct imported package identities needed to explain those references; and
- bounded repository-local excerpts only when the relation to changed evidence
  is explicit.

The final relation vocabulary must be frozen in Phase 1. It must distinguish
syntactic imports from type-proven references and implementations. It must not
claim a dynamic call graph or inferred runtime behavior.

The analyzer locates the nearest selected-snapshot `go.mod`, verifies nested
module ownership, optionally applies repository-contained `go.work`/`use` and
local `replace` targets, selects buildable files through a custom `go/build.Context`,
then parses and type-checks in memory with `go/parser`, `go/ast`, and `go/types`.
It does not traverse transitive imports.

Only facts backed by actual repository-local `types.Object` values may be marked
type-proven. An `implements` relation is emitted only when every involved local
package type-checks without a relevant error. `go/types` may synthesize a fake
package after an importer error, so partial results must never be upgraded to
fully proven facts. External and standard-library imports expose only their
syntactic import identity; their source, export data, members, methods, and
implementation facts are omitted.

### Honest incompleteness

An enriched preview records context status as complete, partial, or unavailable,
with structured, bounded omission reasons. It may still contain the valid
changed-declaration evidence. The user sees that status before confirmation, and
the bundle conveys the same status to the evaluator. The server never silently
substitutes the current checkout, makes a network request, or labels incomplete
analysis as complete.

The existing changed-only v1 path remains independently available. It is not a
hidden fallback performed inside the enriched route.

## Proposed Call Flow

```text
updated /learn client
  -> explicit Git selection, or Pi Session followed by explicit Git selection
  -> additive authenticated Go-context preview route
  -> internal/evidence.Preview with fixed Go-context policy
       -> selected-snapshot Git source
       -> private bounded package/type analysis
       -> changed evidence + context evidence + omissions + budgets
  -> render every model-visible item and the completeness state
  -> explicit model/scope/cost confirmation
  -> daemon-owned single-use continuation retains the exact result
  -> pure evidence-bundle@2 construction
  -> evaluator-input@2 and immutable question prompt v2
  -> assessment input v2 and immutable assessment prompt v2
  -> source-free history manifest and prompt provenance
```

The generic and Pi Session paths remain separate so Session provenance does not
enter generic evidence types. Proposed additive routes are:

- `POST /v1/go-context-evidence-previews`
- `POST /v1/pi-session-go-context-evidence-previews`

The existing `/v1/question-sets` and `/v1/assessment-turns` routes may remain
because the opaque continuation and assessment identifiers let the daemon own
the selected evidence mode. Their strict request and response bodies do not need
new client-supplied fields. This assumption must be confirmed by protocol tests
in Phase 2.

## Evidence Shape and Fixed Budgets

Phase 1 fixes the internal Go structures and budgets for these concepts. Phase 2
must define separate strict JSON and versioned bundle/input shapes without
changing the accepted values:

- applied context policy and analysis/build configuration;
- fixed limits for context files, declarations, relationships, and total bytes;
- context items with stable identities, repository-relative locations, kind,
  display text or excerpt, byte count, and truncation state;
- explicit relationships from changed evidence to context items, with a closed
  relation vocabulary and provenance strength;
- completeness status and bounded omission counts/reasons; and
- aggregate truncation counters sufficient to show what the budgets excluded.

V2 has separate `changed_import` and `context_declaration` item kinds. A changed
import is first-class usable evidence even when the file has no changed
declaration excerpt. Its identity and displayed content come only from the
selected snapshot's parsed import specification. It does not grant access to the
imported package's source or type facts.

All ordering must be deterministic. Identity and ordering must not depend on map
iteration, absolute host paths, temporary paths, timestamps, or loader diagnostic
wording. Error and omission text must not leak source or paths outside the
canonical repository.

The proposed fixed input-side limits are:

| Input work | Limit |
| --- | ---: |
| changed files | existing 20 |
| module/workspace roots | 8 |
| analyzed packages | 32 |
| files per package | 64 |
| total analyzed files | 160 |
| directory entries per package | 256 |
| source bytes per file | 256 KiB |
| aggregate analyzed source | 2 MiB |
| direct import edges | 256 |
| preview analysis deadline | existing 30 seconds |

The proposed fixed output-side limits are:

| Model-visible output | Limit |
| --- | ---: |
| context files | 20 |
| changed-import and context-declaration items combined | 40 |
| context relationships | 100 |
| one context excerpt | 4 KiB |
| aggregate repository-derived context bytes | 64 KiB |
| complete serialized `evaluator-input@2` | 256 KiB |

The aggregate context-byte limit counts every variable repository-derived string,
not only source excerpts. If input discovery exceeds a cap, context is
`unavailable` with `analysis_limit_exceeded`; an arbitrary prefix is never
presented as complete. If deterministic output selection exceeds a cap, the
preview reports `output_truncated` with exact aggregate omitted counts. Phase 1
must prove these limits operationally; changing an accepted shipped value later
is compatibility-sensitive.

## Compatibility and Versioning

### Existing path

The following remain unchanged:

- `/v1/evidence-previews` and `/v1/pi-session-evidence-previews` request and
  response semantics;
- the current fixed changed-declaration limits;
- `evidence-bundle@1`;
- `evaluator-input@1` and `evaluator-assessment-input@1`;
- released v1 question and assessment prompts; and
- existing clients' ability to complete a changed-only review.

Although ADR-0002 allows optional response fields within v1, enriched evidence
must not be silently added to an existing route. An older extension could ignore
the new preview fields while the daemon still sends them to the model, violating
the preview-before-sharing guarantee.

### Enriched path

The enriched path requires:

- additive preview routes with strict request validation;
- `evidence-bundle@2`;
- `evaluator-input@2`;
- `evaluator-assessment-input@2`, because the assessment input embeds the exact
  evaluator input; and
- new immutable question and assessment prompt assets with new major versions.

Question-set and assessment-turn output formats may remain at v1 if their
meaning and allowed values are unchanged. This must be demonstrated rather than
assumed.

The updated extension may choose the enriched route after the same explicit Git
selection. It must render the enriched scope and completeness before asking for
confirmation. A partial or unavailable result is allowed to proceed only when
its exact changed evidence, empty or partial context, and omissions are visibly
confirmed; this is an explicit preview state, not a hidden downgrade.

### History and Pi Session provenance

The bundle manifest hash naturally distinguishes v1 and v2 bundles. Existing
source-free prompt provenance records the released prompt version. No database
schema change is expected.

The Pi Session ID stays in daemon-owned preview continuation, assessment
provenance, and dedicated history-start state only. It remains absent from both
bundle versions, both evaluator-input versions, prompts, RPC, model content,
errors, logs, and generic history output.

## Privacy, Resource, and Security Constraints

- Analysis is local and foreground-bound to the explicit preview request.
- The analysis mechanism must be proven to perform no network access, including
  module, toolchain, checksum-database, proxy, and custom-driver access.
- `go.work`, local `replace` directives, symlinks, and package patterns must not
  cause source reads outside the canonical repository root.
- External source, absolute host paths, environment secrets, VCS configuration,
  and raw loader diagnostics must not enter the preview or model input.
- Production analysis does not read pre-existing external compiled export data,
  external module source, vendor source, or GOROOT source and does not invoke the
  Go command. It therefore does not mutate module, build, or toolchain caches.
- Context discovery work must have input-side bounds or cancellation in addition
  to output budgets; it cannot traverse an unbounded repository and discard the
  result afterward.
- Continuation retention stays bounded, expiring, in-memory, and single-use.
- Durable history remains source-free.
- All evidence and loader-derived text is untrusted content, never instructions.

## Resolved Design Decisions and Phase 1 Proof Obligations

### Snapshot-native loader

Phase 1 will implement one private `snapshot` seam owned by `internal/evidence`
with commit and working-tree adapters. The commit adapter uses streamed bounded
`git ls-tree -z` enumeration and size-before-content `git cat-file` reads. The
working-tree adapter uses Git-tracked plus nonignored untracked files and bounded
contained reads. Neither adapter creates a checkout, archive, overlay backing
file, or other raw-source copy. The `ContextGo` path also disables Git promised-
object lazy fetch, terminal prompting, and optional locks for every Git read;
the existing context-disabled v1 path retains its inherited Git environment.

Both adapters supply selected-snapshot bytes to the same in-memory analysis. The
analysis reads the nearest module file, bounded repository-contained workspace
configuration, build-selected Go files, and direct repository-local imports. It
must verify parity for additions, deletions, symlinks, nested modules,
workspaces, and replacements. A snapshot/configuration it cannot interpret
safely yields unavailable context instead of fallback.

### Dependency and Go baseline

The only proposed direct dependency is `golang.org/x/mod v0.19.0`, for in-memory
module/workspace parsing. It is already selected indirectly and present in
`go.sum`, declares Go 1.18, keeps its needed runtime packages within the `x/mod`
module, and preserves the module's Go 1.21 baseline. Phase 1 must add it as a
direct requirement only after explicit dependency authorization. It must not add
`golang.org/x/tools`, select `go/packages`, upgrade Go, or upgrade another module.

### Fixed build configuration

Every enriched preview visibly records and hashes this fixed initial policy:

- `GOOS` and `GOARCH` are those of the running LearnLoop binary;
- CGO is disabled; a selected package importing `C` reports `cgo_unsupported`;
- custom build tags are empty;
- release and tool tags come from the Go toolchain used to build LearnLoop, and
  that toolchain version is included in the preview and manifest;
- the language version comes from a required `go` directive in the nearest
  selected-snapshot `go.mod`; a missing directive is an
  `unsupported_module_layout`, and a version newer than the compiled toolchain
  is `unsupported_go_version` rather than being guessed;
- a selected-snapshot `go.work` is considered only when it is inside the
  canonical repository; outside-root `use` entries are not followed and are
  reported as `outside_repository_dependency`;
- local `replace` targets are followed only inside the canonical repository;
  external replacements retain identity only and have no type/source facts;
- test variants are included only when the selected change contains a `_test.go`
  file; and
- ambient `GOWORK`, module/proxy/checksum/toolchain settings, custom drivers, and
  automatic toolchain selection do not affect analysis.

### External dependency policy

External and standard-library imports contribute only bounded syntactic import
identities. Phase 1 does not read module-cache, vendor, GOROOT, or compiled export
data and does not invoke `go list`. A missing external importer may make local
type checking partial, but only facts backed by valid repository-local objects
are emitted; `implements` requires all participating local packages to be fully
type-checked.

### Completeness and omission taxonomy

Context is:

- `complete` only when every in-scope changed package and direct repository-local
  dependency is analyzed without an omitted required fact;
- `partial` when valid context remains but external facts, CGO, an outside-root
  workspace/replacement target, parse/type failure, or output budget prevents
  complete coverage; or
- `unavailable` when the selected snapshot/module configuration cannot be safely
  interpreted or an input discovery cap is exceeded.

The closed initial omission vocabulary is:

- `analysis_limit_exceeded`
- `unsupported_module_layout`
- `unsupported_go_version`
- `outside_repository_dependency`
- `cgo_unsupported`
- `external_type_unavailable`
- `context_parse_error`
- `type_incomplete`
- `output_truncated`

Phase 1 must prove that each reason is deterministic, bounded, path/source-safe,
and mapped to the correct completeness state. Raw Git, parser, or type-checker
diagnostics must never become protocol, log, bundle, or model text.

## Phase 1 — Deep Evidence Module and Snapshot Proof

### Objective

Implement and verify the internal snapshot-consistent, bounded Go context model
without exposing it through the daemon, evaluator, prompt, or extension.

### Entry Conditions

- ADR-0007 is accepted.
- Direct `golang.org/x/mod v0.19.0` addition is explicitly authorized.
- The user explicitly authorizes Phase 1.

### Allowed Files

- `internal/evidence/**`
- focused evidence fixtures, if kept under that package
- `go.mod` and `go.sum` only for the accepted dependency decision
- `plans/go-evidence-context-enrichment.md`
- `docs/decisions/ADR-0007-snapshot-consistent-go-context-evidence.md`
- `docs/checkpoints/go-evidence-context-enrichment-phase-1.md`
- `PROJECT.md` only to record verified stable facts

### Work

1. Add direct `golang.org/x/mod v0.19.0` without upgrading Go or any selected
   module, and use only its `modfile` and `module` packages for in-memory
   module/workspace parsing and module/import path validation.
2. Add the fixed private context policy and input/output limits while preserving
   exact changed-only behavior for existing direct callers.
3. Implement the commit and working-tree adapters behind the private `snapshot`
   seam and `evidence.Preview`, including streamed bounded Git output,
   cancellation, size-before-content checks, and root/symlink containment.
4. Implement selected-snapshot module ownership, repository-contained workspace
   and replacement resolution, `go/build.Context` file selection, in-memory
   parsing, and direct repository-local `go/types` checking with a private
   importer.
5. Add deterministic `changed_import` and `context_declaration` items, typed
   relationships, build configuration, completeness, omissions, and truncation
   to an internal enriched result without constructing a model bundle.
6. Enforce no network, Go-command, custom-driver, cache, external-source, vendor,
   GOROOT-source, checkout, archive, or temporary-source access.
7. Cover additions, deletions, import-only changes, historical module files,
   nested modules, workspaces, replacements, symlinks, build constraints, tests,
   CGO, external imports, malformed packages, import cycles, newer language
   versions, and oversized repositories with focused fixtures.
8. Measure CPU, memory, cancellation, and serialized sizes under the accepted
   budgets and adversarial fixtures; record evidence without changing the fixed
   values silently.
9. Prove unchanged changed-only callers and tests retain their exact current
   behavior.

### Phase 1 Acceptance

- Commit and working-tree facts are snapshot-consistent in adversarial tests.
- Analysis cannot access the network or repository-external source.
- Production analysis writes no raw-source checkout or archive.
- Production analysis invokes no Go package driver, reads no external/GOROOT
  source or compiled export data, and mutates no Go cache.
- Work is bounded before and during discovery, not only when serializing output.
- Result ordering, identities, omissions, and truncation are deterministic.
- Import-only changes produce bounded `changed_import` evidence on the enriched
  path while the existing v1 result remains unchanged.
- Partial `go/types` results never produce a fully proven relationship, and
  `implements` is emitted only from fully checked local packages.
- The fixed input/output budgets hold under adversarial CPU/memory/size tests.
- Existing changed-only evidence tests pass unchanged.
- No daemon route, evaluator schema, prompt, extension, database, or user-visible
  behavior changes.

### Phase 1 Result

Completed and finally verified on 2026-09-03:

- `Preview` owns an opt-in internal Go-context policy while its zero-value mode
  preserves existing changed-only results exactly.
- Private commit and working-tree snapshots stream bounded Git enumeration,
  enforce size-before-content reads, respond to cancellation, resolve only
  contained symlinks, and create no source copy. Context-mode Git reads disable
  promised-object lazy fetch, terminal prompting, and optional locks without
  changing the context-disabled v1 path.
- The shared in-memory analyzer applies the accepted module/workspace/build
  policy, traverses only changed packages plus direct repository-local imports,
  and emits only deterministic changed-import/context-declaration items and
  syntactic or fully checked relationships.
- Workspace replacements take precedence over module replacements. An external
  override invalidates an earlier local mapping and retains only the replaced
  module identity; version-specific replacements and a workspace that does not
  contain the changed module fail closed as `unsupported_module_layout`.
- All accepted fixed input/output limits and omission states are executable and
  covered by real-repository and private-adapter tests. An enriched result is
  rejected by the v1 bundle builder pending Phase 2 contracts.
- `golang.org/x/mod v0.19.0` is now the only added direct requirement. The Go
  language baseline remains 1.21, `go.sum` is unchanged, and every other selected
  module version is unchanged.

### Stop Condition

After verification, mark Phase 1 complete, create its current checkpoint, advance
the plan to Phase 2 with `phase_status: awaiting_approval`, and stop for explicit
authorization.

## Phase 2 — Additive Routes and Versioned Evaluator Contracts

### Objective

Expose the retained enriched preview through additive authenticated daemon routes
and carry the exact previewed evidence through new bundle, evaluator-input, and
prompt versions.

### Entry Conditions

- Phase 1 is complete and verified.
- Phase 2 protocol shapes and released prompt versions are recorded in this plan.
- The user explicitly authorizes Phase 2.

### Allowed Files

- `internal/evidence/**`
- `internal/daemon/**`
- `internal/evaluator/**`
- new versioned policy, schema, prompt, and eval assets under `agent/**`
- `plans/go-evidence-context-enrichment.md`
- `docs/decisions/ADR-0007-snapshot-consistent-go-context-evidence.md`
- `docs/checkpoints/go-evidence-context-enrichment-phase-2.md`
- `PROJECT.md` and relevant README files only to document verified behavior

### Work

1. Add the two strict authenticated enriched-preview routes without changing the
   existing preview routes.
2. Retain the exact enriched result in the existing bounded continuation model;
   do not reread the repository after preview.
3. Add pure `evidence-bundle@2` construction with deterministic manifest hashing
   for all changed and context evidence, relations, limits, omissions, and
   completeness.
4. Add `evaluator-input@2` and `evaluator-assessment-input@2` schemas and Go
   contracts.
5. Add immutable v2 question and assessment prompts whose rules permit only the
   selected enriched bundle and prohibit outside knowledge and instruction-like
   evidence.
6. Extend policy and evaluation fixtures for import-only changes, interface/type
   relations, partial context, unavailable context, adversarial content, budget
   exhaustion, and Session provenance isolation.
7. Preserve current question-set and assessment-turn output protocols unless
   tests prove a version change is required; if one is required, stop and update
   the plan before proceeding.
8. Prove generic and Pi Session continuations select the correct v1 or v2 path
   entirely from daemon-owned state.
9. Prove durable history remains source-free and needs no schema migration.

### Phase 2 Acceptance

- Existing routes still produce the exact v1 evidence and evaluator path.
- New routes reject unknown/request-injected fields and produce v2 evidence only.
- All model-visible enriched evidence appeared in the corresponding preview.
- Continuation consumption is single-use and snapshot-stable.
- Session ID isolation tests cover success, rejection, expiration, cancellation,
  assessment, history start, logs, and serialized errors.
- New prompts and schemas pass governance invariants and adversarial eval cases.
- No extension behavior or database schema changes.

### Phase 2 Result

Completed and finally verified on 2026-09-03:

- Added the two separate strict authenticated preview routes. They run the
  accepted internal Go-context policy and expose every model-visible context
  item, relation, build value, fixed limit, completeness state, omission, and
  truncation count without changing either existing preview route.
- Extended the existing bounded continuation store with a private v1/v2
  contract marker, owned Go-context copy, and accounting for every retained
  repository-derived context byte. Generic and Session-bound paths
  select their bundle/input/prompt version solely from this daemon-owned state;
  the client cannot inject a mode and the repository is not reread.
- Added pure `evidence-bundle@2`, `evaluator-input@2`, and
  `evaluator-assessment-input@2` contracts with deterministic full-manifest and
  content hashes, genuine import-only evidence support, detached validation,
  and the fixed complete-input byte cap. Question-set and assessment-turn
  outputs remain unchanged at v1.
- Added and embedded immutable question and assessment prompt v2 assets, plus
  strict documentation schemas, a supplemental local-only evidence policy, and
  synthetic adversarial fixtures. Production Pi adapters choose v1 or v2 from
  the validated runtime schema and retain the existing no-tools process
  isolation.
- Proved exact retained evidence after working-tree mutation, single-use and
  expiry behavior, cancellation, strict request rejection, v1 compatibility,
  Pi Session isolation through question/assessment/error/history paths, and
  source-free completion history without a database migration. The daemon has
  no request logging sink; no new logging path was added.
- No extension, dependency, Go baseline, database schema, existing route, or
  existing output-protocol change was made.

### Stop Condition

After verification, mark Phase 2 complete, create its current checkpoint, advance
the plan to Phase 3 with `phase_status: awaiting_approval`, and stop for explicit
authorization.

## Phase 3 — Pi Extension Preview and Confirmation

### Objective

Make the updated Pi extension request, visibly render, and explicitly confirm the
enriched evidence before any provider invocation.

### Entry Conditions

- Phase 2 is complete and verified.
- The extension UX and context-unavailable presentation are fixed in the plan.
- The user explicitly authorizes Phase 3.

### Allowed Files

- `extensions/pi-learnloop/**`
- extension packaging/test files required by the existing package workflow
- `README.md`
- `PROJECT.md`
- `plans/go-evidence-context-enrichment.md`
- `docs/decisions/ADR-0007-snapshot-consistent-go-context-evidence.md`
- `docs/checkpoints/go-evidence-context-enrichment-phase-3.md`

### Work

1. Route current Git and explicit Pi Session/Git selections to the enriched
   preview routes for the updated extension while preserving compatibility with
   daemon authorization and strict responses.
2. Validate every required v2 preview field before displaying it.
3. Render changed evidence, context evidence, relationships, applied limits,
   completeness, omissions, and truncation in a bounded readable preview.
4. Update confirmation text so it accurately describes the complete displayed
   evidence scope, model, and estimated cost.
5. Permit continuation for partial or unavailable context only after the user
   sees and confirms that exact state; do not silently retry the v1 route.
6. Preserve cancellation, daemon-loss, stale-continuation, Session selection,
   Git binding, question, answer, assessment, and history behavior.
7. Update user documentation with the snapshot, privacy, budget, and
   incompleteness semantics.

### Phase 3 Acceptance

- The user can identify every category of content that may be sent to the model.
- The extension never confirms or sends fields it did not render.
- Context completeness and omissions are apparent before confirmation.
- Cancellation before confirmation causes no provider call and no history row.
- Pi Session content and ID isolation remain intact.
- Extension tests cover working tree, commit range, Session binding, partial and
  unavailable context, budget truncation, unknown responses, and old daemon
  incompatibility.
- Packaging, Go, governance, and full repository checks pass.

### Stop Condition

After verification, mark the plan and its Phase 3 checkpoint complete and stop.
Commit or push only when explicitly authorized by the user.

## Risks and Mitigations

### Mixed-snapshot evidence

Loading a historical changed file against the current checkout can produce
plausible but false context. Phase 1 cannot complete until adversarial fixtures
prove selected-snapshot semantics for both adapters. Inconsistency yields
unavailable context, never a best-effort mixture.

### Unbounded analysis

Package loaders can inspect far more input than the final output contains. Use
discovery bounds, cancellation, package/file limits, and measurements in addition
to serialized byte caps.

### Network and external-path access

Go tooling can consult proxies, checksum services, toolchain downloads,
workspaces, replacements, custom drivers, caches, and environment configuration.
Tests must make those paths observable and fail closed. Environment flags alone
are not accepted as proof.

### Dependency/toolchain drift

Add only direct `golang.org/x/mod v0.19.0`, already selected in the current graph,
and verify `go.mod`, `go.sum`, and the complete graph contain no unrelated
upgrade. Do not add `x/tools` or change the Go 1.21 baseline. A future dependency
or baseline change is a separate high-risk compatibility decision.

### Preview/model mismatch

Adding optional fields to an old response would let old clients ignore context
that a new daemon might send. Additive routes, versioned bundle/input contracts,
and exact retained results prevent that mismatch.

### Prompt regression

More context can encourage broad or speculative questions. V2 prompts and evals
must require citations to selected evidence, distinguish proven relations from
omissions, and prohibit outside knowledge.

### Privacy expansion

Context can expose unchanged repository source that the current product never
sends. The direct scope, separate budgets, explicit preview, no external source,
and no persistence keep that expansion visible and bounded.

### Session provenance leak

Shared context code could tempt callers to embed Session identifiers in evidence.
Keep Session provenance in daemon-owned wrappers and test serialized outputs,
errors, logs, bundle hashes, prompts, and history separately.

## Complete Verification

Run the commands supported by the repository at the end of each applicable
phase. Derive exact package-script names from the current manifests rather than
inventing replacements.

### Focused Go verification

```text
go test -count=1 ./internal/evidence
go test -count=1 ./internal/daemon
go test -count=1 ./internal/evaluator
```

Use only the packages changed by the current phase. Add focused race runs where
the changed code contains shared state or cancellation.

### Full Go verification

```text
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go build ./...
```

Also run the repository's documented standard-cache test and its documented
temporary-cache workaround when the environment requires it. Tests for the
loader must be hermetic and must fail if they attempt network access or escape
the repository fixture.

### Extension verification

From the existing extension workspace, run the manifest-defined type check,
unit tests, and package/dry-run command. Do not add a new test runner or package
manager as part of this feature.

### Agent infrastructure and Git verification

```text
scripts/test-agent-infra.sh
scripts/validate-agent-infra.sh
git diff --check
git status --short --branch
git diff --stat
git diff
```

Review the complete diff and confirm every changed file is allowed by the current
phase. Do not invoke a live model/provider during automated verification. Do not
claim unsupported CI, integration, or production testing.

## Completion Criteria

The plan is complete only when all three phases are explicitly authorized,
implemented, verified, checkpointed, and reviewed; existing v1 behavior remains
compatible; every enriched model-visible fact is previewed and bounded; snapshot
consistency and local-only analysis are proven; Pi Session provenance remains
isolated; durable history remains source-free; and the final plan metadata is
`status: complete` with `phase_status: complete`.
