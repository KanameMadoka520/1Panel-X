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

## Future Requirements

The following areas are acknowledged but are not committed to v1.0 and do not map to current phases:

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

**Coverage:**

- v1.0 requirements: 5 total
- Mapped to exactly one phase: 5
- Unmapped: 0
- Mapped more than once: 0

---
*Requirements defined: 2026-07-10*
*Last updated: 2026-07-10 after v1.0.0-open.1 release-candidate build and artifact verification*
