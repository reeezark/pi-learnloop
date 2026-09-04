---
id: ADR-0009
status: accepted
date: 2026-09-04
supersedes: none
---

# ADR-0009: Split, Verifiable macOS Release Distribution

## Context

Pi LearnLoop's evidence-backed interview loop is implemented, but users still
need a source checkout for both sides of the product: `go run` starts the daemon
and Pi loads the TypeScript package from that checkout. macOS ARM64 is verified
locally; AMD64 is only an intended target. The repository has no release tags,
CI/release workflow, signed native artifact, notarization, npm publication, or
public release operations documentation.

The two runtime modules have different native trust and installation concerns.
The Pi package contains TypeScript loaded by Pi and currently packs exactly the
extension, README, manifest, and license. The daemon is a pure-Go foreground
executable that owns protected runtime/data directories, loopback authentication,
evaluator child processes, and graceful cleanup. Pi 0.84.3 package extensions
have the user's full system access, so an npm lifecycle download or auto-start
would turn extension installation into a native-code execution and update seam.

ADR-0002 requires manual foreground daemon lifecycle and a strict descriptor and
protocol-v1 interface. ADR-0003 fixes Pi 0.84.3 for evaluator compatibility.
ADR-0005 protects schema-v2 local history, and ADR-0008 requires mixed-version
failures to remain explicit without retry or fallback. Distribution must preserve
those decisions rather than hiding installation difficulty in a new privileged
launcher.

The product also spans independent version domains: product releases, local HTTP
protocol, SQLite schema, evaluator input/output schemas, prompt assets, and Pi.
Adding product SemVer to the strict runtime response would couple release cadence
to protocol compatibility and force changes in both runtime adapters even though
the protocol already fails closed on incompatible values.

Official platform evidence gives Node.js 22 a macOS 11 floor for x64 and arm64.
The module retains its Go 1.21 language baseline, but stable release binaries use
the current supported Go 1.27.1 compiler rather than an obsolete Go 1.21
compiler; Go 1.27 requires macOS 13 or newer. macOS 13 is therefore the initial
complete-product technical floor. GitHub documents standard native ARM64 and
Intel macOS runners, npm trusted publishing requires npm CLI 11.5.1 or newer and
Node 22.14.0 or newer on a hosted runner, and Apple provides Developer ID signing
and notarization for direct distribution. Pi LearnLoop's stricter Node
`>=22.19.0` requirement continues to control. These are external prerequisites,
not facts that repository code can guarantee.

## Decision

Accepted on 2026-09-04. The user separately authorized Phase 1 and acquisition
of the checksum-verified official Go 1.27.1 release toolchain. No later phase
or publication is authorized by this acceptance.

### 1. Distribute one product version through two explicit channels

`package.json` is the single product SemVer authority. Every public release uses
an immutable signed Git tag named exactly `v<SemVer>`. The npm Pi package and the
daemon's archive/binary names and embedded diagnostic version all use that same
SemVer.

The npm package remains extension-only. Separate per-architecture ZIP archives
for `darwin-arm64` and `darwin-amd64` are published as GitHub Release assets.
Each archive contains the signed daemon executable, MIT license, and concise
installation/verification instructions. A final `SHA256SUMS` covers the
published ZIP files.

Users install matching daemon and extension versions separately and manually
start `pi-learnloop daemon` in the foreground. There is no npm lifecycle script,
binary download adapter, installer service, automatic startup, service manager,
or updater.

### 2. Keep product version diagnostic and runtime compatibility structural

The executable adds exactly one read-only command:

```text
pi-learnloop version
```

It writes `pi-learnloop <version>` plus LF to stdout and exits zero. Direct
developer builds report `dev`; artifact-builder outputs receive the validated
product SemVer at link time, and only a clean matching signed-tag output is
eligible for publication. The command is the only runtime surface for product
version.

Product SemVer is not added to the daemon descriptor, `/v1/status`, another HTTP
response, evidence, evaluator input, prompt, history, logs, or errors. The
extension and daemon do not perform an exact SemVer handshake. Runtime
compatibility continues to be determined by the accepted strict protocol,
schema, prompt, and Pi-version contracts. Documentation recommends and release
verification installs a matched pair.

### 3. Build closed, inspectable, pure-Go per-architecture artifacts

One local artifact-builder module owns package-version validation, fixed build
flags, architecture matrix, names, staging, ZIP contents, and refusal of
partial/overwrite outcomes. One independent read-only verifier owns the closed
artifact contract, including recorded VCS metadata. The privileged workflow,
not the local builder, owns clean source, exact signed tag, and publication
eligibility. This lets uncommitted authorized implementation exercise the local
module without making a dirty candidate publishable. Workflows adapt those
interfaces to hosted runners and credentials instead of reimplementing build
policy step by step.

