# Project Overview

Pi LearnLoop is a local learning companion for Go developers using the Pi coding agent. Its implemented core lets a developer manually select an Agent-produced Git changeset, analyze the real Go code changes, complete a short evidence-backed technical interview, and inspect source-free local outcomes. The purpose is to verify that the developer can explain the code rather than merely accept AI-generated output.

Current repository status: Agent-development governance, the complete three-phase post-preview question-generation slice accepted in ADR-0003, the complete three-phase volatile answer-assessment slice accepted in ADR-0004, the complete three-phase durable-learning-history slice accepted in ADR-0005, the complete three-phase explicit Pi Session provenance slice accepted in ADR-0006, and the completed Phase 1 snapshot/type-analysis proof plus Phase 2 daemon/evaluator contracts accepted in ADR-0007. The daemon now exposes separate strict authenticated Go-context preview routes for generic and explicit Pi Session selections, retains the exact bounded enriched result, and selects `evidence-bundle@2`, `evaluator-input@2`, `evaluator-assessment-input@2`, and immutable v2 prompts entirely from daemon-owned continuation state. Existing preview routes and their v1 bundles, inputs, prompts, outputs, and behavior remain unchanged. The Pi extension is not yet connected to the new routes, so current `/learn` behavior remains the existing v1 changed-declaration path until Phase 3 visibly renders and confirms all enriched evidence. The `pi-learnloop daemon` also serves strict authenticated question, assessment, repository-history, Session-bound preview, and completion-only Session-review requests. A thin Pi TypeScript extension registers manual `/learn` and `/learn-history`: `/learn` supports the existing direct Git selections plus a manual current-cwd Pi Session path that immediately projects the newest at most 20 results to unique bounded IDs, filters completed reviews once, requires an explicit Git association, renders the preview and association first, then preserves the existing model-sharing confirmations, Q1/Q2/Q3 plus at most one F1, and evidence-backed feedback with a deterministic Go-derived label; `/learn-history` retrieves the 20 newest source-free records for the current canonical repository without a model call. Question generation and each assessment turn use a fresh no-session, no-tools Pi 0.84.3 RPC process with schema-selected released embedded prompts and shared private isolation mechanics. The daemon owns the protected `internal/history` SQLite lifecycle. Schema v2 stores only a nullable bounded Pi Session ID; the dedicated daemon path keeps it outside evidence and model-visible values, propagates it to Session-aware history, and returns only completed candidate IDs for the canonical repository. The direct Git path stores SQL `NULL`. History still records only source-free running/F1/terminal facts, recovers unfinished rows as interrupted without an evaluator call, and reports save failure without hiding or retrying a successful assessment. Pi 0.84.3's accepted list-time privacy/resource limitation remains explicit: it materializes unused candidate messages and metadata in extension memory before LearnLoop can retain only IDs. Enriched extension UI, SSE, and durable worker coordination remain unimplemented.

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

The Go module uses the standard library, the local `git` executable, direct `golang.org/x/mod v0.19.0` for in-memory selected-snapshot module/workspace parsing and path validation, and the approved pure-Go `modernc.org/sqlite v1.35.0` driver. The broader approved target remains partly unimplemented:

| Area | Target | Current Status |
| --- | --- | --- |
| Local service | Go 1.21 module | Evidence core, foreground `pi-learnloop daemon`, and daemon-owned history lifecycle use the approved pure-Go SQLite driver |
| Pi integration | Thin TypeScript Pi extension | Manual Git or bounded current-cwd Session selection, explicit Session-to-Git binding, preview, model-sharing confirmations, three-answer collection, optional F1, and final-result rendering implemented against Pi 0.84.3 |
| Agent integration | Pi RPC with an isolated no-tools evaluator Session | Production Pi 0.84.3 question and assessment adapters, released embedded prompts, strict JSONL/output bounds, and fake-process verification implemented |
| Storage | Local SQLite in WAL mode | Protected schema-v2 history with nullable bounded Pi Session provenance, daemon assessment-lifecycle recording, bounded authenticated repository and completion-only Session queries, and manual generic-history UI rendering implemented |
| Go analysis | `go/parser`, `go/build`, `go/types`, and `golang.org/x/mod/modfile` | Product v1 uses syntax-level changed declarations; separate daemon routes and v2 evaluator contracts expose the bounded selected-snapshot context mode, while extension rendering remains Phase 3 work; `go/packages` is intentionally not used |
| Extension transport | Local HTTP plus later SSE on `127.0.0.1` | The daemon implements versioned authenticated v1 and additive Go-context preview routes, Session review, single-use question, assessment, and bounded history operations; the extension client still uses v1 previews and SSE is not implemented |
| Supported platform | macOS ARM64 and AMD64 | macOS ARM64 verified locally; AMD64 remains unverified |

