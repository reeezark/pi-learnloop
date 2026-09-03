---
id: ADR-0007
status: accepted
date: 2026-09-02
supersedes: none
---

# ADR-0007: Snapshot-Consistent Go Context Is Previewed Evidence

## Context

Pi LearnLoop currently asks questions from bounded excerpts of changed Go
declarations. `internal/evidence.Preview` reads the explicit Git selection,
parses changed files with `go/parser`, and returns a preview. The daemon retains
that exact result; pure bundle construction produces `evidence-bundle@1`, and
the evaluator receives `evaluator-input@1` under an immutable v1 prompt.

This design has a valuable property: the user sees the exact selected evidence
before provider invocation. It also has a known limitation. Syntax-only changed
declarations cannot reliably explain referenced types, interfaces, package
relationships, or direct dependencies. Import-only changes currently produce an
outside-declaration omission, and the evaluator must not fill the gap with
outside knowledge.

Adding Go context expands the amount of repository information that may influence
model output. Package names, type signatures, relationships, source locations,
and unchanged declaration excerpts are not harmless metadata; they are evidence.
They therefore require the same selection, preview, budget, retention, hashing,
and untrusted-content treatment as changed code.

Historical commit analysis creates a second problem. A conventional package
loader observes the filesystem and Go build environment. Overlaying only changed
files from a selected commit onto the current checkout could mix revisions of
unchanged files, `go.mod`, `go.work`, build constraints, and replacement targets.
That result may be plausible but false. Conversely, creating a temporary
worktree or archive writes an additional raw-source copy and can leave residue or
mutate Git administrative state.

Go tooling may also contact module proxies, checksum services, toolchain download
services, or custom package drivers; read external workspace/replacement paths;
or inspect a large dependency graph before output budgets take effect. Output
caps alone do not make such analysis private or bounded.

At the decision date the module declares Go 1.21. Its existing graph indirectly
selects `golang.org/x/mod v0.19.0` and `golang.org/x/tools v0.23.0`; both already
have checksums. They declare Go 1.18 and Go 1.19 respectively. Importing
`x/mod/modfile` in production still requires a direct manifest requirement.

The inspected `go/packages` v0.23.0 default driver writes overlay source bytes to
temporary backing files before invoking `go list -overlay`. A custom driver adds
a subprocess seam, and overlays invalidate export data in that version. The
standard `go/importer` path may invoke `go list -export`, inspect GOROOT source,
and mutate build cache. Neither mechanism satisfies the selected-snapshot,
no-source-copy, no-external-source, and cache-nonmutating rules.

ADR-0002 makes current route semantics and fixed limits compatibility-sensitive.
ADR-0003 requires exact previewed evidence, pure bundle construction, versioned
evaluator inputs, immutable prompts, and no evaluator tools. ADR-0005 restricts
durable history to source-free provenance. ADR-0006 keeps Pi Session ID
provenance outside evidence and model content.

## Decision

Pi LearnLoop will treat bounded Go package, type, and
direct-dependency context as first-class evidence from the exact selected Git
snapshot.

This decision was accepted on 2026-09-02. The same authorization starts Phase 1
and explicitly permits direct `golang.org/x/mod v0.19.0`; it does not authorize
later routes, evaluator contracts, prompts, extension behavior, or phases.

### 1. Preserve and deepen the evidence module

`internal/evidence.Preview(ctx, Request) (Result, error)` remains the primary
module interface. Callers provide a selection and fixed policy; the module owns
Git snapshot access, package/type analysis, context selection, budgeting,
ordering, and omission classification.

Commit and working-tree source access are two genuine production variations. A
private `snapshot` seam inside `internal/evidence` has exactly those two adapters
and hides bounded enumeration, file metadata, reads, and containment. The design
will not expose a general loader plugin interface solely for testing. Module
parsing, build selection, syntax parsing, type checking, context selection, and
omission policy remain inside the evidence implementation. Tests exercise the
real preview module with controlled repositories wherever possible;
adapter-level tests are limited to streaming, cancellation, and size behavior
that the external seam cannot observe reliably.

