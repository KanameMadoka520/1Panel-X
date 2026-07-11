# Phase 11: Branding Image Form (frontend) - Context

**Gathered:** 2026-07-11
**Milestone:** v1.3 Open Image Branding Upload
**Requirement:** BRAND-IMG-02

<domain>
## Phase Boundary

Add community upload + reset controls for the five branding images to the existing settings panel form, wired to the Phase 10 open endpoints. Extends the v1.2 branding form; no new page, no enterprise/xpack overlay dependency.
</domain>

<decisions>
## Implementation Decisions

- **D-01:** Add `logo/logoWithText/favicon/loginImage` to the panel `form` reactive and load them from the authenticated getter (`getXpackSetting(true)` → `/enhancements/search`) alongside the v1.2 fields.
- **D-02:** API helpers `uploadOpenEnhancementAsset(key, file)` (multipart `FormData` via `http.upload`) and `resetOpenEnhancementAsset(key)` in `api/modules/enhancement.ts`. CSRF is automatic — the axios interceptor injects `X-CSRF-Token` from the `pcsrftoken` cookie on every non-GET request, including multipart.
- **D-03:** `el-upload` per asset with `:show-file-list="false"`, `:accept`, a client-side `before-upload` size/type hint (server is authoritative), and a custom `:http-request` that calls the upload helper. A per-asset preview `<img :src="/api/v2/images/<key>?t=previewTick">` (cache-busted after upload/reset) and a reset button appear only when the presence sentinel is set.
- **D-04:** `loginBackground` image upload is shown only when `loginBgType === 'image'`; its preview/reset gate on `form.loginBackground === 'loginBackground'` (the image sentinel), keeping it distinct from the color-mode value.
- **D-05:** Rendering safety — image srcs are fixed paths; no user text is `v-html`'d anywhere (branding text still renders via interpolation). Favicon accept is PNG-only in the client, mirroring the server (T10).
- **D-06:** i18n added for zh + en only (other locales fall back), matching the v1.2 convention.
</decisions>

<specifics>
- The community build has no `@/xpack` overlay, so the open endpoints are the real path; enterprise overlays keep their own settings UI.
- `previewTick` is bumped to `Date.now()` after each upload/reset so the browser refetches the changed image.
</specifics>

<canonical_refs>
- `frontend/src/views/setting/panel/index.vue` — the shared settings form (v1.2 branding block).
- `frontend/src/api/modules/enhancement.ts` — open enhancement API module.
- `frontend/src/api/index.ts:56-64` — CSRF header interceptor.
- `frontend/src/layout/.../Logo.vue`, `views/login/index.vue`, `utils/xpack.ts` — the fixed-path image consumers.
</canonical_refs>

<deferred>
- Login welcome/subtitle/copyright text form (P2).
- Drag-crop / client-side resize (not needed; server bounds dimensions).
</deferred>

---
*Phase: 11-branding-image-form*
