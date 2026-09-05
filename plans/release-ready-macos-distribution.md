---
id: release-ready-macos-distribution
status: superseded
risk: high
current_phase: 3
phase_status: blocked
updated: 2026-09-05
---

# Release-Ready macOS Distribution

## Scope Update: Personal Local Use

On 2026-09-04 the user clarified that the product is for their own development
environment as a Pi extension; on 2026-09-05 they requested continued repair
until it is usable. This public-distribution plan is therefore superseded, not
complete. Preserve the completed Phase 1/2 implementation and CI evidence.
Unimplemented signing, notarization, npm publication and Phase 3 prerequisites
are not blockers for the current local-use goal and must not be pursued under
the old authorization. The current repair design is
`plans/isolated-pi-model-runtime.md`, with its own high-risk phase gates.

## 1. Goal

Turn the source-checkout-only Pi LearnLoop MVP into a verifiable public macOS
release for Apple Silicon and Intel without changing its local security,
evidence, evaluator, history, or foreground-lifecycle behavior.

The release must present one product version across the Pi extension and daemon,
produce inspectable per-architecture daemon artifacts, establish native release
verification, and make stable publication conditional on supply-chain controls
and Apple notarization. The design deepens the existing extension package and
daemon executable modules behind narrow release interfaces; it does not add a
launcher, installer service, update service, or another runtime owner.

## 2. Background

The accepted ADR-0003 through ADR-0008 slices complete the current learning
loop. Distribution remains the largest gap between the implemented MVP and
`PROJECT.md`'s goal of a public, long-term MIT-licensed project:

- users must currently clone the repository, install development dependencies,
  run `go run ./cmd/pi-learnloop daemon`, and point Pi at the local checkout;
- `package.json` is an installable Pi package manifest at version `0.1.0`, but no
  npm publication or release automation exists;
- the npm package allowlist intentionally includes the extension, README, package
  manifest, and root LICENSE, but no daemon executable;
- macOS ARM64 has been exercised locally while AMD64 is only an intended target;
  and
- the repository has no tags, release workflow, continuous-integration workflow,
  changelog, security policy, or contributor guide.

The installed Pi 0.84.3 package documentation treats npm packages as extension
containers and runs their install lifecycle in Pi's package directory. It also
states that package extensions have the user's full system access. Embedding a
platform binary, downloading one in `postinstall`, or starting one from the
extension would therefore enlarge the most security-sensitive seam and couple
Pi package installation to native artifact selection. The smallest honest
design keeps the extension and daemon as separate artifacts with a shared
version.

Authoritative external constraints checked during investigation:

- Node.js 22 supports macOS x64 and arm64 from macOS 11.0. The Go module keeps
  its 1.21 language baseline, but the current stable release toolchain is Go
  1.27.1 and Go 1.27 requires macOS 13 or newer. Stable binaries will use that
  supported toolchain rather than the obsolete Go 1.21 compiler, so the complete
  product adopts macOS 13 as its initial technical floor. See
  <https://raw.githubusercontent.com/nodejs/node/v22.x/BUILDING.md>,
  <https://go.dev/dl/?mode=json>, and
  <https://go.dev/wiki/MinimumRequirements>.
- GitHub currently documents standard hosted macOS ARM64 and Intel runner labels,
  including `macos-15` and `macos-15-intel`; their availability and billing for
  this repository must still be confirmed before workflow implementation. See
  <https://docs.github.com/en/actions/how-tos/write-workflows/choose-where-workflows-run/choose-the-runner-for-a-job>.
- npm trusted publishing exchanges a GitHub-hosted runner's OIDC identity for a
  short-lived publish credential, requires Node 22.14.0 or newer, npm CLI 11.5.1
  or newer, and `id-token: write`, and automatically publishes provenance for
  supported GitHub/GitLab flows. This repository's stricter Node `>=22.19.0`
  requirement still controls its jobs. See
  <https://docs.npmjs.com/trusted-publishers/> and
  <https://docs.npmjs.com/generating-provenance-statements/>.
- Apple directs software distributed outside the Mac App Store to use a
  Developer ID signature and notarization; current custom command-line flows use
  `notarytool`. See <https://developer.apple.com/developer-id/> and
  <https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution>.

## 3. Current Behavior

Verified current behavior is:

- `cmd/pi-learnloop` is a minimal executable adapter. It accepts exactly
  `pi-learnloop daemon`, writes usage/errors to stderr, installs SIGINT/SIGTERM
  cancellation, and exposes no weakening flags or version command.
- `daemon.Run` owns the protected local runtime, foreground process, loopback
  listener, descriptor/token lifecycle, evaluator processes, and optional
  history store. There is no launchd unit, daemonization, background supervisor,
  update path, or installer-owned state.
- `package.json` names `pi-learnloop` version `0.1.0`, requires Node
  `>=22.19.0`, has exactly pinned development dependencies, declares Pi as a
  peer, and has no third-party runtime npm dependency.
- An isolated-cache `npm pack --dry-run --json` contains exactly six paths:
  `LICENSE`, `README.md`, `package.json`, `extensions/pi-learnloop.ts`,
  `extensions/lib/daemon-client.ts`, and `extensions/lib/learn-command.ts`. It
  contains no daemon binary and no bundled dependency.
- Pi can persistently install the extension only from the local checkout as
  documented. The daemon is started separately with `go run`.
- The Go module declares language baseline 1.21. The latest local verification
  ran on Go 1.26.4; an earlier repository checkpoint recorded Go 1.21.13,
  `CGO_ENABLED=0` source-compatibility verification. Neither is the selected
  stable-release compiler: official Go downloads identify Go 1.27.1 as the
  current stable toolchain at this design date.