# Repository Structure

The repository currently contains Agent-development governance, the evidence/continuation/question implementation, evaluator contracts, and validation:

```text
.
├── AGENTS.md
├── PROJECT.md
├── README.md
├── go.mod
├── go.sum
├── package.json
├── package-lock.json
├── tsconfig.json
├── cmd/pi-learnloop/
│   ├── main.go
│   └── main_test.go
├── internal/daemon/
│   ├── assessment_test.go
│   ├── daemon.go
│   ├── daemon_test.go
│   ├── history_query_integration_test.go
│   ├── history_query_test.go
│   ├── continuation.go
│   ├── continuation_test.go
│   ├── go_context_preview.go
│   ├── go_context_preview_test.go
│   ├── pi_session_preview_test.go
│   ├── pi_session_provenance_test.go
│   ├── pi_session_review_query_test.go
│   ├── question_set_test.go
│   ├── runtime.go
│   ├── server.go
│   └── server_test.go
├── internal/assessment/
│   ├── history_test.go
│   ├── service.go
│   └── service_test.go
├── internal/evidence/
│   ├── bundle.go
│   ├── bundle_test.go
│   ├── bundle_v2.go
│   ├── bundle_v2_test.go
│   ├── evidence.go
│   ├── evidence_test.go
│   ├── go_context.go
│   ├── go_context_internal_test.go
│   ├── go_context_test.go
│   ├── snapshot.go
│   └── snapshot_test.go
├── internal/evaluator/
│   ├── assessment_contract.go
│   ├── assessment_contract_test.go
│   ├── assessment_evaluator.go
│   ├── assessment_evaluator_test.go
│   ├── contract.go
│   ├── contract_test.go
│   ├── contract_v2.go
│   ├── contract_v2_test.go
│   ├── evaluator.go
│   ├── evaluator_test.go
│   ├── pi_contract.go
│   ├── pi_contract_test.go
│   ├── pi_rpc.go
│   ├── pi_rpc_assessment_test.go
│   ├── pi_rpc_version_test.go
│   └── pi_rpc_test.go
├── internal/history/
│   ├── migrations/
│   │   ├── 001_initial.sql
│   │   └── 002_pi_session_provenance.sql
│   ├── store.go
│   ├── records.go
│   └── focused protection, migration, lifecycle, recovery, and privacy tests
├── extensions/
│   ├── pi-learnloop.ts
│   └── lib/
│       ├── daemon-client.ts
│       └── learn-command.ts
├── tests/extension/
│   ├── daemon-client.test.ts
│   ├── extension-entry.test.ts
│   ├── learn-command.test.ts
│   └── pi-session-review.test.ts
├── agent/
│   ├── README.md
│   ├── prompts/
│   │   ├── assets.go
│   │   ├── assets_test.go
│   │   ├── README.md
│   │   ├── evaluator-question-generation/v1.0.0.md and v2.0.0.md
│   │   └── evaluator-answer-assessment/v1.0.0.md and v2.0.0.md
│   ├── policies/evaluator-capabilities.json and go-context-evidence.json
│   ├── schemas/
│   │   ├── eval-case.schema.json
│   │   ├── evidence-bundle-v2.schema.json
│   │   ├── evaluator-input-v2.schema.json
│   │   ├── evaluator-assessment-input-v2.schema.json
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
│   ├── post-preview-evaluator-adapter.md
│   ├── answer-assessment-workflow.md
│   ├── durable-learning-history.md
│   ├── explicit-pi-session-review.md
│   └── go-evidence-context-enrichment.md
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
    │   ├── post-preview-evaluator-adapter-phase-2.md
    │   ├── post-preview-evaluator-adapter-phase-3.md
    │   ├── answer-assessment-workflow-phase-1.md
    │   ├── answer-assessment-workflow-phase-2.md
    │   ├── answer-assessment-workflow-phase-3.md
    │   ├── durable-learning-history-phase-1.md
    │   ├── durable-learning-history-phase-2.md
    │   ├── durable-learning-history-phase-3.md
    │   ├── explicit-pi-session-review-phase-1.md
    │   ├── explicit-pi-session-review-phase-2.md
    │   ├── explicit-pi-session-review-phase-3.md
    │   ├── go-evidence-context-enrichment-phase-1.md
    │   └── go-evidence-context-enrichment-phase-2.md
    └── decisions/
        ├── README.md
        ├── ADR-0001-agent-development-lifecycle.md
        ├── ADR-0002-local-daemon-protocol-security.md
        ├── ADR-0003-post-preview-evaluator-boundary.md
        ├── ADR-0004-answer-assessment-lifecycle.md
        ├── ADR-0005-local-learning-history.md
        ├── ADR-0006-explicit-pi-session-provenance.md
        └── ADR-0007-snapshot-consistent-go-context-evidence.md
```

