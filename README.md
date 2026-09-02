# Pi LearnLoop

Pi LearnLoop is a local learning companion for Go developers who use the Pi coding agent. Its current production slice lets you manually choose a Git changeset with `/learn` or choose a current-project Pi Session and explicitly bind it to a Git changeset, inspect the changed-Go evidence, explicitly approve sending only those excerpts to your configured model, answer three evidence-backed learning questions, receive concise per-question feedback plus a deterministic repository-scoped label, and later inspect source-free local results with `/learn-history`.

## Current Status

Implemented:

- bounded changed-Go declaration evidence for commit ranges and working trees;
- an authenticated, IPv4-loopback-only Go daemon;
- a Pi 0.84.x TypeScript extension that registers the manual `/learn` and `/learn-history` commands;
- preview output containing changed files, mapped symbols, approximate excerpt bytes, and truncation details;
- a five-minute, in-memory, single-use continuation that retains the exact bounded preview;
- an authenticated `/v1/question-sets` route and isolated Pi 0.84.3 RPC evaluator that return exactly two code-specific questions and one Go/backend question;
- an explicit confirmation step before the retained evidence is consumed;
- a 30-minute, eight-entry, 1-MiB bounded in-memory assessment state machine with atomic initial/F1 submission and deterministic Go label aggregation;
- an additive assessment descriptor, strict authenticated `/v1/assessment-turns` route, and thin answer/F1/result UI with no client retries;
- a released embedded assessment prompt and production Pi 0.84.3 RPC adapter that starts a fresh isolated process for the initial assessment and, when requested, one F1 assessment;
- deterministic service, protocol, concurrency, cancellation, extension, and fake-process tests for the complete answer flow;
- daemon-owned protected SQLite history at `os.UserConfigDir()/pi-learnloop/data/history.db`, with schema v2 migrations, source-free running/F1/terminal records, one nullable bounded Pi Session ID provenance value, startup interruption marking, and explicit save status in complete assessment responses;
- a strict authenticated `/v1/learning-history-queries` route capped at 50 records and a manual `/learn-history` UI that requests the 20 newest records for the current canonical Git repository without a model call;
- independent strict authenticated `/v1/pi-session-evidence-previews` and `/v1/pi-session-review-queries` routes that keep a bounded Session ID beside retained evidence, propagate it only to Session-aware history, and filter only completed IDs in the canonical repository;
- a manual `/learn` Pi Session path that lists the current cwd once through Pi 0.84.3, immediately projects the newest at most 20 entries to unique bounded IDs, filters completed reviews once, displays only IDs, and requires an explicit Git working-tree or commit-range association before the unchanged preview and model-confirmation flow.

Not implemented:

- SSE, background jobs, Session indexing, or automatic reminders;
- npm publication or release automation.

The preview half of `/learn` never contacts a model provider. After confirmation, the daemon consumes the reviewed continuation once and starts a separate no-session, no-tools Pi RPC process using the active model. Successful questions retain the exact validated input in bounded daemon memory so the user can submit Q1/Q2/Q3 answers. The initial assessment starts a new isolated Pi process; one answered F1 may start one final isolated process. Source-bearing inputs, answers, evaluator output, and RPC streams remain in memory. When protected history storage is available, only the source-free ADR-0005/ADR-0006 allowlist is recorded; a storage failure never hides a successful assessment or triggers another model call.

## Requirements

- macOS on ARM64 or AMD64; only ARM64 is currently verified;
- Go 1.21 or a compatible newer toolchain;
- Node.js 22.19.0 or newer;
- Pi 0.84.3 available as `pi` on the daemon startup `PATH`;
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

Then invoke:

```text
/learn
```

Choose a working tree against an explicit base revision, an explicit commit range, or `Pi Session`. The direct Git choices preserve the existing flow. The Session choice lists the current project's newest Sessions, removes IDs that already have a completed review in this repository, asks you to choose one full ID, and then requires the same explicit Git selection. The preview shows the user-supplied Session/Git association; the Git evidence remains authoritative. After inspecting the preview, confirm whether one evaluation may send those exact retained excerpts to your configured model. The Session ID is not sent to the model. Cancelling, entering an empty revision, or declining confirmation sends no continuation request.

Pi LearnLoop disables Pi Agent retry and auto-compaction for the evaluator and never retries a continuation or model call itself. The supported configuration keeps Pi's external `retry.provider.maxRetries` setting at `0`; the RPC API cannot enforce that setting.

To inspect the newest source-free records for the current Git repository, invoke:

```text
/learn-history
```