Release executables use exact Go 1.27.1 with automatic toolchain download
disabled, `CGO_ENABLED=0`, `GOOS=darwin`, explicit `GOARCH`,
`-mod=readonly`, `-trimpath`, `-buildvcs=true`, and only the SemVer link-time
value. The module's Go 1.21 language baseline remains a separately tested source
compatibility contract. No dependency is added, removed, upgraded, vendored, or
bundled.

Unsigned executables built twice from the same commit and exact toolchain must
have identical hashes. Final signed archives are not called reproducible because
secure timestamps and archive metadata may differ. Their final bytes are instead
identified by published SHA-256 values and the Developer ID signature.

### 4. Require native ARM64 and Intel release evidence

Cross-compilation proves that both Mach-O files can be produced; it does not prove
that they start. A release requires native macOS ARM64 and Intel jobs to execute
the version command and a foreground daemon smoke test with fake evaluator
behavior, validate protected descriptor/status behavior, terminate the daemon,
and confirm cleanup. A live model provider is never part of release verification.

The release record states macOS 13 as the release-toolchain-derived technical
floor and separately names the newer hosted runner images/OS versions actually
exercised. It must not imply that native CI on a newer OS executed the minimum
OS.

### 5. Gate stable native artifacts on Developer ID and notarization

Every stable daemon executable is signed with a Developer ID Application identity,
hardened runtime, no custom entitlements, and a secure timestamp. The signed ZIP
is submitted with `notarytool`; an accepted notarization result, signature
verification, and post-download Gatekeeper assessment are mandatory release
gates.

Signing/notary material exists only in a reviewer-protected release environment
or its delegated secret store. Pull-request and ordinary build jobs cannot access
it. If Apple credentials, native runners, notarization, or verification is
unavailable, the pipeline may produce explicitly unsigned ephemeral test
artifacts but must not publish or describe them as a stable release. Users will
not be instructed to bypass Gatekeeper or quarantine.

### 6. Separate least-privilege CI from controlled publication

Ordinary CI is read-only, has no release environment, and receives neither Apple
nor npm authority. Release preparation is manually dispatched from an existing
matching immutable signed tag, defaults to a non-publishing path, re-runs the
complete repository and artifact verification, and crosses the protected release
environment only after approval.

Workflow actions are pinned to reviewed full commit SHAs. Build/test jobs receive
only read access. GitHub Release publication alone may receive `contents: write`;
npm publication alone receives `id-token: write`.

The extension is published through npm's trusted-publisher OIDC flow on a
GitHub-hosted runner using an exact verified Node version satisfying
`>=22.19.0` and an exact reviewed npm CLI version satisfying `>=11.5.1`.
These publication tools do not enter the package dependency closure. No
long-lived npm write token is stored. Before publication, the package must still
have the exact six current paths and no daemon or bundled dependency. npm
provenance is required.

Final daemon assets are first staged in a draft GitHub Release, the same-version
npm package is then published and independently verified, and the GitHub Release
is made public last. A failure leaves an explicit draft/failed state for manual
resolution; versions and tags are never overwritten or reused.

### 7. Preserve the manual foreground lifecycle and local trust model

Installation documentation uses a user-selected PATH directory and no default
`sudo`, remote shell, auto-start, or silent local-data deletion. Upgrade and
rollback replace both versioned artifacts explicitly. Uninstall removes the
extension and executable but preserves protected local history unless the user
separately chooses to delete it.

Nothing in this decision changes protocol v1, descriptor/status fields, Instance
Tokens, loopback-only transport, proxy bypass, retry/fallback rules, SQLite schema
v2, evaluator/prompt contracts, Pi Session provenance, model isolation, or
source-free persistence.

## Alternatives

### Continue source-checkout-only installation

Rejected as the public release target. It leaves every user to reproduce the Go
build and dependency installation, provides no immutable product version or
native trust evidence, and leaves AMD64 unverified. It remains a supported
development path whose binary reports `dev`.

### Bundle both daemon binaries in the npm package

Rejected. It makes every Pi user download both architectures, expands a small
extension package with native executables, and couples npm contents to Apple
signing/notarization. Pi package installation still would not select/install the
right PATH binary without another privileged mechanism.

### Download or build the daemon in `postinstall`

Rejected. Package installation would perform network/native-code operations with
the user's authority, add platform/network/cache/failure policy, weaken offline
inspection, and make an npm registry event the owner of native artifact trust.