There are currently four released production prompts: v1 and v2 question and assessment assets selected by validated runtime input version. Narrow production Pi RPC adapters, deterministic fixtures for both evaluator seams, and a daemon-connected SQLite history module with a manually triggered repository query UI are implemented, but package publication and CI configuration are not. The repository has a public README and an installable local Pi package manifest. The package has one Pi-provided peer dependency and three exact development dependencies; it has no third-party runtime npm dependency. Source-bearing assessment state remains volatile while the allowlisted learning outcome is durable and locally queryable when storage is available.

# Core Modules

The implemented core modules and adapters are:

- `internal/evidence`: one deep module whose narrow `ResolveRepositoryRoot` interface verifies and canonicalizes a Git working-tree root for preview and history lookup. Its `Preview` interface resolves explicit Git selections, parses zero-context diffs, maps changed lines to Go declarations, applies and retains caller-provided evidence limits, and returns stable errors plus explicit omission/truncation metadata. Its pure `BuildBundle` interface accepts only that bounded result and produces deterministic `E001`-style citations, content hashes, exact byte counts, copied coverage metadata, and a content-addressed manifest without reading more source or exposing the absolute repository root. Tests exercise both the in-memory seam and real temporary Git repositories rather than an invented Git port.
- `internal/evidence` also owns an opt-in Go-context mode behind the same `Preview` seam. Private commit and working-tree snapshots feed one in-memory `go/build`/`go/types` analyzer with bounded selected-snapshot module/workspace data parsed by `x/mod/modfile`; only changed packages and direct repository-local imports are considered. It emits deterministic changed-import/context-declaration items, syntactic/type-checked relations, fixed build/limit metadata, and closed partial/unavailable reasons. It invokes no Go package driver, reads no vendor/module-cache/GOROOT/external source, follows no outside-root configuration or symlink, and writes no source copy or cache. Context-mode Git reads disable promised-object lazy fetch, terminal prompting, and optional locks without changing the context-disabled v1 path. Pure `BuildBundleV2` validates and hashes every changed/context item, relation, build value, limit, omission, and completeness field without rereading the repository; `BuildBundle` continues to reject enriched results so v1 cannot silently carry context.
- `internal/evaluator`: a provider-independent boundary. `NewInput` and `NewInputV2` validate and own JSON-safe copies of their respective bundle versions without a repository root or Session identifier; v2 also enforces the fixed 256-KiB complete serialized-input cap. `ParseQuestionSet` and assessment outputs remain strict v1 contracts while accepting validated E- and C-series references from the selected input version. The production question and assessment adapters freeze a symlink-resolved `pi` path after an exact startup version preflight and select immutable v1 or v2 prompts solely from the validated runtime schema. Each call starts one isolated process without a shell, disables retries/compaction and every discovered capability, sends one LF-framed input, waits for `agent_settled`, validates one final assistant text, applies fixed stream/deadline bounds, and always terminates/reaps the child.
- `internal/assessment`: the answer-lifecycle module. `Start`, `Submit`, and `Close` own validated evaluator values, a fixed model selection, server-owned repository/prompt provenance plus an optional bounded Session ID, cryptographic instance-local capabilities, 30-minute expiry, eight-entry/1-MiB capacity, atomic initial/F1 transitions, safe history lifecycle writes, failure invalidation, cleanup, and deterministic label derivation. Session provenance bypasses evaluator values and is used only by the dedicated history start. Source-bearing values remain volatile; the client cannot supply repository, evidence, questions, model, prompt, credential, or executable provenance.
- `internal/history`: a deep SQLite module. `Open`, `Create`, `CreateWithPiSession`, `MarkFollowUp`, `Complete`, `Fail`, `List`, `ReviewedPiSessionIDs`, and `Close` hide protected path creation, same-owner/mode/symlink/hard-link/local-filesystem checks, one verified connection, ordered forward-only embedded schema migration, exact schema and stored-value validation, immediate transactions, idempotent terminal writes, repository-scoped bounded reads, completion-only Pi Session lookup, and startup conversion of `running` rows to `interrupted`. Schema v2 adds exactly one nullable 1–128-byte source-free `pi_session_id`; v1 rows and current Git-only starts store SQL `NULL`. Generic `Start`, `Record`, and `List` remain Session-free. No Session path, cwd, name, time, message count, parent, leaf, prompt, answer, tool call/result, summary, transcript, source, changed path, question/answer/F1/feedback text, prompt body, RPC/model output, credential, token, or executable path has an API or column. The daemon constructs, queries, and closes the store as a degradable capability and exposes the Session-specific seams only through independent authenticated routes.
- `internal/daemon`: one deep local-runtime module whose `Run` interface owns protected runtime and durable-data directories, per-start Instance Tokens, single-instance locking, loopback HTTP lifecycle, strict protocol decoding, stable error translation, graceful shutdown, and in-memory continuation and assessment services. The two additive Go-context preview routes request the internal context policy and expose every model-visible item, relationship, limit, build value, completeness state, omission, and truncation count. Generic and Session-bound continuations retain owned exact results for five minutes under fixed 8-entry/1-MiB limits and record the v1/v2 evidence contract only in private daemon state. `/v1/question-sets` atomically consumes the continuation and selects the pure bundle/input/prompt version without client fields or a repository reread; the optional Session ID passes only to assessment provenance. Assessment and generic history remain source-free and require no schema migration. Existing preview routes remain on v1, and production never substitutes deterministic fixtures when an evaluator is unavailable.
- `cmd/pi-learnloop`: the minimal public executable adapter. It accepts only `pi-learnloop daemon`, installs `SIGINT`/`SIGTERM` cancellation, and exposes no flags that weaken accepted security defaults.
- `extensions/lib/daemon-client.ts`: one deep local client module whose generic and Session-bound preview, completion-only Session review, question, assessment, and history interfaces hide protected runtime-file validation, exact loopback URL validation, instance verification, proxy-independent HTTP, Instance Token authentication, bounded response decoding, and strict v1 response validation. Preview discovery races retry once; Session review, question, assessment, and history operations never retry.
- `extensions/lib/learn-command.ts`: the manual interaction module. `/learn` accepts only explicit supported selections. Its Session branch accepts only the newest at most 20 unique bounded IDs from the injected Pi listing seam, removes only daemon-reported completed IDs, displays no rich Session metadata, and requires a separate Git selection. Both branches render files, symbols, byte volume and truncation before evaluation, validate active Pi model metadata, require explicit sharing/cost confirmations, collect exactly Q1/Q2/Q3 and at most one F1, render daemon-validated feedback plus the Go-derived label, and warn when the successful result was not saved without starting an Agent turn or retry. `/learn-history` accepts no arguments, requests 20 records for the trusted current working directory, and concisely renders lifecycle, revisions, outcomes, and evaluator provenance without a confirmation or model client.
- `extensions/pi-learnloop.ts`: the minimal Pi adapter. It registers only the manual `/learn` and `/learn-history` commands and injects exactly `SessionManager.list(ctx.cwd, ctx.sessionManager.getSessionDir())` into the Session branch; it contains no evidence, persistence, or evaluation rules.

