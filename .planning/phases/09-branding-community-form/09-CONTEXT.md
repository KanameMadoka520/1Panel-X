# Phase 09: Community Branding Form - Context

**Gathered:** 2026-07-11
**Milestone:** v1.2 Open Branding Text & Login Colors
**Requirement:** BRAND-02

<domain>
## Phase Boundary

Give community admins a minimal UI to set the Phase 08 branding fields (`Title`, `MasterAlias`, `LoginBgType`, `LoginBackground` color, `LoginBtnLinkColor`). The render path is already wired (`use-logo.ts` reads the public endpoint pre-auth); the branding EDITOR lived in the absent xpack overlay, so community had none. This phase adds an open form only. No image upload inputs (deferred to v1.3).
</domain>

<decisions>
## Implementation Decisions

- **D-01:** Reuse the existing open write path `updateXpackSettingByKey(key, value)` → `updateOpenEnhancementSetting` → `POST /core/settings/enhancements/update`, which Phase 08 validates.
- **D-02:** Load current values via `getXpackSetting(true)` (authed `/enhancements/search`), the same call the panel settings page already makes.
- **D-03:** Render all text values with mustache interpolation / `el-input` `v-model` (textContent), never `v-html` — defense in depth alongside Phase 08's server-side reject-`<>`.
- **D-04:** Inputs: `el-input` for Title/MasterAlias (maxlength 64), `el-select` for LoginBgType (image|color), `el-color-picker` (or `el-input` validated) for LoginBackground/LoginBtnLinkColor. Save per-field on confirm.
- **D-05:** Placement: a "Brand & Login" section on the existing community panel settings page (`setting/panel/index.vue`) or a small dedicated component it hosts; do NOT touch the license-gated xpack path. Final placement decided during implementation to match existing form structure.
- **D-06:** i18n: reuse existing branding keys where present (`logoWithText`, `loginImage`, etc. exist in all locales); add any missing labels under the same namespace for zh + en.
</decisions>

<specifics>
- No upload control — image branding is v1.3.
- The form must degrade gracefully if a field is empty (empty = use default), matching Phase 08 semantics.
</specifics>

<canonical_refs>
- `.planning/research/BRANDING-DESIGN.md` — frontend contract (T7 client layer).
- `frontend/src/utils/xpack.ts` (`getXpackSetting`, `updateXpackSettingByKey`), `extensions/xpack.ts:38-41`.
- `frontend/src/views/setting/panel/index.vue` — existing community settings page.
- `frontend/src/api/modules/enhancement.ts` — open endpoints.
</canonical_refs>

<deferred>
- Image upload inputs (logo/favicon/loginImage) — v1.3.
- Login welcome/subtitle/copyright inputs — with P2.
</deferred>

---
*Phase: 09-branding-community-form*
