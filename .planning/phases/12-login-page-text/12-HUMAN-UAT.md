---
phase: 12-login-page-text
requirement: LOGIN-TEXT-01
status: pending
items: 2
---

# Phase 12 Human UAT: Open Login-Page Text (+ login-hero fix)

Requires a running panel with a browser. Do not mark pass without evidence.

## UAT-12-1: Login text renders safely + markup rejected
**Steps:** In Settings → Panel, set login welcome/subtitle/copyright (include one value containing `<b>x</b>`). Log out and view the login page.
**Expected:** the welcome/subtitle appear at the top of the login card and the copyright at the bottom; the markup value is rejected on save with a validation error (never persisted), and any stored text renders literally (no HTML executed). `curl` the anon `/enhancements/public` shows the three text fields (cosmetic) and no watermark/paths.
**Result:** _pending_

## UAT-12-2: Login image/background now display (LOGIN-HERO-RENDER fix)
**Steps:** Upload a login image and set login background type = image with a background image (via the branding form). Log out and view the login page (wide window so the hero shows).
**Expected:** the login page hero image and background now show the **uploaded** image/background (not the defaults) — confirming the reactive-preload fix. Favicon and login button color also reflect the custom branding.
**Result:** _pending_
