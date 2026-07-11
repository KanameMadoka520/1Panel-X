---
phase: 11-branding-image-form
requirement: BRAND-IMG-02
status: partial
items: 2
executed: 2026-07-11
environment: Japan CN2 VPS 154.36.157.138 → SSH tunnel → scripted Chromium (Playwright)
---

# Phase 11 Human UAT: Community Branding Image Form — EXECUTED (live VPS + browser)

Executed against the live v1.3.0-open.1 panel. Branding was set through the actual open endpoints (the form's API contract); the login page was rendered in a real browser via an SSH tunnel and inspected by screenshot + DOM.

## UAT-11-1: Upload, persist, reset (form API contract) — PASS
**Method:** authenticated calls to the same endpoints the form uses (`/enhancements/asset`, `/asset/reset`, `/enhancements/update`).
**Result (PASS):**
- upload logo/logoWithText/favicon/loginImage/loginBackground → `200 success`; each persists and is served with the correct type + nosniff.
- presence sentinels appear in both anon and authed getters after upload; reset clears them and the served file 404s.
- branding text: `Title=<script>…>` → `400 "must not contain angle brackets"`; `LoginBtnLinkColor=url(javascript:…)` → `400`; valid values → `200` (T7 server-side control confirmed live).

## UAT-11-2: Pre-authentication render — PARTIAL (backend fully correct; one upstream login-page gap)
**Method:** logged out, navigated the browser to `…/x1panelxtest`; screenshot + DOM inspection.
**Result:**
- **PASS — custom favicon** applied pre-auth (`link[rel=icon]` → `/api/v2/images/favicon`).
- **PASS — custom login button/link color `#e3a615`** applied (gold button vs. default blue).
- **PASS — all custom images load from the backend** in-browser (loginImage 360×240, loginBackground 600×400, logo 48×48, favicon 32×32 — served correctly with nosniff).
- **GAP — the login-page hero image and background show the built-in defaults**, not the uploaded `loginImage`/`loginBackground`. Root cause is upstream frontend timing, not our backend: `login/index.vue` attempts the image preload once in `onMounted`, before `themeConfig` is populated from the public settings, so the custom login image/background never swap in (the button color/favicon, which apply reactively, do show). The sidebar `Logo.vue` uses a reactive `v-if`+fixed src and is expected to render the custom logo post-login (not re-confirmed by an authed screenshot this session).

**Verdict:** BRAND-IMG-02 form/API contract PASS; pre-auth branding partially visible (favicon + button color). The login-hero image/background is a **known upstream login-page render-timing limitation** exposed (not caused) by this milestone — a candidate frontend follow-up (make `login/index.vue` preload reactively when `themeConfig` updates). Recorded honestly; not marked fully passed.
