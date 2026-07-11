# Project State

## Project Reference

See: `.planning/PROJECT.md`, `.planning/REQUIREMENTS.md`, `.planning/v1.1-MILESTONE-DECISION.md`, `.planning/research/CAPABILITY-MATRIX.md` (updated 2026-07-10)

**Core value:** Deliver a complete, security-conscious, fully open server panel without proprietary code or license bypasses.
**Current milestone:** v1.3 Open Image Branding Upload
**Current focus:** Ship the high-risk image-upload surface deferred from v1.2 — an authenticated upload/reset endpoint with fixed-enum atomic writes and full decode/size validation, PLUS serve-side SVG hardening of the anonymous image route, presence-sentinel storage, a widened-but-strict anon subset, and a community upload form. All under the captured threat model (T1–T10).

## Current Position

Milestone: v1.3 (Phases 10-11) implemented + automatically verified; adversarial security review + release pending. v1.2 shipped `v1.2.0-open.1` (4 UAT pending); v1.1 `v1.1.0-open.1` (6 UAT pending); v1.0 (20 UAT pending). None archived.
Phase: 10 (backend upload + serve hardening) and 11 (frontend form) implemented; gofmt/vet clean, focused Go tests pass, `build:pro` + changed-file ESLint pass.
Status: implementation + automated gates complete; adversarial security-review workflow over the diff in progress; release `v1.3.0-open.1` not yet built. Human UAT (browser/API/pre-auth render) will remain pending without a VPS. Milestone will be `gaps_found`, not archived.
Last activity: 2026-07-11 - Implemented v1.3 backend + frontend; ran the T1–T10 adversarial security review workflow before release.

Progress: v1.0 [##########] gates done (0/5 accepted). v1.1 [##########] released (0/2 accepted). v1.2 [##########] released (0/2 accepted, 4 UAT). v1.3 [########--] implemented + verified, review/release in progress.

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

1. v1.3 = image branding upload (logo/favicon/loginImage): implement under the threat model in `research/BRANDING-DESIGN.md` — serve-side SVG hardening (`router.go:133`), `MaxBytesReader` + `image.DecodeConfig` caps, fixed-enum filenames, CSRF verification on the write routes, `.ico` decision. May fold in login welcome/subtitle/copyright text (P2).
2. Carry forward pending UAT on a VPS/browser: v1.2 (4), v1.1 (6), v1.0 (20).
3. Gotcha: do not run full core `go test ./...` before a release without reverting `x-log.json` (swagger_test.go side-effect); the release script uses focused/compile-only tests.
4. Later: v1.4 threat-modeled secure multi-node (keystone; needs a second VPS).

## Session Continuity

Last session: 2026-07-11
Stopped at: v1.2 implemented, automatically verified, and released as v1.2.0-open.1 (revision 8f9e18fe4); 4 human UAT items pending. Next milestone is v1.3 image branding upload under the captured threat model.
Resume file: None
