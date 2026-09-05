# Pi LearnLoop

Pi LearnLoop is a local learning companion for Go developers who use the Pi coding agent. Its current production slice lets you manually choose a Git changeset with `/learn` or choose a current-project Pi Session and explicitly bind it to a Git changeset, inspect bounded changed-Go evidence plus selected-snapshot Go package/type context, explicitly approve sending only the displayed evidence to your configured model, compose and review three multiline evidence-backed answers, receive concise per-question feedback plus a deterministic repository-scoped label, and later inspect source-free local results with `/learn-history`.

## Current Status

Implemented:

- bounded changed-Go declaration, changed-import, direct repository-local package/type, and relationship evidence for commit ranges and working trees;
- an authenticated, IPv4-loopback-only Go daemon;
- a Pi 0.84.x TypeScript extension that registers the manual `/learn` and `/learn-history` commands;
- a strict enriched preview that displays changed excerpts, C-series context items, relationships, snapshot build policy, every fixed input/output limit, completeness, omissions, hashes, byte estimates, and truncation details;
- a five-minute, in-memory, single-use continuation that retains the exact bounded preview;
- an authenticated `/v1/question-sets` route and isolated Pi 0.84.3 `ModelRuntime` evaluator that return exactly two code-specific questions and one Go/backend question;
- an explicit confirmation step before the retained evidence is consumed;
- a 30-minute, eight-entry, 1-MiB bounded in-memory assessment state machine with atomic initial/F1 submission and deterministic Go label aggregation;
- an additive assessment descriptor, strict authenticated `/v1/assessment-turns` route, and reviewable multiline Q1/Q2/Q3 plus multiline F1 UI with no client retries;
- a released embedded assessment prompt and production Pi 0.84.3 `ModelRuntime` adapter that starts a fresh isolated worker for the initial assessment and, when requested, one F1 assessment;
- deterministic service, protocol, concurrency, cancellation, extension, fake-worker, and actual-SDK no-network tests for the complete answer flow, including chunking-invariant linear stream accounting and an independent event-count limit;
- daemon-owned protected SQLite history at `os.UserConfigDir()/pi-learnloop/data/history.db`, with schema v2 migrations, source-free running/F1/terminal records, one nullable bounded Pi Session ID provenance value, startup interruption marking, and explicit save status in complete assessment responses;
- a strict authenticated `/v1/learning-history-queries` route capped at 50 records and a manual `/learn-history` UI that requests the 20 newest records for the current canonical Git repository without a model call;
- independent strict authenticated `/v1/pi-session-evidence-previews` and `/v1/pi-session-review-queries` routes that keep a bounded Session ID beside retained evidence, propagate it only to Session-aware history, and filter only completed IDs in the canonical repository;
- independent enriched `/v1/go-context-evidence-previews` and `/v1/pi-session-go-context-evidence-previews` routes that retain the exact visible snapshot and select v2 bundle/input/prompt contracts without client-supplied mode fields;
- a manual `/learn` Pi Session path that lists the current cwd once through Pi 0.84.3, immediately projects the newest at most 20 entries to unique bounded IDs, filters completed reviews once, displays only IDs, and requires an explicit Git working-tree or commit-range association before the enriched preview and model-confirmation flow.

Not implemented:

- SSE, background jobs, Session indexing, or automatic reminders;
- npm publication or release automation.

The preview half of `/learn` never contacts a model provider. After confirmation, the daemon consumes the reviewed continuation once and starts a separate bounded Node child running an embedded worker against the matching installed Pi 0.84.3 `ModelRuntime`. The worker creates no AgentSession, Session, tool, extension, or resource loader. Its stream budget charges unique text/thinking deltas plus fixed event overhead, not Pi's repeated cumulative snapshots, and independently caps the stream at 32,768 events. Successful questions retain the exact validated input in bounded daemon memory so the user can compose Q1/Q2/Q3 in Pi's multiline editor, review the three fixed answer IDs, and revise any accepted answer before the existing sharing confirmation. The initial assessment starts a fresh worker; one multiline F1 answer may start one final worker. Source-bearing inputs, answers, evaluator output, and private worker streams remain in memory. When protected history storage is available, only the source-free ADR-0005/ADR-0006 allowlist is recorded; a storage failure never hides a successful assessment or triggers another model call.

