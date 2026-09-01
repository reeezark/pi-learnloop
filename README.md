# Pi LearnLoop

Pi LearnLoop is a local learning companion for Go developers who use the Pi coding agent. Its current development slice lets you manually choose a Git changeset with `/learn`, inspect the changed-Go evidence, explicitly confirm continuation, and receive three locally generated learning questions.

## Current Status

Implemented:

- bounded changed-Go declaration evidence for commit ranges and working trees;
- an authenticated, IPv4-loopback-only Go daemon;
- a Pi 0.84.x TypeScript extension that registers the manual `/learn` command;
- preview output containing changed files, mapped symbols, approximate excerpt bytes, and truncation details;
- a five-minute, in-memory, single-use continuation that retains the exact bounded preview;
- an authenticated `/v1/question-sets` route and deterministic Phase 2 evaluator that return exactly two code-specific questions and one Go/backend question;
- an explicit confirmation step before the retained evidence is consumed.

Not implemented:

- real model calls, answer collection, scoring, or follow-up interviews;
- learning-history persistence or SQLite;
- SSE, background jobs, Session indexing, or automatic reminders;
- npm publication or release automation.

Running `/learn` never starts an Agent turn, contacts a model provider, or saves a learning record. The current three-question output is deterministic and local; the isolated Pi RPC evaluator remains Phase 3 work.

## Requirements

- macOS on ARM64 or AMD64; only ARM64 is currently verified;
- Go 1.21 or a compatible newer toolchain;
- Node.js 22.19.0 or newer;
- Pi 0.84.x; Pi 0.84.3 is the verified baseline;
- Git available on `PATH`.

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

Choose either a working tree against an explicit base revision or an explicit commit range. After inspecting the preview, confirm whether the local deterministic evaluator may consume that exact retained evidence. Cancelling, entering an empty revision, or declining confirmation sends no continuation request.

For a persistent local installation, Pi can load the package directly from the checkout:

```bash
pi install /absolute/path/to/pi-learnloop
```

The package has no third-party runtime npm dependency. `@earendil-works/pi-coding-agent` is a Pi-provided peer dependency and is not bundled.

## Local Security and Privacy

- The daemon binds only to `127.0.0.1` on an operating-system-assigned port.
- Discovery metadata and the per-start Instance Token live under the current user's protected configuration directory.
- The extension accepts only an exact `http://127.0.0.1:<port>` descriptor, verifies the daemon instance before reading the token, bypasses environment HTTP proxies, and retries discovery at most once after a startup race.
- `/learn` submits the trusted Pi working directory and the explicit Git selection. It does not send Pi credentials.
- A successful preview may retain only its bounded evidence in daemon memory for five minutes. The opaque continuation is single-use, has fixed count and byte limits, and is removed on expiry or daemon shutdown.
- Confirmation sends only the opaque continuation ID and non-secret active model identifiers. The current deterministic evaluator remains local, no model is called, and no telemetry is uploaded.

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
