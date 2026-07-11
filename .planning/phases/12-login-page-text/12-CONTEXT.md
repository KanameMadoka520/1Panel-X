# Phase 12: Open Login-Page Text (+ login-hero fix) - Context

**Gathered:** 2026-07-11
**Milestone:** v1.4 Open Login-Page Text
**Requirement:** LOGIN-TEXT-01

<domain>
## Phase Boundary

Wire three operator-authored login-page text fields (`LoginWelcome`, `LoginSubtitle`, `Copyright`) through the existing enhancement seam and render them on the community login page (interpolation only). Bundle the `LOGIN-HERO-RENDER` fix found in the v1.3 live UAT. No new route, no file upload, no HTML rendering.
</domain>

<decisions>
## Implementation Decisions

- **D-01 (T7, server-side XSS):** reuse `validateBrandingText` (`enhancement.go`) — reject `<>`/control chars + rune caps (welcome/subtitle 128, copyright 200). Empty = unset. The validator's fail-closed default is preserved.
- **D-02:** populate `LoginWelcome/LoginSubtitle/Copyright` in the authenticated getter and the anonymous subset (the login page needs them pre-auth); extend `EnhancementSettingUpdate.oneof`; the strict-subset test now includes them and still excludes watermark/paths/bytes.
- **D-03 (client render):** render welcome/subtitle at the top of the login card and copyright as a footer via `{{ }}` interpolation — **never `v-html`** (the threat model explicitly forbids reusing the raw-HTML welcome mechanism). `themeConfig` carries the values (store type + `getXpackSettingForTheme` + `use-logo` populate them from the public endpoint).
- **D-04 (community form):** three `el-input` fields in the existing branding form section, saved via the open update endpoint (`onSaveBranding`).
- **D-05 (`LOGIN-HERO-RENDER` fix):** replace the one-shot `onMounted` preload in `login/index.vue` with a reactive `watch` on `themeConfig.loginImage/loginBgType/loginBackground` (immediate), so the uploaded login image/background swap in once the async public settings populate.
</decisions>

<specifics>
- `themeConfig` gains `loginWelcome/loginSubtitle/copyright` (store interface + default object in `store/modules/global.ts`).
- i18n added for zh + en; other locales fall back.
- `computed`/`watch`/`onUnmounted` are auto-imported (unplugin-auto-import), so `watch` needs no explicit import in `login/index.vue`.
</specifics>

<canonical_refs>
- `core/app/service/enhancement.go` — validator + getters (reuse `validateBrandingText`).
- `core/app/dto/enhancement.go` — the three DTOs.
- `frontend/src/views/login/components/login-form.vue` — render sites.
- `frontend/src/views/login/index.vue` — the LOGIN-HERO reactive fix.
- `frontend/src/utils/xpack.ts`, `global/use-logo.ts`, `store/{interface,modules}` — themeConfig population.
- `.planning/research/BRANDING-DESIGN.md` (T7), `.planning/v1.2-MILESTONE-DECISION.md` (P2 deferral).
</canonical_refs>

<deferred>
- Rich/multi-line/HTML login text.
- v1.5+ secure multi-node.
</deferred>

---
*Phase: 12-login-page-text*
