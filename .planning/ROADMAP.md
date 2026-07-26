# Roadmap: 1Panel-X

## Overview

Milestone v1.0, Open Enhancement First Release, establishes a narrow but usable clean-room enhancement path on top of the official GPL community code. Four independent feature phases deliver open theme and watermark settings, three robot webhook providers, durable ClamAV scheduling, and a license-independent AI Agent soft limit. A final release phase verifies the combined source, produces native Linux artifacts, and records a safe VPS handoff. No future commercial-equivalent domain is part of this milestone.

## Execution Model

- Phases 1 through 4 are independent feature and security domains and may be implemented or reviewed in parallel.
- A phase remains incomplete until its requirement acceptance criteria, tests, relevant manual checks, and scoped commit are complete.
- Phase 5 begins its release gate only after Phases 1 through 4 are accepted.
- All Go tests and release builds run in Linux or WSL. Native Windows Go results are not an acceptable substitute for Linux host behavior.
- Every implementation is clean-room and GPL-3.0-compatible. License emulation, activation bypass, proprietary source use, and false Pro status are prohibited.

## Current Milestone: v1.0 Open Enhancement First Release

- [x] **Phase 1: Open Theme and Watermark** - Implementation and automated verification complete; browser UAT persists separately.
- [x] **Phase 2: Open Webhook Alerts** - Implementation and automated verification complete; real-provider UAT persists separately.
- [x] **Phase 3: Scheduled ClamAV** - Implementation and automated verification complete; VPS/EICAR UAT persists separately.
- [x] **Phase 4: AI Agent Soft Limit** - Implementation and automated verification complete; live capacity UAT persists separately.
- [x] **Phase 5: Reproducible Release and VPS Handoff** - Reproducible native release candidate built and inspected; deployment/rollback UAT persists separately.

## Phase Details

### Phase 1: Open Theme and Watermark

**Goal:** A community administrator can configure theme colors and a login-protected watermark through open settings APIs and the existing UI.
**Depends on:** Nothing
**Requirements:** [THEME-01]
**Success Criteria** (what must be true):

1. The administrator can save and reload custom or preset theme colors plus watermark settings without a commercial license check.
2. Light, dark, and system themes survive refresh, while toggling the watermark preserves the active routed application state.
3. Public settings omit watermark text and invalid stored values fall back safely; authenticated settings return the validated complete configuration.
4. Focused core tests, the production frontend build, and browser checks for login, refresh, theme modes, and watermark rendering pass.

**Plans:** TBD after the existing uncommitted implementation is reviewed and split into a phase plan.

### Phase 2: Open Webhook Alerts

**Goal:** Existing alert rules can deliver through WeCom, DingTalk, and Feishu robots using secure public protocols and auditable results.
**Depends on:** Nothing; may be reviewed in parallel with Phase 1
**Requirements:** [ALERT-01]
**Success Criteria** (what must be true):

1. The administrator can configure all three providers, and each provider sends its documented text payload and validates both HTTP and business response codes.
2. Every success or failure creates an appropriate alert log, and a failed send is counted as an attempt rather than retried forever in the same cycle.
3. The sender permits only HTTPS official robot hosts, verifies TLS, refuses redirects, and enforces bounded timeout and response size.
4. Webhook secrets do not appear in delivery errors or alert logs, and the settings flow uses protected or masked secret handling.
5. Provider, transport, host, response, retry-accounting, and redaction tests pass, with a disposable-robot VPS procedure ready for release validation.

**Plans:** TBD after the current transport and alert-helper changes pass security review.

### Phase 3: Scheduled ClamAV

**Goal:** ClamAV rules can run on durable schedules without overlapping scans or unsafe path and deletion behavior.
**Depends on:** Nothing; may be reviewed in parallel with Phases 1 and 2
**Requirements:** [CLAM-01]
**Success Criteria** (what must be true):

