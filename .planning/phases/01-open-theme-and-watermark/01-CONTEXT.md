# Phase 1: Open Theme and Watermark - Context

**Gathered:** 2026-07-10
**Status:** Locked after implementation commit; manual acceptance remains open

<domain>
## Phase Boundary

This phase provides license-independent theme color and authenticated watermark settings through the existing 1Panel settings UI. It includes a safe unauthenticated theme subset, authenticated full settings, persistence, validation, fallback behavior, and rendering integration. It does not change license state, unlock unrelated commercial features, or replace branding assets beyond the theme and watermark fields already in scope.

</domain>

<decisions>
## Implementation Decisions

### Open settings API
- **D-01:** Use dedicated open endpoints under `/api/v2/core/settings/enhancements` instead of emulating an xpack license response.
- **D-02:** The unauthenticated endpoint returns only `theme` and `themeColor`; watermark content and watermark status remain authenticated.
- **D-03:** Store values in the existing settings repository and validate both updates and previously persisted data. Invalid stored data falls back to defined defaults.

### Frontend integration
- **D-04:** Preserve private extension loading when it exists, but use the open enhancement API as the community fallback.
- **D-05:** Apply the primary color directly through CSS variables in open code. Do not force `isProductPro`, `isXpackOrEE`, or a bound license state.
- **D-06:** Keep one routed `main-container` tree inside `el-watermark`; disabling the watermark changes content to an empty value instead of replacing the routed DOM subtree.

### Validation and exposure
- **D-07:** Accept only `light`, `dark`, or `auto` theme modes; safe hex/rgb/rgba colors; bounded preset lists; and bounded watermark text, size, rotation, and gap values.
- **D-08:** Permit the public enhancement subset through the password-expired middleware while keeping full search and update routes protected by session authentication.

### Acceptance boundary
- **D-09:** Automated implementation verification is complete, but browser visual acceptance is deferred. Theme persistence, system mode, watermark rendering, route-state preservation, and `color-mix()` behavior must be checked in target browsers before THEME-01 is accepted.

### Agent Discretion
- Exact visual tuning of generated primary color shades may change after browser acceptance, provided the stored data contract and license-independent behavior remain stable.

</decisions>

<specifics>
## Specific Ideas

- The public endpoint must never serialize watermark text, even when a watermark is enabled.
- Watermark toggling must not recreate the route application tree or discard active page state.
- Community behavior must be a real open implementation, not a forged Pro status.

</specifics>

<canonical_refs>
## Canonical References

### Product and acceptance
- `.planning/PROJECT.md` - Clean-room, GPL-compatible project boundary and no-license-emulation rule.
- `.planning/REQUIREMENTS.md` - THEME-01 acceptance criteria and milestone completion rule.
- `.planning/ROADMAP.md` - Phase 1 goal, success criteria, and dependency definition.

### Implementation record
- Commit `bd2cb64d5dfaa31b05e2c1b3a2400376df84168d` - Actual Phase 1 implementation and tests.
- `core/app/service/enhancement.go` - Validation, defaults, persistence, and public/full settings split.
- `frontend/src/extensions/theme.ts` - Open CSS variable application with optional private extension loading.
- `frontend/src/layout/index.vue` - Single-tree watermark rendering.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `settingRepo.UpdateOrCreate` and `settingRepo.GetValueByKey` provide the existing persistence path.
- The global theme store and `useTheme` composable already own theme mode and watermark state.
- Existing panel theme and watermark components provide the administrator workflow.

### Established Patterns
- Core routes separate unauthenticated and `SessionAuth` groups.
- Frontend xpack integrations use `import.meta.glob`; the open fallback can coexist without adding private modules.
- Element Plus `el-watermark` handles repeated watermark layout once validated settings reach the global store.

### Integration Points
- `core/router/ro_setting.go` exposes the public and authenticated enhancement routes.
- `frontend/src/extensions/xpack.ts` and `frontend/src/utils/xpack.ts` load and update the open settings when private modules are absent.
- `frontend/src/global/use-theme.ts` and `frontend/src/layout/index.vue` consume the resulting state.

</code_context>

<deferred>
## Deferred Ideas

- Browser acceptance across login, refresh, light, dark, and system modes is deferred to the Phase 5 VPS/browser gate.
- Browser compatibility and visual quality of CSS `color-mix()` generated shades remain a manual acceptance item.
- Custom logos, favicon, login imagery, and broader branding controls are outside this phase.

</deferred>

---

*Phase: 01-open-theme-and-watermark*
*Context locked: 2026-07-10*