This command accepts no arguments, returns at most 20 records newest-first, and does not contact a model. An empty result is normal. If the database is unsafe, corrupt, unreadable, or newer than the running daemon supports, the command reports that local history is unavailable and leaves the database unchanged.

### Optional live smoke test

Automated tests always use a fake Pi executable and never contact a provider. A live smoke test is intentionally manual and opt-in because it transmits reviewed source excerpts and may incur cost:

1. Use a synthetic or otherwise safe repository and review the selected excerpts in the `/learn` preview.
2. Confirm that `pi --version` is exactly `0.84.3`, the intended model is active, Pi credentials are configured, and `retry.provider.maxRetries` remains `0`.
3. Start the daemon, load the extension, invoke `/learn`, and confirm only after reviewing the preview.
4. Verify that the command returns exactly two code-specific questions and one Go/backend question.
5. To smoke-test assessment, enter Q1/Q2/Q3 answers and approve the second disclosure. This resends the same selected excerpts together with the answers and incurs one additional model call. If F1 is returned, answering it resends the retained assessment context and may incur one more call, for at most two assessment calls beyond question generation.
6. Verify either one F1 followed by a complete result or an immediate complete result with three verdicts and one derived label. A warning means the assessment succeeded but local history was unavailable. No automated command in this repository performs this live step.

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
- A successful preview may retain only its bounded evidence in daemon memory for five minutes. A Session-bound preview retains one validated source-free Session ID beside, never inside, that evidence. The opaque continuation is single-use, has fixed count and byte limits, and is removed on expiry or daemon shutdown.
- Confirmation sends only the opaque continuation ID and non-secret active model identifiers to the daemon. The daemon builds the evaluator input from the exact retained preview without rereading the repository.
- Successful production questions may retain their exact validated input for at most thirty minutes under an eight-entry/1-MiB cap. Initial answers and F1 are bounded to 4 KiB each, submissions are atomically single-consume, and completed, failed, expired, or concurrent IDs share a non-retryable unavailable result.
- Daemon assessment state, source, answers, prompt bodies, model output, and feedback are never persisted or logged.
- The daemon opens the protected history store at `os.UserConfigDir()/pi-learnloop/data/history.db`. It accepts only canonical repository identity, revisions, manifest/schema/prompt/model provenance, safe lifecycle status, deterministic label, Q1/Q2/Q3 kinds/verdicts, and an optional bounded source-free Pi Session ID through the dedicated history seam. The current Git-only daemon path stores SQL `NULL`. It rejects symlinked, overbroad, wrong-owner, hard-linked, non-local, corrupt, or newer-schema storage without automatic repair, then keeps preview and assessment available without history.
- A validated initial submission creates one source-free `running` record before evaluation when storage is available. F1 reuses it, completion stores exactly three verdicts, known evaluator failures use bounded safe codes, and restart converts leftover `running` rows to `interrupted` without resuming or retrying evaluation.
- `/learn-history` sends only the current trusted working-directory path and a fixed limit of 20 to the authenticated local daemon. The daemon verifies the canonical Git root and returns only matching source-free records; it never returns the stored canonical root, source, questions, answers, feedback, or records from another repository.
- The dedicated Session review query accepts 1–20 unique bounded IDs, verifies the repository first, and returns only IDs with a complete record in candidate order. Running, failed, interrupted, NULL, and other-repository records do not match; unavailable history is reported explicitly. Session IDs never enter evidence bundles, evaluator inputs, prompts, RPC/model content, errors, logs, or generic history responses.
- Pi 0.84.3's `SessionManager.list` reads candidate Session files and temporarily materializes message-derived `firstMessage` and `allMessagesText` plus unused metadata in the extension process before returning. The manual flow immediately keeps only the newest at most 20 validated IDs and never uses, displays, transmits, logs, caches, indexes, or persists the richer values. This accepted limitation means listing cost still scales with all Session files in the configured current-project Session directory, not only the 20 IDs shown.
- SQLite WAL state is part of the database. Do not copy only `history.db` while the daemon is running; `history.db-wal` and `history.db-shm` may be required for a consistent manual backup. No backup or export command is implemented.
- Production question generation and every assessment turn start a frozen, symlink-resolved Pi 0.84.3 executable directly without a shell. Sessions, tools, extensions, skills, prompt templates, themes, context files, and project approval are disabled.
- Only the selected excerpts and non-secret bundle provenance enter the Pi-managed model request. Credentials never enter HTTP, argv, prompts, logs, persisted records, or model-visible content.
- RPC runtime, stdout, stderr, and final output are bounded; malformed output, discovered commands, tool events, retries, compaction, timeout, cancellation, or child failure fail closed. The child is always terminated and reaped.

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
