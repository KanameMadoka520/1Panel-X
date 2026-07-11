---
phase: 06-webhook-secret-encryption
requirement: ALERT-SEC-01
verification_status: human_needed
automated: passed
environment: WSL Ubuntu, Go 1.26.1 (/tmp/codex-go1.26.1)
date: 2026-07-10
---

# Phase 06 Verification: Webhook Secret At-Rest Encryption

## Automated evidence (passed)

| Check | Command | Result |
|-------|---------|--------|
| Unit: secret helper | `go test ./utils/webhook_alert -count=1` | ok |
| Service + migration focused | `go test ./app/service ./init/migration/migrations -count=1` | ok |
| Full agent regression | `go test ./... -count=1` | exit 0 (all packages ok) |
| gofmt | `gofmt -l` on changed files | clean |
| go vet | `go vet` on affected packages | clean |
| Compile | `go build ./...` (agent) | ok |

### Test coverage mapped to acceptance criteria

- **AC1 (ciphertext at rest, no plaintext in row):** `TestEncryptWebhookAlertConfigURL` asserts the stored config carries the `enc:v1:` prefix and does not contain the plaintext secret; `TestEncryptWebhookURLRoundTrip` asserts the ciphertext differs from and round-trips to the plaintext.
- **AC2 (transparent one-shot migration, safe at zero/fresh):** `TestEncryptWebhookAlertConfigURLMigration` encrypts a seeded plaintext row once, is idempotent on re-run, and leaves non-webhook rows unchanged. Migration selects only webhook types; fresh installs have zero such rows.
- **AC3 (no delivery/masking/edit regression):** full-suite pass; `TestEncryptedWebhookConfigStillMasks` (mask returns `********` over ciphertext, no `access_token`/`enc:v1:` leak); `TestReplaceMaskedWebhookURLDecryptsStored` (masked-edit decrypts the stored secret); existing `TestValidateWebhookAlertConfig`, `TestMaskWebhookAlertConfigs`, and sender tests still pass.
- **AC4 (no secret in logs/API/errors; legacy still delivers):** `TestDecryptWebhookURLLegacyPlaintext` (legacy passthrough) and `TestDecryptWebhookURLCorruptCiphertext` (decrypt error, no value passed to sender); the send helper records a generic decrypt error, not the URL.
- **AC5:** focused tests above; disposable-robot delivery + on-disk ciphertext inspection are human UAT.

## Security review

- Secret at rest: AES-CBC with a random IV per value via the panel `EncryptKey`; identical trust model to existing backup/database secrets.
- Fail-closed: if `EncryptKey` cannot be loaded, `UpdateAlertConfig` returns an error and does not persist a plaintext fallback.
- No plaintext path to API/logs: masking overwrites the URL entirely; decrypt errors are recorded generically.
- Validation runs on plaintext before encryption, so bad hosts are rejected before anything is stored.
- The `enc:v1:` prefix reveals only that a value is encrypted (expected), never key material.

## Human-needed (see 06-HUMAN-UAT.md)

Real WeCom/DingTalk/Feishu robot delivery on a VPS, plus a direct inspection of the alert database file confirming ciphertext at rest, and a browser network/log capture confirming the URL is never exposed.