The remaining target architecture identifies these unimplemented responsibilities:

- Later Pi extension behavior: richer answer editing beyond the implemented Git/Session selection and string-input flow.
- Later daemon capabilities: SSE event delivery, durable worker coordination, and learning lifecycle management beyond the implemented evidence-preview request.
- Go evidence enrichment Phase 3: connect the Pi extension to the additive routes and visibly render and confirm every changed/context item, relationship, fixed limit, completeness state, omission, and truncation count before provider invocation. Current extension behavior remains on v1.

ADR-0005 fixes production history at `os.UserConfigDir()/pi-learnloop/data/history.db`. `daemon.Config.DataDir` accepts an explicit absolute path for integration tests only; there is no environment variable or client-supplied database path.

# High-Level Architecture

Current implemented seam and later target integrations:

```text
Pi TUI extension
    │ manual current-cwd Session list → immediate ID-only projection
    │ Runtime Descriptor + Instance Token
    │ versioned local HTTP; SSE planned
    ▼
cmd/pi-learnloop
    ▼
internal/daemon
    ├── preview/question capability → internal/evidence
    │                              → internal/evaluator question path
    │                              → isolated no-session/no-tools Pi RPC process
    ├── assessment capability      → internal/assessment
    │                              → internal/evaluator assessment path
    │                              → fresh isolated no-session/no-tools Pi RPC process
    │                              → internal/history source-free lifecycle
    │                                 at protected local SQLite data path
    ├── Session review capability  → canonical Git-root verification
    │                              → completion-only bounded ID lookup
    └── history query capability   → bounded repository-scoped internal/history read
```

