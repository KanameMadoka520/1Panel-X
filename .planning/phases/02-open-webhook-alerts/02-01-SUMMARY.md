---
phase: 02-open-webhook-alerts
plan: 01
subsystem: agent-alert-delivery
tags: [go, vue3, webhook, tls, wecom, dingtalk, feishu, redaction]
requires: []
provides:
  - Community WeCom, DingTalk, and Feishu robot alerts
  - HTTPS official-host allowlists and hardened TLS transport
  - Platform response validation and per-attempt AlertLogs
  - Masked webhook URLs in settings responses
affects: [phase-05-release, alerting, secret-storage]
tech-stack:
  added: []
  patterns: [provider allowlist, cloned secure transport, masked secret round-trip, attempted-delivery logging]
key-files:
  created:
    - agent/utils/webhook_alert/sender.go
    - agent/utils/webhook_alert/sender_test.go
    - agent/app/service/alert_webhook_config_test.go
  modified:
    - agent/app/service/alert.go
    - agent/utils/xpack/helper/alert.go
    - frontend/src/views/setting/alert/setting/index.vue
key-decisions:
  - "Restrict community SMS only; open the three standard robot webhook protocols."
  - "Allow only HTTPS official provider hosts and force verified TLS."
  - "Mask API responses while explicitly deferring database at-rest encryption."
patterns-established:
  - "Secret-bearing endpoint validation occurs before persistence and before every delivery."
  - "Every send attempt produces an AlertLog, including provider and transport failures."
requirements-completed: []
requirements-progressed: [ALERT-01]
duration: not-recorded
completed: 2026-07-10
---

# Phase 2: Open Webhook Alerts Summary

**Community alerts can send hardened text webhooks to WeCom, DingTalk, and Feishu/Lark with redacted API responses and auditable success/error attempts; real robots remain untested.**

## Performance

- **Duration:** Not recorded; retrospective artifact.
- **Completed:** 2026-07-10T20:34:14-04:00
- **Tasks:** 3 reconstructed tasks
- **Files modified:** 8

## Accomplishments

- Added provider-specific text payloads and business response validation.
- Enforced HTTPS, official hosts, verified TLS, no redirects, bounded timeout, and bounded response bodies.
- Opened the three webhook configuration/method types to community and international builds while retaining SMS restrictions.
- Added URL masking and masked-edit restoration for settings responses.
- Persisted success or error AlertLogs for delivery attempts, preventing unlimited same-cycle retries after failures.

## Task Commit

1. **Implement open webhook delivery, service integration, UI access, and tests** - `47e0887ee61d75f8f6b17c12b9a0bec90a6e37e8` (`feat`)

Author and committer: `KanameMadoka520 <2441883200@qq.com>`.

## Files Created/Modified

- `agent/app/service/alert.go` - Opens provider types, validates configs, masks responses, and restores masked edits.
- `agent/app/service/alert_webhook_config_test.go` - Config validation, masking, and restoration tests.
- `agent/utils/webhook_alert/sender.go` - Hardened provider transport and response handling.
- `agent/utils/webhook_alert/sender_test.go` - Payload, host, TLS, redirect, timeout, and response tests.
- `agent/utils/xpack/helper/alert.go` - Sends messages and records success/error AlertLogs.
- `frontend/src/views/setting/alert/dash/task/index.vue` - Makes webhook methods selectable without Pro state.
- `frontend/src/views/setting/alert/setting/drawer/index.vue` - Opens provider creation/editing while retaining SMS conditions.
- `frontend/src/views/setting/alert/setting/index.vue` - Opens provider rows and actions while retaining SMS conditions.

## Automated Verification

- Linux/WSL `agent`: `go test ./...` passed.
- Targeted ESLint for the three changed Vue files passed.
- `frontend`: `npm run build:pro` passed.
- Focused tests cover payload contracts, URL allowlists, unsafe targets, trusted/untrusted TLS, transport cloning, redirect refusal, HTTP status, timeouts, response schemas, response bounds, config masking, and masked updates.

## Decisions Made

- Used standard public robot protocols with a strict provider/host matrix.
- Cloned and hardened caller transports so unsafe TLS flags are not inherited or mutated in place.
- Stored errors without URLs and masked settings API responses.
- Kept the existing AlertConfig schema for compatibility, accepting a documented at-rest secret-storage limitation.

## Deviations from Plan

This formal plan was reconstructed after the feature commit. The implementation remained within the Phase 2 boundary.

## Issues Encountered

- Webhook robot URLs embed bearer-like secrets in their paths or query strings, requiring both endpoint allowlisting and careful error formatting.
- The existing database schema stores provider config as JSON. Phase 2 masks the API surface but does not encrypt the database value.

## Residual UAT and Technical Debt

- No disposable WeCom, DingTalk, or Feishu/Lark robot has received a message from the target VPS environment.
- Provider-side rate limits, tenant policies, and regional routing have not been observed live.
- Webhook URLs remain plaintext at rest in `AlertConfig.Config`; compromise of the alert database can expose them.
- ALERT-01 is progressed but not listed in `requirements-completed`.

## User Setup Required

Create disposable robots for the three providers and revoke them after Phase 5 acceptance. Never use production channels for first acceptance.

## Next Phase Readiness

- Automated protocol and integration checks are ready for release packaging.
- Phase 5 must execute one success and one controlled failure against disposable provider endpoints and record the resulting AlertLogs.

---
*Phase: 02-open-webhook-alerts*
*Implementation committed: 2026-07-10*
