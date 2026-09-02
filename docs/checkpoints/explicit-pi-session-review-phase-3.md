---
id: explicit-pi-session-review-phase-3
plan: explicit-pi-session-review
phase: 3
status: current
updated: 2026-09-02
---

# Explicit Pi Session Review Phase 3

## Context

### Goal

Add a thin manual `/learn` Pi Session path that lists the current project's Sessions once, retains only the newest at most 20 unique bounded IDs, filters completed reviews once, requires an explicit Git changeset association, and then reuses the existing preview-before-model learning flow.

### Current Phase

Phase 3 and the three-phase `explicit-pi-session-review` plan are complete. The user explicitly accepted Pi 0.84.3's list-time message materialization limitation and authorized Phase 3 on 2026-09-02.

The Phase 3 baseline is the committed and pushed Phase 2 commit `3fe2a8d4c7ba0e0e0e85e066620c7fe267be6539`, which was verified equal to `origin/main` before Phase 3 began. The completed Phase 3 changes are in the working tree and have not been committed or pushed.

## Completed

- Injected the supported Pi 0.84.3 `SessionManager.list(cwd, sessionDir)` seam from the minimal extension entry and passed exactly the trusted command cwd plus `ctx.sessionManager.getSessionDir()`.
- Kept listing behind the existing no-argument, interactive-UI, trusted-project gates and the user's explicit `Pi Session` menu choice. No hook, startup action, polling, or background work was added.
- Immediately projects only the first 20 Pi results to validated 1–128-byte ASCII Session IDs, deduplicates while preserving Pi's newest-first order, and keeps no richer `SessionInfo` value in the command state.
- Never calls `listAll` and never reads, displays, transmits, logs, caches, indexes, persists, or branches on Session path, cwd, name, parent, creation/modification time, message count, first/all-message text, prompt, answer, tool, summary, transcript, or leaf data.
- Queries the independent authenticated Session-review route exactly once with the bounded IDs and hides only IDs returned by the daemon's completion-only lookup. Unavailable history is explicit and never falls back to treating candidates as unreviewed.
- Presents only full IDs, verifies the returned UI selection came from the available set, and then requires the unchanged explicit working-tree or commit-range input.
- Calls the independent Session-bound preview route with repository, selected ID, and explicit Git selection. The local preview shows the user-selected Session/Git association before the existing question-generation confirmation.
- Preserved the direct Git `/learn` and generic `/learn-history` paths, preview limits, continuation/model confirmation, answer confirmation, Q1/Q2/Q3 and F1 behavior, rendering, and retry rules.
- Added strict client validation for the ID-only review response: exact fields, protocol version, bounded valid unique IDs, request-subset membership, and candidate order. The review query never retries; preview discovery retains the established one-retry race handling.
- Added explicit empty, all-reviewed, invalid-Pi-data, listing-failure, history-unavailable, cancellation, unsupported-capability, daemon/protocol, and invalid-ID handling without inference or hidden fallback.
- Added no dependency, manifest change, protocol change, database change, Go business-code change, Session write, extension-owned storage, marker, snapshot, reminder, pagination, or automatic model call.

## Modified Files

Implementation:

- `extensions/pi-learnloop.ts`
- `extensions/lib/daemon-client.ts`
- `extensions/lib/learn-command.ts`

Focused verification:

- `tests/extension/daemon-client.test.ts`
- `tests/extension/extension-entry.test.ts`
- `tests/extension/pi-session-review.test.ts`

Stable and lifecycle documentation:

- `README.md`
- `PROJECT.md`
- `plans/explicit-pi-session-review.md`
- `docs/decisions/ADR-0006-explicit-pi-session-provenance.md`
- `docs/checkpoints/explicit-pi-session-review-phase-2.md`
- `docs/checkpoints/explicit-pi-session-review-phase-3.md`

## Important Decisions

- The command module accepts a narrow injected lister returning opaque values; production alone adapts Pi's rich `SessionInfo[]`. Projection reads only `id`, so Session discovery complexity stays behind a small testable seam.
- The top `/learn` menu adds `Pi Session` beside the two existing Git choices. Direct Git choices still enter the old flow without an extra source-selection prompt; Session review alone adds ID and association selections.
- The daemon client owns dedicated request construction, authentication/discovery, response bounds, and strict ID-only validation. The command receives only a small reviewed-ID result and does not know history records or storage details.
- Session identity is locally visible only in the manual ID selector and association preview. The unchanged question method receives only the opaque continuation and active non-secret model metadata.
- Pi's rich list is intentionally not retained for paging, labels, caching, or error diagnostics. Invalid entries among the first 20 fail the manual path safely without echoing the invalid value.

