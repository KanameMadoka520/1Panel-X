# Requirements: 1Panel-X

**Defined:** 2026-07-10
**Milestone:** v1.0 Open Enhancement First Release
**Core Value:** Deliver a complete, security-conscious, fully open server panel whose enhanced capabilities can be built, inspected, deployed, and maintained without proprietary code or license bypasses.
**Status:** Release candidate built and automatically verified. No v1.0 requirement is complete until its required browser, external-provider, or VPS acceptance evidence exists.

## Completion Rule

A v1.0 requirement is complete only when all of the following are true:

1. The user-visible behavior is implemented with original GPL-compatible code.
2. Focused automated tests and the relevant Linux build checks pass.
3. Security-sensitive behavior has explicit negative-path coverage.
4. Required browser or VPS acceptance checks are recorded.
5. The change is committed as a scoped commit with the required author and committer identity.

## v1.0 Requirements

### Open Theme and Watermark

- [ ] **THEME-01**: A community build can configure theme colors and an authenticated panel watermark without requiring or emulating a commercial license.

Acceptance criteria:

1. An administrator can save theme mode, custom color or preset color, and watermark settings from the existing settings UI.
2. Theme settings survive refresh and work in light, dark, and system modes; a watermark can be enabled or disabled without replacing the routed application tree.
3. The unauthenticated settings response exposes only the safe theme subset and never exposes watermark text; authenticated users can load the complete settings.
4. Update input and previously persisted values are validated, with corrupt values falling back to safe defaults.
5. Focused core tests, the production frontend build, and browser checks for login, refresh, theme switching, and watermark rendering pass.

### Open Webhook Alerts

- [ ] **ALERT-01**: An administrator can deliver alerts through WeCom, DingTalk, and Feishu robot webhooks without a commercial license.

Acceptance criteria:

1. All three webhook types can be configured through the existing alert settings workflow and generate their documented platform payloads.
2. HTTP status and platform business response codes determine success, and every delivery attempt records a success or error alert log.
3. Failed delivery is counted as an attempted delivery so the same alert is not retried forever in one monitoring cycle.
4. Delivery permits HTTPS official robot hosts only, verifies TLS, refuses redirects, bounds request time and response size, and does not expose complete webhook secrets in errors or logs.
5. Payload, host validation, TLS, redirect, response, retry-accounting, and redaction tests pass; a disposable-robot VPS test procedure is documented.

### Scheduled ClamAV

- [ ] **CLAM-01**: An administrator can create, update, enable, disable, and remove recurring ClamAV scan rules that survive agent restarts.

Acceptance criteria:

1. A valid rule is persisted before its cron callback is registered, and an invalid rule leaves neither a database row nor an active schedule.
2. Updating a rule validates and registers the replacement before removing the old schedule; agent startup restores every enabled rule with a non-empty schedule and refreshes its in-memory entry ID.
3. Manual and scheduled runs of the same rule cannot overlap, including when a high-frequency cron expression fires during an active scan.
4. Names, scan paths, isolation paths, and destructive targets are normalized and constrained; the isolation directory is outside the scan tree, uses restrictive permissions, and cannot resolve to a root or escaped path.
5. Scheduling, restart recovery, concurrency, and path-safety tests pass, followed by an isolated-directory EICAR VPS test that never targets a real website or production path.

### AI Agent Soft Limit

- [ ] **AGENT-01**: AI Agent creation has no license-derived hard count limit and supports an optional operator-defined soft limit.

Acceptance criteria:

1. A missing or zero `AIAgentLimit` setting means unlimited count and permits creation beyond the former limit of five, subject to normal host resources and lifecycle validation.
2. A positive limit from 1 through 1000 is validated and blocks new creation at the configured count with a clear error.
3. The implementation does not change product license state, and existing name, port, application, and lifecycle checks remain active.
4. Focused limit tests pass, App Store metadata is checked for a second count limit, and VPS guidance states that unlimited count does not imply unlimited CPU, memory, disk, ports, or Docker capacity.

### Reproducible Release and VPS Handoff

- [ ] **RELEASE-01**: The v1.0 source revision can be built, verified, packaged, and tested on a Linux VPS using artifacts and instructions outside the source tree.

Acceptance criteria:

