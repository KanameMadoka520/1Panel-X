---
phase: 05-reproducible-release-and-vps-handoff
plan: 01
subsystem: release-and-vps-handoff
tags: [release, linux, amd64, checksums, provenance, vps, rollback]
requires: [01-01, 02-01, 03-01, 04-01]
provides:
  - Reproducible Linux AMD64 native release candidate
  - Double-layer SHA256 verification and exact source provenance
  - VPS installation, feature acceptance, and rollback instructions
  - Persistent human UAT for all live acceptance work
affects: [release, deployment, milestone-v1.0]
tech-stack:
  added: []
  patterns: [clean-tree release, locked toolchains, deterministic archive metadata, external artifact boundary]
key-files:
  created:
    - D:/_CodeNotSync/_1Panel-X/image/releases/v1.0.0-open.1
    - .planning/phases/05-reproducible-release-and-vps-handoff/05-HUMAN-UAT.md
    - D:/_CodeNotSync/_1Panel-X/roadmap/20260710-220107-首版开放增强与发布候选.md
  modified:
    - D:/_CodeNotSync/_1Panel-X/image/build-release.sh
    - .planning/PROJECT.md
    - .planning/REQUIREMENTS.md
    - .planning/ROADMAP.md
    - .planning/STATE.md
key-decisions:
  - "Ship native host-integrated binaries rather than claim a normal runtime container."
  - "Require a clean source revision and exact Node, npm, and Go toolchains."
  - "Keep VPS, browser, provider, EICAR, capacity, and rollback acceptance pending until actually run."
patterns-established:
  - "Release metadata and checksums make the external artifact self-describing."
  - "Human UAT remains machine-discoverable and blocks requirement completion."
requirements-completed: []
requirements-progressed: [RELEASE-01]
duration: approximately-25-minutes-including-rebuilds-and-independent-inspection
completed: 2026-07-10
---

# Phase 5: Reproducible Release and VPS Handoff Summary

**`v1.0.0-open.1` is a reproducible Linux AMD64 release candidate with passed automated gates, verified checksums and ELF binaries, exact source provenance, and a complete VPS handoff; no live VPS acceptance has been claimed.**

## Performance

- **Completed:** 2026-07-10T22:01:07-04:00
- **Source revision:** `cc5d31aa76a4d166f287a98b5d92b1f63c67af3d`
- **Upstream merge base:** `8be2a9ab0270139d0cea2f023ea3f287db2217e0`
- **Release directory:** `D:\_CodeNotSync\_1Panel-X\image\releases\v1.0.0-open.1`

## Accomplishments

- Locked and verified Node 24.14.0, npm 11.14.1, and Go 1.26.1 in WSL.
- Ran `npm ci` with zero reported vulnerabilities and built 7,074 frontend modules.
- Passed focused core, middleware, Agent service, ClamAV, webhook sender, and webhook helper tests.
- Passed core and Agent all-package compile checks for Linux AMD64.
- Built statically linked, stripped Linux x86-64 `1panel-core` and `1panel-agent` binaries.
- Produced standalone binaries, deterministic tar.gz archive, logs, metadata, VPS documentation, and two checksum layers.
- Corrected DrvFs publication and staging-cleanup issues discovered by the release run and independent inspection.

## Automated Verification

- Top-level `SHA256SUMS`: 8/8 entries passed.
- In-archive `SHA256SUMS`: 7/7 entries passed.
- `file`: both standalone binaries are ELF 64-bit LSB x86-64, statically linked, stripped.
- Metadata source revision exactly matches Git HEAD and reports `dirty: false`.
- Metadata statuses: frontend build, focused tests, package compile checks, and native binary build are `passed`.
- Metadata correctly reports VPS acceptance as `not_run_by_build_script`.
- No `.build-*` directory or accidental staging tree remains in the final release directory.

## Primary Checksums

- Core: `077b5906fa1ec02cd496707773e3bdc2c5315346b8e2054427b7cd412a536cb6`
- Agent: `2c56f36c3c01018e3be3a762ae3ed60a9bcc25e2de5d67a15cae8b4b3bb660e8`
- Archive: `8b501727ef445d64a79dcd921a8f70db731894b640a57c4e6ef20e5542545eec`

## Known Limitations

- This is not official Pro or Enterprise licensing and does not include proprietary overlays or license bypasses.
- It implements four first-release enhancement domains, not full commercial parity.
- Webhook URLs remain plaintext in the alert database and its backups.
- AI Agent positive-limit enforcement is non-atomic and has no dedicated UI.
- The inherited frontend build emits large-chunk and `/* #__PURE__ */` placement warnings but completes successfully.

## Human Acceptance Required

Phase 5 persists four VPS handoff tests. Combined with Phases 1 through 4, the project has 20 pending human UAT items. RELEASE-01 remains progressed but not complete.

---
*Phase: 05-reproducible-release-and-vps-handoff*
*Release candidate built: 2026-07-10*
