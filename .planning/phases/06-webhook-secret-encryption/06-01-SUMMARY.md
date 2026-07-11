---
phase: 06-webhook-secret-encryption
requirement: ALERT-SEC-01
plan: 06-01
status: implemented_automated_verified
human_uat: pending
completed: 2026-07-10
---

# Phase 06 Summary: Webhook Secret At-Rest Encryption

## What shipped

The webhook bot URL is now encrypted at rest. The v1.0 open webhook feature masked the URL in the API, logs, and errors but persisted it plaintext inside `AlertConfig.Config` in the alert database (and thus in DB backups). That last secret-at-rest exposure is closed by encrypting the `url` value with the panel's existing `encrypt.StringEncrypt/StringDecrypt` (AES-CBC, random IV, base64, keyed by the DB `EncryptKey`) — the same helper `backup.go` and `database.go` already use. No new key or key-management design was introduced.

**Invariant:** the service layer always holds plaintext; encryption happens only at the persist boundary; decryption happens only where the real URL is consumed.

## Commits

- `7b3c80eb9` feat: encrypt webhook alert URLs at rest
- `e6d6a3472` test: cover webhook secret at-rest encryption

## Files

- `agent/utils/webhook_alert/secret.go` (new) — `EncryptWebhookURL`/`DecryptWebhookURL`/`IsEncryptedWebhookURL` with an `enc:v1:` sentinel; encrypt is idempotent, decrypt passes legacy plaintext through.
- `agent/app/service/alert.go` — encrypt in `UpdateAlertConfig` after validation (covers create + update at one site); decrypt the stored URL in `replaceMaskedWebhookURL` for masked edits.
- `agent/utils/xpack/helper/alert.go` — decrypt before `webhook_alert.Send`; a decrypt failure is recorded as a delivery error without leaking the secret.
- `agent/init/migration/migrations/init.go` + `migrate.go` — `EncryptWebhookAlertConfigURL` migration, registered in `InitAgentDB` (operates on `global.AlertDB`, after `InitSetting` seeds `EncryptKey`).
- Tests: `secret_test.go`, `alert_webhook_secret_test.go`, `alert_secret_migration_test.go`.

## Decisions realized

- D-01..D-04 (plaintext-in-service invariant, `enc:v1:` sentinel, empty handling, helper placement in `webhook_alert` to avoid import cycles) — implemented as planned.
- D-05..D-08 (encrypt at persist; decrypt for send and masked-edit; mask/uniqueness untouched) — implemented; `TestAlertConfig` confirmed to be SMTP-only, so no webhook URL reader was missed.
- D-09 (migration on `global.AlertDB`, webhook types only, idempotent) — implemented and tested.

## Behavior preserved (verified by tests + full regression)

Masking still returns `********`; masked-edit preserves the stored secret; the official-host allowlist, TLS, redirect, timeout, response-bound, and retry-accounting logic are unchanged; non-webhook config types are untouched; no license state is affected.

## Tech debt / not done

- `EncryptKey` rotation / bulk re-encryption tooling is out of scope; the existing key lifecycle is unchanged.
- Human UAT (real robot delivery + on-disk ciphertext inspection) is pending — see `06-HUMAN-UAT.md`.
