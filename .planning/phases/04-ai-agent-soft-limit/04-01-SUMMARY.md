---
phase: 04-ai-agent-soft-limit
plan: 01
subsystem: agent-capacity-and-app-install
tags: [go, ai-agent, settings, app-store, soft-limit, i18n]
requires: []
provides:
  - Default unlimited AI Agent count
  - Optional AIAgentLimit setting from 0 through 1000
  - AI-only App Store metadata limit bypass
  - Generic configured-limit errors in supported languages
affects: [phase-05-release, ai-agents, operator-settings]
tech-stack:
  added: []
  patterns: [zero-means-unlimited setting, internal scoped install hook, soft pre-create capacity check]
key-files:
  created:
    - agent/app/service/agents_limit_test.go
    - agent/app/service/setting_ai_agent_limit_test.go
  modified:
    - agent/app/dto/setting.go
    - agent/app/service/agents.go
    - agent/app/service/app.go
    - agent/app/service/setting.go
key-decisions:
  - "Use AIAgentLimit zero as unlimited and 1..1000 as an operator soft limit."
  - "Bypass App Store metadata limits only through internal AI Agent install hooks."
  - "Disclose non-atomic count-and-create behavior and the absence of a dedicated UI."
patterns-established:
  - "Feature-specific install exceptions remain internal and default to normal enforcement."
  - "Unlimited software count is documented separately from host resource capacity."
requirements-completed: []
requirements-progressed: [AGENT-01]
duration: not-recorded
completed: 2026-07-10
---

# Phase 4: AI Agent Soft Limit Summary

**AI Agent creation now defaults to no software count cap and supports an optional operator soft limit, with the App Store metadata cap bypassed only for AI installs.**

## Performance

- **Duration:** Not recorded; retrospective artifact.
- **Completed:** 2026-07-10T20:37:16-04:00
- **Tasks:** 3 reconstructed tasks
- **Files modified:** 16

## Accomplishments

- Removed the fixed community count of five and its license/xpack condition.
- Added `AIAgentLimit` to agent settings with 0..1000 update validation.
- Treated missing or zero as unlimited and positive values as a soft pre-create limit.
- Added an internal AI-only hook that bypasses App Store metadata count limits while normal app installs retain them.
- Generalized the limit error across 11 language files.

## Task Commit

1. **Implement configurable AI Agent limit, metadata bypass, tests, and i18n** - `c305d759133ae7e22f90f11e16921f0e722f9bed` (`feat`)
2. **Add setting boundary and persistence regression coverage** - `c6dab4e1f06204c43a1a7aa50eeb299dfc558f1e` (`test`)

Author and committer: `KanameMadoka520 <2441883200@qq.com>`.

## Files Created/Modified

- `agent/app/dto/setting.go` - Adds `aiAgentLimit` to settings and update allowlist.
- `agent/app/service/agents.go` - Loads/enforces the soft limit and scopes AI install hooks.
- `agent/app/service/agents_limit_test.go` - Unlimited, below-limit, reached-limit, and metadata-hook tests.
- `agent/app/service/setting_ai_agent_limit_test.go` - Direct setting update tests for normalization, persistence, invalid bounds, and no-change-on-rejection behavior.
- `agent/app/service/app.go` - Adds the internal `SkipAppLimit` hook decision.
- `agent/app/service/setting.go` - Validates and normalizes values from 0 through 1000.
- `agent/i18n/lang/*.yaml` (11 files) - Replaces fixed-five wording with configured-limit wording.

## Automated Verification

- Linux/WSL `agent`: `go test ./...` passed.
- Focused tests cover unlimited, below-limit, reached-limit, normal app hooks, AI metadata bypass hooks, and the complete `AIAgentLimit` setting boundary.
- The combined `frontend`: `npm run build:pro` passed; Phase 4 itself has no frontend source change.

## Decisions Made

- Defined zero as the stable unlimited contract.
- Kept the limit as an operator policy rather than a license entitlement.
- Scoped App metadata bypass to internal AI creation hooks.
- Accepted non-atomic count-and-create semantics for the first release and disclosed them as a soft-limit limitation.

## Deviations from Plan

The formal plan was reconstructed after the feature commit. The implementation remained within the Phase 4 boundary.

## Issues Encountered

- App Store metadata supplied a second limit path, so removing only the fixed Agent count would not have delivered the requested outcome.
- A strict count cap under concurrency would require a reservation/transaction design that is beyond the first-release scope.

## Residual UAT and Technical Debt

- No live sixth, tenth, or twenty-fifth Agent has been created on a VPS.
- The positive limit has not been exercised end-to-end through a live settings update and Agent creation workflow.
- Count then create is non-atomic; concurrent requests may exceed the configured limit.
- There is no dedicated UI for `AIAgentLimit`; configuration uses the existing administrative settings mechanism.
- Unlimited count does not imply unlimited CPU, memory, disk, ports, Docker capacity, or provider quota.
- AGENT-01 is progressed but not listed in `requirements-completed`.

## User Setup Required

Use a VPS sized for the intended number of Agents, assign non-conflicting ports, and monitor Docker/host resources during acceptance.

## Next Phase Readiness

- Automated regression behavior is ready for release packaging.
- Phase 5 must test default unlimited and a positive limit on a live host, record resource usage, and note the non-atomic race.

---
*Phase: 04-ai-agent-soft-limit*
*Implementation committed: 2026-07-10*
