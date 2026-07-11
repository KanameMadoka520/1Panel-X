package webhook_alert

import (
	"strings"

	"github.com/1Panel-dev/1Panel/agent/utils/encrypt"
)

// webhookURLEncPrefix marks a webhook URL value that has been encrypted at
// rest. Values without this prefix are treated as legacy plaintext for
// backward compatibility until the migration upgrades them.
const webhookURLEncPrefix = "enc:v1:"

// IsEncryptedWebhookURL reports whether v is an at-rest encrypted webhook URL.
func IsEncryptedWebhookURL(v string) bool {
	return strings.HasPrefix(v, webhookURLEncPrefix)
}

// EncryptWebhookURL returns the at-rest representation of a plaintext webhook
// URL using the panel's existing symmetric key. Empty input stays empty; an
// already-encrypted value is returned unchanged so the call is idempotent and
// safe to run twice (for example, on a migrated row).
func EncryptWebhookURL(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	if IsEncryptedWebhookURL(plain) {
		return plain, nil
	}
	cipherText, err := encrypt.StringEncrypt(plain)
	if err != nil {
		return "", err
	}
	return webhookURLEncPrefix + cipherText, nil
}

// DecryptWebhookURL returns the plaintext webhook URL from an at-rest value. A
// value without the encryption prefix is returned unchanged (legacy
// plaintext), so delivery keeps working before the migration runs.
func DecryptWebhookURL(stored string) (string, error) {
	if !IsEncryptedWebhookURL(stored) {
		return stored, nil
	}
	cipherText := strings.TrimPrefix(stored, webhookURLEncPrefix)
	if cipherText == "" {
		return "", nil
	}
	return encrypt.StringDecrypt(cipherText)
}