`internal/evidence.BuildBundle`/`BuildBundleV2` and
`internal/evaluator.NewInput`/`NewInputV2` are connected only after an atomic
continuation consume. They deliberately have no repository path or loader
parameter, so they cannot read additional content after the user-visible
preview. Daemon-owned continuation state, never a client field, selects the v1
or v2 path. Runtime inputs are versioned evaluator contracts, not persisted
history formats; question-set and assessment-turn outputs remain at v1.

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

The current extension follows the v1 branch below. An authenticated client may
instead call `/v1/go-context-evidence-previews` or
`/v1/pi-session-go-context-evidence-previews`; those routes run the same explicit
Git selection with the fixed Go-context policy, return all enriched evidence and
completeness metadata, and retain a private v2 contract marker. The unchanged
question and assessment routes then select bundle/input/prompt v2 from that
marker without accepting a mode field or rereading the repository.

1. `pi-learnloop daemon` acquires the protected single-instance runtime lock, opens and recovers the protected history store when available, resolves `pi` from its startup `PATH`, freezes the absolute symlink-resolved path, and requires an exact `0.84.3` version response within two seconds. History failure degrades persistence only; Pi failure disables continuation but not preview.
2. Only after preflight completes, the daemon binds `tcp4` to `127.0.0.1:0` and writes a per-start Instance Token plus non-secret Runtime Descriptor under `os.UserConfigDir()/pi-learnloop/runtime`.
3. A local client validates the descriptor against unauthenticated `GET /v1/status` and reads the protected token. Direct Git review sends authenticated `POST /v1/evidence-previews`. The manual Session path first projects Pi's current-cwd listing to the newest at most 20 unique bounded IDs, queries completed IDs once through authenticated `POST /v1/pi-session-review-queries`, then sends the selected ID plus explicit Git selection through authenticated `POST /v1/pi-session-evidence-previews`.
4. The daemon rejects non-loopback peers, an unadvertised Host, any non-empty Origin, invalid authentication, ambiguous JSON, oversized bodies, invalid selections, and unsafe evidence paths.
5. The daemon applies fixed evidence caps of 20 files, 100 declarations, and 128 KiB of aggregate excerpts, then calls `internal/evidence`. When usable evidence and the current evaluator are available, it retains an owned copy behind a cryptographically random five-minute continuation; empty or capacity-limited previews remain successful with an unavailable reason.
6. The extension renders the preview and, for the Session path, the user-selected full Session ID/Git association. It then validates Pi 0.84.3's active non-secret model identity and asks for explicit confirmation. Decline sends no continuation request; list/query/selection alone never starts a model.
7. Authenticated `POST /v1/question-sets` strictly validates a 4-KiB request, atomically consumes the continuation, builds the bundle from that exact value without repository access, and starts one frozen Pi 0.84.3 RPC executable directly without a shell. Concurrent, expired, used, malformed, wrong-instance, and post-restart IDs share `409 continuation_unavailable`.
8. The adapter uses the embedded released prompt and fixed deny arguments, disables Agent retry and auto-compaction through correlated commands, requires an empty discovered-command list, sends exactly one LF-framed runtime input, rejects tools and unexpected events, waits for `agent_settled`, and validates one final assistant text. It enforces a 120-second deadline, 2-MiB stdout, 64-KiB stderr, and 64-KiB final-text limits, then terminates and reaps the child.
9. The strict result is exactly Q1/Q2 `code_specific` plus Q3 `go_backend`, or `insufficient_evidence`; the extension validates and renders it. The client and daemon never retry the continuation or evaluator call.
10. A successful question result additively reports an assessment descriptor and retains an owned exact context for thirty minutes under eight-entry/1-MiB limits when the production assessment adapter passed startup preflight.
11. The extension collects exactly three bounded answers, confirms evidence/answer sharing and possible cost, and submits once. After the volatile state atomically enters evaluation, the assessment service creates one source-free `running` record when storage is available, then starts a fresh isolated Pi assessment process with the fixed model and released prompt. An F1 marks and reuses the same record; completion atomically stores the Go label and exactly Q1/Q2/Q3 kinds/verdicts; known failures store only a safe code.
12. A complete response strictly reports either a saved `lr1-` record ID or `storage_unavailable`. The validated assessment is still returned after a terminal storage failure, the extension warns once, and no layer retries the assessment. Startup changes leftover `running` rows to `interrupted` without starting Pi or resuming work.
13. Manual `/learn-history` sends the trusted current working directory and limit 20 through authenticated `POST /v1/learning-history-queries`. The daemon reuses Git-root canonicalization, caps any request at 50, queries only that root newest-first, and maps records to a source-free response without returning the canonical root. The client validates the exact lifecycle/provenance/outcome shape and never starts or retries an evaluator.
14. Cancellation reaches Git and evaluator subprocesses; `SIGINT` or `SIGTERM` triggers bounded graceful shutdown, in-memory continuation and assessment cleanup, history close, and instance-aware runtime-file cleanup.

