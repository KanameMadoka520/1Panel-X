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

## Future Milestone Themes

These themes are intentionally outside v1.0. They have no current phase number and must not be described as implemented:

1. Advanced WAF and website request monitoring.
2. Secure multi-node enrollment, synchronization, overview, and RBAC.
3. Website anti-tamper, operations reports, Skills Hub, and AI benchmark testing.
4. Custom repositories, proxy enhancements, model downloads, and vLLM management.
5. Database high availability, a complete AI gateway, virtual machines, mobile clients, local AI site building, and independent SMS delivery.

---
*Roadmap created: 2026-07-10*
*Current milestone: v1.0 Open Enhancement First Release*
