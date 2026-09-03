---
id: go-evidence-context-enrichment-phase-3
plan: go-evidence-context-enrichment
phase: 3
status: current
updated: 2026-09-03
---

# Go Evidence Context Enrichment Phase 3

## Context

### Goal

Connect the Pi extension to the additive enriched preview routes, strictly
validate and visibly render every model-visible evidence category, and require
an accurate model/scope/size/cost confirmation before any continuation is
consumed.

### Current Phase

Phase 3 and the three-phase `go-evidence-context-enrichment` plan are complete.
The user explicitly authorized Phase 3 on 2026-09-03.

The Phase 3 baseline is the committed and pushed Phase 2 commit `4179ef7`, equal
to `origin/main` before implementation began. The completed Phase 3 changes are
in the working tree and have not been committed or pushed.

## Completed

- Switched direct Git preview requests to
  `POST /v1/go-context-evidence-previews` and Session-bound requests to
  `POST /v1/pi-session-go-context-evidence-previews`; there is no v1 fallback.
- Deepened the existing daemon-client module behind its unchanged narrow
  preview methods. It validates the exact enriched response, fixed limits,
  closed enums, analysis counts, explicit collections, sequential C references,
  item hashes and bytes, relation strength/kind pairs, completeness/omission and
  truncation consistency, and exact repository-derived context-byte accounting.
- Rejects legacy v1 responses, missing enriched fields, unknown top-level
  fields, invalid hashes, and inconsistent byte totals before the command can
  render or confirm them.
- Expanded the command's preview to show changed file metadata, each full
  E-series declaration excerpt with code/test kind and SHA-256, every C-series
  context item and hash, all relationships, build/module/workspace/replacement
  values, analysis totals, fixed input/output limits, completeness, omissions,
  and changed/context truncation.
- Escapes control and format characters into visible notation when rendering
  untrusted repository content while retaining the original content and hash in
  the daemon-owned continuation.
- Names the active model and thinking level before question generation, reports
  the exact displayed repository-derived byte estimate and 256-KiB complete
  evaluator-input ceiling, and explicitly states that LearnLoop does not know
  provider pricing.
- Allows partial and unavailable context to proceed only after that exact state
  is rendered and confirmed. Cancellation before confirmation does not consume
  the continuation, call the model, or create history.
- Preserved explicit Session-to-Git binding, Session ID isolation, daemon-loss
  and stale-continuation handling, questions, Q1/Q2/Q3 answers, optional F1,
  assessment/history rendering, and existing no-retry behavior. Question and
  assessment result validation now accepts both E- and C-series references.
- Updated public and stable documentation for snapshot, privacy, budget,
  incomplete-context, old-daemon, and confirmation semantics.
- Added no dependency, package metadata, daemon/evaluator/evidence code,
  database schema, persisted value, background process, Session content, or v1
  route change.

## Modified Files

Extension implementation:

- `extensions/lib/daemon-client.ts`
- `extensions/lib/learn-command.ts`

Extension verification:

- `tests/extension/go-context-fixture.ts`
- `tests/extension/daemon-client.test.ts`
- `tests/extension/learn-command.test.ts`
- `tests/extension/pi-session-review.test.ts`

Stable and lifecycle documentation:

- `README.md`
- `PROJECT.md`
- `plans/go-evidence-context-enrichment.md`
- `docs/decisions/ADR-0007-snapshot-consistent-go-context-evidence.md`
- `docs/checkpoints/go-evidence-context-enrichment-phase-2.md`
- `docs/checkpoints/go-evidence-context-enrichment-phase-3.md`

## Important Decisions

- The daemon-client preview methods remain the sole transport interface used by
  the command. Route choice, discovery/authentication, retry policy, response
  bounds, and all v2 validation stay behind that interface rather than leaking
  protocol orchestration into the UI.
- The updated extension intentionally requires an enriched response. Supporting
  old daemons by silently retrying v1 would permit unseen model evidence or a
  hidden downgrade, so protocol mismatch fails closed with the existing safe
  command error.
- The preview computes E references and changed-content hashes using the same
  ordered v2 rules; C references and hashes are daemon supplied and strictly
  verified. Untrusted content is fully shown in an indented representation with
  controls escaped rather than interpreted by the terminal UI.
- Monetary price cannot be derived from the local protocol. Confirmation uses
  the exact bundle-style repository-derived byte estimate and fixed serialized
  input ceiling, names the active model, and clearly labels provider cost as
  unknown instead of inventing a currency estimate.

## Tests / Verification

Passed on 2026-09-03:

- `npm run typecheck`.
- `npm test`: 49/49 extension tests with permitted temporary IPv4 loopback
  listeners.
- `npm pack --dry-run --json --cache /private/tmp/pi-learnloop-npm-cache`: six
  package entries, 23895-byte archive, 98643-byte unpacked content, and no
  bundled dependency or created tarball.
- `go test -p=1 -count=1 ./...` with Go 1.26.4: all packages passed; daemon
  133.763 seconds, evaluator 21.651 seconds, evidence 169.350 seconds, and
  history 0.850 seconds.
- `go test -race -p=1 -count=1 ./...` with Go 1.26.4: all packages passed;
  daemon 134.251 seconds, evaluator 23.450 seconds, evidence 171.145 seconds,
  and history 3.379 seconds.
- `go vet ./...` and `go build ./...` with Go 1.26.4.
- installed Go 1.21.13 with `GOROOT` cleared, `CGO_ENABLED=0`, an isolated
  temporary cache, and serial package scheduling: all packages passed; daemon
  132.101 seconds, evaluator 23.501 seconds, evidence 198.646 seconds, and
  history 0.945 seconds.
- `scripts/test-agent-infra.sh` and `scripts/validate-agent-infra.sh`.
- Git whitespace and complete-diff review.

The canonical parallel `go test -count=1 ./...` passed daemon, evidence,
history, and fast packages but reproduced the already recorded environment
sensitive fake-Pi assessment preflight timeout under concurrent package load.
The evaluator package immediately passed alone in 23.105 seconds, and the full
serial and race suites passed. One permission-review attempt timed out before
the isolated evaluator command launched; the one permitted retry ran and
passed.

No verification invoked a live Pi executable, provider/model, external network,
real Pi Session, production source copy, or production history database.

## Known Issues

- Session-to-Git association remains an explicit user assertion; Git evidence
  is authoritative and LearnLoop does not infer authorship.
- Pi 0.84.3 still materializes candidate Session messages and metadata in
  extension memory during manual listing, as explicitly accepted in ADR-0006.
- Partial and unavailable context intentionally remain usable after explicit
  confirmation; the omissions are evidence limitations, not hidden fallback.
- macOS AMD64 remains an intended but unverified target.
- The configured CodeGraph MCP capability was unavailable during this run, so
  focused source inspection used bounded direct reads after confirming the
  repository index directory exists.

## Remaining Work

No work remains in this plan. SSE, durable worker coordination, richer answer
editing, and deletion-side evidence remain separate future capabilities.

## Next Step

Review and commit/push the completed Phase 3 working tree only if explicitly
requested. No additional implementation phase is authorized or required by
this plan.

## Do Not Change

- Do not add hidden v1 fallback or send evidence that the strict enriched
  preview did not render.
- Do not infer Git changes from Session time, cwd, name, messages, summaries,
  tool calls, or filesystem activity.
- Do not put Session ID or content into evidence, evaluator values, prompts,
  RPC/model content, logs, errors, or generic history.
- Do not add a dependency, database migration, external source, network loader,
  source cache/copy, background index, hook, snapshot, marker, reminder, or
  extension-owned persistence.
