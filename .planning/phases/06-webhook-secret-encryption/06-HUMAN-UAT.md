---
phase: 06-webhook-secret-encryption
requirement: ALERT-SEC-01
status: pending
items: 3
---

# Phase 06 Human UAT: Webhook Secret At-Rest Encryption

These require a running panel on a VPS and a real (disposable) robot. Do not mark pass without evidence.

## UAT-06-1: Ciphertext at rest (DB inspection)
**Steps:** On the VPS, create a WeCom/DingTalk/Feishu webhook alert config with a real bot URL. Open the agent alert database file (SQLite) and read the `alert_configs` row `config` column.
**Expected:** the `url` value begins with `enc:v1:` and does NOT contain the bot token/key in plaintext. A `sqlite3 ... "select config from alert_configs where type='weCom'"` shows ciphertext.
**Result:** _pending_

## UAT-06-2: End-to-end delivery + no secret exposure
**Steps:** Trigger an alert that uses the webhook config. Watch the robot channel for the message. In the browser, open DevTools Network + the panel alert logs; inspect the config list/detail responses and any error entries.
**Expected:** the message is delivered to the robot; the API responses show `********` for the URL; no request/response/log/error anywhere contains the plaintext bot URL.
**Result:** _pending_

## UAT-06-3: Upgrade migration of an existing plaintext row
**Steps:** Start from a panel that already stored a webhook config under v1.0 (plaintext at rest). Upgrade to the v1.1 binary and let migrations run. Re-inspect the DB row and re-trigger delivery.
**Expected:** the previously-plaintext `url` is now `enc:v1:` ciphertext after the upgrade; delivery still succeeds; masked edit still preserves the secret.
**Result:** _pending_
