---
id: release-ready-macos-distribution-phase-2
plan: release-ready-macos-distribution
phase: 2
status: superseded
updated: 2026-09-04
---

# Release-Ready macOS Distribution Phase 2: Verification Checkpoint

## Context

### Goal

Install and verify read-only native CI and an explicit reviewed-commit manual
bootstrap without changing product behavior or granting publication authority.

### Current Phase

Phase 1 was committed/pushed to main as
`d93dfa5805a7cc172ba2caecfc16d2356422a16c`
(`feat: add local macOS release artifacts`). The user authorized Phase 2 and then
confirmed native ARM64/Intel runner availability and the narrowly scoped
bootstrap plan/ADR amendment on 2026-09-04.

Phase 2 verification resumed after the maintainer supplied the manual bootstrap
run on 2026-09-04. The reviewed installation was committed/pushed as
`2337f1b6dfb99d4b3c998760a19d4d6ea88f9bd4`
(`ci: add read-only native macOS verification`). Its first hosted run failed
GitHub's workflow-expression validation before any native job started. The scoped
fix was committed/pushed as `dd91d494f2fbf96dd0745f36150ef92c02466fec`
(`ci: scope runner cache paths to workflow steps`). Ordinary native CI and manual
bootstrap both succeeded. Both manual native logs have now been inspected and
Phase 2 is complete. At closeout the plan was at Phase 3 `awaiting_approval`.
The subsequent Phase 3 authorization and prerequisite pause are recorded in
`release-ready-macos-distribution-phase-3.md`, now the current checkpoint.

## Completed

- Recovered governance, project, plan, checkpoints, applicable ADRs, manifests,
  command/daemon behavior, and all release scripts from the repository.
- Reverified remote main at the Phase 1 commit, with no tags. GitHub metadata
  confirms a public repository and authenticated maintainer repository access;
  this does not prove a successful runner execution.
- Approved/authorized and active/in-progress metadata validations passed.
- Updated the plan and ADR-0009 with the accepted read-only bootstrap exception.
  Successful signed-tag verification and privileged workflow implementation move
  to separately authorized Phase 3; every stable-publication gate remains.
- Added ordinary push/PR CI with exact event-commit checkout and a two-native-
  runner matrix. Both lanes run governance, extension/package, Go 1.21 pure-Go
  compatibility, Go 1.27.1 test/race/vet/build, and release-artifact self-tests.
- Added a manual main-only bootstrap accepting one exact reviewed commit equal
  to the event SHA and calling that same commit's CI workflow. It has no
  publication mode, release environment, signing job, secret, OIDC, write access,
  persisted checkout credential, cache upload, or artifact upload.
- Extended only the existing release self-test: optional exact clean commit
  validation and both binaries' clean embedded provenance, plus native packaged
  foreground-daemon smoke in a child-only temporary home. Fake Pi accepts only
  two version preflights; status/authentication and SIGTERM cleanup are checked.
  The existing artifact builder/verifier interfaces and implementations are unchanged.

## Modified Files

- `.github/workflows/ci.yml`
- `.github/workflows/release.yml`
- `scripts/test-release-artifacts.sh`
- `PROJECT.md`
- `plans/release-ready-macos-distribution.md`
- `docs/decisions/ADR-0009-release-ready-macos-distribution.md`
- `docs/checkpoints/release-ready-macos-distribution-phase-1.md` (superseded metadata)
- This Phase 2 checkpoint

All are Phase 2 allowed files. No Go, extension, dependency manifest, package
version/content, runtime protocol, history schema, or user data changed.

The implementation and accepted design amendment are pushed. After `dd91d49`,
only this checkpoint, the plan's lifecycle status, and PROJECT's verification
status are local uncommitted closeout changes. Do not claim these closeout
changes have been committed or pushed.

## Important Decisions

codebase-design keeps artifact policy in the existing builder/verifier modules;
the manual workflow reuses CI rather than duplicating its native verification.
Native smoke tests cross the actual packaged executable interface, without a
production adapter or test-only daemon flag.

Reviewed official action release notes, tag-to-commit identities, README, and
action inputs on 2026-09-04:

- checkout v7.0.1: `3d3c42e5aac5ba805825da76410c181273ba90b1`
- setup-node v7.0.0: `820762786026740c76f36085b0efc47a31fe5020`
- setup-go v7.0.0: `b7ad1dad31e06c5925ef5d2fc7ad053ef454303e`

The jobs pin Node 22.19.0 (the declared floor), Go 1.21.13 for source
compatibility, and Go 1.27.1 for release verification. Actions' internal Node 24
runtime is separate from the project Node runtime. Explicit runner labels are
`macos-15` ARM64 and `macos-15-intel`; neither proves execution on macOS 13.

## Tests / Verification

Passed so far:

- Release self-tests twice with exact Go 1.27.1 and the existing local Node
  v26.0.0: repeat-built binaries, static archive/build/dependency checks, all
  previous refusal/tamper/cleanup cases, new expected-commit refusal cases, and
  native ARM64 foreground smoke. Local uncommitted candidates honestly retain
  `vcs.modified=true`; the later exact-clean-commit runs are recorded below.
