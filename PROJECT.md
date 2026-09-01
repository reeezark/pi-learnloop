# Project Overview

Pi LearnLoop is a planned local learning companion for Go developers using the Pi coding agent. After a developer manually selects one or more Agent-produced changesets, the tool is intended to analyze the real Go code changes and conduct a short, evidence-backed technical interview. The purpose is to verify that the developer can explain the code rather than merely accept AI-generated output.

Current repository status: Agent-development governance and the deterministic Phase 2 post-preview learning slice accepted in ADR-0003. The `pi-learnloop daemon` command exposes explicit commit-range and working-tree previews, retains the exact bounded result behind a five-minute single-use in-memory continuation, and serves strict authenticated question-set requests. A thin Pi 0.84.x TypeScript extension registers manual `/learn`, renders the preview first, asks for explicit confirmation, and renders two code-specific questions plus one Go/backend question. The current evaluator adapter is deterministic and local; it exercises `BuildBundle`, `evaluator-input@1`, and strict `evaluator-question-set@1` validation without a Pi process or model call. The released prompt and fixed isolated Pi 0.84.3 argument contract exist, but the production RPC adapter, persistence, SSE, answer collection, and later learning workflow have not been implemented.

# Project Goals

- Let the user manually select unreviewed Pi Sessions or Git changesets with `/learn`.
- Map changed Go code to real functions, methods, types, interfaces, tests, and dependencies.
- Ask two code-specific questions, one underlying Go/backend question, and at most one targeted follow-up.
- Record a repository-scoped result as `understood`, `partial`, or `review_needed`, backed by code evidence.
- Keep execution and learning records local by default.
- Provide a reliable local task engine that can recover after process interruption.
- Remain suitable for public, long-term development as an MIT-licensed project.

# Non-Goals

The initial product scope does not include:

- Automatic learning reminders or Git hooks.
- Coding exercises or generated bug-fixing challenges.
- Spaced repetition or automatic review scheduling.
- A web interface.
- Multi-Agent orchestration or remote Agent control.
- Languages other than Go.
- Linux or Windows support.
- Team collaboration or cloud synchronization.
- Automatic resume generation.

# Tech Stack

The Go module uses only the standard library and the local `git` executable. The broader approved target remains partly unimplemented:

| Area | Target | Current Status |
| --- | --- | --- |
| Local service | Go 1.21 module | Evidence core and foreground `pi-learnloop daemon` command implemented with the standard library |
| Pi integration | Thin TypeScript Pi extension | Manual preview, explicit local-evaluation confirmation, active model metadata propagation, and three-question rendering implemented against Pi 0.84.3 |
| Agent integration | Pi RPC with an isolated no-tools evaluator Session | Runtime contracts, released prompt, evaluator interface, and deterministic local adapter implemented; production RPC process adapter remains planned |
| Storage | Local SQLite in WAL mode | Planned |
| Go analysis | `go/parser`, later `go/types` and `golang.org/x/tools/go/packages` | Syntax-level changed-declaration mapping and an evaluator-ready bundle projection are implemented; type/dependency analysis remains planned |
| Extension transport | Local HTTP plus later SSE on `127.0.0.1` | Versioned authenticated evidence-preview and single-use question-set operations implemented; SSE not implemented |
| Supported platform | macOS ARM64 and AMD64 | macOS ARM64 verified locally; AMD64 remains unverified |

# Repository Structure

The repository currently contains Agent-development governance, the evidence/continuation/question implementation, evaluator contracts, and validation:

