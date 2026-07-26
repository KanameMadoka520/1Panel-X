# Project State

## Project Reference

See: `.planning/PROJECT.md`, `.planning/REQUIREMENTS.md`, `.planning/v1.1-MILESTONE-DECISION.md`, `.planning/research/CAPABILITY-MATRIX.md` (updated 2026-07-10)

**Core value:** Deliver a complete, security-conscious, fully open server panel without proprietary code or license bypasses.
**Current milestone:** TWO tracks in flight (user opted into WAF-proper alongside monitoring, 2026-07-13). v1.6 Website Access Monitoring: Phase 16+17+18 DONE (parser/aggregator + backend store/tailer/geo/API + dashboard frontend), review done, **release pending**. v1.7 WAF-Proper (own-engine): **Phase 19–23 ALL DONE and pushed to `origin/main`**, release pending. v1.5 Secure Multi-Node SHIPPED `v1.5.0-open.1` (gaps_found, Slice C deferred).
**Current focus:** WAF-proper is a REAL engine, not a deferral: the closed 1pwaf/OpenResty Lua engine is ABSENT so we ship our own — OWASP Coraza v3 + CRS v4 (PL1) as a loopback reverse-proxy sidecar (`coraza-gateway/`, separate Go module) behind nginx TLS termination.
- **Phase 19** engine (compile-then-swap W9, detection/block modes, recover block-vs-pass W1, no-leak block page W7, body caps W3): 10/10 CI tests — SQLi/XSS/traversal→403, clean→200, detection passes+logs.
- **Phase 20** attack-event store: gateway emits Coraza JSON audit log (`-audit-log`); agent `wafaudit` parser (ModSecurity brackets in `error_message`, primary attack rule over anomaly-eval, W6-sanitized via exported `weblog.Clean`); `waf.db` store + offset tailer + host→websiteID resolution + retention prune + `POST /websites/:id/waf/events`.
- **Phase 21–22** (`16d3856b7`→`d0de5a76b`): compose packaging + `wafconfig` deterministic config generation (SHA-256 `generation`) + per-site nginx `root.conf` `proxy_pass` real re-routing + generation-gated readiness + nginx check/reload rollback + aligned `client_max_body_size`. **Verified live on the experiment VPS.**
- **Phase 23a** (`990916105`) per-site IP allow/deny lists, full stack: gateway `ipacl.go` (deny→403 in BOTH modes, evaluated before CRS; allow bypasses CRS but is still proxied through the hardened reverse proxy and still body-capped; single IP + IPv4/IPv6 CIDR; invalid entry fails startup), agent `NormalizeIPList` (canonicalize/dedupe/sort, cap 512), `allow_ips`/`deny_ips` text columns, two textareas in the site WAF tab.
- **Phase 23b** (`29e227248`) two-tier spine (milestone decision #3): `WafGlobalPolicy` single row (fixed PK upsert), site mode stores an explicit `"inherit"` sentinel (gorm drops zero-valued fields carrying a `default` tag, so `""` would silently become the column default and lose intent), `effectivePolicyMode` resolution, `MergeIPLists` (global ∪ site, still capped), `GET/POST /websites/waf/global`, `effectiveMode` in the status response, inherit option + global-defaults dialog in the UI. **The data plane is unaware of the two tiers — merging happens entirely control-plane-side in `writeGatewayConfig`.**

Docs: `.planning/research/WAF-ENGINE-DESIGN.md` (W1–W12), `.planning/v1.7-MILESTONE-DECISION.md` (7 open decisions, provisional defaults adopted), **`.planning/research/WAF-PRO-PARITY.md` (2026-07-26 — observed Pro WAF surface from user-supplied screenshots + honesty traps R1–R7)**. v1.6 monitoring design: `.planning/research/WEBSITE-MONITORING-DESIGN.md`.

## Current Position

Milestone: v1.5 Secure Multi-Node SHIPPED `v1.5.0-open.1` (revision `6d363f6ef`, dirty=false, dual-layer checksums 8/8+7/7). All 6 releases present; v1.0–v1.4 re-verify OK. Prior: v1.4/v1.3(BRAND-IMG-01 live)/v1.2/v1.1/v1.0. None archived.
Phase: 13 (registry+CA+enrollment), 14 (core→node mTLS proxy + agent cert pin + node-mode-if-provisioned + bootstrap), 15 (community node UI + honest admin-gate) all implemented + verified + released.
Status: `gaps_found` — automated gates PASS both modules (core nodepki 10 + node service 4 + helper 3; agent nodepki 4 + helper 3; full core go test exit 0; ESLint 0 + build:pro), adversarial review 0 confirmed defects (N7/N10/N13 adopted). Slice C (cross-network live acceptance) + browser UAT pending on a 2nd VPS. Not archived.
Last activity: 2026-07-26 - WAF Phase 21→23 completed and pushed to `origin/main` (`16d3856b7`, `b8d4c1216`, `7e440655d`, `d0de5a76b`, `990916105`, `29e227248`), all authored by KanameMadoka520, worktree clean, 0/0 ahead/behind origin. User supplied 1Panel Pro WAF screenshots (10) → observed surface recorded in `.planning/research/WAF-PRO-PARITY.md`; a Pro-parity gap matrix + phase plan is being produced. User explicitly deferred human UAT ("暂时没法验证，之后再说") and asked to keep building WAF features — UAT items therefore stay `pending`, never marked passed.

Progress: v1.0–v1.5 released. v1.6 monitoring [########··] Phase 16+17 (backend, CI) + Phase 18 (dashboard frontend, build:pro+ESLint clean) done; review done; **release pending**. v1.7 WAF-proper [##########] Phase 19 (real engine 10/10 CI) + Phase 20 (attack-event store, CI) + Phase 21 gateway multi-site routing + Phase 21–22 packaging/config-generation/nginx wiring (**VPS-verified**) + Phase 23 per-site IP allow/deny lists + two-tier global defaults — all CI-green (gateway + `wafconfig` + `waf_control` Go tests, ESLint, `build:pro`); **release pending**. Neither v1.6 nor v1.7 released yet. v1.5 agent binary b0fbe93f (agent source has since changed again for WAF).

Next domain (post-parity recon): community WAF feature build-out toward the observed Pro surface — rate limiting + temporary IP bans, URL/User-Agent black-white lists, geo restrictions, file-upload limits, CDN real-IP header list, CRS rule-group toggles, custom rules, and a dedicated top-level WAF page (overview/reports/logs/bans/lists/site/global). Every item is bound by the R1–R7 honesty traps in `WAF-PRO-PARITY.md` — in particular: no request/4xx/5xx counters we do not actually measure, no regex rule table we do not actually enforce, no OpenResty ban semantics we do not actually have.

Adversarial review (17-agent workflow, both tracks) done 2026-07-15: 13 candidates → 7 CONFIRMED, 6 refuted. All fixed + CI-tested except #7 (documented): (1) monitoring split-bucket undercount → atomic additive SaveFinalized (unique index + ON CONFLICT accumulate); (2) WAF secondary-domain mis-attribution → hostResolver includes Domains; (3/4) WAF non-idempotent ingest → TxID dedup (ON CONFLICT DO NOTHING); (5) gateway real client-IP → WithRealIPHeader(X-Real-IP); (6) response banner leak → StripFingerprintHeaders (W7); (7) log-rotation loss → documented known limitation.

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

1. **Community WAF build-out toward the observed Pro surface** (user's current directive, 2026-07-26). Recon + gap matrix + phase plan first, then implement data-plane capability → control-plane storage → frontend, one CI-verifiable phase per commit. Spec: `.planning/research/WAF-PRO-PARITY.md`.
2. **Release v1.6 (monitoring) and v1.7 (WAF)** — both are code-complete and CI-green but unreleased. v1.7 packaging is new territory: the third Go module `coraza-gateway` (binary/image + compose asset) must enter the release artifact set, extending the v1.5 release-script pattern.
3. Carry-forward human UAT debt (**user deferred verification 2026-07-26; keep `pending`, never fabricate**): v1.7 WAF browser/live-traffic (blocking, IP lists, two-tier), v1.6 dashboard browser, v1.5 Slice C cross-network mTLS (needs a 2nd VPS), v1.4 login-text render, v1.3 image-form visual, v1.1 webhook ciphertext + AI-limit concurrency, v1.0 ClamAV EICAR.
4. Gotcha: never run `go fmt -w` / `gofmt -w` across the tree — it churns Windows CRLF into LF across ~80 files. Use `gofmt -l` to check and `gofmt -w <specific file>` to fix.
5. Gotcha: do not run full core `go test ./...` before a release without reverting `x-log.json` (swagger_test.go side-effect); the release script uses focused/compile-only tests.
6. Toolchain gotcha (release): `export PATH` does not survive the `wsl.exe -- bash -c` boundary and bash skips the codex npm symlink; run the release from a script file with a `/tmp/relbin` shim (node/npm/go) piped `sed 's/\r$//' | bash`. See `scratchpad/run-release-v15.sh`.
7. Toolchain note (WAF dev): Go lives at `/home/lainy/codex-go1.26.1/go` (persistent — `/tmp` is tmpfs and is wiped on WSL restart); `scratchpad/run-waf-tests.sh` runs gofmt + vet + test across the gateway, `wafconfig`, and the WAF service in one pass.
8. Frontend note: the repo-wide `type-check` has 13 pre-existing upstream errors unrelated to this project; verify changed code and report the baseline honestly rather than attributing it here. `node_modules/.bin` shims can be missing — invoke `node ./node_modules/vite/bin/vite.js` / `node ./node_modules/eslint/bin/eslint.js` directly instead of reinstalling.

## Session Continuity

Last session: 2026-07-26
Stopped at: WAF Phase 21–23 complete and pushed (through `29e227248`); worktree clean and synced with `origin/main`. User supplied 10 Pro WAF screenshots and directed continued WAF feature work with human UAT explicitly deferred. Observed Pro surface captured in `.planning/research/WAF-PRO-PARITY.md`; a 6-area code recon + gap matrix + phase plan is in flight. Nothing is released yet for v1.6 or v1.7.
Resume file: None
