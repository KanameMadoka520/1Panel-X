# Project State

## Project Reference

See: `.planning/PROJECT.md`, `.planning/REQUIREMENTS.md`, `.planning/v1.1-MILESTONE-DECISION.md`, `.planning/research/CAPABILITY-MATRIX.md` (updated 2026-07-10)

**Core value:** Deliver a complete, security-conscious, fully open server panel without proprietary code or license bypasses.
**Current milestone:** v1.2 Open Branding Text & Login Colors
**Current focus:** Ship the safe text/color slice of branding (no file upload) — wire the already-declared, frontend-consumed enhancement fields with server-side XSS controls and a strict anonymous subset, plus a minimal community edit form. Image upload deferred to v1.3.

## Current Position

Milestone: v1.2 (Phases 8-9). v1.1 (Phases 6-7) shipped `v1.1.0-open.1` with 6 UAT pending; v1.0 (Phases 1-5) with 20 UAT pending. None archived.
Phase: 8 (backend) and 9 (frontend form) planned; implementation next.
Status: Milestone defined from a source-verified branding design + adversarial threat model (verdict design_sound, ship-P1-first). Planning docs written; code next.
Last activity: 2026-07-11 - Ran a 6-agent branding investigation (seam already wired, image serve exists but no upload, SVG serve-XSS risk), chose the text/color slice and deferred image upload to v1.3.

Progress: v1.0 [##########] gates done (0/5 accepted). v1.1 [##########] released (0/2 accepted). v1.2 [#---------] planned, implementation pending.

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

1. Implement Phase 8 (backend branding wiring); run `go test ./core/app/service` + full core regression.
2. Implement Phase 9 (community branding form); run `npm run build:pro` + ESLint.
3. Write SUMMARY/VERIFICATION/HUMAN-UAT for phases 8-9; build `v1.2.0-open.1`; roadmap note; v1.2 audit.
4. Carry forward the pending UAT: v1.1 (6 items) + v1.0 (20 items) still need a VPS.
5. v1.3 = image branding upload (honor the threat model in `BRANDING-DESIGN.md`).

## Session Continuity

Last session: 2026-07-10
Stopped at: v1.1 implemented, automatically verified, and released as v1.1.0-open.1 (revision 21d19773c); 6 human UAT items pending.
Resume file: None