- Ruby/Psych YAML parsing and bash syntax validation for every workflow run
  step; inspected triggers, job dependencies, read-only permissions, exact pins,
  disabled caches, non-persisted checkout credentials, and native matrix.
- Ten executions of the actual bootstrap gate with synthetic environment values:
  correct main/SHA passes; wrong event/ref/tag/SHA, empty/uppercase/newline input,
  and shell-substitution/injection-like input fail without interpreting the input.
- Extension typecheck, 58/58 provider-free tests, and isolated-cache
  `npm pack --dry-run --ignore-scripts --json`: exactly the six existing paths,
  no bundled dependency, no tarball or publication.
- Both Agent-infrastructure scripts and Git whitespace validation.
- Dependency-manifest diff is empty.
- Go 1.21.13 with `CGO_ENABLED=0 go test -count=1 ./...`: all seven packages pass.
- Go 1.27.1 `go test -count=1 ./...`, `go test -race -count=1 ./...`,
  `go vet ./...`, and `CGO_ENABLED=0 go build` to a temporary output: all pass.
- After installation commit `2337f1b`, the exact-commit release self-test passed
  on the clean checkout. Both binaries record that complete SHA with
  `vcs.modified=false`; repeat hashes are ARM64
  `49ff47f19c2e47892bba08a3441ead24ae3994ff7100964b569474811c5e28d5`
  and AMD64
  `d3de4245c73595d8a22de3907a41a53d7ea899e1dfe5e045bc3ecde12a3cac13`.
  ARM64 foreground smoke passed; these are unsigned local candidates only.
- The exact-commit release self-test also passed on clean `dd91d49`: both
  binaries record the complete latest SHA and `vcs.modified=false`. Repeat hashes
  are ARM64 `51d264d54b69b95fd7d7a1c20cd0b0947592bbd4eb7d4715f15859948b00c112`
  and AMD64 `36d572a68f829333a9ee0c70ef9e8c6563f342590e12dfe9b0f8ccbad36cefc9`.
  Native ARM64 foreground smoke passes. Subsequent hosted Intel execution is
  confirmed below; these local cross-builds alone did not prove it.

Initial broad Go attempts inherited an unrelated local Go 1.26.4 `GOROOT`,
causing compiler-version mismatch before tests. Successful reruns used
`env -u GOROOT`, exact toolchain paths, isolated caches, and automatic switching
disabled. This changes only test-process environment, not system configuration.

Failed hosted verification:

- <https://github.com/reeezark/pi-learnloop/actions/runs/33846093860> rejected
  `runner.temp` in job-level `env` at CI line 40. No jobs/artifacts were created.
  Generic YAML and shell parsing had passed but cannot validate GitHub context
  availability. The fix moves only npm cache configuration to step-level `env`,
  where `runner` is available; permissions and release authority are unchanged.
- A focused regression check reproduced the invalid job context before the fix.
  It passed after the fix, together with YAML, shell, and read-only policy checks.
  Official context availability confirms `runner` is allowed in step-level env,
  not job-level env:
  <https://docs.github.com/en/actions/reference/workflows-and-actions/contexts>.

Hosted evidence inspected on 2026-09-04:

- Corrected push run
  <https://github.com/reeezark/pi-learnloop/actions/runs/33846613156>: success,
  both native jobs passed, total duration 17m 32s. Run metadata and the rendered
  summary identify `push`, main, and exact
  `dd91d494f2fbf96dd0745f36150ef92c02466fec`.
- Ordinary ARM64 job:
  <https://github.com/reeezark/pi-learnloop/actions/runs/33846613156/job/100939801399>.
- Ordinary Intel job:
  <https://github.com/reeezark/pi-learnloop/actions/runs/33846613156/job/100939801304>.
  Its complete log was retrieved. It confirms native x64/amd64, Node v22.19.0,
  macOS 15.7.9 (24G830), image `macos15 20260824.0482.1`, and read-only contents/
  metadata token permissions. Both governance checks, typecheck, 58/58 extension
  tests, exact six-file npm dry run, Go 1.21.13 pure-Go tests, Go 1.27.1
  tests/race/vet/build, release self-tests, and final clean checkout check passed.
- The Intel log confirms both repeat-built binary hashes match the clean
  `dd91d49` values above. The exact-commit self-test checks both binaries'
  `vcs.revision` against that SHA and `vcs.modified=false` before those success
  lines. Native diagnostic/closed invocation and packaged foreground daemon,
  protected discovery/auth/status, fake Pi, and SIGTERM cleanup all passed.
- Maintainer-dispatched bootstrap
  <https://github.com/reeezark/pi-learnloop/actions/runs/33847304094>: success,
  total duration 14m 35s. Metadata and the rendered summary identify
  `workflow_dispatch`, main, and the same full reviewed SHA.