## Requirements

- macOS on ARM64 or AMD64; native tests have passed on both architectures,
  although the macOS 13 compatibility floor has not been executed;
- Go 1.21 or a compatible newer toolchain;
- Node.js 22.19.0 or newer;
- an importable npm-style Pi 0.84.3 installation available as `pi` on the daemon startup `PATH`, with its matching SDK package intact;
- Git available on `PATH`;
- a model and credentials configured through Pi. Pi LearnLoop never reads or accepts the credential itself.

## Run Locally

Install the exact development dependencies without lifecycle scripts:

```bash
npm install --ignore-scripts
```

Start the foreground daemon in one terminal:

```bash
go run ./cmd/pi-learnloop daemon
```

Start Pi with the extension from the repository you want to inspect:

```bash
pi -e /absolute/path/to/pi-learnloop/extensions/pi-learnloop.ts
```

For development, start the daemon and load the extension from the same checkout.
After changing either side, stop the foreground daemon, restart it from that
checkout, and restart Pi with the matching extension path. The daemon freezes
the `node` and `pi` resolved from its startup `PATH`; confirm `node --version` is
22.19.0 or newer and `pi --version` is exactly 0.84.3 before starting it.

Then invoke:

```text
/learn
```

Choose a working tree against an explicit base revision, an explicit commit range, or `Pi Session`. The Session choice lists the current project's newest Sessions, removes IDs that already have a completed review in this repository, asks you to choose one full ID, and then requires the same explicit Git selection. The preview shows the user-supplied Session/Git association; Git remains the evidence authority.

The enriched preview visibly separates changed declarations from changed-import/context items and relationships. It also shows the exact selected-snapshot build configuration, analysis totals, fixed discovery and output limits, context status (`complete`, `partial`, or `unavailable`), closed omission reasons, content hashes, and both changed/context truncation counts. Partial or unavailable context is not replaced with the older changed-only route: you may continue only after seeing and explicitly confirming that state. The confirmation names the active model, estimates repository-derived evidence bytes, and states the 256-KiB complete evaluator-input cap; Pi LearnLoop cannot know the provider's monetary price. The Session ID is not sent to the model. Cancelling, entering an empty revision, or declining confirmation sends no continuation request and creates no history record. An updated extension fails closed against an older daemon response that lacks the required enriched fields.

When questions are available, `/learn` first discloses the multiline editor's privacy and resource limits. Q1, Q2, and Q3 open in order with empty answer drafts; after all three are valid, an ID-only menu lets you continue, edit one answer with its accepted text as prefill, or cancel. Answers are trimmed at the boundary, limited to 4,096 UTF-8 bytes, and may contain LF line breaks but no other control characters. Invalid drafts reopen with a generic warning and never replace an accepted answer. Cancelling an initial answer or the review discards the local collection; cancelling an edit preserves the previous answer. F1 uses the same editor without another review menu. A new extension that receives `invalid_request` from an older daemon fails closed without flattening or retrying the answer and asks you to update the daemon and extension together.

Pi LearnLoop creates no Pi Agent retry or auto-compaction lifecycle and never retries a continuation or model call itself. The isolated worker forces provider retries to zero for its one call without changing the user's Pi settings. It preserves Pi's validated global proxy, transport, timeout, thinking-budget, retry-delay, and provider-attribution behavior inside the child.

### Troubleshooting question or assessment failures

Errors after confirmation name the stage that actually failed: question
generation or answer assessment. A runtime-unavailable message can mean the
isolated worker failed initialization before any provider request; verify the
foreground daemon's exact Pi/Node requirements above and restart the matching
daemon and extension. A lost-daemon message means the outcome is unknown. An
incompatible-response message means the daemon and extension must be updated
together. An expired preview or assessment cannot be reused. LearnLoop never
retries any of these requests automatically, so inspect the message and start a
fresh `/learn` flow deliberately.