`BuildBundle` remains pure. It receives only the exact retained preview result
and performs no Git, filesystem, process, package-loader, or network work.

### 2. Require exact snapshot semantics

Every context fact for a commit range must be derived from the selected head
commit's complete relevant build inputs. Every fact for working-tree review must
use the existing working-tree selection semantics. The implementation must not
fill historical gaps from the current checkout.

The commit adapter enumerates the selected tree with streamed bounded
`git ls-tree -z` output, verifies Git object type and size, and reads permitted
blobs with `git cat-file`. The working-tree adapter enumerates Git-tracked plus
nonignored untracked files and performs bounded regular-file reads only after
canonical-root and symlink containment checks. Potentially large enumeration or
content does not use the existing full-buffer Git helper. Every `ContextGo` Git
read sets `GIT_NO_LAZY_FETCH=1`, `GIT_TERMINAL_PROMPT=0`, and
`GIT_OPTIONAL_LOCKS=0` so a partial clone cannot silently fetch promised objects,
prompt, or take an optional write lock. Context-disabled v1 callers keep their
existing inherited Git environment.

The shared in-memory analyzer locates the nearest selected-snapshot `go.mod`,
verifies nested module ownership, applies only repository-contained workspace and
replacement configuration, selects files with a custom `go/build.Context`, and
parses/type-checks with `go/parser`, `go/ast`, and `go/types`. The loader accounts
for added/deleted files, build constraints, symlinks, nested modules, workspaces,
and replacements without invoking a package driver. If it cannot establish
consistency, context is marked unavailable. It does not guess, mix snapshots, or
silently substitute a different selection.

Production analysis will not create a temporary worktree, checkout, source
archive, or other raw-source copy. If no loader meets the selected-snapshot rule
without such a copy, the implementation plan becomes blocked and this decision
must be reconsidered.

### 3. Limit the initial context to direct, repository-local evidence

Initial context is limited to changed packages, their direct repository-local
dependencies, and directly related facts:

- package identity and the applied build configuration;
- repository-local declarations directly referenced by changed declarations;
- type, method, interface, and implementation relationships proven by the
  accepted analysis mechanism;
- direct imported package identities needed to explain those references; and
- bounded repository-local excerpts whose relation to changed evidence is
  explicit.

The relation vocabulary will be closed and versioned. It will distinguish a
syntactic import from a type-proven reference or implementation. It will not
claim dynamic calls or runtime behavior.

Transitive dependency traversal, call graphs, whole-repository context,
deletion-side reconstruction, generated-code workflows, and external dependency
source are excluded. External and standard-library imports contribute only
bounded syntactic import identities. Their source, vendor copies, module-cache
content, GOROOT source, compiled export data, members, methods, and implementation
facts are not read or used. A private repository-local importer does not traverse
transitive imports.

Only facts backed by actual repository-local `types.Object` values may be marked
type-proven. Because `go/types` may continue with fake packages after importer
errors, `implements` is emitted only when every participating local package
type-checks without a relevant error. Partial type information never becomes a
fully proven relationship.

### 4. Preview and budget every model-visible fact

The enriched preview will separately show:

- changed-declaration evidence;
- changed-import evidence, including import-only changes;
- context items and their stable repository-relative identities;
- relationships from changed evidence to context items;
- the applied analysis/build configuration;
- fixed file, declaration, relationship, and byte limits;
- context status: complete, partial, or unavailable; and
- bounded omissions and aggregate truncation.

Every model-visible excerpt and derived fact is included in these budgets. Work
discovery also receives input-side limits and cancellation; the implementation
must not traverse unbounded input and discard excess only at serialization time.

Ordering and identities are deterministic and independent of absolute host
paths, temporary paths, timestamps, map iteration, or raw tool diagnostics.

The v2 item vocabulary distinguishes `changed_import` from
`context_declaration`. A changed import is usable evidence even when no changed
declaration excerpt exists, but it grants no access to imported source or type
facts.

The fixed input-side limits are:

```text
changed files:                    existing 20
module/workspace roots:                    8
analyzed packages:                        32
files per package:                        64
total analyzed files:                    160
directory entries per package:           256
source bytes per file:                256 KiB
aggregate analyzed source:              2 MiB
direct import edges:                      256
preview analysis deadline: existing 30 seconds
```

