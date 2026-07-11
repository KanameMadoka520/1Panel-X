---
phase: 10-branding-image-upload
requirement: BRAND-IMG-01
status: passed
items: 3
executed: 2026-07-11
environment: Japan CN2 VPS 154.36.157.138 (Debian 12), official 1Panel v2.2.3 scaffolding with v1.3.0-open.1 binaries swapped in, Docker-free install on port 34543
---

# Phase 10 Human UAT: Branding Image Upload — EXECUTED (live VPS)

Executed on a live panel running the exact `v1.3.0-open.1` binaries (core sha `2838497e81…`, agent `fc53d4dc29…`, verified in place). Evidence = actual HTTP responses captured over an authenticated session (login via the panel's real RSA+AES hybrid scheme). Note: 1Panel returns HTTP 200 with the real status in the JSON `code` field, so results below are the JSON `code`/`message`.

## UAT-10-1: Serve hardening — no active content, nosniff — PASS
**Method:** placed a real PNG, a PNG+`<svg>` polyglot, a pure `<svg><script>`, and an HTML file at the served names under `<InstallDir>/1panel/uploads/theme`; `curl -i` each; also uploaded real assets and re-served them.
**Result (PASS):**
- valid PNG → `200 image/png` + `X-Content-Type-Options: nosniff`
- **PNG+`<svg>` polyglot → `200 image/png`** (never `image/svg+xml`) + nosniff
- **pure `<svg><script>` → `200 application/octet-stream`** (the old `<svg>` force-override is gone) + nosniff
- HTML `<script>` → `application/octet-stream` + nosniff
- unknown name `/api/v2/images/evilname` → `404`; traversal `/api/v2/images/../../etc/passwd` → `404` (enum gate)
- real uploaded logo/logoWithText/favicon served `image/png`+nosniff; loginImage served `image/gif`+nosniff.

## UAT-10-2: Upload validation rejects unsafe files — PASS
**Method:** authenticated multipart uploads to `POST /settings/enhancements/asset`.
**Result (PASS):**
- SVG → `400 "svg, xml, and html uploads are not allowed"` (T1)
- 3 MB file → `400 "http: request body too large"` (T4, `MaxBytesReader`)
- crafted 20000×20000 PNG → `400 "image dimensions are out of range"` (T5, pre-decode cap)
- GIF as favicon → `400 "favicon must be a PNG image"` (T10)
- unknown key `nope` → `400 "unsupported branding asset"` (T3)
- corrupt-CRC PNG → `400 "png: invalid checksum"` (full-decode integrity)
- **no `X-CSRF-Token` header → real `403`** (T9, CSRF enforced)
- valid PNG/GIF → `200 success`; served end-to-end with correct type + nosniff.

## UAT-10-3: Anonymous presence-only + reset — PASS
**Method:** `curl` anon `GET /enhancements/public`; upload then reset.
**Result (PASS):**
- anon payload exposes cosmetic fields + image **presence sentinels only** (`logo='logo'`, `favicon='favicon'`, …) — **no `watermark`**, no bytes, no paths. With a watermark configured+enabled, `watermark` appears only in the authenticated payload, never anon.
- reset each asset → `200 success`; the file is removed (`/api/v2/images/<key>` → `404`) and the sentinel cleared in both anon and authed responses.

**Verdict:** BRAND-IMG-01 acceptance criteria met on a live panel. Deploy was Docker-free; the box's existing nginx/frps stayed active throughout.
