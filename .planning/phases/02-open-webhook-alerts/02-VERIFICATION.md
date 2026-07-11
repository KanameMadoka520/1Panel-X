---
phase: 02-open-webhook-alerts
verified: 2026-07-10T20:46:26-04:00
status: human_needed
score: 5/6 must-haves verified
requirements:
  - ALERT-01
---

# Phase 2: Open Webhook Alerts Verification Report

**Phase Goal:** Existing alert rules can deliver through WeCom, DingTalk, and Feishu robots using secure public protocols and auditable results.
**Verified:** 2026-07-10T20:46:26-04:00
**Status:** human_needed

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Community and international builds can configure/select the three webhook providers | VERIFIED | Service restriction map retains SMS only; changed Vue conditions restrict SMS only. |
| 2 | Provider payloads and business responses are implemented | VERIFIED | Sender code and focused payload/response tests pass for all providers. |
| 3 | Delivery uses a hardened HTTPS transport | VERIFIED | Official-host allowlists, TLS verification, TLS 1.2 minimum, no redirects, timeout, and response limit are implemented and tested. |
| 4 | Every success/failure attempt is logged without a complete URL in the error | VERIFIED | `TestFailedWebhookDeliveryCountsAsAttempt` exercises the real helper/sender path, proves one AlertError plus one AlertTask is stored, suppresses the second same-cycle send, and asserts the secret is absent. |
| 5 | Settings responses do not return stored webhook secrets | VERIFIED | List/page masking and masked-edit restoration tests pass. Database storage is still plaintext at rest. |
| 6 | Real WeCom, DingTalk, and Feishu/Lark robots accept messages from a VPS | NEEDS HUMAN | No real-provider test evidence exists. |

**Score:** 5/6 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `agent/utils/webhook_alert/sender.go` | Hardened provider sender | VERIFIED | Substantive payload, URL, TLS, timeout, redirect, and response logic. |
| `agent/app/service/alert.go` | Community config validation and masking | VERIFIED | Opens webhook types, validates URLs, masks reads, restores masked updates. |
| `agent/utils/xpack/helper/alert.go` | Delivery and attempt logging | VERIFIED | Calls sender and saves success/error AlertLogs. |
| Phase 2 Vue files | Existing UI availability | VERIFIED | License/region restrictions now apply to SMS only. |

**Artifacts:** 4/4 verified

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| Alert config update | URL policy | `ValidateURL` | WIRED | Unsafe endpoints are rejected before persistence. |
| Alert helper | Provider sender | `webhook_alert.Send` | WIRED | Rendered alert content is delivered through hardened transport. |
| Delivery result | Alert quota/logging | `SaveAlertLog` | WIRED | Both success and failure create attempted-delivery records. |
| Settings API | Stored config | Mask/restore helpers | WIRED | Reads redact URLs; unchanged masked edits recover the existing URL. |

**Wiring:** 4/4 connections verified

## Requirements Coverage

| Requirement | Status | Blocking Issue |
|-------------|--------|----------------|
| ALERT-01: Open robot webhook alerts | NEEDS HUMAN | Disposable real-provider VPS delivery has not run. |

**Coverage:** 0/1 requirements fully accepted; implementation and automated verification are complete.

## Automated Verification Passed

- Linux/WSL `agent`: `go test ./...`
- Release focused gate: `go test ./utils/xpack/helper -count=1`
- Targeted ESLint for Phase 2 frontend changes
- `frontend`: `npm run build:pro`
- Commit author and committer identity verification

## Anti-Patterns and Residual Risk

- No license bypass, fake response, arbitrary-host webhook, insecure TLS flag, redirect following, or secret-bearing delivery error was found.
- **Known security debt:** `AlertConfig.Config` stores the webhook URL JSON in plaintext at rest. API masking does not protect a copied database.

## Human Verification Required

### 1. WeCom disposable robot
**Test:** Configure a disposable WeCom robot on the VPS, send a test alert, then induce a controlled invalid-key failure.
**Expected:** Valid send reaches the channel; invalid send creates one error AlertLog and does not expose the URL.
**Why human:** Requires a real tenant, outbound network, and provider response.

### 2. DingTalk disposable robot
**Test:** Repeat success and controlled failure with a disposable DingTalk robot.
**Expected:** Text payload is accepted and business-code failure is recorded once.
**Why human:** Provider policy and tenant behavior cannot be established by local fixtures.

### 3. Feishu or Lark disposable robot
**Test:** Test the appropriate regional official host with a disposable robot.
**Expected:** Modern or legacy success response is accepted; a provider failure becomes a redacted error log.
**Why human:** Requires live regional provider infrastructure.

### 4. Settings secret exposure audit
**Test:** Create, list, edit without changing URL, and inspect browser network responses and application logs.
**Expected:** The full URL is accepted only on write; subsequent reads show `********`; logs contain no complete URL.
**Why human:** End-to-end API/UI/log observation requires a running panel.

## Gaps Summary

No automated implementation gap was found. Real-provider acceptance is deferred. Plaintext database storage is disclosed technical debt and must not be represented as encrypted secret storage.

## Verification Metadata

**Verification approach:** Goal-backward review plus focused sender/config tests and full package/build gates.
**Must-haves source:** `02-01-PLAN.md` and ALERT-01.
**Automated checks:** 5 categories passed, 0 failed.
**Human checks required:** 4.
**Known residual risk:** External provider behavior and database at-rest exposure.

---
*Verified: 2026-07-10T20:46:26-04:00*
*Verifier: Codex retrospective phase audit*