```text
.
├── AGENTS.md
├── PROJECT.md
├── README.md
├── go.mod
├── package.json
├── package-lock.json
├── tsconfig.json
├── cmd/pi-learnloop/
│   ├── main.go
│   └── main_test.go
├── internal/daemon/
│   ├── daemon.go
│   ├── daemon_test.go
│   ├── continuation.go
│   ├── continuation_test.go
│   ├── question_set_test.go
│   ├── runtime.go
│   ├── server.go
│   └── server_test.go
├── internal/evidence/
│   ├── bundle.go
│   ├── bundle_test.go
│   ├── evidence.go
│   └── evidence_test.go
├── internal/evaluator/
│   ├── contract.go
│   ├── contract_test.go
│   ├── evaluator.go
│   ├── evaluator_test.go
│   ├── pi_contract.go
│   └── pi_contract_test.go
├── extensions/
│   ├── pi-learnloop.ts
│   └── lib/
│       ├── daemon-client.ts
│       └── learn-command.ts
├── tests/extension/
│   ├── daemon-client.test.ts
│   ├── extension-entry.test.ts
│   └── learn-command.test.ts
├── agent/
│   ├── README.md
│   ├── prompts/
│   │   ├── README.md
│   │   └── evaluator-question-generation/v1.0.0.md
│   ├── policies/evaluator-capabilities.json
│   ├── schemas/
│   │   ├── eval-case.schema.json
│   │   └── run-record.schema.json
│   ├── evals/
│   │   ├── README.md
│   │   └── cases/
│   └── fixtures/run-record/
├── plans/
│   ├── README.md
│   ├── agent-development-foundation.md
│   ├── changeset-evidence-preview.md
│   ├── evaluator-ready-evidence-bundle.md
│   └── post-preview-evaluator-adapter.md
├── scripts/
│   ├── test-agent-infra.sh
│   └── validate-agent-infra.sh
└── docs/
    ├── checkpoints/
    │   ├── README.md
    │   ├── agent-development-foundation-phase-2.md
    │   ├── changeset-evidence-preview-phase-1.md
    │   ├── changeset-evidence-preview-phase-2.md
    │   ├── changeset-evidence-preview-phase-3.md
    │   ├── evaluator-ready-evidence-bundle-phase-1.md
    │   ├── post-preview-evaluator-adapter-phase-1.md
    │   └── post-preview-evaluator-adapter-phase-2.md
    └── decisions/
        ├── README.md
        ├── ADR-0001-agent-development-lifecycle.md
        ├── ADR-0002-local-daemon-protocol-security.md
        └── ADR-0003-post-preview-evaluator-boundary.md
```

There is currently one released production question-generation prompt and one deterministic local Phase 2 adapter, but no evaluator process adapter, database, release publication, or CI configuration. The repository has a public README and an installable local Pi package manifest. The package has one Pi-provided peer dependency and three exact development dependencies; it has no third-party runtime npm dependency. The foreground daemon and extension form a development-ready local slice, not a published release.

# Core Modules

The implemented core modules and adapters are:

- `internal/evidence`: one deep module whose `Preview` interface resolves explicit Git selections, parses zero-context diffs, maps changed lines to Go declarations, applies and retains caller-provided evidence limits, and returns stable errors plus explicit omission/truncation metadata. Its pure `BuildBundle` interface accepts only that bounded result and produces deterministic `E001`-style citations, content hashes, exact byte counts, copied coverage metadata, and a content-addressed manifest without reading more source or exposing the absolute repository root. Tests exercise both the in-memory seam and real temporary Git repositories rather than an invented Git port.
- `internal/evaluator`: a provider-independent boundary. `NewInput` validates a complete `evidence.Bundle` and owns a JSON-safe copy without a repository root; `ParseQuestionSet` accepts only one bounded strict JSON object with fixed Q1/Q2/Q3 kinds and valid bundle references; `QuestionEvaluator` accepts only that input plus validated non-secret model metadata; the Phase 2 deterministic adapter exercises the same validator without a process or provider. `BuildPiArguments` freezes the future isolated Pi 0.84.3 argument list without resolving or spawning a process.
- `internal/daemon`: one deep local-runtime module whose `Run` interface owns protected runtime discovery, per-start Instance Tokens, single-instance locking, loopback HTTP lifecycle, strict protocol decoding, stable error translation, graceful shutdown, and an in-memory continuation store. Preview may retain an owned copy for five minutes under fixed 8-entry/1-MiB limits; `/v1/question-sets` atomically consumes it before `BuildBundle` and evaluation, so no repository reread or duplicate evaluation can occur.
- `cmd/pi-learnloop`: the minimal public executable adapter. It accepts only `pi-learnloop daemon`, installs `SIGINT`/`SIGTERM` cancellation, and exposes no flags that weaken accepted security defaults.
- `extensions/lib/daemon-client.ts`: one deep local client module whose preview and question interfaces hide protected runtime-file validation, exact loopback URL validation, instance verification, proxy-independent HTTP, Instance Token authentication, bounded response decoding, and v1 schema validation. Preview discovery races retry once; continuation never retries.
- `extensions/lib/learn-command.ts`: the `/learn` interaction module. It accepts only explicit supported selections, renders files, symbols, byte volume and truncation before evaluation, validates active Pi model metadata, requires explicit confirmation, and renders the strict three-question result without starting an Agent turn.
- `extensions/pi-learnloop.ts`: the minimal Pi adapter. It registers only the manual `/learn` command and contains no evidence, persistence, or evaluation rules.