The fixed output-side limits are:

```text
context files:                              20
changed-import/context-declaration items:   40
context relationships:                     100
one context excerpt:                     4 KiB
aggregate repository-derived context:    64 KiB
complete serialized evaluator-input@2:  256 KiB
```

The aggregate context limit counts every variable repository-derived string, not
only excerpt bytes. Exceeding an input discovery cap makes context unavailable;
the implementation does not present an arbitrary discovered prefix as complete.
Output selection occurs after deterministic ordering and reports exact omitted
counts. Phase 1 must prove CPU, memory, cancellation, and serialized-size
behavior under these limits. Once this ADR is accepted, the fixed values are
compatibility-sensitive.

### 5. Make incompleteness explicit

The enriched result may contain valid changed evidence while context is partial
or unavailable. `complete` means every in-scope changed package and direct
repository-local dependency was analyzed without an omitted required fact.
`partial` means valid context remains but an external fact, CGO, outside-root
configuration, parse/type failure, or output limit prevents full coverage.
`unavailable` means the selected snapshot/module configuration cannot be safely
interpreted or an input discovery cap was exceeded.

The closed initial omission vocabulary is
`analysis_limit_exceeded`, `unsupported_module_layout`,
`unsupported_go_version`, `outside_repository_dependency`, `cgo_unsupported`,
`external_type_unavailable`, `context_parse_error`, `type_incomplete`, and
`output_truncated`. Reasons and counts are shown in the preview, retained in the
continuation, hashed into the v2 bundle, and sent to the evaluator. Raw loader,
Git, parser, and type-checker diagnostics are never used as omission text. A user
may proceed only after confirming the exact completeness state.

This is not a silent fallback. The enriched route never internally retries the
changed-only v1 route and never labels absent context as complete. The existing
v1 route remains an independent compatible path.

### 6. Use additive preview routes

The enriched path will use two new authenticated routes:

- `POST /v1/go-context-evidence-previews`
- `POST /v1/pi-session-go-context-evidence-previews`

Existing `/v1/evidence-previews` and `/v1/pi-session-evidence-previews` semantics
remain unchanged. Optional fields will not be added to those routes for enriched
evidence, because an older client could ignore the new preview while the server
sends unseen content to the model.

The existing question-set and assessment-turn routes may continue to use opaque
daemon-owned identifiers. The continuation records which evidence contract was
previewed, so the client cannot inject or change the mode after confirmation.
If strict protocol tests show that those routes require a public shape change,
the implementation must stop and amend the approved plan before proceeding.

### 7. Version bundles, evaluator inputs, and prompts

The enriched path will introduce:

- `evidence-bundle@2`;
- `evaluator-input@2`;
- `evaluator-assessment-input@2`; and
- immutable v2 question and assessment prompt assets.

The bundle manifest covers changed evidence, context evidence, relationships,
configuration, applied limits, completeness, omissions, and truncation. The
evaluator receives exactly that selected bundle and retains no tools or outside
knowledge. Evidence text remains untrusted content rather than instructions.

The existing v1 bundles, inputs, and released prompts remain available for
existing routes and clients. Question-set and assessment-turn output versions
remain unchanged only if their semantics and allowed values do not change.

### 8. Keep provider invocation after explicit confirmation

The updated Pi extension renders all enriched content, applied budgets,
completeness, omissions, model identity, and estimated cost before any provider
call. It sends only the opaque continuation after confirmation. The daemon then
uses the exact retained result; it does not reread the repository.

Cancellation before confirmation invokes no provider and creates no history
record. Partial or unavailable context requires the same explicit confirmation
as complete context.

### 9. Preserve history and Pi Session isolation

No source, excerpt, context item, relationship, evaluator input, answer, prompt,
or transcript is persisted. Existing source-free bundle-manifest hashes and
prompt provenance distinguish the enriched run. No database migration is
currently required.