- The accepted transport exposes protocol v1 and strict response shapes. The
  descriptor and `/v1/status` have no product-version field. SQLite schema v2,
  evaluator input versions, prompt versions, and Pi 0.84.3 compatibility are
  independent version domains.
- The Git repository currently has no release tag or `.github` workflow. The
  package/repository's public reachability, npm name ownership, Apple credentials,
  and release-environment configuration were not provable from repository files.

## 4. Relevant Call Chain

The proposed daemon artifact path is:

```text
clean accepted release commit
→ package.json SemVer
→ matching immutable signed Git tag v<SemVer>
→ release artifact builder
→ exact Go 1.27.1 toolchain + CGO_ENABLED=0
→ unsigned darwin/arm64 and darwin/amd64 executables
→ repeat-build binary digest comparison
→ Developer ID Application signing with hardened runtime and timestamp
→ per-architecture ZIP archives with LICENSE and release README
→ Apple notarytool submission and accepted-result verification
→ final SHA-256 manifest
→ protected GitHub Release publication
```

The proposed extension path is:

```text
same clean signed tag and package.json SemVer
→ npm ci with the committed lockfile
→ typecheck + extension tests + npm package dry-run allowlist check
→ GitHub-hosted trusted-publisher OIDC identity
→ npm publication with registry provenance
```

The installed runtime path remains:

```text
user installs the matching daemon archive to a PATH directory
→ user installs the same-version npm Pi package
→ user manually starts pi-learnloop daemon in the foreground
→ daemon writes its protected descriptor and token
→ Pi loads the extension
→ existing authenticated discovery and strict protocol-v1 flow
```

The product version is release provenance and a diagnostic. Runtime
compatibility continues to be enforced by the existing protocol, schema, strict
response, and Pi-version contracts rather than by an exact SemVer handshake.

## 5. Relevant Files

- `package.json` and `package-lock.json`: current package version, npm allowlist,
  exact development dependency closure, Node floor, and Pi extension entry.
- `go.mod` and `go.sum`: Go 1.21 language baseline and fixed implementation
  dependencies that release work must not change.
- `cmd/pi-learnloop/main.go` and `main_test.go`: the only public executable
  command seam and its unsupported-invocation behavior.
- `internal/daemon/runtime.go`, `daemon.go`, `server.go`, and their tests: current
  foreground daemon, protected runtime descriptor, shutdown, and protocol
  invariants that release packaging must preserve.
- `extensions/pi-learnloop.ts`, `extensions/lib/daemon-client.ts`, and extension
  tests: package-loaded extension and existing strict daemon compatibility seam.
- `README.md` and `PROJECT.md`: present source-checkout installation, verified
  platform state, security guarantees, and remaining release gap.
- `scripts/test-agent-infra.sh` and `scripts/validate-agent-infra.sh`: governance
  verification that every phase continues to run.
- ADR-0002, ADR-0003, ADR-0005, and ADR-0008: foreground lifecycle, strict local
  protocol, exact Pi adapter, protected persisted state, and matched-update
  constraints that distribution must not weaken.
- Pi 0.84.3's installed `docs/packages.md`: current package installation and
  extension trust model.

Files proposed later by this plan include a local release builder and artifact
verifier under `scripts/`, narrowly scoped GitHub Actions workflows, public
release/support documents, and phase checkpoints. They do not exist today.

## 6. Scope

This task may:

- define `package.json`'s SemVer as the single product-release version and require
  an exact matching `v<SemVer>` Git tag;
- add a read-only `pi-learnloop version` diagnostic whose release value is
  injected into the daemon binary while local unversioned builds report `dev`;
- build separate `darwin-arm64` and `darwin-amd64` daemon executables and ZIP
  archives without adding a runtime dependency;
- add a release artifact builder that owns version validation, exact build flags,
  artifact naming, archive composition, and refusal of inconsistent inputs;
- add a separate artifact verifier that owns architecture, embedded version,
  archive contents, hashes, and local smoke checks;
- add least-privilege continuous-integration and manually gated release
  workflows using reviewed full-commit action pins;
- require native test/smoke jobs on GitHub-hosted ARM64 and Intel macOS runners;
- add Developer ID signing, hardened runtime, secure timestamp, Apple notarization,
  and final checksum gates for stable daemon artifacts;
- use npm trusted publishing from a GitHub-hosted runner after registry/repository
  prerequisites are confirmed;
- document same-version installation, upgrade, rollback, uninstall, verification,
  support, vulnerability reporting, and release history; and
- update lifecycle metadata, `PROJECT.md`, and phase checkpoints as authorized
  phases finish.

### Allowed files by phase

Phase 1 may modify only:

- `cmd/pi-learnloop/main.go`
- `cmd/pi-learnloop/main_test.go`
- `scripts/build-release-artifacts.sh`
- `scripts/verify-release-artifacts.sh`
- `scripts/test-release-artifacts.sh`
- `PROJECT.md`
- `plans/release-ready-macos-distribution.md`
- `docs/decisions/ADR-0009-release-ready-macos-distribution.md`
- `docs/checkpoints/release-ready-macos-distribution-phase-1.md`

Phase 2 may modify only:

- `.github/workflows/ci.yml`
- `.github/workflows/release.yml`
- `scripts/build-release-artifacts.sh`
- `scripts/verify-release-artifacts.sh`
- `scripts/test-release-artifacts.sh`
- `package.json` only if a repository-supported dry-run command is needed without
  changing the package contents, dependency graph, name, or version
- `PROJECT.md`
- `plans/release-ready-macos-distribution.md`
- `docs/decisions/ADR-0009-release-ready-macos-distribution.md`
- `docs/checkpoints/release-ready-macos-distribution-phase-1.md`
- `docs/checkpoints/release-ready-macos-distribution-phase-2.md`