1. Create, update, enable, disable, and delete operations keep database rows and cron entries consistent, including failure paths.
2. Agent restart restores every enabled non-empty schedule and stores the new in-memory entry ID.
3. Manual and scheduled runs of the same rule cannot overlap, including under a high-frequency schedule.
4. Scan, isolation, and destructive target paths are normalized and constrained; isolation is outside the scan tree and uses restrictive permissions.
5. Focused scheduling and safety tests pass, followed by an EICAR test confined to a disposable VPS directory with no production `remove` target.

**Plans:** TBD after lifecycle, concurrency, and path-safety review of the current draft.

### Phase 4: AI Agent Soft Limit

**Goal:** AI Agent count is governed by host capacity and an optional operator setting rather than product license state.
**Depends on:** Nothing; may be reviewed in parallel with Phases 1 through 3
**Requirements:** [AGENT-01]
**Success Criteria** (what must be true):

1. Missing or zero `AIAgentLimit` permits creation beyond five without changing product license state.
2. A validated positive value from 1 through 1000 blocks creation at the configured count with a clear error.
3. Existing name, port, application, and lifecycle validation remains active, and App Store metadata does not impose a hidden second count limit.
4. Focused limit tests pass and release guidance explains capacity limits and the non-atomic nature of count-then-create enforcement.

**Plans:** TBD after the existing limit test and application metadata review are complete.

### Phase 5: Reproducible Release and VPS Handoff

**Goal:** An operator can reproduce, inspect, install, smoke-test, and roll back the v1.0 build from a clearly identified source revision.
**Depends on:** Phases 1, 2, 3, and 4
**Requirements:** [RELEASE-01]
**Success Criteria** (what must be true):

1. `npm ci`, the production frontend build, focused tests, package compile checks, and Linux AMD64 core and agent builds pass from the documented toolchains.
2. The external `image` directory contains native release artifacts, checksums, build reproduction material, source revision metadata, and complete VPS instructions.
3. VPS documentation covers prerequisites, backup, install or binary replacement, startup, rollback, smoke testing, and isolated security-sensitive acceptance checks.
4. Source changes are separated into feature and release commits, and every new commit has the required author and committer identity.
5. A timestamped external roadmap note with a Chinese change summary records delivered behavior, tests, artifacts, limitations, and deferred VPS evidence.

**Plans:** 1 plan complete. Release candidate `v1.0.0-open.1` was built from `cc5d31aa76a4d166f287a98b5d92b1f63c67af3d` with all automated gates passed.

## Progress

All five implementation plans and automated gates are complete. The milestone remains a release candidate rather than an accepted release because 20 persisted browser, provider, VPS, EICAR, capacity, and rollback UAT items remain pending.

| Phase | Requirement | Plans Complete | Status | Completed |
|-------|-------------|----------------|--------|-----------|
| 1. Open Theme and Watermark | THEME-01 | 1/1 | Human UAT pending | 2026-07-10 |
| 2. Open Webhook Alerts | ALERT-01 | 1/1 | Human UAT pending | 2026-07-10 |
| 3. Scheduled ClamAV | CLAM-01 | 1/1 | Human UAT pending | 2026-07-10 |
| 4. AI Agent Soft Limit | AGENT-01 | 1/1 | Human UAT pending | 2026-07-10 |
| 5. Reproducible Release and VPS Handoff | RELEASE-01 | 1/1 | Human UAT pending | 2026-07-10 |

## Prior Milestone: v1.1 Open Enhancement Hardening (shipped v1.1.0-open.1)

Chosen from a source-verified 53-capability inventory (`.planning/research/CAPABILITY-MATRIX.md`) that found **no** commercial capability which is merely a UI/license gate over a complete OSS backend (`uiGateOnly = ∅`). Rather than build new attack surface on top of two disclosed debts, v1.1 pays them down first, as two independent single-node phases that are fully CI-testable without a VPS. Decision record: `.planning/v1.1-MILESTONE-DECISION.md`.

- [x] **Phase 6: Webhook Secret At-Rest Encryption** - Implemented and automatically verified; real-robot / DB-inspection UAT persists separately. [ALERT-SEC-01]
- [x] **Phase 7: Atomic AI Agent Limit + UI** - Implemented and automatically verified; live-capacity / browser UAT persists separately. [AGENT-02]