Current end-to-end flow:

1. The user manually invokes `/learn`; there is no automatic reminder or background Session indexing.
2. The user explicitly selects a Git changeset, or selects one unreviewed current-project Pi Session ID and then explicitly associates a Git changeset. The Session list/query alone starts no model.
3. The implemented extension and daemon produce the bounded, inspectable evidence preview and explicit continuation.
4. The implemented isolated Pi RPC adapter generates and validates the initial three questions from the exact retained evidence.
5. The assessment module owns the exact source-bearing evaluator input, fixed question set, fixed model selection, three bounded answers, and at most one F1 exchange in volatile memory; it separately binds server-owned canonical-root, optional source-free Session ID, and immutable prompt provenance for allowlisted history writes. The Session ID never enters evaluator or RPC values.
6. The implemented Pi UI collects answers and drives the strict authenticated stages; the production assessment adapter starts one isolated process per turn, while deterministic adapters exercise complete and follow-up paths without a provider.
7. The strict result contract accepts three evidence-backed verdicts, and Go derives `understood`, `partial`, or `review_needed` deterministically.
8. The protected history module durably stores the source-free repository-scoped lifecycle when available and safely degrades otherwise. The manual `/learn-history` command exposes its bounded repository query without a model call; an empty result is normal, and unavailable/corrupt/newer storage is reported without repair or rewriting.

# External Dependencies

Current runtime dependencies:

- the local `git` executable;
- the local `pi` executable at exactly version `0.84.3`, resolved from the daemon startup `PATH`; Pi owns model selection and credentials;
- Go 1.21 or a compatible newer Go toolchain;
- direct `golang.org/x/mod v0.19.0`, used only for in-memory `go.mod`/`go.work` parsing and module/import path validation by the internal context analyzer; it preserves the Go 1.21 baseline and introduced no selected-version or `go.sum` change;
- direct `modernc.org/sqlite v1.35.0` and the exact indirect module graph recorded in `go.sum`; the driver does not require CGO;
- Node.js `>=22.19.0` and Pi 0.84.x for the extension;
- Pi-provided peer `@earendil-works/pi-coding-agent: "*"`, which is not bundled.

Exact extension development dependencies:

- `@earendil-works/pi-coding-agent@0.84.3` (MIT);
- `@types/node@22.19.19` (MIT);
- `typescript@5.9.3` (Apache-2.0).

The extension uses Node's built-in test runner and built-in filesystem and HTTP modules. There is no third-party runtime npm dependency.

Any additional SQLite, migration-framework, ORM, or Go-analysis dependency remains `TODO / Need Confirmation` until introduced through a separately approved plan.

# Engineering Constraints

- Learning starts only through an explicit user command.
- The initial analyzer supports Go repositories only.
- The first release supports macOS ARM64 and AMD64 only.
- The evaluator is independent from the Session that produced the code.
- The evaluator must not receive edit or unrestricted command tools.
- The daemon listens only on loopback and authenticates clients with a local secret.
- The implemented v1 listener is IPv4-only `127.0.0.1` on an operating-system-assigned port; non-loopback transport requires a new ADR.
- Runtime directories use mode `0700`; descriptor, token, and lock files use `0600`; ambiguous ownership, permissions, or symlinks fail closed.
- History storage requires a real same-owner local data directory at mode `0700` and a single-link regular database at mode `0600`; schema v2, its non-destructive v1 migration, WAL, `synchronous=FULL`, foreign keys, `trusted_schema=OFF`, and a 5-second busy timeout are compatibility-sensitive. Corrupt, unsafe, or newer databases are never automatically rewritten, repaired, downgraded, or deleted.
- API credentials remain managed by Pi; the Go daemon must not persist them.
- Only the selected evidence bundle is sent to the configured evaluator model.
- Pi 0.84.3 Session listing temporarily materializes message-derived values and other unused metadata for all candidate files in extension memory. The accepted manual path must immediately retain only the newest at most 20 validated IDs and must never use, display, transmit, log, cache, index, or persist the richer values.
- Users can inspect the files and approximate code volume before evaluation.
- Learning data remains local and no telemetry is uploaded by default.
- Changes must favor a small, testable architecture over speculative extensibility.

# Compatibility Requirements

