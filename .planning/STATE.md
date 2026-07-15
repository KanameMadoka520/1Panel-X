# Project State

## Project Reference

See: `.planning/PROJECT.md`, `.planning/REQUIREMENTS.md`, `.planning/v1.1-MILESTONE-DECISION.md`, `.planning/research/CAPABILITY-MATRIX.md` (updated 2026-07-10)

**Core value:** Deliver a complete, security-conscious, fully open server panel without proprietary code or license bypasses.
**Current milestone:** TWO tracks in flight (user opted into WAF-proper alongside monitoring, 2026-07-13). v1.6 Website Access Monitoring: Phase 16+17 DONE (parser/aggregator + backend store/tailer/geo/API), Phase 18 (dashboard frontend) pending. v1.7 WAF-Proper (own-engine): design done, **Phase 19 (`coraza-gateway` engine) + Phase 20 (attack-event store) DONE (all CI)**. v1.5 Secure Multi-Node SHIPPED `v1.5.0-open.1` (gaps_found, Slice C deferred).
**Current focus:** WAF-proper is now a REAL engine, not a deferral: the closed 1pwaf/OpenResty Lua engine is ABSENT so we ship our own — OWASP Coraza v3 + CRS v4 (PL1) as a loopback reverse-proxy sidecar (`coraza-gateway/`, separate Go module) behind nginx TLS termination. Phase 19 binary (engine compile-then-swap W9, detection/block modes, recover block-vs-pass W1, no-leak block page W7, body caps W3) has 10/10 CI tests: SQLi/XSS/traversal→403, clean→200, detection passes+logs. Phase 20 (attack-event store): gateway emits Coraza JSON audit log (`-audit-log`); agent `wafaudit` parser (parses ModSecurity brackets in error_message, picks primary attack rule over anomaly-eval, W6-sanitizes all attacker fields via exported `weblog.Clean`); `waf.db` store + offset tailer + host→websiteID resolution + retention prune + `POST /websites/:id/waf/events`; all CI-tested (parser real-schema fixtures, temp-sqlite store/list/filter/prune). Next WAF: Phase 21 packaging (compose app + audit-file volume contract) → 22 nginx wiring → 23 frontend. Docs: `.planning/research/WAF-ENGINE-DESIGN.md`, `.planning/v1.7-MILESTONE-DECISION.md` (7 open decisions, provisional defaults adopted). v1.6 monitoring design: `.planning/research/WEBSITE-MONITORING-DESIGN.md`.

## Current Position

Milestone: v1.5 Secure Multi-Node SHIPPED `v1.5.0-open.1` (revision `6d363f6ef`, dirty=false, dual-layer checksums 8/8+7/7). All 6 releases present; v1.0–v1.4 re-verify OK. Prior: v1.4/v1.3(BRAND-IMG-01 live)/v1.2/v1.1/v1.0. None archived.
Phase: 13 (registry+CA+enrollment), 14 (core→node mTLS proxy + agent cert pin + node-mode-if-provisioned + bootstrap), 15 (community node UI + honest admin-gate) all implemented + verified + released.
Status: `gaps_found` — automated gates PASS both modules (core nodepki 10 + node service 4 + helper 3; agent nodepki 4 + helper 3; full core go test exit 0; ESLint 0 + build:pro), adversarial review 0 confirmed defects (N7/N10/N13 adopted). Slice C (cross-network live acceptance) + browser UAT pending on a 2nd VPS. Not archived.
Last activity: 2026-07-13 - v1.5 shipped/verified/documented. Opened v1.6: 2 recon probes (monitoring surface + WAF engine absence), design + decision docs, Phase 16 core (weblog parser + aggregator, 9 fixture tests PASS) committed. 73 ahead of baseline, all KanameMadoka520, worktree clean.

Progress: v1.0–v1.5 released. v1.6 monitoring [######····] Phase 16+17 done (parser/aggregator + store/tailer/geo/API, all CI-tested), Phase 18 frontend + review + release pending. v1.7 WAF-proper [###·······] Phase 19 (real engine, 10/10 CI) + Phase 20 (attack-event store, CI) done, Phases 21-23 next. Neither v1.6 nor v1.7 released yet. v1.5 agent binary b0fbe93f.

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