Phase 3 may modify only:

- `.github/workflows/ci.yml` and `.github/workflows/release.yml` for the deferred
  signed-tag, protected signing/notary, and publication gates
- `scripts/build-release-artifacts.sh`, `scripts/verify-release-artifacts.sh`,
  and `scripts/test-release-artifacts.sh` for signed-archive verification
- `README.md`
- `SECURITY.md`
- `CONTRIBUTING.md`
- `CHANGELOG.md`
- `package.json` only for the authorized release version and publication metadata
  that does not expand package contents or dependencies
- `package-lock.json` only if the authorized version change updates its root
  package metadata and makes no dependency change
- `PROJECT.md`
- `plans/release-ready-macos-distribution.md`
- `docs/decisions/ADR-0009-release-ready-macos-distribution.md`
- `docs/checkpoints/release-ready-macos-distribution-phase-2.md`
- `docs/checkpoints/release-ready-macos-distribution-phase-3.md`

Phase 3 also contains external publication mutations: the signed Git tag, GitHub
Release, release assets, and npm package version. Those actions require explicit
Phase 3 authorization after their exact targets and prerequisites are verified.
Any additional file or external target requires stopping and amending the
approved plan before it is changed.

## 7. Out of Scope

- Implementing any part of Phase 1 while this plan and ADR remain unaccepted.
- Bundling either daemon binary in the npm package, adding a native npm package,
  downloading/building a daemon during npm lifecycle scripts, or executing a
  daemon from package installation.
- Automatic daemon discovery outside the accepted protected descriptor,
  automatic startup, daemonization, launchd, login items, background supervision,
  scheduled checks, self-update, automatic extension update, or rollback code.
- Homebrew taps/formulas, MacPorts, `.pkg`, `.dmg`, App Store distribution, a
  universal/fat binary, or an installer UI.
- Linux, Windows, containers, remote daemon access, non-loopback listening, web
  UI, or remote Agent control.
- Changes to HTTP routes, descriptor/status shapes, Instance Token handling,
  protocol v1, CORS, proxy bypass, retries, database schema v2, migration rules,
  evidence/evaluator contracts, prompts, model selection, history contents, or
  Pi Session behavior.
- Adding, removing, or upgrading Go or npm package dependencies; upgrading Go's
  1.21 language baseline, Node/Pi runtime requirements, Pi 0.84.3, SQLite, or
  `golang.org/x/mod`. The separately pinned Go 1.27.1 release compiler and npm
  11.5.1+ publication client are release tools, not manifest dependencies.
- Putting a product version into `/v1/status`, the runtime descriptor, evidence,
  evaluator input, prompts, model-visible content, history, logs, or daemon
  errors.
- Persisting signing/notarization credentials, npm credentials, OIDC tokens,
  release secrets, build directories, or unredacted tool output in the repository
  or release assets.
- Publishing an unsigned/unnotarized artifact as stable, telling users to bypass
  Gatekeeper/quarantine, or claiming an architecture/OS was verified from
  cross-compilation alone.
- Fixing unrelated product features, SSE/durable workers, packaging cleanup,
  dependency maintenance, or repository-wide formatting.

## 8. Proposed Changes

### 8.1 Use one version with two distribution channels

`package.json` will remain the only product SemVer authority. A release tag must
be exactly `v` plus that value, and every daemon binary and archive name must use
the same value. The npm Pi package remains extension-only; GitHub Releases carry
the daemon's native ZIP archives and checksum manifest. Release instructions
will require installing matching versions, but ordinary runtime operation will
not compare them.

This keeps the package module deep: Pi receives only its TypeScript extension
and the package metadata it already knows how to load. Native artifact selection,
Apple trust, and PATH installation remain outside Pi's package lifecycle.

### 8.2 Add a diagnostic version command without widening the runtime protocol

The executable adapter will accept `pi-learnloop version` in addition to the
existing exact `daemon` invocation. It will write exactly
`pi-learnloop <version>\n` to stdout and exit zero. Unsupported invocations will
retain exit code 2 and show the updated closed command set. Daemon failures will
retain their existing stderr and exit-code behavior.

Direct developer builds report `dev`. The artifact builder injects the validated
package SemVer into a private `main` package variable at link time; only output
from a clean matching signed tag is eligible for publication. The value is not
configuration, a compatibility authority, or runtime state and must not enter
any local transport or persisted/model-visible value.

### 8.3 Make the artifact builder a deep release module

`scripts/build-release-artifacts.sh <version> <output-directory>` will expose the
only local production-artifact build interface. Behind it, the script will:

- accept only canonical SemVer equal to `package.json`;
- require exactly Go 1.27.1 instead of allowing Go's automatic toolchain
  download or silently accepting another installed compiler;
- build with `CGO_ENABLED=0`, `GOOS=darwin`, `GOARCH=arm64|amd64`,
  `-mod=readonly`, `-trimpath`, `-buildvcs=true`, and only the link-time version
  value;
- produce `pi-learnloop_<version>_darwin_arm64.zip` and
  `pi-learnloop_<version>_darwin_amd64.zip` through fixed staging directories;
- include the executable, root `LICENSE`, and a small release-install README in
  each ZIP; and
- refuse to overwrite a nonempty output directory or emit a partial success.

It will not sign, notarize, upload, install, or run either executable. Those are
separate privileged release stages. No new Go interface or general build
framework will be introduced: the script is the narrow adapter around the
existing `cmd/pi-learnloop` module.

