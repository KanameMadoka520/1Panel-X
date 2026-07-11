---
phase: 03-scheduled-clamav
plan: 01
subsystem: agent-clamav-scheduling
tags: [go, vue3, clamav, cron, gorm, filesystem-safety, concurrency]
requires: []
provides:
  - Durable ClamAV schedule lifecycle and startup restoration
  - Atomic per-rule execution claim
  - Confined 0700 quarantine directories and safe removal
  - Community schedule UI
affects: [phase-05-release, clamav, filesystem-safety]
tech-stack:
  added: []
  patterns: [persist-before-register, replace-before-remove, atomic database claim, confined path reconstruction]
key-files:
  created:
    - agent/app/service/clam_schedule_test.go
    - agent/utils/clam/clam_security_test.go
  modified:
    - agent/app/service/clam.go
    - agent/cron/cron.go
    - agent/utils/clam/clam.go
    - agent/utils/xpack/helper/multi_node.go
    - frontend/src/views/toolbox/clam/index.vue
    - frontend/src/views/toolbox/clam/operate/index.vue
key-decisions:
  - "Persist new rules before cron registration and roll back both sides on failure."
  - "Use one conditional database update to serialize manual and scheduled scans."
  - "Reject root, symlink, traversal, and overlapping scan/quarantine paths."
patterns-established:
  - "External scheduler state is replaced transactionally around persisted identifiers."
  - "Destructive filesystem operations reconstruct targets from validated components."
requirements-completed: []
requirements-progressed: [CLAM-01]
duration: not-recorded
completed: 2026-07-10
---

# Phase 3: Scheduled ClamAV Summary

**ClamAV rules now have durable community schedules, restart restoration, per-rule run serialization, and confined quarantine paths; live daemon and EICAR acceptance is deferred.**

## Performance

- **Duration:** Not recorded; retrospective artifact.
- **Completed:** 2026-07-10T20:36:04-04:00
- **Tasks:** 3 reconstructed tasks
- **Files modified:** 8

## Accomplishments

- Added create/update/status/delete schedule lifecycle with rollback behavior.
- Restored enabled schedules at agent startup and persisted new entry IDs.
- Prevented overlapping manual and scheduled scans with an atomic database claim.
- Rejected schedule changes and deletion while a rule is executing.
- Added strict rule-name, scan-path, quarantine-path, permission, symlink, root, overlap, traversal, and removal checks.
- Exposed schedule controls and columns to community users.

## Task Commit

1. **Implement durable schedules, scan serialization, path protections, UI access, and tests** - `fd6a1244476dbd25823653317e16058e708c4717` (`feat`)

Author and committer: `KanameMadoka520 <2441883200@qq.com>`.

## Files Created/Modified

- `agent/app/service/clam.go` - Lifecycle, restoration, normalization, running-state guards, and atomic claim.
- `agent/app/service/clam_schedule_test.go` - Lifecycle, invalid spec, restart, path, running-state, and atomic claim tests.
- `agent/cron/cron.go` - Restores ClamAV schedules during agent cron startup.
- `agent/utils/clam/clam.go` - Schedule registration and secure quarantine creation/removal.
- `agent/utils/clam/clam_security_test.go` - Quarantine permissions, traversal, and symlink tests.
- `agent/utils/xpack/helper/multi_node.go` - Connects the community provider to open Clam scheduling.
- `frontend/src/views/toolbox/clam/index.vue` - Shows status/spec columns and safely formats specs.
- `frontend/src/views/toolbox/clam/operate/index.vue` - Opens schedule editing independently of Pro state.

## Automated Verification

- Linux/WSL `agent`: `go test ./...` passed.
- Targeted ESLint for the two changed Vue files passed.
- `frontend`: `npm run build:pro` passed.
- Focused tests cover create/update/disable/delete, invalid expressions, restart restoration, path normalization, unsafe names, mutation while running, atomic claims, quarantine permissions, traversal, and symlink rejection.

## Decisions Made

- Used persisted rule IDs as cron callback identities.
- Registered replacement schedules before removing old entries.
- Shared one service path for manual and scheduled execution.
- Confined quarantine and deletion below validated base/rule components with `0700` permissions.

## Deviations from Plan

The phase plan was reconstructed after the feature commit. The implementation scope matches the scheduled ClamAV boundary.

## Issues Encountered

- Scheduler state and database state require explicit compensation because the cron library and database do not share a transaction.
- Filesystem safety requires both lexical validation and real-path/symlink checks.

## Residual UAT and Technical Debt

- No target VPS has restarted the agent and demonstrated restored schedules firing.
- No high-frequency real cron schedule has been observed during a long-running Clam scan.
- No isolated EICAR file has been detected, moved/copied, logged, and cleaned on a real VPS.
- Host-specific ClamAV daemon permissions and scan performance remain unknown.
- CLAM-01 is progressed but not listed in `requirements-completed`.

## User Setup Required

Use a disposable absolute directory on a non-production VPS. Begin with `move` or `copy`, never a production website path and never an initial `remove` EICAR test.

## Next Phase Readiness

- Automated scheduling and safety behavior is ready for release packaging.
- Phase 5 must record restart, cron, overlap, and isolated EICAR evidence before CLAM-01 is accepted.

---
*Phase: 03-scheduled-clamav*
*Implementation committed: 2026-07-10*
