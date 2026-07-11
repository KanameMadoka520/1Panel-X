# Phase 10 Summary: Branding Image Upload (backend + serve hardening)

**Requirement:** BRAND-IMG-01
**Date:** 2026-07-11
**Status:** Implemented + automatically verified; adversarial security review recorded; browser/API human UAT pending.

## What shipped

- **Serve-side hardening (T1/T2)** — `core/init/router/router.go` `RegisterImages`: removed the `<svg>`→`image/svg+xml` force-override; the route now serves only the fixed asset enum (`service.IsBrandingAssetFileName`), rejects non-regular files (`Lstat`), sets Content-Type from a raster allowlist identical to the upload whitelist (png/jpeg/gif/webp; everything else → `application/octet-stream`), and always sends `X-Content-Type-Options: nosniff`. New helper `sniffImageContentType`.
- **Upload service (new `core/app/service/enhancement_asset.go`)** — `SaveAsset(assetKey, data)`: fixed-enum `brandingAssets` map (key → settingKey/maxBytes/requirePNG); non-empty + per-asset size cap (2 MiB / 256 KiB favicon, T4); `looksLikeMarkup` reject of SVG/XML/HTML (T1); `image.DecodeConfig` format whitelist + favicon-PNG-only (T10) + `≤16 MP` dimension cap **before** full `image.Decode` (T5); atomic temp+chmod+rename into `uploads/theme/<assetKey>` with `isWithinDir` prefix assert (T3/T6); store the presence sentinel. `ResetAsset` removes the exact basename + clears the sentinel. Decoders registered by side-effect (png/jpeg/gif + `golang.org/x/image/webp`, promoted to a direct dependency).
- **API + routes** — `UploadEnhancementAsset` (`MaxBytesReader` body cap before parse) + `ResetEnhancementAsset` in `api/v2/enhancement.go`; `POST /enhancements/asset` + `/asset/reset` in the authed `settingRouter` group (SessionAuth + PasswordExpired), inheriting the global `CSRFTokenGuard` (T9). Image keys are **not** in the `/enhancements/update` `oneof`.
- **DTO + validator** — authed getter populates `Logo/LogoWithText/Favicon/LoginImage`; `PublicEnhancementSettingInfo` widened with the four presence sentinels (T8); validator gains fixed-sentinel cases for the four image keys and the `LoginBackground` image sentinel (dual-use with color); fail-closed `default` preserved. New `EnhancementAssetReset` DTO with an enum `oneof`.

## Tests
`core/app/service/enhancement_asset_test.go` (new) + extended `enhancement_test.go`: serve enum helper; validator sentinel accept/reject; happy path (file + sentinel written, no temp residue); jpeg/gif accepted; favicon PNG-only; SVG/XML/HTML rejected; non-image rejected; crafted huge-dimension PNG (pixel bomb) rejected with no file written; oversize bytes rejected (general + favicon); unknown key rejected; empty rejected; `LoginBackground` sentinel survives read-path; reset removes + clears + idempotent + unknown-key rejected; subset test updated to prove the four image presence sentinels are public while watermark/watermarkShow stay forbidden.

## Security review
A dedicated adversarial workflow (7 independent T1–T10 lenses + a completeness critic → per-finding skeptic verification, 15 agents) found **0 confirmed defects**. All 7 critic hygiene notes were adversarially refuted as not-a-bug. Three of them were adopted as quality polish anyway: corrected an over-reaching decode comment, tightened the serve allowlist to exactly match the upload whitelist (no drift), and aligned the frontend `accept` lists with the backend format set.

## Deferred
Login welcome/subtitle/copyright text (P2); `.ico` favicon (rejected by design).
