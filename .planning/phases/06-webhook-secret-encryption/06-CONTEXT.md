# Phase 06: Webhook Secret At-Rest Encryption - Context

**Gathered:** 2026-07-10
**Milestone:** v1.1 Open Enhancement Hardening
**Requirement:** ALERT-SEC-01

<domain>
## Phase Boundary

This phase removes the last plaintext-secret exposure from the v1.0 open webhook alert feature: the bot webhook URL stored inside `AlertConfig.Config` (`{"displayName","url"}`) is currently persisted plaintext in the alert database and therefore in database backups. This phase encrypts the `url` value at rest using the panel's existing `encrypt.StringEncrypt/StringDecrypt` helper (AES-CBC, random IV, base64, keyed by the DB `EncryptKey`), the same mechanism `backup.go` and `database.go` already use. A transparent migration upgrades existing plaintext rows.

It does NOT change the webhook wire protocol, the API masking behavior (`********`), the official-host allowlist, TLS enforcement, retry accounting, or any non-webhook alert config type (email/SMS/common). It does not add key rotation (the existing `EncryptKey` lifecycle is unchanged), and it does not touch alert *logs* (which already redact the URL and never store it).
</domain>

<decisions>
## Implementation Decisions

### Storage format and invariant
- **D-01:** The service layer always holds the **plaintext** URL. Encryption happens only at the persist boundary; decryption happens only where the real URL is consumed. This keeps `validateWebhookAlertConfig` (`ValidateURL`) and the mask-restore path operating on real URLs.
- **D-02:** Encrypted values carry an explicit sentinel prefix `enc:v1:` so read paths disambiguate ciphertext from legacy plaintext without relying on decrypt-error heuristics. `EncryptWebhookURL` is idempotent (no double-encrypt when the prefix is already present); `DecryptWebhookURL` passes legacy plaintext (no prefix) through unchanged.
- **D-03:** Empty URL encrypts/decrypts to empty (no prefix, no ciphertext).

### Placement (avoids import cycles)
- **D-04:** String-level `EncryptWebhookURL`/`DecryptWebhookURL` live in `agent/utils/webhook_alert` (new `secret.go`), which both `agent/app/service` and `agent/utils/xpack/helper` already import; `webhook_alert -> encrypt` adds no cycle. JSON-envelope handling (unmarshal `dto.AlertWebhookConfig`, transform `url`, marshal) stays at each call site (they already own the dto).

### Change points
- **D-05 (encrypt at persist):** `AlertService.UpdateAlertConfig` — after all validation and mask-restore, encrypt the `url` for webhook types before both persist branches (update `upMap["config"]` and create `copier`).
- **D-06 (decrypt for send):** `agent/utils/xpack/helper/alert.go createWebhookAlertLog` — decrypt `webhookConfig.Url` after unmarshal, before `webhook_alert.Send`.
- **D-07 (decrypt for edit-preserve):** `replaceMaskedWebhookURL` — decrypt the stored URL before copying it into the incoming config, so re-validation and re-encryption see plaintext.
- **D-08 (no change, verified safe):** `maskWebhookAlertConfigs` overwrites the `url` with `********` regardless of ciphertext (never leaks). `alertConfigDisplayName`/`alertConfigSMSPhone` read `displayName`/`phone`, not `url`. `TestAlertConfig` tests SMTP only.

### Migration
- **D-09:** New migration `EncryptWebhookAlertConfigURL` (ID `20260711-encrypt-webhook-alert-config-url`) registered in `InitAgentDB` and operating on `global.AlertDB` explicitly — matching the existing `AddTableAlert`/`InitAlertConfig` precedent (AlertConfig lives in `global.AlertDB`). It selects only `WeCom/DingTalk/FeiShu` rows, encrypts each `url`, and updates in place. Idempotent (prefix guard + gormigrate one-shot). Fresh installs have zero webhook rows, so no `EncryptKey` dependency there; on upgrade, `InitSetting` (which seeds `EncryptKey`) runs earlier in the same migrator.

### Acceptance boundary
- **D-10:** Automated Go focused tests cover encrypt roundtrip, prefix idempotency, legacy passthrough, mask preservation over ciphertext, edit-with-mask preserving the stored secret, and the migration encrypting a seeded plaintext row once. A real disposable-robot end-to-end delivery + a DB-file inspection confirming ciphertext at rest remain human UAT.
</decisions>

<specifics>
## Specific Ideas

- The threat closed is "secret readable in the alert DB file and in backups", not "secret in transit" (already HTTPS/TLS-validated) or "secret in API/logs" (already masked/redacted in v1.0).
- Never introduce a second key or a config flag; the panel's single `EncryptKey` is the established root.
- The migration must be safe to run when there are zero webhook rows and when `EncryptKey` is freshly seeded.
</specifics>

<canonical_refs>
## Canonical References

- `.planning/REQUIREMENTS.md` - ALERT-SEC-01 acceptance criteria.
- `agent/utils/encrypt/encrypt.go` - `StringEncrypt`/`StringDecrypt`, `EncryptKey` source.
- `agent/app/service/alert.go` - config CRUD, masking, restore, validation.
- `agent/utils/xpack/helper/alert.go:41-68` - the single webhook delivery point.
- `agent/init/migration/migrate.go:17-96` + `migrations/init.go:380-415` - migration registration and AlertConfig-on-AlertDB precedent.
- `agent/app/service/backup.go:196,200` - the at-rest encryption pattern to mirror.
</canonical_refs>

<deferred>
## Deferred Ideas
- `EncryptKey` rotation / re-encryption tooling (out of scope; existing key lifecycle unchanged).
- Encrypting non-secret config fields (displayName is not a secret).
</deferred>

---
*Phase: 06-webhook-secret-encryption*