To inspect the newest source-free records for the current Git repository, invoke:

```text
/learn-history
```

This command accepts no arguments, returns at most 20 records newest-first, and does not contact a model. An empty result is normal. If the database is unsafe, corrupt, unreadable, or newer than the running daemon supports, the command reports that local history is unavailable and leaves the database unchanged.

### Optional live smoke test

Automated tests use fake workers plus the actual Pi 0.84.3 SDK with synthetic credentials and intercepted transport; they never contact a real provider. A live smoke test is intentionally manual and opt-in because it transmits reviewed source excerpts and may incur cost:

1. Use a synthetic or otherwise safe repository and review the selected excerpts in the `/learn` preview.
2. Confirm that `pi --version` is exactly `0.84.3`, its matching SDK package is importable, the intended model is active, and Pi credentials are configured.
3. Start the daemon, load the extension, invoke `/learn`, and confirm only after reviewing the preview.
4. Verify that the command returns exactly two code-specific questions and one Go/backend question.
5. To smoke-test assessment, accept the editor disclosure, compose Q1/Q2/Q3 with Pi's multiline editor, use the fixed-ID review menu, and approve the answer-sharing confirmation. This resends the same displayed changed/context evidence together with the answers and incurs one additional model call. If F1 is returned, answering it in the same editor resends the retained assessment context and may incur one more call, for at most two assessment calls beyond question generation.
6. Verify either one F1 followed by a complete result or an immediate complete result with three verdicts and one derived label. A warning means the assessment succeeded but local history was unavailable. No automated command in this repository performs this live step. The maintained working tree completed this flow on 2026-09-05 with Pi 0.84.3 and `deepseek/deepseek-v4-pro`; provider credentials and selected repository evidence remain environment-specific.

For a persistent local installation, Pi can load the package directly from the checkout:

```bash
pi install /absolute/path/to/pi-learnloop
```

The package has no third-party runtime npm dependency. `@earendil-works/pi-coding-agent` is a Pi-provided peer dependency and is not bundled.

## Local Security and Privacy

