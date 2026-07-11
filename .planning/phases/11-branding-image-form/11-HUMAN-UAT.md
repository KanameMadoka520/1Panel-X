---
phase: 11-branding-image-form
requirement: BRAND-IMG-02
status: pending
items: 2
---

# Phase 11 Human UAT: Community Branding Image Form

Requires a running panel with a browser. Do not mark pass without evidence.

## UAT-11-1: Upload, preview, persist, reset
**Steps:** In Settings → Panel, upload a logo, logo-with-text, favicon, and login image; set login background type = image and upload a background. Reload the page. Reset one asset.
**Expected:** each upload shows a preview and a success toast; after reload the previews persist; the sidebar logo, browser-tab favicon, and login-page image/background reflect the uploads; reset removes the asset and the UI falls back to the default. An oversize or wrong-type file shows a client error before upload.
**Result:** _pending_

## UAT-11-2: Pre-authentication render
**Steps:** Log out. Observe the login page. (Optionally set a brand title with literal `<`/`>` beforehand.)
**Expected:** the login page renders the custom login image/background and brand text before authentication; brand text is shown literally (no HTML execution); the favicon reflects the uploaded one.
**Result:** _pending_
