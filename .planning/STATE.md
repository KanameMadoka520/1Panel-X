# Project State

## Project Reference

See: `.planning/PROJECT.md`, `.planning/REQUIREMENTS.md`, `.planning/v1.1-MILESTONE-DECISION.md`, `.planning/research/CAPABILITY-MATRIX.md` (updated 2026-07-10)

**Core value:** Deliver a complete, security-conscious, fully open server panel without proprietary code or license bypasses.
**Current milestone:** v1.5 Secure Multi-Node (keystone) — Slice A (backend PKI + enrollment) + Slice B (community node UI). Slice C (cross-network live acceptance) deferred, no 2nd VPS.
**Current focus:** All 3 v1.5 phases implemented + verified. Phase 13 (registry+CA+enrollment), Phase 14 (core→node mTLS proxy + agent cert pin + node-mode-if-provisioned + bootstrap), Phase 15 (community node management page at /settings/nodes, honest admin-gate — no license forging; add→enrollment-token/list/delete). Next: task 24 (adversarial N1-N15 review → release v1.5.0-open.1 → docs/memory).

## Current Position

Milestone: v1.5 Secure Multi-Node IN PROGRESS. Phases 13 + 14 (all backend, both modules) done + committed (58 ahead of baseline, all KanameMadoka520, worktree clean). Prior: v1.4 `v1.4.0-open.1`, v1.3 `v1.3.0-open.1` (BRAND-IMG-01 live), v1.2/v1.1/v1.0. None archived.
Phase: 13 (registry + CA + enrollment token) and 14 (core→node mTLS proxy + agent cert pin + node-mode-if-provisioned + bootstrap) implemented + automatically verified. Phase 15 (community node UI + honest re-gate) pending; then task 24 (review + release).
Status: v1.5 backend automated gates PASS — core nodepki 10 tests (crypto + single-box loopback mTLS proof + N1/N2/N3/N5/N6/N8/N13), core node service 4 tests (N14 SSRF/port/N1 replay/N2 forged), core helper 3 tests (N5 pin proxy/resolve), agent nodepki 4 tests, agent helper 3 tests (IsProvisionedNode default-master exhaustive/ApplyEnrollment/N6 ValidateCertificate); gofmt/vet/build clean both modules. No release yet (after Phase 15 + adversarial review, task #24).
Last activity: 2026-07-13 - Phase 15 committed in 3 scoped commits (feat api/feat page+routes/feat i18n); ESLint 0 + build:pro OK. 62 ahead of baseline.

Progress: v1.0–v1.4 released (branding cluster v1.2–v1.4 feature-complete). v1.5 [#########·] Phases 13+14+15/3 IMPLEMENTED + verified (backend both modules + community node UI). Remaining: task 24 — adversarial N1-N15 review + release v1.5.0-open.1 + docs/memory. Cross-network live boot + operate flow deferred (Slice C, needs 2nd VPS).

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

1. **Next major milestone: v1.5 secure multi-node** — the keystone the 15+ commercial capabilities depend on. Needs a full threat model (identity, enrollment token, mutual mTLS, rotation, replay, audit, failure consistency) and a **second VPS** to accept. The branding cluster (v1.2–v1.4) is now feature-complete, so this is the next domain.
2. Carry-forward VPS/browser UAT: v1.4 (2 — login-text render, hero-fix visual), v1.3 image-form visual, v1.1 (webhook on-disk ciphertext, AI-limit concurrency — needs Docker), v1.0 (ClamAV EICAR, etc.).
3. Gotcha: do not run full core `go test ./...` before a release without reverting `x-log.json` (swagger side-effect); the release script uses focused/compile-only tests.
4. Release toolchain: run from a script file with a `/tmp/relbin` shim (node/npm/go), `sed 's/\r$'"'"'/' | bash` — `export PATH` doesn't survive `wsl.exe -- bash -c` and bash skips the codex npm symlink. See `scratchpad/run-release-v14.sh`.
3. Gotcha: do not run full core `go test ./...` before a release without reverting `x-log.json` (swagger_test.go side-effect); the release script uses focused/compile-only tests.
4. Toolchain gotcha (release): `export PATH` does not survive the `wsl.exe -- bash -c` boundary and bash skips the codex npm symlink; run the release from a script file with a `/tmp/relbin` shim (node/npm/go) piped `sed 's/\r$//' | bash`. See `scratchpad/run-release-v13.sh`.
5. Later: v1.4 threat-modeled secure multi-node (keystone; needs a second VPS).

## Session Continuity

Last session: 2026-07-11
Stopped at: v1.4 (login-page text + LOGIN-HERO fix) implemented, automatically verified, adversarially security-reviewed (0 confirmed defects), and released as v1.4.0-open.1 (revision 3692325f8); 2 human UAT items pending. Branding cluster v1.2–v1.4 feature-complete. Next major milestone is v1.5 secure multi-node (needs a 2nd VPS + full threat model).
Resume file: None