Release: `v1.1.0-open.1` built from `21d19773c` (dual-layer checksums verified, `dirty=false`, VPS acceptance `not_run`). Milestone audit: `gaps_found` — 6 human UAT items pending, not archived.

### Phase 6: Webhook Secret At-Rest Encryption

**Goal:** No plaintext webhook bot secret survives in the alert database or its backups, with zero behavior regression.
**Depends on:** v1.0 Phase 2 (Open Webhook Alerts), already implemented.
**Requirements:** [ALERT-SEC-01]
**Success Criteria:**

1. Webhook `url` is stored ciphertext (sentinel-prefixed, AES via the existing `EncryptKey`); the plaintext never appears in the row.
2. A transparent migration encrypts existing plaintext rows once and is safe at zero rows / fresh key.
3. Delivery decrypts before sending; API still masks to `********`; masked-edit preserves the stored secret; allowlist/TLS/redirect/timeout/retry behavior unchanged.
4. Focused Go tests (roundtrip, idempotency, legacy passthrough, mask-over-ciphertext, edit-preserve, one-shot migration) pass; real-robot delivery + on-disk ciphertext inspection persist as human UAT.

**Plans:** 06-01 defined.

### Phase 7: Atomic AI Agent Limit + Management UI

**Goal:** `AIAgentLimit` cannot be exceeded by concurrent creation, and operators can manage it from the UI.
**Depends on:** v1.0 Phase 4 (AI Agent Soft Limit), already implemented.
**Requirements:** [AGENT-02]
**Success Criteria:**

1. A positive limit N is never exceeded under concurrency; failed installs release their slot; in-flight returns to zero.
2. Missing/zero remains unlimited; 1..1000 validation and semantics unchanged.
3. The reservation holds no lock across the install and reads the committed count under its guard.
4. A UI control (0 = unlimited) reads/writes `AIAgentLimit` via the existing setting endpoint; concurrency test passes; `npm run build:pro` and ESLint pass; live/resource checks persist as human UAT.

**Plans:** 07-01 defined.

## Prior Milestone: v1.2 Open Branding Text & Login Colors (shipped v1.2.0-open.1)

The safe, fully CI-verifiable slice of branding — text and login colors, **no file upload** — wiring already-declared, already-frontend-consumed enhancement fields and adding the missing community form. Chosen after a source-verified design + adversarial threat model (`.planning/v1.2-MILESTONE-DECISION.md`, `.planning/research/BRANDING-DESIGN.md`).

- [x] **Phase 8: Branding Backend Wiring** - implemented and automatically verified; browser/API UAT persists separately. [BRAND-01]
- [x] **Phase 9: Community Branding Form** - implemented and automatically verified; browser UAT persists separately. [BRAND-02]

Release: `v1.2.0-open.1` built from `8f9e18fe4` (dual-layer checksums verified, `dirty=false`, VPS `not_run`; agent binary byte-identical to v1.1). Milestone audit: `gaps_found` — 4 human UAT items pending, not archived.

## Prior Milestone: v1.3 Open Image Branding Upload (shipped v1.3.0-open.1)

The high-risk item deferred from v1.2: uploading branding images (logo, logo-with-text, favicon, login image, login background image), built entirely under the captured threat model. First from-scratch multipart file-write surface, so serve-side SVG hardening shipped in the same change. **BRAND-IMG-01 verified live on a VPS 2026-07-11** (Docker-free); BRAND-IMG-02 form/API verified live, login-hero render gap fixed in v1.4. Decision: `.planning/v1.3-MILESTONE-DECISION.md`; audit: `.planning/v1.3-MILESTONE-AUDIT.md`.

- [x] **Phase 10: Branding Image Upload (backend + serve hardening)** - verified live (serve allowlist + nosniff, SVG/pixel-bomb/size/format rejection, fixed-enum atomic write, presence sentinels, CSRF). [BRAND-IMG-01]
- [x] **Phase 11: Community Branding Image Form** - form/API verified live; login-hero render fixed in v1.4. [BRAND-IMG-02]

## Current Milestone: v1.4 Open Login-Page Text

