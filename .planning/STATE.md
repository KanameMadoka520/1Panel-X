# Project State

## Project Reference

See: `.planning/PROJECT.md`, `.planning/REQUIREMENTS.md`, `.planning/v1.1-MILESTONE-DECISION.md`, `.planning/research/CAPABILITY-MATRIX.md` (updated 2026-07-10)

**Core value:** Deliver a complete, security-conscious, fully open server panel without proprietary code or license bypasses.
**Current milestone:** v1.1 Open Enhancement Hardening
**Current focus:** Pay down the two source-verified debts of the shipped v1.0 features (plaintext webhook secret at rest; non-atomic AI Agent limit + missing UI) as two independent single-node phases, fully CI-verified in WSL.

## Current Position

Milestone: v1.1 (Phases 6-7); v1.0 (Phases 1-5) remains a release candidate with 20 human UAT pending (not archived, not regressed).
Phase: 6 and 7 implemented, automatically verified, and released as `v1.1.0-open.1`.
Status: `gaps_found` — implementation and all automated gates complete; 6 human UAT items (3 per phase) pending on a VPS. Milestone not archived.
Last activity: 2026-07-10 - Built and independently inspected `v1.1.0-open.1` from source revision `21d19773ca6ec9a786931b533ff6292e71f7d409`.

Progress: v1.0 [##########] 100% implementation + automated gates (0/5 accepted, UAT pending). v1.1 [##########] 100% implementation + automated gates + reproducible release (0/2 accepted, 6 UAT pending).

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

1. Transfer `image/releases/v1.1.0-open.1` to a disposable or fully snapshotted VPS; verify both checksum layers.
2. Execute the 6 v1.1 human UAT items (`06-HUMAN-UAT.md`, `07-HUMAN-UAT.md`); record results honestly.
3. Also execute the outstanding 20 v1.0 UAT items where the environment allows.
4. Re-run the v1.1 milestone audit once acceptance evidence exists; only then consider archiving.
5. Next milestone candidate: v1.2 Branding / white-label / custom-login (enhancement-setting cluster; build a file-upload negative-path harness + pre-auth safe subset).

## Session Continuity

Last session: 2026-07-10
Stopped at: v1.1 implemented, automatically verified, and released as v1.1.0-open.1 (revision 21d19773c); 6 human UAT items pending.
Resume file: None