- Its reviewed-commit gate passed:
  <https://github.com/reeezark/pi-learnloop/actions/runs/33847304094/job/100941930054>.
- Manual ARM64 job:
  <https://github.com/reeezark/pi-learnloop/actions/runs/33847304094/job/100941967546>.
  The jobs response reports every verification step successful, including native
  smoke and final clean source. A subsequent log retry succeeded: native arm64,
  Node v22.19.0, macOS 15.7.7 (24G720), image `macos15 20260727.0256.1`,
  Go 1.21.13/1.27.1, both governance checks, typecheck, 58/58 extension tests,
  exact six-file package dry run, all Go test/race/vet/build checks, both expected
  binary hashes and clean provenance, native diagnostic/foreground smoke, and
  final clean checkout check passed. Permissions are contents/metadata read-only.
- Manual Intel job:
  <https://github.com/reeezark/pi-learnloop/actions/runs/33847304094/job/100941967429>.
  The rendered job page reports success in 14m 16s; its earlier jobs response
  still showed release-toolchain checks in progress and is not the final result.
  The log retry succeeded and confirms native x64/amd64, Node v22.19.0, macOS
  15.7.9 (24G830), image `macos15 20260824.0482.1`, Go 1.21.13/1.27.1,
  read-only contents/metadata permissions, both governance checks, typecheck,
  58/58 extension tests, the exact six-file npm dry run, all Go test/race/vet/build
  checks, both matching binary hashes and clean provenance, native diagnostic/
  foreground smoke, and final clean checkout. No detail is inferred from the
  ordinary Intel log instead of this manual run.
- Both artifact endpoints returned `artifacts: []`; final rendered run summaries
  also show no artifacts. Reviewed workflow code has no upload/cache-save,
  secrets/environment, OIDC, signing, tag, or publication path.
- A fresh remote read confirms main still equals the reviewed SHA and no tags
  exist. The three local closeout documents have not been committed or pushed.

No signed-tag, signing/notarization, or publication verification was run. No live
model was called. macOS 13 is a technical floor, not an executed test platform.

Documentation closeout verification on 2026-09-04: `scripts/test-agent-infra.sh`,
`scripts/validate-agent-infra.sh`, and `git diff --check` passed. The status,
diff stat, and complete diff contain only the three allowed closeout documents;
workflow, script, and dependency-manifest diffs against `dd91d49` are empty.

## Known Issues

- CodeGraph tools are unavailable in this session; focused source reads were
  used without changing the existing index.
- The earlier lack of connector dispatch and signed-out browser was resolved by
  the maintainer's manual run link, not by changing authentication or workflow
  permissions. No repeat dispatch is needed for the already successful run.
- The first browser navigation timed out; recovery reached the public Actions
  page and confirmed the signed-out state. No account setting was changed.
- The authenticated run query eventually returned; a public API fallback received
  HTTP 403. The rendered GitHub run page supplied the exact validation diagnostic.
- The first-run jobs endpoint confirms an empty job list. Earlier artifact and
  corrected-run queries returned HTTP 503. This turn retrieved both run metadata,
  the manual jobs response, both empty artifact lists, and the complete ordinary
  Intel log. The ordinary jobs response still returned HTTP 503; ordinary ARM64
  and both initial manual log requests timed out after 300 seconds. Retrying the
  two manual native logs succeeded. The duplicate ordinary ARM64 log was not
  retrieved; its run status is confirmed by the successful ordinary matrix,
  while the complete same-commit manual ARM64 log supplies the native evidence.
  Retrieval failures are not CI failures or successful log inspections.
- Browser summary reads succeeded, but restoring the browser for further log
  inspection was blocked by an automatic approval timeout (not a user denial or
  evidence that the read is unsafe). No alternate browser, credential extraction,
  account/permission change, or access-control workaround was attempted.
- Exact release SemVer, protected-environment approver, trusted signer, Apple
  credentials, and npm ownership/trusted publisher remain Phase 3 prerequisites.

## Remaining Work

No Phase 2 implementation or hosted acceptance work remains. The three local
closeout documents await explicit commit/push direction. This turn changes no
workflow, script, business code, or dependency; local business suites were not
rerun for documentation-only closeout. The recorded local suite and verified
ordinary/manual hosted suites remain the execution evidence.

Phase 3 still requires separate authorization and confirmation of the exact
release version, protected-environment approver, signing identity/Apple access,
npm ownership/trusted publisher, and intended external mutations. Successful
unsigned bootstrap does not satisfy any signed-tag or stable-publication gate.

## Next Step

Report the Phase 2 verification result and stop. Commit/push the three closeout
documents only when directed; confirm Phase 3 prerequisites before its separately
authorized work. Do not collect credentials, change settings, or start Phase 3.

## Do Not Change

No business code, Go 1.21 baseline, dependency, protocol, schema, model/evidence
behavior, daemon lifecycle, package version/contents, or user data. No tag,
release, npm publication, environment/secret mutation, signing/notarization,
OIDC, or artifact/cache upload. Phase 3 is not authorized.
