---
phase: 08-branding-backend-wiring
requirement: BRAND-01
plan: 08-01
status: implemented_automated_verified
human_uat: pending
completed: 2026-07-11
---

# Phase 08 Summary: Branding Backend Wiring

## What shipped

Wired the already-declared, frontend-consumed text/color branding fields through the existing v1.0 enhancement seam in CORE — no file upload, no new route, no image fields. `Title`, `MasterAlias`, `LoginBgType`, `LoginBackground` (color), `LoginBtnLinkColor` are now writable via the existing update endpoint, readable via the authenticated getter, and exposed via the widened anonymous subset so the login page renders them pre-auth.

## Commits
- `6a56f68b8` feat: wire open branding text and login colors
- `e63e43829` test: cover open branding validation and anonymous subset

## Files
- `core/app/service/enhancement.go` — validator cases + `validateBrandingText`; populate the five fields in `GetSettingInfo` (authed) and `GetPublicSettingInfo` (anon subset).
- `core/app/dto/enhancement.go` — widen `PublicEnhancementSettingInfo`; extend `EnhancementSettingUpdate` `oneof`.
- `core/app/service/enhancement_test.go` — subset-assertion, branding validation, corrupt-fallback tests.

## Security controls realized
- **T7 (persisted markup / pre-auth XSS):** `Title`/`MasterAlias` reject `<`/`>` and control chars and cap at 64 runes — server-side, holds regardless of frontend rendering.
- **T8 (anon-surface):** `PublicEnhancementSettingInfo` stays a strict subset; `TestPublicEnhancementSettingIsStrictSubset` asserts the public key set ⊆ authed and excludes watermark/image keys.
- Colors validate via the existing `isSafeCSSColor`; `LoginBgType` is a strict enum; the validator `default` still fails closed; corrupt stored values fall back to empty.

## Tech debt / not done
- Image fields (logo/favicon/loginImage) stay unpopulated until v1.3.
- Human UAT (browser pre-auth render) pending — `08-HUMAN-UAT.md`.