The last deferred branding slice (P2): operator-authored login-page welcome/subtitle/copyright text, wired through the enhancement seam and rendered on the community login page as interpolation (never `v-html`). Bundles the `LOGIN-HERO-RENDER` fix found in the v1.3 live UAT. Decision: `.planning/v1.4-MILESTONE-DECISION.md`.

- [x] **Phase 12: Open Login-Page Text (+ login-hero fix)** - implemented and automatically verified (server-side reject-`<>`/control-chars + rune caps T7, strict anon subset T8, interpolation render, community form; reactive login-image/background preload fix); adversarial security review recorded; live login render persists as human UAT. [LOGIN-TEXT-01]

### Phase 12: Open Login-Page Text (+ login-hero fix)

**Goal:** A community build sets login welcome/subtitle/copyright text (rendered safely), and an uploaded login image/background actually displays on the login page.
**Depends on:** v1.2 (enhancement text seam) + v1.3 (login image upload), implemented.
**Requirements:** [LOGIN-TEXT-01]
**Success Criteria:**

1. `LoginWelcome`/`LoginSubtitle` (≤128) and `Copyright` (≤200) writable via update + readable via authed/anon getters; fail-closed default preserved.
2. Server-side reject `<>`/control chars + rune caps (T7); login page renders via interpolation, never `v-html`.
3. Anon endpoint stays a strict subset (cosmetic text only; no watermark/paths/bytes), proven by the subset test (T8).
4. `login/index.vue` preloads loginImage/loginBackground reactively so uploads display (fixes `LOGIN-HERO-RENDER`).
5. Focused + full Go tests pass; ESLint + `build:pro` pass; adversarial review recorded; live login render is human UAT.

**Plans:** 12-01 defined.

### Phase 10: Branding Image Upload (backend + serve hardening)

**Goal:** A community build uploads/validates/stores/serves branding images safely, with the anonymous serve route hardened against stored-XSS.
**Depends on:** v1.2 (branding text/colors, enhancement seam), already implemented.
**Requirements:** [BRAND-IMG-01]
**Success Criteria:**

1. Serve route: fixed-enum only, raster-allowlist Content-Type, `nosniff`, no `<svg>` override (T1/T2).
2. Upload: PNG/JPEG/GIF/WEBP (favicon PNG-only, T10); SVG/XML/HTML rejected; `DecodeConfig` dimension cap before full decode (T5); `MaxBytesReader` + per-asset byte cap (T4).
3. Fixed-enum atomic write inside `uploads/theme` with prefix assert (T3/T6); reset removes exact file + clears sentinel.
4. Presence-sentinel storage; strict anon subset (T8); authed + global CSRF on write routes (T9); image keys absent from the text `oneof`.
5. Focused tests + full core regression pass; adversarial review recorded; browser/API is human UAT.

**Plans:** 10-01 defined.

### Phase 11: Community Branding Image Form

**Goal:** A community administrator uploads/resets branding images from the open settings form.
**Depends on:** Phase 10.
**Requirements:** [BRAND-IMG-02]
**Success Criteria:**

1. Upload + reset controls for the five images; login-background image control gated on bg-type = image.
2. Multipart via the open endpoint with automatic CSRF; client size/type hint, server authoritative.
3. Fixed served-path image srcs; no `v-html`; previews cache-bust after upload/reset.
4. `npm run build:pro` + changed-file ESLint pass; browser set/persist + pre-auth render are human UAT.

**Plans:** 11-01 defined.

## Current Milestone: v1.5 Secure Multi-Node (keystone)

The keystone the 15+ commercial capabilities depend on: authenticated node enrollment + mutual-TLS federation, clean-room. Grounded in a triangulated recon (4 probes) + a full threat model (N1–N15, `.planning/research/NODE-ENROLLMENT-DESIGN.md`) + an adversarial security review (10 agents, **0 confirmed defects**). Slice A (backend PKI+enrollment) + Slice B (community node UI) ship now; Slice C (cross-network live acceptance) is deferred — needs a second VPS. Decision: `.planning/v1.5-MILESTONE-DECISION.md`.