The remaining target architecture identifies these unimplemented responsibilities:

- Later Pi extension behavior: explicit Session selection and answer collection beyond the implemented Git-selection and question-display flow.
- Later daemon capabilities: SSE event delivery, durable worker coordination, and learning lifecycle management beyond the implemented evidence-preview request.
- Changeset capture: association among repositories, Pi Sessions, commits, and working-tree changes.
- Go evidence enrichment: type/dependency analysis and any expansion beyond the bytes represented in the current preview. The syntax-level evaluator-ready bundle is product-connected, but evidence expansion remains out of scope.
- Assessment engine: isolated Pi RPC process, answer collection, follow-up, and evaluation. Continuation state, deterministic question generation, and question delivery are implemented.
- Persistence: SQLite migrations, durable jobs, leases, event cursors, concepts, questions, and answers.

`TODO / Need Confirmation`: assign extension, assessment, and persistence module paths only through their approved implementation phases.

# High-Level Architecture

Current implemented seam and later target integrations:

```text
Pi TUI extension
    │ Runtime Descriptor + Instance Token
    │ versioned local HTTP; SSE planned
    ▼
cmd/pi-learnloop
    ▼
internal/daemon
    │ authenticated preview + in-memory single-use continuation
    ▼
internal/evidence
    │ pure BuildBundle → evaluator-input@1
    ▼
internal/evaluator
    │ deterministic local adapter (current)

Isolated Pi RPC evaluator and SQLite jobs (planned)
```

`internal/evidence.BuildBundle` and `internal/evaluator.NewInput` are connected only after an atomic continuation consume. They deliberately have no repository path or context parameter, so they cannot read additional content after the user-visible preview. The runtime input and strict question result are versioned evaluator contracts, not daemon HTTP or persisted formats.

The Pi extension should remain thin. Business rules, persistence, analysis, and reliable execution belong in the Go daemon. The evaluator must be separated from the development Session and restricted to read-only evidence.

# Main Data Flow

Implemented internal flow:

1. A caller supplies a repository path, an explicit commit range or working-tree selection, and positive evidence limits.
2. The module resolves the canonical Git root and commit hashes.
3. Git zero-context diffs identify changed Go files and new-side line ranges; untracked, non-ignored Go files are included for working-tree selections.
4. `go/parser` maps changed ranges to functions, methods, types, interfaces, variables, and constants.
5. The module returns stable ordering, bounded UTF-8 excerpts, the exact applied limits, truncation counts, and explicit reasons for changes that cannot map to new declarations.
6. After explicit continuation, the pure bundle builder validates the retained budget and structure, excludes empty evidence, assigns stable citations, hashes exact content and a canonical metadata-only manifest, and omits the absolute repository root.

Implemented daemon flow:

1. `pi-learnloop daemon` acquires the protected single-instance runtime lock and binds `tcp4` to `127.0.0.1:0`.
2. It writes a per-start Instance Token and non-secret Runtime Descriptor under `os.UserConfigDir()/pi-learnloop/runtime`.
3. A local client validates the descriptor against unauthenticated `GET /v1/status`, reads the protected token, and sends `POST /v1/evidence-previews` with the custom `PiLearnLoop` authorization scheme.
4. The daemon rejects non-loopback peers, an unadvertised Host, any non-empty Origin, invalid authentication, ambiguous JSON, oversized bodies, invalid selections, and unsafe evidence paths.
5. The daemon applies fixed evidence caps of 20 files, 100 declarations, and 128 KiB of aggregate excerpts, then calls `internal/evidence`. When usable evidence and the current evaluator are available, it retains an owned copy behind a cryptographically random five-minute continuation; empty or capacity-limited previews remain successful with an unavailable reason.
6. The extension renders the preview, validates Pi 0.84.3's active non-secret model identity, and asks for explicit confirmation. Decline sends no continuation request.
7. Authenticated `POST /v1/question-sets` strictly validates a 4-KiB request, atomically consumes the continuation, builds the bundle from that exact value without repository access, and invokes the deterministic local adapter. Concurrent, expired, used, malformed, wrong-instance, and post-restart IDs share `409 continuation_unavailable`.
8. The strict result is exactly Q1/Q2 `code_specific` plus Q3 `go_backend`, or `insufficient_evidence`; the extension validates and renders it. The client never retries continuation.
9. Cancellation reaches Git subprocesses; `SIGINT` or `SIGTERM` triggers bounded graceful shutdown, in-memory continuation cleanup, and instance-aware runtime-file cleanup.

