# Phase 3: Scheduled ClamAV - Context

**Gathered:** 2026-07-10
**Status:** Locked after implementation commit; VPS/EICAR acceptance remains open

<domain>
## Phase Boundary

This phase makes ClamAV scan rules schedulable in community builds and keeps database state, cron entries, restart recovery, execution serialization, and quarantine paths consistent. It covers create, update, enable, disable, delete, restore, manual/scheduled execution, and UI access to schedules. It does not install ClamAV, prove host-specific daemon behavior, or authorize scanning production data during first acceptance.

</domain>

<decisions>
## Implementation Decisions

### Schedule lifecycle
- **D-01:** Validate the standard cron expression before persistence. On create, persist the rule first so it has an ID, then register the callback; roll back both database and cron state on failure.
- **D-02:** On update or enable, register the replacement before persisting/removing the old entry. Remove the replacement if persistence fails, and remove the old entry only after the new state is committed.
- **D-03:** Agent startup restores each enabled rule with a non-empty valid spec and writes the new in-process `EntryID`; invalid rules are disabled instead of silently left active.
- **D-04:** Update, status change, and delete reject rules marked as executing.

### Execution serialization
- **D-05:** Manual and scheduled runs share `HandleOnce` and claim execution with one conditional database update (`id = ? AND is_executing = false`). Only one caller can start a rule.
- **D-06:** Existing record completion resets `is_executing`; task-construction failure also clears the claim.

### Path and destructive-action safety
- **D-07:** Normalize rule names to one bounded path component without separators, traversal names, or control characters.
- **D-08:** Scan and quarantine base paths must be absolute existing real directories, resolve symlinks, reject filesystem roots, and must not overlap in either direction.
- **D-09:** Quarantine output is restricted to `<infected-base>/1panel-infected/<rule>/<run>`, rejects symlink components, and uses mode `0700`.
- **D-10:** Removal reconstructs the confined rule path from validated components rather than accepting an arbitrary deletion target.

### UI and acceptance
- **D-11:** Expose schedule status/spec columns and schedule editing without a Pro license check; unrelated alert licensing behavior remains unchanged.
- **D-12:** Automated lifecycle and path tests are complete. A real ClamAV daemon, cron timing, restart, and isolated EICAR flow have not been exercised on a VPS.

### Agent Discretion
- Operational logging and cron display formatting may be refined after VPS observation without weakening persistence, serialization, or path constraints.

</decisions>

<specifics>
## Specific Ideas

- EICAR acceptance must use a disposable directory and a move/copy quarantine strategy. It must never use a real website directory or an initial `remove` strategy.
- A high-frequency schedule firing during an active scan must receive `TaskIsExecuting`, not start a second scanner.
- Restart recovery must update the in-memory cron entry ID stored in the database.

</specifics>

<canonical_refs>
## Canonical References

### Product and acceptance
- `.planning/PROJECT.md` - Security and clean-room constraints.
- `.planning/REQUIREMENTS.md` - CLAM-01 acceptance criteria.
- `.planning/ROADMAP.md` - Phase 3 goal and success criteria.

### Implementation record
- Commit `fd6a1244476dbd25823653317e16058e708c4717` - Actual Phase 3 implementation and tests.
- `agent/app/service/clam.go` - Lifecycle, restart, path normalization, and atomic execution claim.
- `agent/utils/clam/clam.go` - Scheduler callback and confined quarantine operations.
- `agent/cron/cron.go` - Startup restoration hook.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- Existing GORM Clam repository stores `EntryID`, status, spec, and `is_executing`.
- Global robfig/cron instance provides standard expression parsing and callback registration.
- Existing task and Clam record flow resets execution state on completion.

### Established Patterns
- GPL `MultiNodeProvider` is the extension boundary used by community scheduling.
- Manual scans already pass through `HandleOnce`; scheduled callbacks can reuse it.
- Existing Vue schedule controls already produce the spec object and only required license-gate removal.

### Integration Points
- `agent/cron/cron.go` calls `RestoreClamSchedules` before starting the scheduler.
- `agent/utils/clam/clam.go` calls the registered service handler by persisted rule ID.
- The Clam list/editor Vue files expose status, schedule display, and schedule input.

</code_context>

<deferred>
## Deferred Ideas

- Real VPS restart recovery, high-frequency scheduling, and Clam daemon integration.
- Isolated EICAR detection and quarantine acceptance.
- Production-scale scan duration, disk pressure, and alert-volume tuning.

</deferred>

---
*Phase: 03-scheduled-clamav*
*Context locked: 2026-07-10*
