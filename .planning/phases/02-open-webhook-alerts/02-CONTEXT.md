# Phase 2: Open Webhook Alerts - Context

**Gathered:** 2026-07-10
**Status:** Locked after implementation commit; real-provider acceptance remains open

<domain>
## Phase Boundary

This phase opens WeCom, DingTalk, and Feishu/Lark robot webhook delivery to community builds using public HTTPS protocols. It covers configuration, UI availability, payload construction, transport hardening, platform response validation, alert-log accounting, and response masking. SMS remains license-restricted. This phase does not emulate a commercial license and does not add a proprietary notification service.

</domain>

<decisions>
## Implementation Decisions

### Provider protocol
- **D-01:** Implement the documented text payload for each provider in a standalone `webhook_alert` package.
- **D-02:** Treat both HTTP status and platform business codes as delivery results. Support Feishu `code` and legacy `StatusCode` responses.

### Network security
- **D-03:** Accept HTTPS only, port 443 only, no URL userinfo, no IP literal, and only official provider hosts: `qyapi.weixin.qq.com`, `oapi.dingtalk.com`, `open.feishu.cn`, and `open.larksuite.com`.
- **D-04:** Clone the supplied HTTP transport, force certificate verification, require at least TLS 1.2, clear caller-controlled SNI, refuse redirects, use a 10-second timeout, and cap response bodies at 64 KiB.
- **D-05:** Delivery errors name the platform and result but never include the complete webhook URL or token.

### Alert integration
- **D-06:** Community restrictions continue to apply to SMS only; WeCom, DingTalk, and Feishu configs and methods are accepted without setting Pro state.
- **D-07:** Save an AlertLog for every delivery attempt. A failed delivery is persisted as `AlertError`, so quota accounting treats it as an attempted send rather than retrying forever in the same cycle.
- **D-08:** Mask webhook URLs as `********` in list/page responses and restore the stored URL when an edit submits the mask unchanged.

### Secret storage boundary
- **D-09:** API responses and delivery errors are redacted, but the existing `AlertConfig.Config` database field stores the webhook JSON, including its URL secret, in plaintext at rest. Database encryption is explicitly deferred and must not be described as complete.

### Acceptance boundary
- **D-10:** Automated tests and builds are complete. Actual sends to disposable WeCom, DingTalk, and Feishu/Lark robots from a VPS have not been executed.

### Agent Discretion
- Provider message formatting may evolve as long as payload contracts, host restrictions, redaction, and attempt logging remain stable.

</decisions>

<specifics>
## Specific Ideas

- International builds must be able to use the three webhook types; only SMS stays restricted.
- No redirect may move a secret-bearing URL to a different destination.
- A settings read must never return the stored webhook token after creation.

</specifics>

<canonical_refs>
## Canonical References

### Product and acceptance
- `.planning/PROJECT.md` - Clean-room and security constraints.
- `.planning/REQUIREMENTS.md` - ALERT-01 acceptance criteria.
- `.planning/ROADMAP.md` - Phase 2 goal and success criteria.

### Implementation record
- Commit `47e0887ee61d75f8f6b17c12b9a0bec90a6e37e8` - Actual Phase 2 implementation and tests.
- `agent/utils/webhook_alert/sender.go` - Payload, URL, TLS, timeout, redirect, and response policy.
- `agent/app/service/alert.go` - Community availability, configuration validation, and API masking.
- `agent/utils/xpack/helper/alert.go` - Delivery and success/error AlertLog creation.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- Existing AlertConfig and AlertLog repositories provide persistence and quota-count inputs.
- Existing alert sender/helper flow already resolves configured methods and rendered message content.
- Existing settings pages already model provider configurations and method selection.

### Established Patterns
- Alert provider behavior is injected through `xpack.AlertProvider`; the GPL helper now supplies real webhook behavior.
- Alert configuration is JSON stored in `AlertConfig.Config`.
- UI access restrictions are computed per provider type.

### Integration Points
- `agent/app/service/alert.go` validates and masks configuration.
- `agent/utils/xpack/helper/alert.go` sends and records attempts.
- The three changed Vue settings files expose providers and remove non-SMS license restrictions.

</code_context>

<deferred>
## Deferred Ideas

- Encrypt webhook URLs at rest or move them into a dedicated secret store.
- Execute real disposable-robot tests for all supported providers from the VPS release environment.
- Any provider-specific rich-card or signature-secret mode beyond the current robot URL text payload.

</deferred>

---
*Phase: 02-open-webhook-alerts*
*Context locked: 2026-07-10*