## Tests / Verification

Passed:

- tracer and focused Session command tests: `node --test tests/extension/pi-session-review.test.ts` (6 tests);
- focused Session client tests through authenticated loopback HTTP: `node --test --test-name-pattern='Session' tests/extension/daemon-client.test.ts` (5 tests);
- production-entry test proving the exact cwd/current-Session-directory list call;
- `npm run typecheck`;
- `npm test` (45 tests);
- `npm pack --dry-run --json --cache /private/tmp/pi-learnloop-npm-cache-phase3-01a05fde`;
- `CGO_ENABLED=0 go test -count=1 ./...`;
- `go test -race -count=1 -tags netgo ./...`;
- `go vet -tags netgo ./...`;
- `go build -tags netgo -o /private/tmp/pi-learnloop-phase3-final-01a05fde/pi-learnloop ./cmd/pi-learnloop`;
- `scripts/test-agent-infra.sh`;
- `scripts/validate-agent-infra.sh`;
- `git diff --check`.

The temporary npm cache and ARM64 Mach-O executable were inspected as applicable and removed. No automated verification contacted a provider, read a real Pi Session, wrote production history, or changed a dependency.

The first dry-run pack attempt failed before packing because the sandbox could not write the existing user npm cache. Repeating with the explicit temporary cache above passed; no package tarball was created. Focused and full loopback client tests required the permitted local networking environment after the restricted sandbox rejected `127.0.0.1` binds.

Representative synthetic resource observation used the installed Pi 0.84.3 implementation and temporary JSONL files only:

| Synthetic files | Message bytes/file | Total message bytes | Listed / retained IDs | List time | Heap delta while listed | RSS delta while listed | Heap delta after ID projection + GC |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 20 | 64 KiB | 1.25 MiB | 20 / 20 | 4.8 ms | 6.51 MiB | 1.23 MiB | 0.27 MiB |
| 100 | 256 KiB | 25 MiB | 100 / 20 | 27.4 ms | 49.55 MiB | 26.66 MiB | 0.30 MiB |
| 250 | 64 KiB | 15.63 MiB | 250 / 20 | 26.8 ms | 34.11 MiB | 36.36 MiB | -0.11 MiB |

These are one-host observations, not benchmarks or compatibility guarantees. They confirm that Pi's temporary materialization scales with all candidate files even though LearnLoop immediately retains at most 20 IDs. Every synthetic directory was removed after observation.

## Known Issues

- Session-to-Git association is an explicit user assertion. The Git preview remains authoritative; LearnLoop does not prove that the Session authored every selected change.
- Pi 0.84.3 still reads and temporarily materializes message-derived values for every candidate file returned by `SessionManager.list` before LearnLoop can project IDs. The user explicitly accepted this privacy/resource cost for the manual bounded flow.
- Reviewed filtering is advisory. A completion after the one query can make the UI stale, and duplicate completed reviews remain intentionally valid.
- Only macOS ARM64 was built and observed locally. AMD64 remains an intended but unverified target.
- The configured CodeGraph MCP capability was unavailable during this run, so the required focused inspection used bounded direct source and declaration reads instead.

## Remaining Work

No work remains in this plan. SSE, background coordination, evidence enrichment, and richer answer editing remain separate future capabilities and are not implied by this completion.

## Next Step

Review and commit/push the completed Phase 3 working tree only if explicitly requested. No additional implementation phase is authorized or required by this plan.

## Do Not Change

- Do not infer Git changes from Session time, cwd, name, messages, summaries, tool calls, or filesystem activity.
- Do not add Session fields to existing strict preview, question, assessment, or generic history requests/responses.
- Do not place Session ID in evidence, evaluator values, prompts, RPC/model content, logs, errors, or generic history output.
- Do not add a dependency, index, hook, snapshot, marker, reminder, background worker, Session-file parser/write, extension-owned store, paging, automatic repair, or downgrade.
