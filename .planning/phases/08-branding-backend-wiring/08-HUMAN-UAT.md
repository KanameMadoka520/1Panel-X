---
phase: 08-branding-backend-wiring
requirement: BRAND-01
status: pending
items: 2
---

# Phase 08 Human UAT: Branding Backend Wiring

Requires a running panel. Do not mark pass without evidence.

## UAT-08-1: Anonymous endpoint exposes only the cosmetic subset
**Steps:** Set brand/login values (via the Phase 09 form or the API). `curl` the unauthenticated `GET /api/v2/core/settings/enhancements/public`.
**Expected:** response contains `theme,themeColor,title,masterAlias,loginBgType,loginBackground,loginBtnLinkColor` only; no `watermark`, `logo`, `favicon`, `loginImage`, versions, or paths.
**Result:** _pending_

## UAT-08-2: Markup is rejected end to end
**Steps:** POST `Title` = `<script>alert(1)</script>` to `/enhancements/update`.
**Expected:** rejected with a validation error; nothing persisted; the login page never renders the markup.
**Result:** _pending_