- Preserve stored learning records across compatible daemon upgrades once a storage format is released.
- Treat database migrations and Pi extension/daemon protocol changes as compatibility-sensitive.
- Treat ADR-0002 descriptor fields, `/v1` JSON fields, error codes, fixed evidence caps, and Instance Token behavior as compatibility-sensitive.
- Treat ADR-0003's runtime evaluator schemas, released prompt version, exact Pi 0.84.3 support, fixed question order, and deny-argument contract as compatibility-sensitive.
- Treat ADR-0004's assessment schemas, exact Q1/Q2/Q3 answer order, single-F1 limit, verdict set, text bounds, and deterministic label mapping as compatibility-sensitive.
- Treat ADR-0005's `lr1-` record IDs, status/failure/label/verdict enums, protection requirements, and no-repair/no-downgrade policy, plus ADR-0006's schema v2 nullable Pi Session ID, 1–128-byte ASCII contract, completion-only reviewed meaning, v1 migration, and old-binary fail-closed behavior, as compatibility-sensitive.
- Treat ADR-0007's additive enriched routes, v2 bundle/input/prompt contracts,
  snapshot consistency, local-only source policy, build configuration, closed
  context item/relation/omission vocabularies, and fixed input/output limits as
  compatibility-sensitive.
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

Current Go evidence tests cover commit ranges, working-tree and untracked files, declaration kinds and method identity, rename/deletion outcomes, non-Go changes, malformed source, repository escape protection, stable error codes, deterministic evidence limits, exact Preview-to-Bundle projection, citation ordering, test classification, canonical manifest hashing, privacy-safe path handling, and fail-closed budget/structure/insufficiency behavior. Evaluator contract and fake-process tests cover bundle integrity and copy ownership, strict JSON and duplicate-key rejection, fixed question shape and references, insufficient evidence, assessment initial/follow-up invariants, answer and feedback bounds, one-follow-up enforcement, exact reference validation, all 27 verdict-to-label combinations, deterministic assessment behavior, exact Pi argument/model mapping, executable resolution/version preflight, LF framing, response correlation, `agent_settled`, documented streaming events, Unicode separators, discovered commands, tool events, missing model/auth behavior, timeout, cancellation, stdout/stderr caps, invalid output, child exit, and process reaping without a provider call.

Internal Go-context evidence tests additionally cover commit/working-tree parity,
historical head isolation, additions/deletions, import-only evidence, direct local
references and implementations, nested modules, workspace and replacement
precedence, build constraints, test variants, CGO/external/malformed/cyclic
partial states, newer/unsupported layouts, outside configuration and symlinks,
vendor exclusion, cancellation, deterministic ordering, and every fixed budget
class. The v1 result is compared unchanged and rejects enriched input at its
bundle boundary.

Phase 2 tests additionally cover genuine import-only `evidence-bundle@2`
construction, deterministic manifest coverage, detached bundle validation,
complete evaluator-input byte bounds, C-series question/assessment references,
v1 JSON compatibility, schema-selected production prompts, strict additive
generic/Session routes, exact retained evidence after repository mutation,
single-use continuation behavior, cancellation and expiry errors, Session
isolation, source-free completion history, and adversarial evaluator fixtures.

Current daemon integration tests cover loopback discovery, preflight-before-discovery publication, commit-range and working-tree previews through real Git repositories, fixed evidence caps, token authentication, Host/Origin/CORS defenses, strict JSON, size and media-type limits, stable safe errors, single-instance locking, runtime permissions, symlink rejection, token rotation, stale-state replacement, cancellation propagation, shutdown cleanup, and a production-composed repository-history query that starts no evaluator. History route tests cover canonical nested paths, cross-repository isolation, newest-first source-free response shape, the 50-record cap, exact JSON, authentication, body/media limits, empty results, invalid repositories, and unavailable storage. Continuation tests cover exact retained working-tree evidence after the source is changed, five-minute expiry, count/byte capacity, owned copies, single and concurrent consume, strict question requests, protected routing, empty evidence, and restart loss. Daemon tests install a fake Pi executable and never call a provider. Command tests verify that no unsupported security-weakening arguments are accepted.

Assessment service and route tests cover owned copies, thirty-minute expiry, count/byte capacity without live eviction, cleanup, strict stage fields, malformed and replayed IDs, invalid submissions without state mutation, atomic concurrent consume, evaluator failure invalidation, one F1, final label mapping, running-before-evaluator ordering, one-record F1 completion, safe failure codes, cancellation, terminal storage failure, response loss, server-owned prompt/schema/model provenance, excluded-content absence, and the absence of a production deterministic fallback. Assessment fake-Pi tests additionally cover exact initial/follow-up inputs, a fresh process per turn, response correlation, strict schema/reference failures, isolation-event rejection, timeout, cancellation, stream/output caps, early exit, opaque errors, and process reaping; daemon integration verifies production composition without a provider.

