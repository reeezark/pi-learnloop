---
id: release-ready-macos-distribution-phase-3
plan: release-ready-macos-distribution
phase: 3
status: current
updated: 2026-09-05
---

# Release-Ready macOS Distribution Phase 3: Prerequisite Checkpoint

## Scope Handoff (2026-09-05)

The user clarified personal local Pi-extension use and requested repair until
LearnLoop works. The release plan is now superseded; this remains its final
historical checkpoint, not the current product task. The earlier prerequisite
record below is preserved for audit. Do not resume its Apple/npm questions or
publication work. Continue from `isolated-pi-model-runtime-phase-1.md` and its
draft plan/ADR gates instead. Phase 3 was not implemented or completed.

## Context

### Goal

Prepare the controlled first signed/notarized macOS release and matching npm
extension under accepted ADR-0009, without changing product behavior or dependencies.

### Current Phase

The user explicitly authorized Phase 3 on 2026-09-04. Authorization is recorded;
implementation is blocked at the startup gate because required release choices
and external prerequisites are not confirmed. No Phase 3 workflow, public
installation documentation, signing, tag, or publication work has started.

## Completed

- Rechecked local status, complete diff, recent commits, governance, project
  facts, Phase 3 scope/prerequisites, Phase 2 evidence, ADR-0009, the manifest,
  and the installed read-only bootstrap.
- Local HEAD and the existing origin/main tracking ref are both
  `dd91d494f2fbf96dd0745f36150ef92c02466fec`. No fresh remote or account-setting
  query was made this turn; do not infer external readiness from the tracking ref.
- Preserved the three uncommitted Phase 2 closeout documents. Its successful
  ordinary/manual native verification remains in the superseded Phase 2 record.
- Validated approved/authorized plan metadata, then recorded blocked status and
  this current checkpoint. The authorization does not need to be requested again
  for the same Phase 3 scope once prerequisites and exact targets are resolved.

## Modified Files

This turn changes only `plans/release-ready-macos-distribution.md`, the Phase 2
checkpoint's handoff metadata/context, and this new Phase 3 checkpoint. The
existing `PROJECT.md` closeout diff is preserved, not extended. All four dirty
paths are within the approved Phase 3 file allowlist; no commit or push occurred.

## Important Decisions

ADR-0009 remains unchanged. A stable release requires a matching immutable signed
tag, protected human approval, Developer ID signing/notarization, native checks,
and npm trusted-publisher OIDC. Phase 2's unsigned bootstrap supplies verification
evidence only. No secret value should be sent through chat, logs, or repository
files, and no identity/account capability is inferred from phase authorization.

## Tests / Verification

The approved/authorized metadata validation passed. After recording this pause,
`scripts/test-agent-infra.sh`, `scripts/validate-agent-infra.sh`, and
`git diff --check` passed. The full tracked diff and new checkpoint were reviewed;
workflow, script, and dependency diffs remain empty. No business, release,
provider, signing, or publication test was run at this prerequisite gate; prior
successful tests are not a Phase 3 release.

## Known Issues

- Phase 2 closeout documents are not committed; explicit commit/push direction
  remains unresolved and the checkout is not clean.
- The manifest says `0.1.0`, but the exact first release version and resulting
  Git tag/GitHub Release/npm version have not been selected by the user.
- The protected `release` environment approver, partial-release recovery owner,
  and trusted Git-tag signer/public verification identity are not named.
- Apple Developer ID Application/notary access and npm `pi-learnloop` ownership/
  trusted-publisher eligibility are unconfirmed, not proven absent. No account,
  keychain, credential store, or repository permission was changed or inspected
  to guess these prerequisites.

## Remaining Work (Superseded Scope)

The public-release prerequisites below remain unfinished, but they are no longer
current work: release version, protected reviewer, signing owner, Apple/notary
access and npm trusted-publisher readiness. Do not request or pursue them unless
the user explicitly revives this superseded plan.

## Next Step (Superseded Scope)

Take no further action under this plan. Continue from
`isolated-pi-model-runtime-phase-1.md`; no automation or alternate publication
route is authorized by the abandoned release scope.

## Do Not Change

No business code, dependency, Go 1.21 baseline, protocol/schema/prompt, model or
history behavior, package contents, credential/account permissions, signed tag,
release asset, or npm registry state while this prerequisite gate is unresolved.
Do not silently skip notarization, use a long-lived npm write token, bypass
Gatekeeper, or invent a release identity/version to make progress.