Pi Session IDs remain only in daemon-owned preview continuation, assessment
provenance, and dedicated history-start state. They remain absent from both
evidence bundle versions, all evaluator inputs and prompts, RPC, model content,
logs, errors, and generic history responses. Session messages and metadata are
not inputs to Go context analysis.

### 10. Require local-only, repository-confined analysis

Evidence analysis will perform no network access. It must not trigger module,
checksum, toolchain, or custom-driver downloads. It must not follow `go.work`,
local replacements, symlinks, or other configuration into source outside the
canonical repository root.

Absolute paths, external source, environment secrets, VCS configuration, and raw
loader diagnostics do not enter previews, errors, logs, bundles, or model
content. Production analysis does not invoke the Go command or a custom package
driver, read module-cache, vendor, GOROOT, external source, or compiled export
data, or mutate build, module, or toolchain caches.

### 11. Pin dependency and build semantics

The only new direct dependency is `golang.org/x/mod v0.19.0`, used solely for
in-memory parsing of selected-snapshot module/workspace files and validation of
module/import paths before repository discovery. It is already selected
indirectly and present in `go.sum`, declares Go 1.18, and adds no new module to
the current graph; its required runtime packages remain within `x/mod`. Phase 1
makes it direct only after explicit dependency authorization. It does not add
`golang.org/x/tools`, choose `go/packages`, upgrade another module, or change the
Go 1.21 baseline.

Every enriched preview visibly records and hashes this fixed initial build
policy:

- `GOOS` and `GOARCH` are those of the running LearnLoop binary;
- CGO is disabled; a package importing `C` reports `cgo_unsupported`;
- custom build tags are empty;
- release/tool tags and the recorded toolchain version come from the toolchain
  used to build LearnLoop;
- the language version comes from a required `go` directive in the nearest
  selected-snapshot `go.mod`; a missing directive or a version newer than
  LearnLoop's compiled toolchain is unsupported rather than guessed;
- an in-repository selected-snapshot `go.work` may contribute only when it lists
  the changed module, and only repository-contained `use` targets are followed;
- local `replace` targets are followed only inside the canonical repository;
- `go.work` replacements take precedence over module replacements, and an
  external workspace replacement invalidates an earlier repository-local
  mapping while retaining only the replaced module identity;
- version-specific replacements are unsupported by the initial bounded loader
  and make context unavailable rather than guessing which requirement they
  affect;
- test variants are included only when the selected change includes `_test.go`;
  and
- ambient `GOWORK`, proxy, checksum, toolchain, custom-driver, and automatic
  toolchain-selection settings do not affect evidence.

A Go baseline upgrade, wider build-tag/CGO policy, external compiled-fact policy,
or changed fixed budget requires a separate compatibility review; none is
implicitly authorized by accepting this ADR.

## Consequences

### Positive

- Questions can be grounded in direct Go package/type relationships instead of
  asking about isolated changed excerpts.
- Import-only and cross-file changes can carry honest, visible context when the
  selected snapshot supports it.
- The evidence module becomes deeper: callers receive one bounded preview rather
  than coordinating Git, packages, types, budgets, and failure semantics.
- Additive routes preserve old clients and prevent unseen optional fields from
  crossing the model seam.
- Exact retention continues to prevent preview/evaluation time-of-check versus
  time-of-use drift.
- History and Pi Session provenance stay source-free and isolated.

### Negative

- Snapshot-correct Go analysis is substantially more complex and potentially
  more expensive than parsing changed files.
- The daemon must support v1 and v2 bundle/input/prompt paths concurrently.
- Fixed context limits and build-configuration semantics become compatibility
  commitments once shipped.
- Some repositories will produce partial or unavailable context because network,
  external paths, unsupported build configurations, or missing dependency facts
  are rejected.
- Phase 1 adds one direct `x/mod` requirement, even though that exact module and
  version are already selected indirectly.
- The no-source-copy loader is implemented and verified internally, but it adds
  bounded Git subprocess and in-memory parsing/type-checking cost before the
  enriched path is exposed.
- The initial loader fails closed for version-specific replacements and for a
  discovered workspace that does not list the changed module.

## Alternatives Considered

### Ask the model to infer context from changed excerpts