### Let the extension download, launch, update, or supervise the daemon

Rejected. It conflicts with ADR-0002's manual foreground lifecycle and adds
process ownership, binary trust, update rollback, concurrency, and credential
decisions to the extension's already privileged system-access seam.

### Publish one unsigned universal binary

Rejected. A universal artifact does not supply native runtime evidence, increases
artifact size, and does not solve Gatekeeper publisher trust. Separate archives
make architecture and test provenance explicit. A universal binary may be
reconsidered only as an additional convenience artifact after both thin binaries
remain authoritative.

### Publish unsigned GitHub binaries with checksum only

Rejected for stable distribution. A checksum detects accidental/captured-byte
change only when obtained from a trusted source; it does not establish an Apple
recognized publisher or satisfy the selected Gatekeeper/notarization policy.

### Start with Homebrew, `.pkg`, or `.dmg`

Rejected for the first release. Each adds a maintained packaging/installation
interface, update semantics, and potentially another publication principal before
the underlying signed thin artifacts are proven. Those channels can later consume
the same release assets through a separate plan.

### Put product SemVer in `/v1/status` and require an exact match

Rejected. It changes a strict public response and both adapters without evidence
that every product release is protocol-incompatible. Existing protocol/schema
checks already fail closed where compatibility matters; a diagnostic command and
matched installation guidance solve provenance without coupling version domains.

### Trust cross-compilation for Intel

Rejected. Mach-O structure is not runtime evidence. A stable AMD64 claim requires
a native Intel job to execute the binary and daemon lifecycle.

### Use a long-lived npm automation token

Rejected when trusted publishing is available. OIDC limits authority to an exact
hosted workflow identity and avoids a reusable write secret. If trusted-publisher
eligibility cannot be confirmed, publication blocks rather than silently falling
back.

### Publish automatically on every matching tag push

Rejected. Signing, notarization, registry publication, and GitHub Release creation
are non-atomic external mutations. Manual dispatch from an already reviewed
signed tag plus protected-environment approval makes the exact source and intent
observable before privileged work.

## Implementation Status

Phase 1 completed on 2026-09-04: the additive diagnostic, exact-Go-1.27.1
local builder, independent static verifier, and release-artifact self-tests are
implemented. The small script interfaces hide all staging, ZIP composition,
bounded inspection, native Mach-O version decoding, dependency/build metadata,
and checksum rules using the existing Node runtime and native macOS tools.
The verifier never executes an artifact. Handled builder cancellation terminates
and reaps its child process group before cleaning private staging.

The downloaded toolchain matched the official go.dev size and SHA-256. Both
unsigned candidate architectures passed repeated-binary-hash and static checks;
only ARM64 diagnostic execution was performed. These local candidates record
the uncommitted source state and are not publishable. Go 1.21 source compatibility
and the dependency graph remain unchanged. Native Intel CI, clean signed-tag
eligibility, Developer ID/notarization, and publication still require their
separate authorized phases.

## Consequences

- Users receive inspectable native artifacts for both supported Mac architectures
  and a small Pi-native extension package, at the cost of a deliberate two-step
  installation and matched-version responsibility.
- The release pipeline has two narrow deep modules—the artifact builder and
  verifier—while workflows remain orchestration adapters rather than independent
  build-policy owners.
- Stable publication depends on GitHub native-runner access, npm package/trusted-
  publisher eligibility, an Apple Developer ID, notarization service access, and
  a protected human-approved environment. Missing prerequisites block stable
  release.
- macOS 13 is the initial technical floor from the supported Go 1.27 release
  toolchain, but hosted release evidence will normally exercise newer systems.
  Both facts must be visible and must be revisited when Node, Pi, Go, or Apple
  support changes.
- Product SemVer becomes observable without modifying or persisting any runtime
  protocol value. Old source-built binaries report `dev`; the daemon and extension
  retain their existing structural compatibility behavior.
- Pre-signing executables can be reproducibility-checked; final timestamped signed
  archives are integrity-addressed but not promised byte-reproducible.
- A partially failed cross-registry release cannot be rolled back atomically.
  Draft-first publication and no version/tag reuse make recovery explicit rather
  than pretending it is transactional.
- Homebrew, installers, automatic lifecycle, universal convenience artifacts, and
  additional platforms remain possible later, but must build on the signed thin
  artifacts and receive separate design/authorization.
- Accepting this ADR approves the architecture only. It does not authorize any
  implementation phase, credential/environment mutation, Git tag, GitHub Release,
  or npm publication. Each high-risk phase in the corresponding plan requires
  explicit authorization.
