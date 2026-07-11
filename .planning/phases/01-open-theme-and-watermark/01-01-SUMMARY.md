---
phase: 01-open-theme-and-watermark
plan: 01
subsystem: core-settings-and-frontend-theme
tags: [go, vue3, theme, watermark, validation, session-auth]
requires: []
provides:
  - License-independent theme color and watermark settings
  - Safe public theme settings response and authenticated full settings
  - Validated persistence with corrupt-value fallback
  - Single-tree watermark rendering
affects: [phase-05-release, browser-uat, frontend-theme]
tech-stack:
  added: []
  patterns: [open fallback behind optional private extension, public-safe DTO split, validated setting persistence]
key-files:
  created:
    - core/app/api/v2/enhancement.go
    - core/app/dto/enhancement.go
    - core/app/service/enhancement.go
    - core/app/service/enhancement_test.go
    - frontend/src/api/modules/enhancement.ts
  modified:
    - core/router/ro_setting.go
    - frontend/src/extensions/theme.ts
    - frontend/src/extensions/xpack.ts
    - frontend/src/layout/index.vue
    - frontend/src/views/setting/panel/index.vue
key-decisions:
  - "Expose only theme mode and color before login; keep watermark settings authenticated."
  - "Implement open behavior directly without changing license state."
  - "Keep one routed DOM tree when watermark state changes."
patterns-established:
  - "Open enhancement fallback: use public GPL endpoints when optional private modules are absent."
  - "Persisted settings are validated on write and read, with safe defaults for corrupt historical data."
requirements-completed: []
requirements-progressed: [THEME-01]
duration: not-recorded
completed: 2026-07-10
---

# Phase 1: Open Theme and Watermark Summary

**Community builds now have real theme color and authenticated watermark behavior without license emulation; automated verification is complete and browser acceptance is deferred.**

## Performance

- **Duration:** Not recorded; this artifact reconstructs an already completed implementation.
- **Completed:** 2026-07-10T20:32:59-04:00
- **Tasks:** 3 reconstructed implementation and verification tasks
- **Files modified:** 21

## Accomplishments

- Added dedicated open enhancement APIs with separate public and authenticated response models.
- Added strict validation for theme modes, CSS colors, preset lists, watermark text, font size, rotation, and spacing.
- Made corrupt stored settings fall back to safe defaults and extended validation to the legacy Theme update path.
- Removed theme color and watermark license gates while preserving optional private extension hooks.
- Kept the routed application under one watermark wrapper so toggling does not replace the main route tree.

## Task Commit

1. **Implement open theme and watermark settings, tests, and frontend wiring** - `bd2cb64d5dfaa31b05e2c1b3a2400376df84168d` (`feat`)

Author and committer: `KanameMadoka520 <2441883200@qq.com>`.

## Files Created/Modified

- `core/app/api/v2/enhancement.go` - Public/full search and authenticated update handlers.
- `core/app/api/v2/entry.go` - Registers the enhancement service.
- `core/app/dto/enhancement.go` - Public, authenticated, and update DTOs.
- `core/app/service/enhancement.go` - Defaults, persistence, validation, and safe fallback.
- `core/app/service/enhancement_test.go` - Settings validation, data minimization, and fallback tests.
- `core/app/service/setting.go` - Applies enhancement validation to the legacy Theme update path.
- `core/middleware/password_expired.go` - Allows only the public enhancement subset during password-expired flow.
- `core/middleware/password_expired_test.go` - Confirms public/full route separation.
- `core/router/ro_setting.go` - Registers public and session-authenticated endpoints.
- `frontend/src/api/helper/check-status.ts` - Accepts the public enhancement request path.
- `frontend/src/api/modules/enhancement.ts` - Open enhancement API client.
- `frontend/src/extensions/theme.ts` - Applies open primary CSS variables.
- `frontend/src/extensions/xpack.ts` - Falls back to open search and update APIs.
- `frontend/src/global/use-theme.ts` - Loads the open theme settings path.
- `frontend/src/layout/components/Sidebar/components/Collapse.vue` - Keeps theme interactions available in community mode.
- `frontend/src/layout/index.vue` - Renders one watermark-wrapped route tree.
- `frontend/src/routers/index.ts` - Loads public theme information during routing initialization.
- `frontend/src/store/modules/global.ts` - Stores open theme and watermark state without Pro status.
- `frontend/src/utils/xpack.ts` - Loads theme settings independently of license state.
- `frontend/src/views/setting/panel/index.vue` - Exposes theme and watermark controls.
- `frontend/src/views/setting/panel/theme-color/index.vue` - Saves custom and preset colors through the open API.

## Automated Verification

- Linux/WSL `core`: `go test ./...` passed.
- Targeted ESLint for the changed Phase 1 frontend files passed.
- `frontend`: `npm run build:pro` passed.
- Focused tests cover unsafe colors, invalid watermark bounds, unknown keys, valid defaults, public watermark exclusion, corrupt stored values, legacy Theme validation, and password-expired route separation.

## Decisions Made

- Used dedicated enhancement endpoints instead of altering xpack license or binding responses.
- Limited the unauthenticated contract to theme mode and theme color.
- Applied primary colors in open code and retained optional private hooks as extensions, not prerequisites.
- Preserved one route DOM tree across watermark enable/disable transitions.

## Deviations from Plan

This is a retrospective plan artifact. The implementation preceded the formal phase plan, but its scope matches the Phase 1 boundary. No unrelated commercial feature was opened.

## Issues Encountered

- Full frontend type checking has known upstream failures and is not used as a Phase 1 pass signal. Targeted ESLint and the production build passed.
- CSS `color-mix()` is build-valid but still requires visual and compatibility checks in the browsers used for VPS acceptance.

## Residual UAT and Technical Debt

- Browser login and public settings inspection have not been performed.
- Theme persistence after refresh in light, dark, and system modes has not been visually accepted.
- Watermark enable/disable rendering and active route-state preservation have not been manually accepted.
- `color-mix()` shade output has not been inspected across target browsers.
- THEME-01 is therefore progressed but not listed in `requirements-completed`.

## User Setup Required

A browser-accessible Phase 5 VPS or equivalent Linux test instance is required for final acceptance. No external SaaS account is required.

## Next Phase Readiness

- The implementation and automated regression gate are ready for combined release builds.
- Phase 5 must run and record the deferred browser checks before THEME-01 can be marked complete.

---
*Phase: 01-open-theme-and-watermark*
*Implementation committed: 2026-07-10*
