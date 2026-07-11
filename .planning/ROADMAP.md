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

## Current Milestone: v1.3 Open Image Branding Upload

The high-risk item deferred from v1.2: uploading branding images (logo, logo-with-text, favicon, login image, login background image), built entirely under the captured threat model. This is the project's first from-scratch multipart file-write surface, so serve-side SVG hardening ships in the same change. Decision: `.planning/v1.3-MILESTONE-DECISION.md`.

- [x] **Phase 10: Branding Image Upload (backend + serve hardening)** - implemented and automatically verified (serve allowlist + nosniff, SVG/pixel-bomb/size/format rejection, fixed-enum atomic write, presence sentinels, CSRF via global guard); adversarial security review recorded; browser/API UAT persists separately. [BRAND-IMG-01]
- [x] **Phase 11: Community Branding Image Form** - implemented and automatically verified (upload + reset controls, multipart with auto-CSRF, no `v-html`); browser UAT persists separately. [BRAND-IMG-02]

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

## Future Milestone Themes

These themes are intentionally outside v1.0–v1.3. They have no current phase number and must not be described as implemented. Ordering reflects the dependency graph and risk ranking in `.planning/research/CAPABILITY-MATRIX.md`:

1. **Login welcome/subtitle/copyright text (P2)** - the remaining branding slice: new login-page text fields rendered as interpolation (never `v-html`), server-side reject-`<>`. A small phase, foldable after v1.3 accepts.
2. **v1.4 Secure multi-node** - the keystone 15+ capabilities depend on; needs a full threat model (identity, enrollment token, mutual mTLS, rotation, replay, audit, failure consistency) and a second VPS to accept.

1. Advanced WAF and website request monitoring.
2. Secure multi-node enrollment, synchronization, overview, and RBAC.
3. Website anti-tamper, operations reports, Skills Hub, and AI benchmark testing.
4. Custom repositories, proxy enhancements, model downloads, and vLLM management.
5. Database high availability, a complete AI gateway, virtual machines, mobile clients, local AI site building, and independent SMS delivery.

---
*Roadmap created: 2026-07-10*
*Current milestone: v1.1 Open Enhancement Hardening (v1.0 remains release-candidate with 20 UAT pending)*
