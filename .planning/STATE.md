# Project State

## Project Reference

See: `.planning/PROJECT.md`, `.planning/REQUIREMENTS.md`, `.planning/v1.1-MILESTONE-DECISION.md`, `.planning/research/CAPABILITY-MATRIX.md` (updated 2026-07-10)

**Core value:** Deliver a complete, security-conscious, fully open server panel without proprietary code or license bypasses.
**Current milestone:** v1.3 Open Image Branding Upload
**Current focus:** Ship the high-risk image-upload surface deferred from v1.2 — an authenticated upload/reset endpoint with fixed-enum atomic writes and full decode/size validation, PLUS serve-side SVG hardening of the anonymous image route, presence-sentinel storage, a widened-but-strict anon subset, and a community upload form. All under the captured threat model (T1–T10).

## Current Position

Milestone: v1.3 (Phases 10-11) shipped `v1.3.0-open.1`. v1.2 shipped `v1.2.0-open.1` (4 UAT pending); v1.1 `v1.1.0-open.1` (6 UAT pending); v1.0 (20 UAT pending). None archived.
Phase: 10 (backend upload + serve hardening) and 11 (frontend form) implemented, automatically verified, adversarially security-reviewed, and released.
Status: `gaps_found` — implementation + all automated gates + a 0-confirmed-defect adversarial security review complete. **Live VPS UAT executed 2026-07-11** (Japan CN2 VPS, our binaries on official v2.2.3, Docker-free): BRAND-IMG-01 **verified live** (serve hardening, upload rejection, anon presence-only, reset, CSRF); BRAND-IMG-02 form/API **verified live**, but the community login-page hero image/background do not display the uploaded loginImage/loginBackground (upstream `login/index.vue` preload-timing gap — favicon + button color + backend serving all confirmed). One follow-up (`LOGIN-HERO-RENDER`) recorded; milestone still not archived.
Last activity: 2026-07-11 - Live VPS UAT of `v1.3.0-open.1` (revision `39c0a51db`); recorded results in `10/11-HUMAN-UAT.md` and the v1.3 audit.

Progress: v1.0 [##########] gates done (0/5 accepted). v1.1 [##########] released (0/2 accepted). v1.2 [##########] released (0/2 accepted, 4 UAT). v1.3 [##########] released (0/2 accepted, 5 UAT pending).

## Repository Snapshot

- Branch: `open-pro-v1`, tracking `upstream/dev-v2`
- Upstream baseline: `8be2a9ab0270139d0cea2f023ea3f287db2217e0`
- HEAD before v1.1 work: `508403749` (2 docs commits ahead of the v1.0.0-open.1 release revision `cc5d31aa7`)
- v1.0 release: `image/releases/v1.0.0-open.1` (dual-layer checksums verified 8/8 + 7/7; must not be overwritten)
- Toolchain confirmed working: Go 1.26.1 at `/tmp/codex-go1.26.1/go/bin/go` (WSL Ubuntu), Node 24.14.0, npm 11.14.1, Docker 29.2.1. Security-critical focused tests pass at HEAD (regression baseline clean).
- Commit identity configured locally: `KanameMadoka520 <2441883200@qq.com>`

## Accumulated Context

### Decisions

- v1.1 hardens shipped features before adding surface; there is no cheap UI-gate capability to grab (`uiGateOnly = ∅`).
- Reuse the panel's existing `EncryptKey` + `encrypt.StringEncrypt/Decrypt` for the webhook secret — no new key-management design.
- Make `AIAgentLimit` race-free with a mutex-guarded slot reservation (count read inside the lock + in-flight counter); do not hold a lock across the container install.
- Dropped the speculative capability-registry and demoted UAT-automation from the milestone (per the adversarial critique).
- Sequence branding (v1.2, enhancement-setting cluster) before threat-modeled multi-node (v1.3, keystone, needs 2nd VPS).

### Blockers and Concerns

- Full VPS acceptance still unavailable; v1.1 is deliberately chosen to be fully CI-verifiable without one, but real-robot delivery, on-disk ciphertext inspection, live concurrency, and browser UAT remain human debt.
- Webhook encryption migration must be safe with zero rows and a freshly seeded key; must not break live alerts (legacy plaintext still delivers until migrated).
- Frontend `type-check` has known upstream errors; verify changed-code behavior and report baseline failures without attributing them to this project.

## Next Actions

1. **`LOGIN-HERO-RENDER` follow-up (frontend-only):** make `frontend/src/views/login/index.vue` preload the uploaded loginImage/loginBackground reactively (watch `themeConfig`) instead of a one-shot `onMounted` check, so an uploaded login image/background actually shows on the community login page. Discovered in the 2026-07-11 live UAT. Would ship as v1.3.1 or fold into P2.
2. Remaining carry-forward UAT: v1.3 login-hero visual (after the fix), v1.1 (webhook on-disk ciphertext, AI-limit concurrency — needs Docker), v1.0 (ClamAV EICAR, etc.). v1.3 BRAND-IMG-01 + text/form items are now verified live.
3. Next milestone candidate: login welcome/subtitle/copyright text (P2) — new login-page text fields, server-side reject-`<>`, rendered as interpolation (never `v-html`); its own small phase.
3. Gotcha: do not run full core `go test ./...` before a release without reverting `x-log.json` (swagger_test.go side-effect); the release script uses focused/compile-only tests.
4. Toolchain gotcha (release): `export PATH` does not survive the `wsl.exe -- bash -c` boundary and bash skips the codex npm symlink; run the release from a script file with a `/tmp/relbin` shim (node/npm/go) piped `sed 's/\r$//' | bash`. See `scratchpad/run-release-v13.sh`.
5. Later: v1.4 threat-modeled secure multi-node (keystone; needs a second VPS).

## Session Continuity

Last session: 2026-07-11
Stopped at: v1.3 implemented, automatically verified, adversarially security-reviewed (0 confirmed defects), and released as v1.3.0-open.1 (revision 39c0a51db); 5 human UAT items pending. Next candidate is login welcome/subtitle/copyright text (P2).
Resume file: None
