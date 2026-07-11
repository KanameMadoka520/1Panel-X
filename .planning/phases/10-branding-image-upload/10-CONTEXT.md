# Phase 10: Branding Image Upload (backend + serve hardening) - Context

**Gathered:** 2026-07-11
**Milestone:** v1.3 Open Image Branding Upload
**Requirement:** BRAND-IMG-01

<domain>
## Phase Boundary

Add the CORE backend for uploading branding images — `logo`, `logoWithText`, `favicon`, `loginImage`, `loginBackground` (image) — plus the mandatory serve-side hardening of the pre-existing anonymous image route. No frontend (Phase 11). No login welcome/subtitle/copyright text (deferred P2). This is the project's first from-scratch multipart file-write surface; every control in the `BRANDING-DESIGN.md` threat model (T1–T6, T8, T9, T10) is honored here.
</domain>

<decisions>
## Implementation Decisions

- **D-01 (T1/T2, serve hardening — REQUIRED):** rewrite `RegisterImages` (`core/init/router/router.go`). Remove the `<svg>`→`image/svg+xml` force-override. Serve only the fixed asset enum (`IsBrandingAssetFileName`), reject non-regular files (`Lstat`), set Content-Type from a raster allowlist (`http.DetectContentType` collapsed to `application/octet-stream` for anything else), and always emit `X-Content-Type-Options: nosniff`. Serve safety must not depend on upload validation.
- **D-02 (T3):** the asset key is a fixed server-side enum → hardcoded basename/settingKey/size-cap/require-PNG (`brandingAssets` map in `enhancement_asset.go`). The client filename is never used. `filepath` prefix assertion before every write (`isWithinDir`).
- **D-03 (T4):** `http.MaxBytesReader` caps the request body before multipart parse (handler); the service re-checks the authoritative per-asset byte cap (2 MiB general / 256 KiB favicon) on the decoded bytes.
- **D-04 (T5):** `image.DecodeConfig` first → reject unknown format or `width*height > 16 MP` BEFORE a full `image.Decode`. Full decode then confirms integrity (rejects header-only polyglots).
- **D-05 (T1 upload half):** reject SVG/XML/HTML by a bounded, case-folded magic-byte scan (`looksLikeMarkup`) before any decoder runs.
- **D-06 (T6):** atomic write — `os.CreateTemp` + `Chmod 0644` + `os.Rename` into `uploads/theme/<assetKey>`; cleanup targets the exact basename (reset removes the exact name, no glob).
- **D-07 (T10):** favicon is PNG-only (`requirePNG`); the stdlib has no `.ico` decoder, so `.ico` is never accepted on MIME/extension alone.
- **D-08 (presence model):** the stored setting value is the asset-key sentinel (`logo`, `favicon`, …), matching the frontend's existing `themeConfig.<field>` truthiness / exact-match consumers. Never a path or bytes. Validator gains fixed-sentinel cases for the four image keys; `LoginBackground` additionally accepts its image sentinel (dual-use with color).
- **D-09 (T8):** widen `PublicEnhancementSettingInfo` with the four image presence sentinels only. They gate images already anonymously served at `/api/v2/images/*`, so nothing new leaks. The subset test still forbids `watermark`/`watermarkShow` and any non-cosmetic key.
- **D-10 (T9):** the two new POST routes sit in the authed `settings` group (SessionAuth + PasswordExpired) under `/api/v2/core`, inheriting the global `CSRFTokenGuard`. No CSRF code is added; coverage is verified against the existing middleware. Image keys are NOT added to the `/enhancements/update` write `oneof` — settable only via the upload/reset service.
</decisions>

<specifics>
- Decoders registered by side-effect import: `image/png`, `image/jpeg`, `image/gif` (stdlib) + `golang.org/x/image/webp` (promoted from indirect to a direct dependency; no new module downloaded).
- Serve route stays in `PublicGroup` (anon, pre-CSRF) because the login page needs it before auth; only the WRITE routes are authed.
- `global.CONF.Base.InstallDir` roots the theme dir, matching the pre-existing serve path.
</specifics>

<canonical_refs>
- `.planning/research/BRANDING-DESIGN.md` — threat model T1–T10.
- `core/app/service/enhancement_asset.go` — registry, validation, atomic write (new).
- `core/app/service/enhancement.go` — validator cases, DTO population.
- `core/init/router/router.go` — `RegisterImages` serve hardening.
- `core/router/ro_setting.go`, `core/app/api/v2/enhancement.go` — routes + handlers.
</canonical_refs>

<deferred>
- Frontend upload form (Phase 11).
- Login welcome/subtitle/copyright text (P2, its own future phase).
- `.ico` favicon support (rejected by design unless a vetted ICO validator is added).
</deferred>

---
*Phase: 10-branding-image-upload*
