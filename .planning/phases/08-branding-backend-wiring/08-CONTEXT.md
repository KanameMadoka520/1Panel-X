# Phase 08: Branding Backend Wiring - Context

**Gathered:** 2026-07-11
**Milestone:** v1.2 Open Branding Text & Login Colors
**Requirement:** BRAND-01

<domain>
## Phase Boundary

Wire the already-declared, frontend-consumed text/color branding fields through the existing v1.0 enhancement seam in CORE only: `Title` (brand text), `MasterAlias`, `LoginBgType` (enum), `LoginBackground` (CSS color), `LoginBtnLinkColor` (CSS color). No file upload, no new route, no image fields (deferred to v1.3), no watermark/theme change.
</domain>

<decisions>
## Implementation Decisions

- **D-01:** Extend `validateEnhancementSetting` (`core/app/service/enhancement.go:97`) with one case per new key. The fail-closed `default` stays, so a forgotten case rejects the write (never opens a hole) — every key is covered by a test.
- **D-02:** Text fields (`Title`, `MasterAlias`): reject `<`/`>` and control characters and cap length (≤64 runes). This is the primary, server-side XSS control (T7) and holds regardless of how the frontend renders.
- **D-03:** `LoginBgType`: `oneof "" | "image" | "color"` (empty = unset). `LoginBackground`, `LoginBtnLinkColor`: empty OR `isSafeCSSColor` (`enhancement.go:154`). Empty string is a valid "unset → frontend default" for every new field.
- **D-04:** Populate the new fields in `GetSettingInfo` (authed) via `loadValidatedEnhancementValue` with empty-string defaults; corrupt stored values fall back to the default.
- **D-05:** Widen `GetPublicSettingInfo` + `PublicEnhancementSettingInfo` to include exactly `Title, MasterAlias, LoginBgType, LoginBackground, LoginBtnLinkColor` (the cosmetic pre-auth set). Keep it a strict SUBSET of the authed struct; a test asserts the public JSON key set ⊆ authed and contains no `watermark`/path/version/secret keys (T8).
- **D-06:** Extend `EnhancementSettingUpdate` `oneof` (`dto/enhancement.go:25`) to accept the five keys; keep `Value max=8192`. Do not add image/logo/favicon keys.
- **D-07:** Do NOT duplicate panel name here; the general `PanelName` setting remains the displayed panel name. `Title` is brand/logo text only.
</decisions>

<specifics>
- Defaults are empty strings so an unconfigured panel behaves exactly as today (frontend built-in defaults).
- The validator is also invoked from `setting.go:179` for the `Theme` key — extending it is centralized and safe.
- Image fields (`Logo`, `LogoWithText`, `Favicon`, `LoginImage`) stay unpopulated/empty in the open build until v1.3.
</specifics>

<canonical_refs>
- `.planning/research/BRANDING-DESIGN.md` — full design + threat model (T7, T8).
- `core/app/service/enhancement.go` — validator, getters, `isSafeCSSColor`, `loadValidatedEnhancementValue`.
- `core/app/dto/enhancement.go` — the three DTOs to touch.
- `frontend/src/global/use-logo.ts`, `extensions/xpack.ts:38-41` — the pre-auth consumer this feeds.
</canonical_refs>

<deferred>
- Image upload/serve (v1.3, P3).
- Login welcome/subtitle/copyright text (P2).
</deferred>

---
*Phase: 08-branding-backend-wiring*