1. A clean frontend production build and Linux AMD64 core and agent builds are reproducible with the documented Node and Go toolchains.
2. `D:\_CodeNotSync\_1Panel-X\image` contains the release bundle, native binaries, checksums, reproducible build instructions or scripts, source revision metadata, and `README-VPS.md`.
3. VPS instructions cover prerequisites, backup, binary replacement or installation, startup, rollback, smoke tests, and safe feature-specific acceptance checks.
4. Each material update has a separate Markdown record under `D:\_CodeNotSync\_1Panel-X\roadmap` whose filename contains a timestamp and a Chinese change summary.
5. Feature and release work is split into reviewable commits, and every new commit records `KanameMadoka520 <2441883200@qq.com>` as both author and committer.

## v1.1 Requirements (current milestone: Open Enhancement Hardening)

**Note:** The five v1.0 requirements above remain **Human UAT Pending** (not regressed, not accepted). v1.1 hardens the shipped v1.0 features; it does not supersede those requirements. Rationale and evidence: `.planning/v1.1-MILESTONE-DECISION.md`, `.planning/research/CAPABILITY-MATRIX.md`.

### Webhook Secret At-Rest Encryption

- [ ] **ALERT-SEC-01**: A community build stores webhook bot URLs encrypted at rest, with no plaintext secret in the alert database or its backups, and no behavior regression for delivery, masking, or validation.

Acceptance criteria:

