---
phase: 11-branding-image-form
requirement: BRAND-IMG-02
verification_status: human_needed
automated: passed
environment: WSL Ubuntu, Node 24.14.0 (/tmp/codex-node24.14.0)
date: 2026-07-11
---

# Phase 11 Verification: Community Branding Image Form

## Automated evidence (passed)

| Check | Command | Result |
|-------|---------|--------|
| Production build | `npm run build:pro` | ✓ built (7076+ modules) |
| Changed-file ESLint | `eslint src/views/setting/panel/index.vue src/api/modules/enhancement.ts src/lang/modules/{en,zh}.ts` | 0 errors |

### Mapped to acceptance criteria
- **AC1 (upload + reset controls, bg-image gating):** the `v-for` group covers logo/logoWithText/favicon/loginImage; the login-background image control is `v-if="loginBgType === 'image'"` and its preview/reset gate on the `loginBackground` sentinel.
- **AC2 (multipart + auto-CSRF; server authoritative):** `uploadOpenEnhancementAsset` posts `FormData`; CSRF header is injected by the interceptor; `before-upload` is a client hint only (the Phase 10 service re-validates authoritatively).
- **AC3 (fixed served paths; no v-html; cache-bust):** previews use `/api/v2/images/<key>?t=previewTick`; no branding value is `v-html`'d; `previewTick` bumps after upload/reset.
- **AC4:** `build:pro` + ESLint pass; browser set/persist + pre-auth render are human UAT.

## Human-needed (11-HUMAN-UAT.md)
Browser: upload/reset each asset and confirm it persists and renders (sidebar logo, favicon, login page image/background); confirm the login page renders custom images pre-authentication; confirm branding text still renders literally (no HTML execution).

## Note
The build emits the frontend into the gitignored embed dirs (`core/cmd/server/web/assets`, `index.html`); the worktree stays clean and the release build regenerates it. The frontend `type-check` has known pre-existing upstream errors unrelated to this change; the production build (esbuild transpile) and ESLint are the gates.