`scripts/verify-release-artifacts.sh <version> <artifact-directory>` will be the
independent read-only verifier. It will check the exact file set, ZIP entries and
permissions, Mach-O architecture, embedded diagnostic version, Go build
metadata, recorded VCS revision/modified state, and SHA-256 manifest. It will
reject missing/malformed provenance, extra entries, and mixed versions. It does
not decide whether the recorded commit is eligible for release; the privileged
workflow owns the clean signed-tag source gate. This separation lets Phase 1
exercise the builder before its authorized changes are committed without making
a dirty candidate publishable.

Unsigned executables built twice from the same commit with the same exact
toolchain must have identical hashes. ZIP and signed bytes are not declared
reproducible: archive metadata and Apple's secure timestamp make that promise
dishonest. Final signed archives receive post-notarization SHA-256 values.

### 8.4 Separate ordinary CI from privileged release publication

The ordinary CI workflow will use read-only permissions and no release secrets.
It will run Go 1.21 source-compatibility checks plus Go 1.27.1 release-toolchain
checks, extension checks under the declared Node version, Agent-infrastructure
checks, package allowlist dry run, and release-script self-tests. Pull-request
code will never execute in a context that can access Apple or npm publication
authority.

Release preparation will be manual and reference an existing immutable signed
tag. The release workflow must validate the tag, package version, checked-out
commit, dependency lockfiles, and complete test suite before reaching a protected
`release` environment. Build and smoke jobs will run natively on both
`macos-15` ARM64 and `macos-15-intel` rather than treating cross-compilation as
execution evidence. Runner labels remain explicit, not floating
`macos-latest`, and any change to them requires a reviewed workflow diff.

All third-party or GitHub-maintained actions must be pinned to reviewed full
commit SHAs. Their pins are release-tool dependencies even though the
application dependency manifests remain unchanged. Job permissions will be
declared at the narrowest scope: build/test jobs get read-only contents;
publication alone may receive `contents: write`; npm publication alone receives
`id-token: write`.

On 2026-09-04 the user approved a narrowly isolated first-run exception: Phase 2
may commit/push the reviewed read-only workflows to the default branch and run
bootstrap verification of that exact 40-lowercase-hex commit. This is verification,
not release preparation or signed-tag eligibility. The manual bootstrap requires
`refs/heads/main` and an explicit commit equal to the dispatch event SHA; it
cannot select arbitrary source, tags, or publication modes. It reuses the same
read-only native suite as CI, with no secrets, environment, OIDC, write permission,
cache upload, artifact upload, signing, or publication job. Ordinary push/PR CI
checks the exact event SHA, including a PR merge ref, without privileged triggers.

Successful signed-tag verification and implementation of protected signing/notary
and OIDC publication jobs move to separately authorized Phase 3. Stable
publication, repository environment changes, npm trusted-publisher configuration,
and release credentials also remain Phase 3 external mutations. Every original
publication trust gate remains mandatory; bootstrap success grants none of them.

### 8.5 Require Apple trust gates for stable daemon artifacts

The stable release job will sign each native daemon executable with a Developer
ID Application identity, hardened runtime, no custom entitlements, and a secure
timestamp. It will archive the signed executable, submit each ZIP with
`notarytool`, wait for an accepted result, and verify both signature validity and
Gatekeeper assessment on a freshly downloaded/extracted artifact before
publication completes.

Apple credentials and certificate material must live only in a protected GitHub
environment or its delegated secret store, be materialized only for the signing
job, and be deleted by runner teardown. Logs and artifacts must never include
their values. If credentials, Apple service access, either native runner, or
verification is unavailable, a test artifact may be retained only as an
explicitly unsigned non-release artifact; the workflow must fail closed before
creating/updating a stable GitHub Release or publishing npm.

After notarization, the workflow generates one `SHA256SUMS` covering only the
final ZIP files. GitHub's authenticated release page, its asset digest, the
manifest, and the Developer ID signature provide complementary transport,
integrity, and publisher evidence; a checksum alone is not an authenticity
mechanism.

### 8.6 Publish the extension through npm trusted publishing

Phase 3 will configure one npm trusted publisher for the exact GitHub repository,
workflow filename, and protected environment. The workflow will use a
GitHub-hosted runner and short-lived OIDC identity; no long-lived npm write token
will be added. The publication job must pin and verify an exact Node version that
satisfies `>=22.19.0` and an exact reviewed npm CLI version that satisfies
`>=11.5.1`, independently of the application's package dependency closure.
Publication must run `npm ci`, typecheck, tests, and
`npm pack --dry-run --json`, then prove the exact six-entry extension-only
allowlist before `npm publish`.

The GitHub Release and npm publish operations are one controlled release
transaction operationally, but neither can be rolled back atomically. The job
therefore publishes immutable signed daemon assets first as a draft, publishes
the same-version npm package second, verifies the registry result, and only then
makes the GitHub Release public. Any failure leaves an explicit failed/draft
state for manual resolution; it must not overwrite a tag, silently republish a
version, or substitute a different commit.

### 8.7 Document an honest manual installation and support contract

Public documentation will state:

- macOS 13 or newer on Apple Silicon or Intel, Node `>=22.19.0`, Pi 0.84.3,
  Git, and an available configured model;
- which native runner/OS versions validated the release, separately from the
  minimum technical floor;
- how to verify SHA-256, Developer ID signature, notarization/Gatekeeper, daemon
  version, and npm package version;
- how to place the daemon in a user-selected PATH directory, install the exact
  matching npm package version with Pi, and manually start the foreground daemon;
- how to upgrade both artifacts together, roll back to an earlier complete
  pair, uninstall without deleting history by default, and deliberately remove
  local data if desired;
- that no auto-update, service manager, telemetry, cloud sync, background
  Session indexing, or recovery from an interrupted release is provided; and
- the supported vulnerability-reporting path and release/change history.

No installation step may use `sudo` by default, disable Gatekeeper, remove
quarantine attributes as a workaround, execute a remote shell, or hide that the
daemon and extension are separately installed.

