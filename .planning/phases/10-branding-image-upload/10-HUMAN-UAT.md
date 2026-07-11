---
phase: 10-branding-image-upload
requirement: BRAND-IMG-01
status: pending
items: 3
---

# Phase 10 Human UAT: Branding Image Upload

Requires a running panel with an authenticated admin session (and a browser/curl). Do not mark pass without evidence.

## UAT-10-1: Serve hardening — no active content, nosniff
**Steps:** Upload a valid PNG logo (Phase 11 form or `POST /api/v2/core/settings/enhancements/asset` with `key=logo`, `file=@logo.png`). `curl -i http://<panel>/api/v2/images/logo`. Then craft a valid PNG that also contains `<svg onload=alert(1)>` in a trailing/tEXt chunk, upload it, and re-fetch.
**Expected:** both responses carry `Content-Type: image/png` (never `image/svg+xml` / `text/html`) and `X-Content-Type-Options: nosniff`; the browser renders/download­s them as an image and never executes the embedded markup. A request for an unknown name (e.g. `/api/v2/images/evil`) returns 404.
**Result:** _pending_

## UAT-10-2: Upload validation rejects unsafe files
**Steps:** Attempt to upload (a) a real `.svg`, (b) an HTML file renamed `.png`, (c) a >2 MB image, (d) a >256 KB favicon or a non-PNG favicon, (e) a decompression-bomb image (tiny file, huge declared dimensions).
**Expected:** each is rejected with a clear error and no file appears under `<InstallDir>/1panel/uploads/theme`; a normal PNG/JPEG/GIF/WEBP (favicon PNG) succeeds and lands at the fixed basename (no extension, no temp residue).
**Result:** _pending_

## UAT-10-3: Anonymous endpoint exposes only presence sentinels; reset works
**Steps:** With a logo/favicon/loginImage set, `curl` the unauthenticated `GET /api/v2/core/settings/enhancements/public`. Then reset an asset via `POST /enhancements/asset/reset`.
**Expected:** the response's `logo/logoWithText/favicon/loginImage` are the bare sentinel strings (their own key) — never bytes, paths, versions; no `watermark`. After reset the file is gone and the field is empty; the UI falls back to the default.
**Result:** _pending_