- The daemon binds only to `127.0.0.1` on an operating-system-assigned port.
- Discovery metadata and the per-start Instance Token live under the current user's protected configuration directory.
- The extension accepts only an exact `http://127.0.0.1:<port>` descriptor, verifies the daemon instance before reading the token, bypasses environment HTTP proxies, and retries discovery at most once after a startup race.
- `/learn` submits the trusted Pi working directory and the explicit Git selection. A Session review also sends only the selected bounded Session ID through the two dedicated local routes. It does not send Pi credentials.
- A successful enriched preview may retain only its bounded changed/context evidence in daemon memory for five minutes. Repository-derived Go context is capped at 64 KiB and the complete serialized evaluator input at 256 KiB. A Session-bound preview retains one validated source-free Session ID beside, never inside, that evidence. The opaque continuation is single-use, has fixed count and byte limits, and is removed on expiry or daemon shutdown.
- Confirmation sends only the opaque continuation ID and non-secret active model identifiers to the daemon. The daemon builds the evaluator input from the exact retained preview without rereading the repository.
- Successful production questions may retain their exact validated input for at most thirty minutes under an eight-entry/1-MiB cap. Initial answers and F1 are bounded to 4 KiB each and allow only LF among control characters; the authenticated assessment route has a dedicated 32-KiB pre-decode body cap. Submissions are atomically single-consume, and completed, failed, expired, or concurrent IDs share a non-retryable unavailable result.
- LearnLoop does not save editor drafts. Pi 0.84.3's explicitly invoked external-editor shortcut writes the current draft to an OS-temporary `pi-editor-*/prompt.md`, launches the configured editor, and attempts cleanup on a best-effort basis. The editor or environment may retain swap, backup, recovery, history, or telemetry artifacts, and Pi may materialize an oversized draft before LearnLoop validates the 4-KiB limit. LearnLoop never invokes the shortcut automatically, retains only accepted bounded values after the editor returns, and cannot promise secure memory erasure.
- Daemon assessment state, source, answers, prompt bodies, model output, and feedback are never persisted or logged.
- The daemon opens the protected history store at `os.UserConfigDir()/pi-learnloop/data/history.db`. It accepts only canonical repository identity, revisions, manifest/schema/prompt/model provenance, safe lifecycle status, deterministic label, Q1/Q2/Q3 kinds/verdicts, and an optional bounded source-free Pi Session ID through the dedicated history seam. The current Git-only daemon path stores SQL `NULL`. It rejects symlinked, overbroad, wrong-owner, hard-linked, non-local, corrupt, or newer-schema storage without automatic repair, then keeps preview and assessment available without history.
- A validated initial submission creates one source-free `running` record before evaluation when storage is available. F1 reuses it, completion stores exactly three verdicts, known evaluator failures use bounded safe codes, and restart converts leftover `running` rows to `interrupted` without resuming or retrying evaluation.
- `/learn-history` sends only the current trusted working-directory path and a fixed limit of 20 to the authenticated local daemon. The daemon verifies the canonical Git root and returns only matching source-free records; it never returns the stored canonical root, source, questions, answers, feedback, or records from another repository.
- The dedicated Session review query accepts 1–20 unique bounded IDs, verifies the repository first, and returns only IDs with a complete record in candidate order. Running, failed, interrupted, NULL, and other-repository records do not match; unavailable history is reported explicitly. Session IDs never enter evidence bundles, evaluator inputs, prompts, RPC/model content, errors, logs, or generic history responses.
- Pi 0.84.3's `SessionManager.list` reads candidate Session files and temporarily materializes message-derived `firstMessage` and `allMessagesText` plus unused metadata in the extension process before returning. The manual flow immediately keeps only the newest at most 20 validated IDs and never uses, displays, transmits, logs, caches, indexes, or persists the richer values. This accepted limitation means listing cost still scales with all Session files in the configured current-project Session directory, not only the 20 IDs shown.
- SQLite WAL state is part of the database. Do not copy only `history.db` while the daemon is running; `history.db-wal` and `history.db-shm` may be required for a consistent manual backup. No backup or export command is implemented.
- At daemon startup, production freezes symlink-resolved Node and Pi executables and verifies Node >=22.19.0, exact Pi 0.84.3 package ownership, and required SDK/helper exports. Every question or assessment turn starts Node directly without a shell and runs the embedded worker from memory; source and answers enter only its private stdin frame, never argv or disk.
- Only the displayed selected excerpts, context items, relationships, build/limit/completeness metadata, omissions, truncation, and non-secret bundle provenance enter the Pi-managed model request. Context analysis uses the selected Git snapshot, reads no external/module-cache/vendor/GOROOT source, performs no network access, and persists no source. Credentials never enter HTTP, argv, prompts, logs, persisted records, or model-visible content.
- Private request, model-event stream, stdout, stderr, and final output are bounded; malformed or duplicate framing, tool calls, error/abort/length completion, retry, timeout, cancellation, or child failure fail closed. The child is always reaped. Global Pi settings are read once through a write-forbidden snapshot; project settings and Session files are not opened by the worker.

The Instance Token does not protect against root, a malicious process already running as the same user, or a compromised extension that the user has trusted.

## Development

Authoritative checks for the implemented extension are:

```bash
npm run typecheck
npm test
```

Authoritative checks for the Go implementation and Agent governance are:

```bash
go test ./...
go test -race ./...
go vet ./...
scripts/test-agent-infra.sh
scripts/validate-agent-infra.sh
```

Read `AGENTS.md`, `PROJECT.md`, the active plan under `plans/`, and its current checkpoint before changing the project.
