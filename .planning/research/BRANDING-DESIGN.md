# Branding / Custom-Login Design + Threat Model

**Source-verified** 2026-07-11 against revision `edce563ab` (3 source probes + branding research + adversarial threat model). Every claim below is anchored to `path:line` in the probe evidence (workflow `wgegzscqj`).

## The seam (already exists — extend it, do not build parallel)

- Routes: anon `GET /core/settings/enhancements/public`, auth `GET .../search`, auth `POST .../update` (`core/router/ro_setting.go:16,33,34`). Auth routes sit behind SessionAuth + PasswordExpired.
- Handlers are thin pass-throughs (`core/app/api/v2/enhancement.go:10,30`).
- **The DTO already declares all 9 branding fields** (`Title, Logo, LogoWithText, Favicon, LoginImage, LoginBgType, LoginBackground, LoginBtnLinkColor, MasterAlias`) — `core/app/dto/enhancement.go:3-16` — but the service never populates them (dead in the open build).
- Anon gate is a **separate smaller DTO** `PublicEnhancementSettingInfo` (Theme+ThemeColor only) — `dto/enhancement.go:19`, `service/enhancement.go:61`. This is the pre-auth surface to widen.
- Write allowlist is `oneof=Theme ThemeColor Watermark WatermarkShow`, `Value max=8192` (`dto/enhancement.go:25-26`).
- Validator `validateEnhancementSetting` is a switch with a **fail-closed default that rejects unknown keys** (`service/enhancement.go:97,148`); reusable `isSafeCSSColor` (`:154`), `loadValidatedEnhancementValue` safe-default fallback (`:89`).
- Persistence = generic key/value settings table (`core/app/repo/setting.go:71,92`); no migration needed for text values.

## Frontend contract (already wired, community-side)

- `use-logo.ts` reads `title/logo/logoWithText/loginImage/loginBgType/loginBackground/loginBtnLinkColor/favicon` from `getXpackSetting()` (default `authenticated=false`) → `searchXpackSetting(false)` → `getPublicOpenEnhancementSetting()` → `/enhancements/public` (`frontend/src/extensions/xpack.ts:38-41`, `global/use-logo.ts:6-16`). **So the render path consumes the anon subset pre-auth with no frontend change.**
- Image READ already served: `RegisterImages` GET `/api/v2/images/*filename` from `<InstallDir>/1panel/uploads/theme`, traversal-guarded by `filepath.Base` (`core/init/router/router.go:114,117`).
- **The branding EDITOR was in the absent xpack overlay** (`src/xpack` does not exist); `setting/license-required/index.vue` is a license wall, not the editor. Community has no branding edit form → a minimal open form is required to make branding settable.

## Phase decomposition (by blast radius)

- **P1 (low risk, this milestone — v1.2):** text/color fields `Title, MasterAlias, LoginBgType(enum), LoginBackground(color), LoginBtnLinkColor` — backend wiring + a minimal community edit form. No upload, no new route, no new anon bytes. XSS control is server-side (reject `<>`/control chars + rune caps), fully Go-testable; anon subset stays a strict subset of the authed DTO (subset-assertion test).
- **P2 (medium — defer):** new login text `LoginWelcome, LoginSubtitle, Copyright` — new DTO fields + new login-page render sites. Render as textContent/interpolation, never v-html (do NOT reuse the raw-HTML welcome mechanism at `auth.go:230`). Primary control still server-side reject-`<>`.
- **P3 (high — defer to its own milestone, v1.3):** image upload `Logo/LogoWithText/Favicon/LoginImage/LoginBackground(image)` — one authenticated `POST /settings/enhancements/asset` writing fixed-enum filenames into `uploads/theme` atomically. **No upload endpoint exists today** (`router.go:115`); the serve half already exists.

## Threat model (must be honored when P3/P2 land)

| # | Vector | Sev | Required control |
|---|--------|-----|------------------|
| T1 | **Stored SVG XSS, pre-auth same-origin** | critical | `RegisterImages` force-sets `image/svg+xml` for any body containing `<svg` (`router.go:133`), served anon on the login page. **Reject SVG on upload (magic-byte + `<svg` scan) AND harden serve: remove the svg force-override, set fixed Content-Type per assetKey + `X-Content-Type-Options: nosniff`.** Serve-side hardening is REQUIRED, not optional — upload decode cannot fix a serve-side sniff bug. |
| T2 | Polyglot content-type confusion | high | Same serve-side fix (fixed Content-Type per fixed basename, drop `<svg` override, nosniff). A PNG containing `<svg` still gets mis-served otherwise. |
| T3 | Path traversal / filename injection | high | `assetKey` = fixed server-side enum → hardcoded basename; never trust client `Filename`; `filepath.Join`+prefix assert; tmp+rename. |
| T4 | Size / in-memory buffering DoS | medium | `http.MaxBytesReader` before `ParseMultipartForm` (~2MB general / ~256KB favicon); small `MaxMultipartMemory`; 413 on overflow. (No cap exists today; gin default 32MB.) |
| T5 | Decompression / pixel-bomb DoS | medium | `image.DecodeConfig` first, reject `width*height` over a bound (e.g. >16MP) BEFORE full `image.Decode`. |
| T6 | Overwrite / TOCTOU of theme assets | low | cleanup constrained to exact fixed basenames (no glob), atomic tmp+rename. |
| T7 | **Persisted HTML in text fields (P1/P2)** | high | server-side reject `<>`/control chars + rune caps in `validateEnhancementSetting`; frontend renders as textContent/interpolation, never v-html. Both layers. |
| T8 | Unauth data leak / enumeration via widened public DTO | medium | keep `PublicEnhancementSettingInfo` a strict SUBSET of the authed struct; expose only cosmetic text/colors + boolean presence flags + opaque etag, never bytes/paths/watermark/versions. **CI test asserts the public key set ⊆ authed and contains no path/watermark/version keys.** |
| T9 | Missing CSRF/origin on auth POST | medium | verify the SessionAuth group enforces CSRF/same-origin for state-changing POSTs (update + future asset); if cookie-only, add an origin/CSRF check. Verify against actual middleware, do not assume. |
| T10 | `.ico` vs `image.Decode` contradiction | medium | stdlib has no `.ico` decoder → either drop `.ico` (require PNG favicon) or add an explicit ICO validator; never accept `.ico` on extension/MIME alone. Resolve before P3. |

## Open decisions carried to P3 (v1.3)
- Reject SVG (recommended) + neuter serve-side `<svg` override as defense-in-depth (confirm no xpack asset relies on it).
- Uploaded files at `<InstallDir>/1panel/uploads/theme/<fixed assetKey>`, fixed-enum names only.
- Size caps ~2MB / ~256KB favicon; MIME png/jpeg/webp(+ico?) via `image.Decode(Config)`.
- Panel-name canonical source = existing `PanelName` (LoginSetting) which already drives `document.title`; enhancement `Title` = logo/brand text only (avoid two competing name sources).
- Explicit sign-off that the cosmetic anon subset is acceptable world-readable pre-login.