## 9. Compatibility

- Existing `pi-learnloop daemon` behavior, stdout/stderr expectations, signals,
  protected runtime paths, and cleanup remain unchanged. `version` is an additive
  command; unsupported invocations remain closed.
- Protocol v1, the descriptor, `/v1/status`, all request/response keys, body
  limits, error semantics, SQLite schema v2, evidence/evaluator schemas, prompt
  assets, and durable rows do not change.
- Product SemVer does not replace protocol/schema/prompt/Pi versions. An exact
  runtime SemVer match is recommended and installed by documentation, not
  silently enforced. Existing strict validation remains the compatibility
  authority.
- The npm tarball retains its current extension-only file set and peer/runtime
  dependency behavior. Pi still supplies its own runtime and loads TypeScript
  from the package.
- Go stays at language baseline 1.21, stable releases use the exact supported Go
  1.27.1 compiler and remain pure Go with `CGO_ENABLED=0`, Pi stays exactly
  0.84.3 for evaluator compatibility, and no dependency manifest changes except
  an authorized root package version update.
- Existing source-checkout development remains possible with `go run`; such a
  build reports `dev` and is never presented as a released binary.
- Both macOS architectures receive separate artifacts. No caller must detect or
  unpack a universal binary, and no architecture fallback is provided.
- Initial product floor is macOS 13 because the supported Go 1.27 release
  toolchain is stricter than the Node 22 runtime. The release notes must also
  record the newer native runner versions actually tested; a build on those
  runners is not evidence that macOS 13 itself was executed.

## 10. Risks

- **Split-version installation:** users can install a daemon and extension from
  different releases. Mitigation: exact-version commands, copyable paired
  installation steps, matched upgrade/rollback guidance, and preserved strict
  protocol failures. Do not introduce an unplanned network handshake.
- **Supply-chain compromise:** mutable action tags, arbitrary refs, dependency
  drift, or long-lived registry tokens could alter artifacts. Mitigation: signed
  immutable source tag, lockfiles, exact toolchains, full-SHA action pins, OIDC,
  least privilege, protected environment, and post-download verification.
- **Signing-secret exposure:** pull-request code or logs could access Developer
  ID/notary material. Mitigation: never expose secrets to PR/build jobs; gate
  signing in a reviewer-protected environment and inspect logs for redaction.
- **False reproducibility claim:** signatures, timestamps, and ZIP metadata make
  final archives nondeterministic. Mitigation: compare only pre-signing executable
  hashes and publish final checksums without calling final archives reproducible.
- **Architecture false confidence:** a cross-compiled Mach-O can still fail at
  runtime. Mitigation: native ARM64 and Intel command/daemon smoke tests are
  mandatory release gates.
- **Minimum-OS overclaim:** selected hosted runners do not execute macOS 13.
  Mitigation: state the release-toolchain-derived technical floor and the actual
  verified OS/runner matrix separately; do not imply minimum-version execution
  coverage.
- **Apple service or credential absence:** stable direct distribution cannot meet
  the selected trust policy. Mitigation: block stable publication and retain only
  clearly labeled ephemeral unsigned verification artifacts.
- **Partial release:** npm versions and GitHub tags/assets are immutable in
  different systems. Mitigation: stage assets in a draft, publish npm only after
  daemon verification, publish the GitHub Release last, and document manual
  recovery without tag/version reuse.
- **Version drift:** tag, package, binary, archive, changelog, and release title
  can disagree. Mitigation: derive/compare every value with the package manifest
  and reject any mismatch before privileged work.
- **Package-content expansion:** a broad npm allowlist could ship source or
  binaries unintentionally. Mitigation: assert the exact current six paths and
  fail on extras, not merely on forbidden-name patterns.
- **Runner availability/cost changes:** GitHub labels and account eligibility are
  external state. Mitigation: confirm before Phase 2 and treat runner-label
  changes as reviewed release-interface changes.
- **Registry/repository eligibility:** npm name ownership, public visibility, or
  trusted-publisher support may be missing. Mitigation: verify them before Phase
  2/3 and block rather than fall back to a broad token or private provenance gap.
- **Accidental publication:** tag triggers or permissive workflow inputs can make
  test runs release. Mitigation: manual dispatch, exact signed-tag validation,
  default non-publishing mode, protected environment approval, and no publication
  authority in ordinary CI.
- **Release script shallowness:** distributing orchestration across workflow steps
  can make local and CI artifacts diverge. Mitigation: one artifact-builder module
  owns build policy and one verifier owns the closed artifact contract; workflows
  only adapt credentials and hosted runners.

## 11. Implementation Phases

### Phase 1: Local version and unsigned artifact contract

Prerequisite: ADR-0009 is accepted and the user explicitly authorizes Phase 1.

ADR-0009 was accepted and Phase 1 explicitly authorized on 2026-09-04,
including obtaining Go 1.27.1 after verifying the official go.dev checksum.
The Go 1.21 module baseline and all dependency versions remain unchanged.

Completed on 2026-09-04. The implementation and verification evidence are
recorded in `docs/checkpoints/release-ready-macos-distribution-phase-1.md`.
The local modules use the existing Node runtime and native macOS ZIP/Mach-O
inspection tools without package additions. They publish only the two ZIPs and
`SHA256SUMS` after independent static verification. Output must be outside the
checkout and have an existing parent; ordinary failure or handled cancellation
cleans private staging and leaves no partial published set. SIGKILL or host
failure cannot guarantee temporary-directory cleanup.

Static diagnostic inspection follows the file-backed Go string header through
the frozen toolchain's Mach-O segments and native `nm` symbol, rather than
executing the candidate or accepting an arbitrary matching string. The verifier
also compares dependency versions and sums to the unchanged manifests and
checks exact README/license bytes. Both inspection and hashes use the same
private archive copies. These are integrity/consistency checks, not signatures
or proof of release eligibility.