Target end-to-end flow beyond the deterministic Phase 2 slice:

1. The user manually invokes `/learn`; there is no automatic reminder or background Session indexing.
2. The user explicitly selects a Git changeset, or later an unreviewed Session after that capability is designed.
3. The implemented extension and daemon produce the bounded, inspectable evidence preview and explicit continuation.
4. The implemented deterministic adapter is replaced by an isolated Pi RPC adapter using the released `evaluator-question-generation@1.0.0` prompt; it must preserve the same exact-retained-evidence and strict question-set boundary.
5. A later daemon creates an idempotent assessment job.
6. The user answers in the Pi TUI; the evaluator may ask one targeted follow-up.
7. The evaluator returns a structured result with evidence-backed weaknesses.
8. The daemon stores the repository-scoped assessment result locally.

# External Dependencies

Current runtime dependencies:

- the local `git` executable;
- Go 1.21 or a compatible newer Go toolchain;
- Node.js `>=22.19.0` and Pi 0.84.x for the extension;
- Pi-provided peer `@earendil-works/pi-coding-agent: "*"`, which is not bundled.

Exact extension development dependencies:

- `@earendil-works/pi-coding-agent@0.84.3` (MIT);
- `@types/node@22.19.19` (MIT);
- `typescript@5.9.3` (Apache-2.0).

The extension uses Node's built-in test runner and built-in filesystem and HTTP modules. There is no third-party runtime npm dependency.

There are no third-party Go modules and no `go.sum`.

Planned integrations:

- Pi coding agent extension and RPC interfaces.
- SQLite accessed by the Go service.
- Go analysis packages required by the approved implementation.

Future SQLite, evaluator, or Go-analysis dependencies remain `TODO / Need Confirmation` until introduced through separately approved plans.

# Engineering Constraints

- Learning starts only through an explicit user command.
- The initial analyzer supports Go repositories only.
- The first release supports macOS ARM64 and AMD64 only.
- The evaluator is independent from the Session that produced the code.
- The evaluator must not receive edit or unrestricted command tools.
- The daemon listens only on loopback and authenticates clients with a local secret.
- The implemented v1 listener is IPv4-only `127.0.0.1` on an operating-system-assigned port; non-loopback transport requires a new ADR.
- Runtime directories use mode `0700`; descriptor, token, and lock files use `0600`; ambiguous ownership, permissions, or symlinks fail closed.
- API credentials remain managed by Pi; the Go daemon must not persist them.
- Only the selected evidence bundle is sent to the configured evaluator model.
- Users can inspect the files and approximate code volume before evaluation.
- Learning data remains local and no telemetry is uploaded by default.
- Changes must favor a small, testable architecture over speculative extensibility.

# Compatibility Requirements

- Preserve stored learning records across compatible daemon upgrades once a storage format is released.
- Treat database migrations and Pi extension/daemon protocol changes as compatibility-sensitive.
- Treat ADR-0002 descriptor fields, `/v1` JSON fields, error codes, fixed evidence caps, and Instance Token behavior as compatibility-sensitive.
- Treat ADR-0003's runtime evaluator schemas, released prompt version, exact Pi 0.84.3 support, fixed question order, and deny-argument contract as compatibility-sensitive.
- Do not change command behavior, assessment labels, configuration defaults, or evidence-sharing behavior silently.
- macOS ARM64 and AMD64 are the only initial compatibility targets.
- Linux, Windows, multi-language analysis, and team use are future possibilities, not current compatibility promises.

# Development Workflow

For medium- and high-risk work:

1. Inspect code, manifests, existing plans, ADRs, checkpoints, and Git state.
2. Write a task plan in `plans/` using the repository template and lifecycle metadata.
3. Resolve design dependencies and compatibility questions before implementation.
4. Implement in reviewable phases with the smallest viable change set.
5. Run the repository-supported focused tests and broader checks required by risk.
6. Review the complete diff and update stable documentation when facts change.
7. Record a checkpoint when pausing, changing Sessions, or handing work to another Agent.
8. Create an ADR only for a long-lived architecture or compatibility decision.

