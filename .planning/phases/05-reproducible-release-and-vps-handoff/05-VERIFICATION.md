---
phase: 05-reproducible-release-and-vps-handoff
verified: 2026-07-10T22:01:07-04:00
status: human_needed
score: 4/5 must-haves verified
requirements:
  - RELEASE-01
---

# Phase 5: Reproducible Release and VPS Handoff Verification Report

**Phase Goal:** An operator can reproduce, inspect, install, smoke-test, and roll back the v1.0 build from a clearly identified source revision.
**Status:** human_needed

## Goal Achievement

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Exact documented Linux toolchains build the committed source revision | VERIFIED | Node 24.14.0, npm 11.14.1, and Go 1.26.1 ran the clean-tree release script from `cc5d31aa7`. |
| 2 | The external release directory contains the intended release evidence | VERIFIED | Nine top-level deliverables exist: two binaries, archive, two docs, JSON metadata, two logs, and SHA256SUMS. |
| 3 | Provenance, checksums, archive contents, and binaries are internally consistent | VERIFIED | 8/8 top-level and 7/7 in-archive checksums passed; metadata HEAD matched; both binaries are Linux x86-64 ELF. |
| 4 | VPS instructions cover backup, replacement, startup, feature tests, and rollback | VERIFIED | `README-VPS.md` documents prerequisites, consistent backups, atomic replacement, smoke tests, all four feature checks, and rollback. |
| 5 | A real VPS can install, exercise, and roll back the candidate | NEEDS HUMAN | No VPS deployment, browser session, provider robot, EICAR run, multi-Agent capacity run, or rollback rehearsal was performed. |

## Automated Verification Passed

- `npm ci`: 713 packages audited, 0 vulnerabilities.
- `npm run build:pro`: passed, 7,074 modules transformed.
- Focused tests: core service/middleware, Agent service, ClamAV utility, webhook sender, and webhook helper passed.
- Core and Agent `go test ./... -run '^$' -count=1` compile checks passed.
- Linux AMD64 core and Agent binary builds passed.
- Top-level and archive checksum verification passed.
- Metadata revision, toolchains, target, upstream base, locks, and verification states were inspected.
- No temporary build directory or accidental staging tree remains.

## Human Verification Required

### 1. VPS prerequisites, checksum, and backup gate
**Test:** Upload the entire release directory to a disposable or snapshotted compatible VPS, verify both checksum layers, compare the official v2 baseline, and create a consistent binary plus database backup.
**Expected:** All checksums pass, the baseline is compatible, and a restorable pre-deployment state exists.

### 2. Atomic replacement and service smoke test
**Test:** Stop services, atomically replace both binaries, start Agent then Core, inspect status/journal, and open the core panel workflows.
**Expected:** Both services remain active with no migration panic or restart loop, and core pages remain usable.

### 3. Execute Phase 1 through Phase 4 UAT
**Test:** Run the persisted theme/watermark, real robot, ClamAV/EICAR, and AI Agent capacity tests.
**Expected:** Each result is recorded as pass, issue, skipped, or blocked in its `*-HUMAN-UAT.md` file.

### 4. Rollback rehearsal
**Test:** Restore previous binaries and, where necessary, the consistent conf/database backup or full VPS snapshot.
**Expected:** The previous official installation becomes healthy and readable again.

## Gaps Summary

No automated release gap remains. Live installation and rollback acceptance are deferred, so status is `human_needed`, not `passed`.

---
*Verified: 2026-07-10T22:01:07-04:00*
*Verifier: Codex release and artifact audit*