Repeat builds in the authorized, uncommitted checkout matched for both
architectures and honestly record `vcs.modified=true`. Native ARM64 diagnostic
execution passed; Intel native execution and clean signed-tag release checks
remain Phase 2 work. No commit, push, tag, signing, or publication was performed.

- Change plan/ADR metadata to approved/accepted, then active/in-progress before
  implementation.
- Add the bounded `version` command with `dev` default and release link-time
  injection; preserve the exact daemon path and unsupported-invocation behavior.
- Add the deep local artifact builder, independent verifier, and deterministic
  self-test for two unsigned per-architecture executables/ZIPs.
- Enforce package-version equality, exact Go 1.27.1 version,
  `CGO_ENABLED=0`, fixed flags/names/contents, non-overwrite, and partial-output
  cleanup.
- Test repeat-built unsigned executable hashes, both Mach-O architectures,
  embedded versions, recorded VCS metadata, archive allowlists, invalid/mismatched
  versions, and safe output-directory refusal.
- Do not add workflows, signing/notarization, public installation docs, tags,
  releases, npm changes, dependencies, or external mutations.
- Update `PROJECT.md`, record a current Phase 1 checkpoint, advance the plan to
  Phase 2 `awaiting_approval`, and stop.

### Phase 2: Native CI and read-only bootstrap verification

Prerequisite: Phase 1 is committed, the runner/account prerequisites below are
confirmed, and the user explicitly authorizes Phase 2.

The user authorized Phase 2 and the Phase 1 commit/push on 2026-09-04.
Phase 1 is now committed and pushed to `origin/main` as
`d93dfa5805a7cc172ba2caecfc16d2356422a16c`. The user then confirmed native
ARM64/Intel runner availability and approved the plan/ADR bootstrap amendment
on 2026-09-04. The earlier prerequisite pause is resolved; actual successful
hosted execution is still required, not inferred from that confirmation.

The remote has no tags. The approved sequencing installs a reviewed, read-only,
no-upload workflow first, then verifies its exact commit on both native runners.
Successful signed-tag verification and privileged workflow implementation move
to Phase 3, where tag creation can be separately authorized. GitHub requires a
manually dispatched workflow to exist on the default branch.
See <https://docs.github.com/en/actions/how-tos/manage-workflow-runs/manually-run-a-workflow>.
The Phase 2 verification record is
`docs/checkpoints/release-ready-macos-distribution-phase-2.md`. No tag, approval
rule, credential, or publication authority is created by this exception.

The read-only implementation was installed as `2337f1b`, followed by the scoped
GitHub context fix `dd91d494f2fbf96dd0745f36150ef92c02466fec`; both were pushed
to main on 2026-09-04. All local verification passed. The corrected native run
<https://github.com/reeezark/pi-learnloop/actions/runs/33846613156> succeeded.
The maintainer supplied manual bootstrap run
<https://github.com/reeezark/pi-learnloop/actions/runs/33847304094> for that exact
main SHA on 2026-09-04. Phase 2 is complete: both runs succeeded with both native
lanes passed, both artifact queries returned empty lists, and final run summaries
show no artifacts. Both manual native logs were retrieved and inspected after
transient connector failures. They confirm exact toolchains, complete suites,
clean source/binary provenance, repeat-built hashes, native diagnostic/foreground
smoke, and final checkout cleanliness. Actual manual platforms were ARM64 macOS
15.7.7 (24G720), image `20260727.0256.1`, and Intel macOS 15.7.9 (24G830),
image `20260824.0482.1`; macOS 13 was not executed. The checkpoint records exact
run/job URLs, digests, and the earlier failed installation run. The three
closeout documents remain uncommitted; no Phase 3 implementation or external
publication is authorized by successful Phase 2 verification.

- Add ordinary least-privilege CI for exact Go/Node toolchains, Go/extension/
  governance suites, npm allowlist, and release-script tests.
- Add a manually dispatched bootstrap workflow that accepts only the exact
  reviewed commit at the dispatched main ref and repeats the same verification.
- Pin every action to a reviewed full commit SHA and disable dependency caching
  and persisted checkout credentials for all jobs. Pin Node 22.19.0, source
  compatibility Go 1.21.13, and release Go 1.27.1; verify actual versions and
  native architecture. Keep all manifests unchanged.
- Extend the existing release self-test with native foreground-daemon smoke and
  an optional explicit expected commit argument. With that argument, reject a
  dirty checkout or wrong HEAD and require both binaries' embedded revision to
  match with `vcs.modified=false`. Default local tests still permit dirty
  unsigned candidates. No artifact builder/verifier interface change is needed.
- Run command/daemon smoke tests natively on explicit ARM64 and Intel macOS
  runners; record the actual images/OS versions as release evidence.
- Add no signing/notary, environment, OIDC, cache/artifact upload, or publication
  job, including disabled privileged jobs. Missing runner or verification is a
  failed bootstrap, not grounds for a cross-compiled fallback.
- Validate workflow syntax and non-publishing execution without creating a Git
  tag, GitHub Release, npm version, or public artifact.
- Update `PROJECT.md`, record a current Phase 2 checkpoint, advance the plan to
  Phase 3 `awaiting_approval`, and stop.

### Phase 3: Public documentation and controlled first release

Prerequisite: Phase 2 is committed, all external prerequisites are confirmed,
the exact release version is chosen, and the user explicitly authorizes Phase 3
including the named external GitHub/npm mutations.