1. Saving a WeCom/DingTalk/Feishu config persists the `url` as ciphertext (sentinel-prefixed, AES via the panel's existing `EncryptKey`); the plaintext URL never appears in the stored row.
2. A transparent migration encrypts existing plaintext webhook rows exactly once and is safe with zero rows and a freshly seeded key.
3. Delivery still succeeds (the sender decrypts before sending); API list/detail still returns `********`; editing with a masked URL preserves the stored secret; the official-host allowlist, TLS, redirect, timeout, response-bound, and retry-accounting behavior are unchanged.
4. The secret never appears in alert logs, API responses, or error messages; legacy plaintext rows still deliver until migrated.
5. Focused Go tests cover encrypt roundtrip, prefix idempotency, legacy passthrough, mask-over-ciphertext, edit-preserve, and one-shot migration; a disposable-robot delivery plus an on-disk ciphertext inspection are recorded as human UAT.

### Atomic AI Agent Limit and Management UI

- [ ] **AGENT-02**: `AIAgentLimit` enforcement is race-free under concurrent creation, and operators can view and set it from a dedicated UI control.

Acceptance criteria:

1. With a positive limit N, concurrent creation can never commit more than N agents; a failed install releases its reserved slot; the in-flight counter returns to zero at rest.
2. Missing or zero limit remains unlimited with no reservation overhead; the 1..1000 validation and the unlimited/soft-cap semantics from AGENT-01 are unchanged.
3. The reservation holds no lock across the container install; it reads the committed count under its guard so prior commits are always reflected.
4. A UI control (0 = unlimited) reads and writes `AIAgentLimit` through the existing validated setting endpoint, with a hint that unlimited count is not unlimited host resources.
5. A concurrency focused test proves the cap holds; `npm run build:pro` and changed-file ESLint pass; live multi-request and resource observation are recorded as human UAT.

## v1.2 Requirements (current milestone: Open Branding Text & Login Colors)

**Note:** v1.0's 20 and v1.1's 6 human UAT items remain pending. v1.2 delivers the safe text/color slice of branding; image upload is deferred to v1.3. Rationale: `.planning/v1.2-MILESTONE-DECISION.md`, threat model `.planning/research/BRANDING-DESIGN.md`.

### Branding Backend Wiring

- [ ] **BRAND-01**: A community build can persist and serve brand text and login colors through the enhancement seam, with server-side XSS controls and a strict anonymous subset, and no image-upload surface.

Acceptance criteria:

1. `Title`, `MasterAlias`, `LoginBgType`, `LoginBackground`, `LoginBtnLinkColor` are writable via the existing update endpoint and readable via the authenticated getter; each unknown key still fails closed.
2. Text fields reject `<`/`>` and control characters and cap length; colors validate via the existing safe-CSS-color check; `LoginBgType` is a strict enum; empty means unset.
3. The anonymous endpoint exposes exactly the cosmetic subset (theme, themeColor, title, masterAlias, loginBgType, loginBackground, loginBtnLinkColor) and never watermark, image bytes/paths, versions, or secrets — proven by a subset-assertion test.
4. Corrupt stored values fall back to safe defaults; no license state is touched; no new route or file write is added.
5. Focused Go tests and full core regression pass.

### Community Branding Form

- [ ] **BRAND-02**: A community administrator can set the branding text and login colors from an open settings form; the login page renders them pre-authentication.

Acceptance criteria:

1. A community form loads current values (authenticated getter) and saves each field through the open update endpoint; backend validation errors are surfaced.
2. Text renders as textContent/interpolation, never `v-html`.
3. No image-upload control is present.
4. `npm run build:pro` and changed-file ESLint pass; browser rendering is recorded as human UAT.

## v1.3 Requirements (current milestone: Open Image Branding Upload)

**Note:** v1.0's 20, v1.1's 6, and v1.2's 4 human UAT items remain pending. v1.3 adds the high-risk image-upload surface deferred from v1.2, under the captured threat model. Rationale: `.planning/v1.3-MILESTONE-DECISION.md`, threat model `.planning/research/BRANDING-DESIGN.md`.

### Branding Image Upload (backend + serve hardening)

- [ ] **BRAND-IMG-01**: A community build can upload, validate, store, and serve branding images (logo, logo-with-text, favicon, login image, login background image) safely, with the anonymous image-serve route hardened against stored-XSS.

Acceptance criteria:

1. The image-serve route (`RegisterImages`) serves only the fixed asset enum, sets Content-Type from a raster allowlist (never `image/svg+xml`/HTML), always sends `X-Content-Type-Options: nosniff`, and no longer force-serves `<svg>` bodies as SVG (T1/T2).
2. Upload accepts only PNG/JPEG/GIF/WEBP raster images (favicon PNG-only, T10); rejects SVG/XML/HTML by magic-byte scan; bounds dimensions (≤16 MP) via `DecodeConfig` before full decode (T5); enforces per-asset byte caps (2 MB / 256 KB favicon) with `MaxBytesReader` before parse (T4).
3. Filenames are a fixed server-side enum (never the client filename), written atomically (temp+rename) inside `uploads/theme` with a prefix assertion (T3/T6); reset removes the exact file and clears the setting.
4. Settings store only a presence sentinel (never bytes or paths); the widened anonymous subset exposes only those cosmetic presence flags and still never watermark/paths/versions/secrets, proven by the subset test (T8). The write routes are behind SessionAuth + the global CSRF guard (T9); image keys are not settable through the text update endpoint.
5. Focused Go tests (serve allowlist, format/SVG/pixel-bomb/size/favicon rejection, atomic write + sentinel, reset, enum) and full core regression pass; an adversarial security review of the diff is recorded; browser/API acceptance is human UAT.

### Community Branding Image Form

- [ ] **BRAND-IMG-02**: A community administrator can upload and reset branding images from the open settings form; the login page and sidebar render them.

Acceptance criteria:

1. The settings form offers upload + reset for each of the five images, gated so the login-background image control appears only when the login background type is image.
2. Uploads use the open asset endpoint via multipart with automatic CSRF; a client-side size/type hint is shown but the server remains authoritative.
3. Image references are fixed served paths; no branding value is `v-html`'d; previews cache-bust after upload/reset.
4. `npm run build:pro` and changed-file ESLint pass; browser set/persist and pre-auth render are recorded as human UAT.

## v1.4 Requirements (current milestone: Open Login-Page Text)

**Note:** v1.0–v1.3 human UAT debt persists (v1.3 BRAND-IMG-01 verified live 2026-07-11). v1.4 ships the last deferred branding slice (P2 login text) plus the `LOGIN-HERO-RENDER` fix found in the v1.3 live UAT. Rationale: `.planning/v1.4-MILESTONE-DECISION.md`.

### Open Login-Page Text

- [ ] **LOGIN-TEXT-01**: A community build can set login-page welcome, subtitle, and copyright text, rendered safely on the login page; and an uploaded login image/background now actually displays on the community login page.

Acceptance criteria:

1. `LoginWelcome`, `LoginSubtitle` (≤128 runes) and `Copyright` (≤200 runes) are writable via the existing update endpoint and readable via the authenticated + anonymous getters; the fail-closed validator still rejects unknown keys.
2. Each rejects `<`/`>` and control characters and caps length (server-side XSS control, T7); the login page renders them as interpolation (`{{ }}`), never `v-html`.
3. The anonymous endpoint stays a strict subset of the authenticated DTO (the three text fields are cosmetic; no watermark/paths/bytes/versions), proven by the subset test.
4. `login/index.vue` preloads the uploaded loginImage/loginBackground reactively (watch `themeConfig`), so an uploaded login image/background displays on the community login page (fixes `LOGIN-HERO-RENDER`).
5. Focused Go tests + full core regression pass; changed-file ESLint + `npm run build:pro` pass; an adversarial security review of the diff is recorded; live login-page render is human UAT.

## v1.5 Requirements (current milestone: Secure Multi-Node — keystone)

**Note:** v1.0–v1.4 human UAT debt persists. v1.5 opens the keystone: authenticated node enrollment + mutual-TLS federation, as a clean-room re-implementation. Slice A (backend PKI+enrollment) + Slice B (community node UI) ship now; Slice C (cross-network live acceptance) is deferred — needs a second VPS. Rationale: `.planning/v1.5-MILESTONE-DECISION.md`, threat model `.planning/research/NODE-ENROLLMENT-DESIGN.md` (N1–N15).

### Node Registry + CA + Enrollment (backend)

- [ ] **NODE-ENROLL-01**: A community build can register remote nodes, mint single-use HMAC enrollment tokens, and sign per-node leaf certificates from a core-owned private CA, without any commercial license.

Acceptance criteria:

1. A `nodes` registry (model/migration/repo/service/authenticated CRUD API) is served under `/core/nodes/*`, including the read endpoints the community frontend already calls (`/core/nodes/list`, `/core/nodes/simple/all`).
2. Core lazily generates and persists (encrypted at rest) a private CA + a core client cert + an HMAC enrollment secret; it signs a node's CSR into a leaf whose CN is imposed by core, not taken from the CSR (N13).
3. Enrollment tokens are single-use (atomic burn, N1), HMAC-authenticated (N2, constant-time), nodeId-scoped + short-TTL (N3), and carry the master fingerprint the joining node pins (N4). The enroll endpoint is token-gated, sessionless, rate-limited, and audit-logged (N13).
4. Node address is validated as a bare host/IP (no scheme/creds/path/control chars, N14); a node can be revoked (row preserved for audit, N10).
5. Focused Go tests (crypto chain, CN imposition, token replay/forgery/expiry, revoke) + full core regression pass; cross-network live enrollment is human UAT (Slice C).

### Core→Node mTLS Proxy + Agent Validation + Bootstrap (backend)

- [ ] **NODE-PROXY-01**: Core proxies to enrolled nodes over mutual TLS with per-node fingerprint pinning; the agent pins the master and enters node mode only after enrollment, never regressing the single-host posture.

Acceptance criteria:

1. Core resolves the node from the registry (never client input, N14) and dials `https://addr:port` presenting its client cert, verifying `RootCAs=CA` AND the node's exact server-cert fingerprint (N5/N8); no `InsecureSkipVerify` and no plaintext fallback on the node path (N12); revoked/offline nodes are refused (N10).
2. The agent `ValidateCertificate` pins the master's client-cert fingerprint and fails closed (N6); an unauthenticated caller cannot assert `X-Panel-User` (N7 — core strips inbound, agent binds to the mTLS master).
3. The agent enters node mode ONLY when provisioned (`scope=node` + server/root certs); default stays single-host master, nil-DB safe at pre-DB init; a community install never binds the 0.0.0.0 mTLS listener.
4. The node bootstrap generates its keypair + CSR locally (private key never transmitted, N9), pins the master from the token, and persists certs+scope, writing the Proxy-Id file, flipping to node mode last.
5. Focused Go tests both modules (loopback mTLS pin match/mismatch, foreign-CA reject, provisioned-node decision, ApplyEnrollment) + an adversarial security review are recorded; live cross-network proxy + node-mode boot are human UAT (Slice C).

### Community Node Management UI + Honest Re-gate (frontend)

- [ ] **NODE-UI-01**: A community administrator can add, list, revoke, and remove nodes from an open settings page, gated by a genuine admin signal — never by forging license state.

Acceptance criteria:

1. A `/settings/nodes` page (static community route) lists nodes, adds a node (form → shows the single-use enrollment token, copyable, with expiry), revokes, and deletes.
2. The page is reachable via a settings sub-nav entry gated on `isAdmin` only; `isProductPro`/`isXpackOrEE`/license flags are untouched — no license state is forged.
3. Token/text render via interpolation, never `v-html`; API clients call the open `/core/nodes` endpoints; i18n zh + en.
4. `npm run build:pro` and changed-file ESLint pass; live browser add→token/list/revoke/delete is human UAT.

## Future Requirements

The following areas are acknowledged but are not committed to v1.0, v1.1, or v1.2 and do not map to current phases:

### Security and Monitoring

- Advanced WAF logs, trends, attack maps, blocking records, ACLs, region rules, and exports.
- Website traffic and request monitoring, rankings, device and source analysis, and historical exports.
- Website anti-tamper protection, exclusions, auditing, and recovery integration.

### Multi-Node and Access Control

- Authenticated node enrollment, identity rotation, health, grouping, upgrades, and resource overview.
- File, image, certificate, application dependency, and configuration synchronization.
- Users, roles, node scopes, view or manage permissions, and API allowlists.

### Operations and AI Platform

- Operations reports, security scoring, scheduled exports, Skills Hub, and AI benchmark testing.
- Custom application repositories, enhanced proxy management, model downloads, and vLLM management.
- A locally defined AI gateway with routing, content controls, usage metering, and auditable storage.

### High-Risk Independent Domains

- MySQL, PostgreSQL, and Redis high availability and failover.
- KVM or libvirt virtual machines, storage, networking, VNC, snapshots, and templates.
- Mobile or PWA clients, local AI site building, and independently contracted SMS delivery.

## Out of Scope

| Item | Reason |
|------|--------|
| License generation, activation bypass, forced Pro state, or binary patching | These actions do not create maintainable open functionality and introduce legal and security risk. |
| Copying, decompiling, or redistributing proprietary `xpack` or `enterprise` code | All enhancements must be clean-room implementations based on public GPL interfaces, public documentation, open protocols, and lawfully observable behavior. |
| Claiming official Pro licensing or branding this build as an official commercial release | 1Panel-X is an independent GPL-derived project and must not misrepresent vendor authorization. |
| Empty pages, fake APIs, mock data, or no-op handlers presented as feature parity | A capability is not complete without real behavior and verification evidence. |
| Advanced WAF, monitoring, multi-node, RBAC, anti-tamper, reports, database HA, AI gateway, or VM parity in v1.0 | Each is a separate security or operations domain reserved for future milestones. |
| Replacing the full official installer and upgrade service in v1.0 | The first release uses a documented native-binary build and VPS handoff while an independent installer is designed later. |
| Treating a privileged runtime container as normal isolation | The panel requires deep host integration; containers may reproduce builds but are not the default runtime security boundary. |

## Traceability

Each current requirement maps to exactly one phase.

| Requirement | Phase | Status |
|-------------|-------|--------|
| THEME-01 | Phase 1 | Human UAT Pending |
| ALERT-01 | Phase 2 | Human UAT Pending |
| CLAM-01 | Phase 3 | Human UAT Pending |
| AGENT-01 | Phase 4 | Human UAT Pending |
| RELEASE-01 | Phase 5 | Human UAT Pending |
| ALERT-SEC-01 | Phase 6 | Human UAT Pending (v1.1) |
| AGENT-02 | Phase 7 | Human UAT Pending (v1.1) |
| BRAND-01 | Phase 8 | Human UAT Pending (v1.2) |
| BRAND-02 | Phase 9 | Human UAT Pending (v1.2) |
| BRAND-IMG-01 | Phase 10 | Verified live (v1.3) |
| BRAND-IMG-02 | Phase 11 | Human UAT Pending (v1.3) |
| LOGIN-TEXT-01 | Phase 12 | Human UAT Pending (v1.4) |
| NODE-ENROLL-01 | Phase 13 | Human UAT Pending (v1.5) |
| NODE-PROXY-01 | Phase 14 | Human UAT Pending (v1.5) |
| NODE-UI-01 | Phase 15 | Human UAT Pending (v1.5) |

**Coverage:**

- v1.0: 5; v1.1: 2; v1.2: 2; v1.3: 2; v1.4: 1; v1.5: 3 total
- Mapped to exactly one phase: 15
- Unmapped: 0
- Mapped more than once: 0

---
*Requirements defined: 2026-07-10*
*Last updated: 2026-07-13 after defining milestone v1.5 (NODE-ENROLL-01/NODE-PROXY-01/NODE-UI-01) — secure multi-node keystone: backend PKI+enrollment+mTLS proxy (both modules) + community node UI; cross-network live acceptance deferred (Slice C, needs 2nd VPS)*