Key finding: the community source already ships a **hollow mTLS shell** (two-mode listener, `RequireAndVerifyClientCert`, encrypted cert settings, `Proxy-Id`, the `CurrentNode` interceptor + switcher) — hardwired to single-host master. v1.5 supplies the missing security pieces behind the existing build-tag seams; the single-host posture is never regressed (node mode is opt-in after enrollment).

- [x] **Phase 13: Node Registry + CA + Enrollment (backend)** — nodes registry + core-owned private CA + single-use HMAC enrollment token (N1 atomic burn / N2 HMAC / N3 TTL+scope / N4 master-fp embed) + token-gated CSR-signing enroll endpoint (N13 CN-imposed, rate-limited, audit-logged); SSRF addr validation (N14); revoke keeps the audit row (N10). Single-box loopback mTLS proof. [NODE-ENROLL-01]
- [x] **Phase 14: Core→Node mTLS Proxy + Agent Validation + Bootstrap (backend)** — real remote proxy (per-node fingerprint pin N5/N8, registry-only target N14, no downgrade N12, revoked/offline refused N10); agent `ValidateCertificate` pins the master (N6) + inbound `X-Panel-User` stripped (N7); `LoadNodeInfo` node-mode-only-if-provisioned, default master, nil-DB safe; node bootstrap (local keypair+CSR, key never transmitted N9). [NODE-PROXY-01]
- [x] **Phase 15: Community Node Management UI + Honest Re-gate (frontend)** — `/settings/nodes` page (add→enrollment token/list/revoke/delete), gated on `isAdmin` only — **no license state forged**; reuses the existing `CurrentNode` interceptor. [NODE-UI-01]

Release: `v1.5.0-open.1`. Milestone audit: `gaps_found` — cross-network live acceptance (Slice C) + browser UAT pending; not archived.

## Current Milestone: v1.6 Website Access Monitoring (code-complete, unreleased)

The CI-verifiable half of the monitoring/WAF domain: parse each site's nginx access log into durable per-hour statistics and rankings, then surface them as a dashboard. Design: `.planning/research/WEBSITE-MONITORING-DESIGN.md`.

- [x] **Phase 16: Access-log parser + aggregator (pure functions)** — nginx `main`-format parser + time-bucketed PV/UV/QPS/status-class aggregation + top-N ranking, with M1 control-character stripping and M2 bounded-DoS handling. 13/13 CI.
- [x] **Phase 17: Monitoring backend** — new `website_stat.db` (`WebsiteAccessStat` / `WebsiteAccessRank` / `WebsiteAccessCursor`); per-site offset-incremental `access.log` tail; **hold-back settlement of closed hourly buckets only, so UV is exact**; geo ranking reuses the existing `agent/utils/geo` (graceful degradation when the mmdb is absent); crash-self-healing idempotent writes; 30-day retention; 10-minute cron; `POST /websites/:id/monitor/stat|rank`.
- [x] **Phase 18: Monitoring dashboard (frontend)** — range 24h/7d/30d, PV / peak-UV / traffic cards, PV·UV and 2xx/4xx/5xx trend charts, URI/IP/Referer/region top-N tables. ESLint + `build:pro` clean.

Adversarial review done (2026-07-15). Release: **pending** — no artifact built yet.

## Current Milestone: v1.7 WAF-Proper, own engine (code-complete, unreleased)

The closed `1pwaf`/OpenResty Lua engine is **absent** from the community tree (its `createWafConfig`/`moveDefaultWafConfig`/`delWafConfig` helpers are filesystem shims that must never be surfaced as a working WAF — red-line W11). So the community WAF is a **real engine of our own**: OWASP Coraza v3 + CRS v4 (PL1) compiled into a loopback reverse-proxy sidecar (`coraza-gateway/`, a third Go module) placed behind OpenResty's TLS termination. Threat model W1–W12: `.planning/research/WAF-ENGINE-DESIGN.md`. Decision: `.planning/v1.7-MILESTONE-DECISION.md`.

