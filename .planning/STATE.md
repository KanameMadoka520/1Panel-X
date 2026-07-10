# Project State

## Project Reference

See: `.planning/PROJECT.md` and `.planning/REQUIREMENTS.md` (updated 2026-07-10)

**Core value:** Deliver a complete, security-conscious, fully open server panel without proprietary code or license bypasses.
**Current milestone:** v1.0 Open Enhancement First Release
**Current focus:** Review and verify Phase 1 while preserving the parallel draft work for Phases 2 through 4.

## Current Position

Phase: 1 of 5 (Open Theme and Watermark)
Plan: No formal phase plan yet; an existing uncommitted implementation is under review
Status: In progress, not accepted
Last activity: 2026-07-10 - Research was committed and the five-requirement milestone roadmap was drafted while feature changes remained in the working tree.

Progress: [----------] 0% (0 of 5 phases accepted)

## Repository Snapshot

- Branch: `open-pro-v1`, tracking `upstream/dev-v2`
- Upstream baseline: `8be2a9ab0270139d0cea2f023ea3f287db2217e0`
- Latest planning commit at snapshot: `ad71fe9aa7d3b45fc016451c12393c271fc36931`
- Worktree: dirty with draft theme, webhook, ClamAV, and AI Agent changes plus tests
- External `image` directory: empty; no v1.0 release artifact exists
- External `roadmap` directory: empty; no material update note has been created
- Commit identity configured locally: `KanameMadoka520 <2441883200@qq.com>`

## Phase Review Status

| Phase | Current evidence | Remaining before acceptance |
|-------|------------------|-----------------------------|
| 1. Theme and watermark | Core API, service, tests, and frontend fallback changes are present | Re-run core tests, production build, and browser UAT; review public data split; commit separately |
| 2. Webhook alerts | Sender, helper, UI, and tests are present | Complete HTTPS, host, TLS, redirect, secret-redaction, and failed-attempt review; Linux tests; commit separately |
| 3. Scheduled ClamAV | Service, cron, helper, UI, and tests are present | Resolve lifecycle, restart, concurrency, path, permission, and destructive-action review; Linux tests and isolated VPS UAT; commit separately |
| 4. AI Agent limit | Soft-limit implementation and focused tests are present | Check secondary App Store limits and concurrency semantics; Linux regression tests; commit separately |
| 5. Release | No artifacts or VPS guide yet | Accept Phases 1-4, build Linux AMD64 release, create checksums and docs, perform smoke test, and commit release material |

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

1. Review and verify Phase 1, then create its scoped commit.
2. Complete security and lifecycle review for Phases 2 and 3, with focused Linux tests.
3. Finish Phase 4 metadata and regression checks, then commit it separately.
4. Run the combined frontend and Linux build gate.
5. Produce external release artifacts, VPS instructions, and the timestamped roadmap note during Phase 5.

## Session Continuity

Last session: 2026-07-10
Stopped at: Planning documents drafted while all feature implementations remained incomplete and under review
Resume file: None
