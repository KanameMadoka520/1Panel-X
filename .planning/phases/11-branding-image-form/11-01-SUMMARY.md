# Phase 11 Summary: Community Branding Image Form

**Requirement:** BRAND-IMG-02
**Date:** 2026-07-11
**Status:** Implemented + automatically verified; browser human UAT pending.

## What shipped

- **API** (`frontend/src/api/modules/enhancement.ts`): `uploadOpenEnhancementAsset(key, file)` posts a multipart `FormData{key,file}` to `/core/settings/enhancements/asset` via `http.upload`; `resetOpenEnhancementAsset(key)` posts to `/asset/reset`. CSRF is automatic — the axios interceptor injects `X-CSRF-Token` from the `pcsrftoken` cookie on every non-GET request, including multipart.
- **Form** (`frontend/src/views/setting/panel/index.vue`): added `logo/logoWithText/favicon/loginImage` to the reactive form, loaded from the authenticated getter. New controls:
  - A `v-for` group of upload+reset controls for logo / logo-with-text / favicon / login image.
  - A login-background image upload+reset shown only when `loginBgType === 'image'`, gated on the `loginBackground` sentinel.
  - Each control: an `el-upload` (`:show-file-list="false"`, `:accept`, a client-side `before-upload` size/type hint, and a custom `:http-request` calling the upload helper), a presence-gated preview `<img :src="/api/v2/images/<key>?t=previewTick">`, and a reset button. `previewTick` is bumped after upload/reset to cache-bust.
  - Branding values render only via interpolation/fixed image srcs; **no `v-html`**.
- **i18n** (`en.ts` + `zh.ts`): `brandImages, brandLogo, brandLogoWithText, brandFavicon, brandLoginImage, brandLoginBgImage, brandUpload, brandImageHelper, brandFaviconHelper, brandImageTooLarge, brandImageBadType`.

## Tests
`npm run build:pro` (Node 24.14.0) passes; changed-file ESLint is clean (0 errors). Client `accept`/`before-upload` hints are aligned with the backend format set (png/jpeg/gif/webp; favicon PNG); the server remains authoritative.

## Deferred
Login welcome/subtitle/copyright text form (P2); drag-crop / client resize (unnecessary — server bounds dimensions).