- [x] **Phase 19: Engine** — compile-then-swap atomic reload (W9), detection/block modes, decode/normalize (W2/W4), recover block-vs-pass (W1), listener timeouts + body caps (W3), no-leak block page + fingerprint stripping (W7). 10/10 CI: SQLi/XSS/traversal→403, clean→200, detection passes+logs, bad ruleset keeps the old engine, oversize rejected, panic recovered.
- [x] **Phase 20: Attack-event store** — gateway emits a Coraza JSON audit log; agent `wafaudit` parser (ModSecurity brackets in `error_message`, primary attack rule over anomaly-eval, all attacker fields W6-sanitized); `waf.db` store + offset tailer + host→websiteID resolution + TxID-deduped idempotent ingest + retention prune + `POST /websites/:id/waf/events`.
- [x] **Phase 21: Packaging + config generation** — compose app asset (our own directory, never `1pwaf/data`); `agent/utils/wafconfig` deterministic routing-table generation with a SHA-256 `generation` hash; port-insensitive host normalization with explicit collision rejection (W12); multi-site Host dispatch with unknown-Host default-deny.
- [x] **Phase 22: Per-site nginx wiring** — real `root.conf` `proxy_pass` re-routing to the gateway, aligned `client_max_body_size`, generation-gated readiness probe, nginx check/reload with full rollback of config + policy + gateway on any failure, honest `protected` derived from gateway readiness **and** actual routing. **Verified live on the experiment VPS.**
- [x] **Phase 23: Site policy UI + IP access lists + two-tier spine** — per-site enable/mode/attack-log tab; per-site IP allow/deny lists (deny→403 in both modes evaluated before CRS; allow bypasses CRS yet is still proxied through the hardened reverse proxy and still body-capped; IPv4/IPv6 + CIDR; invalid entries fail startup; canonicalized/deduped/sorted, capped at 512); panel-wide `WafGlobalPolicy` defaults with an explicit `"inherit"` site sentinel and control-plane list merging (`MergeIPLists`), `GET/POST /websites/waf/global`.

Automated gates green across all three modules (gateway + `wafconfig` + `waf_control` Go tests, ESLint, `build:pro`). Release: **pending**. Live blocking, IP-list enforcement, and two-tier behavior in a browser remain **human UAT** — explicitly deferred by the user on 2026-07-26 and therefore still `pending`, never marked passed.

## Next Domain: Community WAF Pro-Parity Build-Out

The user supplied 10 screenshots of the commercial WAF on 2026-07-26; the observed **user-visible surface** (7 tabs: overview / attack reports / intercept log / ban log / black-white lists / site settings / global settings) is recorded in `.planning/research/WAF-PRO-PARITY.md`, together with honesty traps R1–R7. Candidate capabilities, none yet implemented: rate limiting (access/attack/404/URL) with temporary IP bans, URL / User-Agent / IP-group black-white lists, geo access restrictions, file-upload limits, a CDN real-IP header list, CRS rule-group toggles, custom condition→action rules, and a dedicated top-level WAF page. A gap matrix and phase plan are being produced from a 6-area code recon; nothing here may be described as implemented until it is.

## Future Milestone Themes

These themes are intentionally outside v1.0–v1.7. They have no current phase number and must not be described as implemented. Ordering reflects the dependency graph and risk ranking in `.planning/research/CAPABILITY-MATRIX.md`:

1. Website anti-tamper, operations reports, Skills Hub, and AI benchmark testing.
2. Custom repositories, proxy enhancements, model downloads, and vLLM management.
3. Multi-node synchronization, overview, and RBAC (enrollment + mTLS federation already shipped in v1.5).
4. Database high availability, a complete AI gateway, virtual machines, mobile clients, local AI site building, and independent SMS delivery.

---
*Roadmap created: 2026-07-10 · last updated: 2026-07-26*
*Current milestones: v1.6 Website Access Monitoring and v1.7 WAF-Proper are both code-complete and CI-green but **unreleased**. Next domain is the community WAF Pro-parity build-out. v1.0–v1.5 shipped; v1.5 Slice C (cross-network acceptance) and all accumulated browser/VPS UAT debt persist.*