The user explicitly authorized Phase 3 on 2026-09-04. This resolves the phase
authorization gate, not the outstanding release-version, identity, account,
credential-availability, or clean-source prerequisites. No implementation or
external publication has started. Phase 2 closeout documents remain uncommitted
and their commit/push direction must be resolved before the clean-source gate.
After validating approved/authorized metadata, the startup gate paused Phase 3
as blocked pending those decisions. The current recovery record is
`docs/checkpoints/release-ready-macos-distribution-phase-3.md`. Do not treat the
manifest's `0.1.0` as an authorized first release, infer a signer/reviewer, or
replace the accepted signed/notarized/OIDC gates with an unsigned/token fallback.

- Implement and verify the deferred signed-tag eligibility, protected
  signing/notary, OIDC, and controlled publication jobs. The read-only bootstrap
  is never a substitute for those gates. Re-review action pins and workflow trust
  boundaries before introducing any privileged job.
- Add public installation, verification, upgrade, rollback, uninstall, support,
  security-reporting, contribution, and changelog documentation.
- Update only the authorized root package version/metadata and matching lockfile
  root metadata; re-prove that dependencies and npm contents did not change.
- Configure the protected GitHub `release` environment, Apple credentials, and
  exact npm trusted publisher outside the repository.
- Create and verify the immutable signed `v<SemVer>` source tag only after a clean
  complete test run.
- Run the release workflow; sign/notarize and natively verify both daemon
  archives, create final checksums, stage a draft GitHub Release, publish the
  extension with npm provenance, verify the registry artifact, then publish the
  GitHub Release.
- Download both public daemon assets and the public npm tarball afresh; verify
  version, checksum, signature/notarization, architecture, exact contents, Pi
  installation, foreground daemon startup, descriptor cleanup, `/learn` smoke
  with the fake evaluator only, and uninstall/rollback instructions.
- Never invoke a live model provider during release verification.
- Update `PROJECT.md`, record the final checkpoint, mark the plan complete, and
  stop after reporting immutable release URLs and digests.

## 12. Acceptance Criteria

1. `package.json` SemVer, signed Git tag, daemon diagnostic, archive names,
   changelog heading, GitHub Release title, and npm version are exactly equal.
2. Local unversioned builds report `pi-learnloop dev`; released binaries report
   `pi-learnloop <SemVer>` on stdout with exit code zero.
3. `pi-learnloop daemon` and all accepted security/runtime behavior remain
   unchanged; unsupported invocations remain closed with exit code 2.
4. The release builder produces exactly one ARM64 and one AMD64 unsigned daemon
   executable and fixed ZIP contents without overwriting an existing nonempty
   output directory or leaving partial success.
5. Two clean builds from the same commit and exact Go 1.27.1 toolchain produce
   identical unsigned executable SHA-256 values for each architecture.
6. The independent verifier rejects wrong versions, architectures, names, ZIP
   entries/permissions, missing/malformed build or VCS metadata, checksums, or
   extra artifacts. The release workflow separately rejects dirty source.
7. Native macOS ARM64 and Intel jobs each execute focused command and foreground
   daemon smoke tests; cross-compilation alone cannot satisfy this criterion.
8. Ordinary CI has read-only permissions and no Apple/npm release authority.
9. Release preparation/publication starts only from a matching clean signed tag,
   defaults to non-publishing, uses reviewed full-SHA action pins, and requires a protected
   environment before privileged jobs.
   Phase 2's explicitly approved reviewed-commit bootstrap is read-only, has no
   upload or publication capability, and cannot establish release eligibility.
10. Stable daemon binaries have valid Developer ID Application signatures,
    hardened runtime, secure timestamps, accepted Apple notarization, successful
    post-download Gatekeeper assessment, and published final SHA-256 values.
11. The npm package is published through the configured trusted publisher with
    short-lived OIDC and provenance; no long-lived npm write token exists.
12. The published npm tarball contains exactly the current six extension-package
    paths and contains no daemon, dependency, credential, build output, workflow,
    test, history, or source snapshot.
13. A stable GitHub Release is not made public unless the same-version npm package
    and both signed/notarized daemon assets are independently downloadable and
    verified.
14. Public docs clearly separate the macOS 13 technical floor from the actual
    native release-test matrix and state Node `>=22.19.0`, Pi 0.84.3, Git, and
    model prerequisites.
15. Install, upgrade, rollback, uninstall, checksum/signature/notarization
    verification, security reporting, and partial-release recovery are documented
    without default `sudo`, Gatekeeper bypass, remote-shell execution, auto-start,
    or automatic update.
16. No HTTP/descriptors, schema, prompt, model, evidence, history, dependency,
    daemon-lifecycle, or extension interaction behavior changes beyond the
    additive local `version` command.
17. Both source-checkout development and released installation pass all
    repository-supported Go, extension, governance, and release-artifact tests
    without a live model call.
18. Every phase changes only its allowlisted files/external targets, records a
    checkpoint, reviews the complete diff, and stops at the high-risk approval
    gate.

## 13. Verification

### Draft design verification

- Run `scripts/test-agent-infra.sh`.
- Run `scripts/validate-agent-infra.sh`.
- Run `git diff --check`.
- Inspect `git status`, `git diff --stat`, and the complete `git diff`; confirm
  only this plan and ADR-0009 changed.
- Do not run business-code tests for the design-only task.

### Phase 1 verification

- Run `gofmt` on the two command files and inspect their focused diff.
- Run focused `go test ./cmd/pi-learnloop` tests for daemon, version, stdout,
  stderr, exit codes, and unsupported invocations.
- Run release-script self-tests in fresh temporary directories for successful
  builds and every failure/cleanup path.
- Build twice with exact Go 1.27.1 and `GOTOOLCHAIN=local`;
  compare per-architecture unsigned executable SHA-256 values.