# Testing Strategy

Agent-development infrastructure has two dependency-free verification commands:

```text
scripts/test-agent-infra.sh
scripts/validate-agent-infra.sh
```

The implemented Go modules have these authoritative checks:

```text
go test ./...
go test -race ./...
go vet ./...
```

The implemented Pi extension has these manifest-derived checks:

```text
npm run typecheck
npm test
```

The Agent validator checks four synthetic evaluator failure categories, released prompt metadata and safety requirements, the deny-by-default capability policy, stable asset versions, and privacy-safe run provenance without calling a model.

Current Go evidence tests cover commit ranges, working-tree and untracked files, declaration kinds and method identity, rename/deletion outcomes, non-Go changes, malformed source, repository escape protection, stable error codes, deterministic evidence limits, exact Preview-to-Bundle projection, citation ordering, test classification, canonical manifest hashing, privacy-safe path handling, and fail-closed budget/structure/insufficiency behavior. Evaluator contract tests cover bundle integrity and copy ownership, strict JSON and duplicate-key rejection, fixed question shape and references, insufficient evidence, output/error bounds, and the exact isolated Pi argument/model mapping without spawning Pi.

Current daemon integration tests cover loopback discovery, commit-range and working-tree previews through real Git repositories, fixed evidence caps, token authentication, Host/Origin/CORS defenses, strict JSON, size and media-type limits, stable safe errors, single-instance locking, runtime permissions, symlink rejection, token rotation, stale-state replacement, cancellation propagation, and shutdown cleanup. Phase 2 tests additionally cover exact retained working-tree evidence after the source is changed, five-minute expiry, count/byte capacity, owned copies, single and concurrent consume, strict question requests, protected routing, empty evidence, and restart loss. Command tests verify that no unsupported security-weakening arguments are accepted.

Current extension tests cover manual command registration, explicit commit-range and working-tree selections, trust/UI/input gates, empty previews, approximate UTF-8 excerpt volume, truncation display, recoverable errors, protected discovery paths, exact IPv4-loopback descriptor validation, status-before-token ordering, environment-proxy bypass, authenticated requests, one preview discovery retry, stable daemon error propagation, response limits, and v1 success-schema validation. Phase 2 tests cover decline-without-request, missing model metadata, exact model propagation, three-question rendering, invalid result rejection, and zero continuation retries.

The remaining target strategy includes:

- Unit coverage for changeset selection, Go AST mapping, concept normalization, and evaluation schemas.
- Transactional tests for SQLite jobs, leases, retries, idempotency, and event cursors.
- Integration tests using a fake Pi RPC evaluator rather than live paid model calls.
- End-to-end fixture repositories containing representative Go changesets.
- Race testing for future worker and streaming code.
- Crash-recovery tests that restart the daemon during each durable job state.
- Packaging smoke tests for supported macOS architectures.

`TODO / Need Confirmation`: add persistence, evaluator, release packaging, and cross-architecture commands only after their relevant manifests and scripts exist.

# Known Risks / Important Notes

- The Pi extension and RPC APIs may evolve; isolate them behind narrow adapters.
- Pi 0.84.3's published declaration graph contains upstream type-resolution errors under full library checking, so this project uses `skipLibCheck` while retaining strict checking for its own TypeScript sources.
- LLM evaluation can be inconsistent; require structured output, code evidence, and repeatable fixtures.
- The capability policy is a required development contract, not runtime enforcement; future adapters must prove enforcement through implementation tests.
- Mapping multiple Sessions to one changeset can be ambiguous; selection must remain explicit and inspectable.
- Source code privacy depends on evidence minimization, repository ignore rules, and a clear pre-evaluation preview.
- The current deterministic adapter is a Phase 2 contract fixture, not a model-backed assessment. It must be replaced, not supplemented with a second execution path, when Phase 3 is authorized.
- A durable SQLite worker queue can become unnecessary complexity if the state machine is not kept small.
- Syntax parsing does not yet provide type or dependency information, and deletion-only hunks are reported explicitly rather than reconstructed from the base-side declaration.
- On this macOS 26 development machine, the default Homebrew Go 1.21.13 external linker aborts network-enabled test binaries with `missing LC_UUID`; Go 1.21 passes with `CGO_ENABLED=0` or `-tags netgo`, and the unmodified test, race, vet, and build commands pass with the already-installed Go 1.26.4 toolchain. The module language version remains Go 1.21.
