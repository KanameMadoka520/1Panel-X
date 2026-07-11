# Project State

## Project Reference

See: `.planning/PROJECT.md` and `.planning/REQUIREMENTS.md` (updated 2026-07-10)

**Core value:** Deliver a complete, security-conscious, fully open server panel without proprietary code or license bypasses.
**Current milestone:** v1.0 Open Enhancement First Release
**Current focus:** Transfer and exercise the v1.0.0-open.1 release candidate on a disposable or fully snapshotted VPS.

## Current Position

Phase: 5 of 5 (Reproducible Release and VPS Handoff)
Plan: 05-01 complete
Status: Release candidate built; 20 human UAT items pending
Last activity: 2026-07-10 - Built and independently inspected `v1.0.0-open.1` from source revision `cc5d31aa76a4d166f287a98b5d92b1f63c67af3d`.

Progress: [##########] 100% implementation and automated gates; 0 of 5 requirements accepted because human UAT remains.

## Repository Snapshot

- Branch: `open-pro-v1`, tracking `upstream/dev-v2`
- Upstream baseline: `8be2a9ab0270139d0cea2f023ea3f287db2217e0`
- Latest source revision used by the release: `cc5d31aa76a4d166f287a98b5d92b1f63c67af3d`
- Worktree: clean after the final release build
- External `image` directory: contains `releases/v1.0.0-open.1` with native binaries, archive, checksums, logs, metadata, and VPS instructions
- External `roadmap` directory: contains the timestamped Chinese v1 release-candidate note
- Commit identity configured locally: `KanameMadoka520 <2441883200@qq.com>`

## Phase Review Status

| Phase | Current evidence | Remaining before acceptance |
|-------|------------------|-----------------------------|
| 1. Theme and watermark | Implementation, core tests, target lint, and production build passed | Browser login, refresh, theme-mode, watermark, and CSS visual UAT |
| 2. Webhook alerts | Sender/config/retry-accounting tests and production build passed | Real disposable WeCom, DingTalk, and Feishu/Lark delivery plus end-to-end secret inspection |
| 3. Scheduled ClamAV | Lifecycle, compensation, root, path, restart, concurrency, and package tests passed | Real agent restart, cron timing, overlap, and isolated EICAR UAT |
| 4. AI Agent limit | Capacity, metadata hook, setting boundary, persistence, and package tests passed | Live creation beyond five, positive limit, race characterization, and resource observation |
| 5. Release | Checksums, ELF architecture, metadata, archive contents, and cleanup verified | VPS backup, replacement, startup, smoke test, feature UAT, and rollback rehearsal |

## Accumulated Context

### Decisions

- Implement user outcomes one feature at a time; never force global Pro state.
- Use only public GPL interfaces, public documentation, open protocols, and lawfully observable behavior.
- Keep native Linux binaries as the release target; containers may reproduce builds but are not the default runtime boundary.
- Run Go verification in Linux or WSL with Go 1.26.1; native Windows compilation is not authoritative for Linux-specific syscall code.
- A requirement is complete only after implementation, verification, relevant VPS evidence, and a scoped commit.

### Blockers and Concerns

- Webhook robot URLs contain secrets and require strict host validation and redaction.
- ClamAV scheduling touches destructive paths and cannot ship before path containment, restart recovery, and scan serialization are proved.
- AI Agent count-then-create soft enforcement is not atomic and may be exceeded by concurrent requests.
- Frontend `type-check` has known upstream errors; verify changed-code behavior and report baseline failures without attributing them to v1.0.
- Full VPS acceptance has not run. Local or WSL test success alone does not make the milestone complete.

## Next Actions

1. Upload the complete `image/releases/v1.0.0-open.1` directory to a disposable or fully snapshotted VPS.
2. Verify both checksum layers and compare the installed official v2 baseline to `RELEASE-METADATA.json`.
3. Execute the 20 persisted human UAT items in Phases 1 through 5.
4. Record pass, issue, skipped, or blocked results in each `*-HUMAN-UAT.md`.
5. Close requirements only after their required acceptance evidence exists.

## Session Continuity

Last session: 2026-07-10
Stopped at: Release candidate and automated evidence complete; human VPS/browser/provider acceptance pending
Resume file: None