- Inspect each executable with `file`/native Mach-O tooling and `go version -m`;
  run the ARM64 binary on ARM64 and defer AMD64 execution to its native Phase 2
  runner.
- Extract each ZIP into a fresh directory and run the independent verifier
  against exact contents, modes, diagnostic versions, and checksums.
- Run the compatibility suite under Go 1.21.13, then run `go test ./...`,
  `go test -race ./...`, `go vet ./...`, and
  `CGO_ENABLED=0 go build ./cmd/pi-learnloop` under Go 1.27.1.
- Run `npm run typecheck`, `npm test`, and isolated-cache
  `npm pack --dry-run --json`; confirm packaging remains unchanged.
- Run both Agent-infrastructure scripts, `git diff --check`, and the full end
  review required by `AGENTS.md`.

### Phase 2 verification

- Inspect every workflow trigger, job dependency, `if` expression, environment,
  permission, shell interpolation, and full-SHA action pin; no untrusted PR value
  may reach a privileged shell or environment.
- Run the shared ordinary push/PR suite on the exact reviewed event commit;
  inspect its PR merge-ref checkout and prove no secret,
  `id-token: write`, `contents: write`, upload, tag, or publication path is
  reachable.
- Commit/push the reviewed read-only implementation as approved, then run the
  manual bootstrap for that exact main commit; require all Go, extension,
  governance, package, artifact, and native ARM64/Intel smoke jobs to pass.
  Record run/job URLs, actual architecture/OS/image, revision, and absence of
  uploaded artifacts. A local or cross-compiled test is not hosted success.
- On each native runner, run `pi-learnloop version`, start the foreground daemon
  with the fake Pi evaluator, validate the protected descriptor/status path, send
  SIGTERM, and confirm cleanup without a live provider call.
- Verify bootstrap rejects non-main refs, malformed/mismatched commit inputs,
  dirty source/build provenance, and failed/missing native jobs. Review the
  unreachable publication boundary: no Phase 2 job has its permission or code.
  Exercise the bootstrap input gate locally with synthetic invalid event values
  and run the actual default non-publishing path on GitHub. Successful signed-tag
  and missing-signing/environment rejection tests belong to Phase 3.
- Run both Agent-infrastructure scripts and the full Git end review.

### Phase 3 verification

- Before mutation, confirm repository visibility, npm ownership/trusted-publisher
  configuration, GitHub environment reviewers, Apple Developer ID/notary access,
  exact release SemVer, and clean synchronized default branch.
- Run the full Phase 2 suite, `npm publish --dry-run`, exact tarball allowlist,
  dependency diff, changelog/install command review, and signed-tag verification.
- Verify signing with `codesign`, notarization with `notarytool`, Gatekeeper with
  `spctl`, Mach-O architecture with native tools, final checksums, and GitHub's
  release-asset digest where available.
- Download public assets rather than reusing workspace output; install each on
  its native architecture, validate the diagnostic version and foreground daemon
  smoke, and verify clean removal/rollback instructions without deleting history
  implicitly.
- Download/inspect the public npm tarball, verify provenance and exact contents,
  install the exact version through Pi, and smoke the extension against the
  matching daemon with fake evaluator behavior only.
- Confirm the GitHub tag/Release/npm version are immutable and mutually linked;
  record URLs, hashes, runner images, signing identity metadata, and notarization
  submission identifiers without credentials.
- Run both Agent-infrastructure scripts, `git diff --check`, and the full Git end
  review before marking the plan complete.

## 14. Open Questions

- Confirmed on 2026-09-04: <https://github.com/reeezark/pi-learnloop> is visibly
  public when viewed without signing in. At that initial inspection the Actions
  page had no workflow execution evidence; completed Phase 2 runs are now recorded
  above. Public visibility alone does not prove authenticated
  Actions policy, account eligibility, or npm trusted-publisher configuration.
- Confirmed on 2026-09-04: the user approved the read-only reviewed-commit
  bootstrap and initial workflow commit/push/run sequence. Successful signed-tag
  verification and privileged jobs are deferred to Phase 3 without weakening
  release trust. Native hosted execution remains an evidence gate.
- `TODO / Need Confirmation`: Is the unscoped npm name `pi-learnloop` owned and
  available to this maintainer, and can its trusted-publisher record be bound to
  the exact GitHub repository/workflow/environment? Do not publish or fall back
  to a token until confirmed.
- `TODO / Need Confirmation`: Is an Apple Developer Program membership,
  Developer ID Application certificate, and non-interactive `notarytool`
  credential available for the protected environment? Absence blocks a stable
  release under ADR-0009.
- Confirmed on 2026-09-04: the user authorized obtaining the official Go 1.27.1
  toolchain. Its Darwin ARM64 archive was downloaded into an isolated temporary
  directory and matched go.dev's SHA-256
  `ee215d57e0ec269c60cc9ceca68e6bda321ba9ee5afe24f4b0988703c2d87d12`
  and 68,100,347-byte size. This does not change the Go 1.21 module baseline.
- Confirmed by the maintainer on 2026-09-04: `macos-15` and `macos-15-intel`
  may be used for Phase 2. If execution shows either unavailable, stop instead
  of claiming cross-compiled verification.
- `TODO / Need Confirmation`: What exact first public SemVer is authorized? The
  manifest currently says `0.1.0`, but that is repository state, not permission
  to create `v0.1.0` or publish it.
- `TODO / Need Confirmation`: Who approves the protected `release` environment
  and owns partial-release recovery? This must be named operationally before
  privileged workflow activation.
- `TODO / Need Confirmation`: Does the maintainer accept a two-step daemon plus
  extension install and manual foreground startup as the initial public UX? It
  is the proposed security-preserving decision; changing it would require a new
  lifecycle/installer investigation and ADR amendment.