Rejected. It violates the selected-bundle-only policy, encourages outside
knowledge, and cannot distinguish repository facts from plausible guesses.

### Add optional context fields to existing preview responses

Rejected. ADR-0002 permits optional response fields, but an old client could
ignore them while a new daemon includes them in model input. The user would not
have previewed everything sent.

### Analyze historical changes against the current checkout

Rejected. Unchanged files and module/workspace configuration may differ, creating
mixed-snapshot facts that look authoritative.

### Create a temporary worktree, checkout, or source archive

Rejected for the proposed initial design. It writes another raw-source copy, can
leave residue after interruption, may mutate Git administrative state, and
broadens the privacy and cleanup contract.

### Use `go/packages` with an overlay

Rejected. The default v0.23.0 driver writes overlay source into temporary
backing files and invokes `go list -overlay`. This violates the no-source-copy
rule and delegates snapshot interpretation and cache behavior to the Go command.

### Use a custom `GOPACKAGESDRIVER`

Rejected for the initial design. It creates another subprocess interface, and
the inspected `go/packages` version invalidates export data when overlays exist,
which can expand source loading and type checking beyond the bounded local scope.
The private snapshot seam is smaller and has two actual adapters.

### Use `go/importer` for standard-library or external compiled facts

Rejected. The documented importer is not a reliable module loader, and its
compiler-data path may invoke `go list -export`, read GOROOT source, and mutate
the build cache. External imports remain syntactic identities only.

### Allow package loading to download missing modules or toolchains

Rejected. Evidence generation must remain local and predictable; automatic
network access changes privacy, latency, reproducibility, and cost semantics.

### Follow external `go.work` or `replace` paths

Rejected. Explicit Git selection authorizes the canonical repository, not
arbitrary neighboring source trees. Such dependencies are reported as omitted or
unavailable.

### Send third-party dependency source

Rejected. Direct external source greatly expands evidence scope and is not needed
to identify a syntactic import. External compiled facts are also excluded from
the initial policy because their read-only, cache-nonmutating behavior cannot be
guaranteed by the rejected loaders.

### Build an unbounded whole-repository or transitive call graph

Rejected. It adds discovery cost, produces speculative relationships, and
conflicts with the smallest-direct-context goal.

### Select `golang.org/x/tools` or upgrade Go simultaneously

Rejected. The initial design does not need `x/tools`; v0.25.0 requires Go 1.22
and v0.49.0 requires Go 1.25. Upgrading the Go baseline or adding a broader
loader dependency is a separate compatibility decision.

### Expose a general package-loader interface

Rejected for now. Commit and working-tree adapters are private implementation
variations. A public or broadly injected loader abstraction would expose
complexity without a second external consumer or implementation.

### Persist an enriched analysis cache or background index

Rejected. It creates new source-derived durable state, invalidation rules,
privacy exposure, and lifecycle complexity before foreground bounded analysis is
proven insufficient.

### Prioritize richer editing or a durable worker first

Not selected. Richer editing improves interaction but not evidence quality. A
durable job system is broader infrastructure without current evidence that
foreground execution is the limiting product problem. Neither alternative
addresses the evaluator's missing repository context.

## Acceptance Conditions

The chosen loader, exact dependency, Go baseline, build configuration, external
fact policy, fixed limits, omission taxonomy, and Phase 1 proof strategy are
recorded. Acceptance and Phase 1 dependency authorization were explicit. Later
high-risk phases in `plans/go-evidence-context-enrichment.md` still require
separate authorization.

## References

- `AGENTS.md`
- `PROJECT.md`
- `plans/go-evidence-context-enrichment.md`
- `docs/decisions/ADR-0002-local-daemon-protocol-security.md`
- `docs/decisions/ADR-0003-post-preview-evaluator-boundary.md`
- `docs/decisions/ADR-0004-answer-assessment-lifecycle.md`
- `docs/decisions/ADR-0005-local-learning-history.md`
- `docs/decisions/ADR-0006-explicit-pi-session-provenance.md`
- `agent/README.md`
- `agent/policies/evaluator-capabilities.json`
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