History tests use only temporary protected databases and cover exact schema migration, SQLite setting verification, permissions and symlink/hard-link rejection, future/corrupt/unexpected-schema preservation, full stored-value preflight, source-free field allowlists, rollback, WAL reopen, repository isolation and bounds, running/complete/failed/interrupted transitions, idempotent terminal writes, conflicts, daemon-open recovery with zero evaluator calls, and storage-unavailable daemon degradation with `CGO_ENABLED=0`.

Current extension tests cover both manual command registrations, explicit commit-range and working-tree selections, trust/UI/input gates, empty previews, approximate UTF-8 excerpt volume, truncation display, recoverable errors, protected discovery paths, exact IPv4-loopback descriptor validation, status-before-token ordering, environment-proxy bypass, authenticated requests, one preview discovery retry, stable daemon error propagation, response limits, and v1 success-schema validation. Post-preview tests cover decline-without-request, question and answer sharing/cost/retry disclosure, missing model metadata, exact model propagation, three-question rendering, assessment descriptors, exact answer payloads, cancellation before submission, unavailable assessment handling, one F1, final-result rendering, strict saved/unsaved history descriptors, storage-failure warning, malformed-result rejection, and zero continuation or assessment retries. History client/UI tests cover one bounded authenticated request, exact newest-first lifecycle/provenance/outcome validation, rejection of extra canonical-root metadata, no retry on unavailable storage, empty history, and concise manual rendering without a model client.

The remaining target strategy includes:

- Unit coverage for changeset selection, Go AST mapping, concept normalization, and evaluation schemas.
- Transactional tests for any future SQLite jobs, leases, retries, and event cursors beyond the implemented history-record state machine.
- Additional integration coverage must continue using a fake Pi RPC evaluator rather than live paid model calls.
- End-to-end fixture repositories containing representative Go changesets.
- Race testing for future worker and streaming code.
- Crash-recovery tests for any future durable job states beyond the implemented running-attempt interruption marker.
- Packaging smoke tests for supported macOS architectures.

`TODO / Need Confirmation`: add new persistence formats, evaluator capabilities, release packaging, and cross-architecture commands only after their relevant manifests and scripts exist.

# Known Risks / Important Notes

- The Pi extension and RPC APIs may evolve; isolate them behind narrow adapters.
- Pi 0.84.3's published declaration graph contains upstream type-resolution errors under full library checking, so this project uses `skipLibCheck` while retaining strict checking for its own TypeScript sources.
- LLM evaluation can be inconsistent; require structured output, code evidence, and repeatable fixtures.
- Deterministic evaluator adapters are test fixtures and must never become production fallbacks.
- The capability policy is a required development contract, not runtime enforcement; future adapters must prove enforcement through implementation tests.
- Mapping multiple Sessions to one changeset can be ambiguous; selection must remain explicit and inspectable.
- Source code privacy depends on evidence minimization, repository ignore rules, and a clear pre-evaluation preview.
- Production question generation and answer assessment depend on exactly Pi 0.84.3 being visible on the daemon startup `PATH`; unavailable or mismatched Pi leaves preview operational but disables the corresponding evaluator capability.
- Pi LearnLoop disables Agent retry and auto-compaction and performs no product retry. Pi/provider transport configuration remains external; the supported configuration requires `retry.provider.maxRetries` to stay `0` because RPC cannot enforce it.
- History databases intentionally have no automatic retention or repair. The source-free metadata can grow until a separately approved deletion/pruning design exists, and any future backup must account for WAL state.
- `modernc.org/sqlite v1.35.0` is intentionally pinned to the accepted Go 1.21-compatible dependency graph. Driver upgrades require a separate compatibility and migration review.
- A durable SQLite worker queue can become unnecessary complexity if the state machine is not kept small.
- The product-connected v1 path is still syntax-only. The internal type/dependency context proof is not model-visible until separately authorized Phase 2/3 routes, contracts, prompts, and preview UI exist; deletion-only hunks remain explicitly reported rather than reconstructed from base-side source.
- On this macOS 26 development machine, the default Homebrew Go 1.21.13 external linker aborts network-enabled test binaries with `missing LC_UUID`; Go 1.21 passes with `CGO_ENABLED=0` or `-tags netgo`, and the unmodified test, race, vet, and build commands pass with the already-installed Go 1.26.4 toolchain. The module language version remains Go 1.21.
