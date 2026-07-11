package webhook_alert

import (
	"strings"
	"testing"

	"github.com/1Panel-dev/1Panel/agent/global"
)

// setTestEncryptKey installs a fixed in-memory key so encrypt.StringEncrypt
// does not fall back to the database during unit tests.
func setTestEncryptKey(t *testing.T) {
	t.Helper()
	global.CONF.Base.EncryptKey = "unit-test-key-0123456789"
}

func TestEncryptWebhookURLRoundTrip(t *testing.T) {
	setTestEncryptKey(t)
	plain := "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=abc-123-secret"

	stored, err := EncryptWebhookURL(plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !IsEncryptedWebhookURL(stored) {
		t.Fatalf("expected encrypted prefix, got %q", stored)
	}
	if strings.Contains(stored, plain) || strings.Contains(stored, "abc-123-secret") {
		t.Fatalf("plaintext secret leaked into stored value %q", stored)
	}

	back, err := DecryptWebhookURL(stored)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if back != plain {
		t.Fatalf("round-trip mismatch: got %q want %q", back, plain)
	}
}

func TestEncryptWebhookURLEmpty(t *testing.T) {
	setTestEncryptKey(t)
	stored, err := EncryptWebhookURL("")
	if err != nil {
		t.Fatalf("encrypt empty: %v", err)
	}
	if stored != "" {
		t.Fatalf("expected empty, got %q", stored)
	}
	back, err := DecryptWebhookURL("")
	if err != nil {
		t.Fatalf("decrypt empty: %v", err)
	}
	if back != "" {
		t.Fatalf("expected empty, got %q", back)
	}
}

func TestEncryptWebhookURLIdempotent(t *testing.T) {
	setTestEncryptKey(t)
	plain := "https://oapi.dingtalk.com/robot/send?access_token=zzz"

	once, err := EncryptWebhookURL(plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	twice, err := EncryptWebhookURL(once)
	if err != nil {
		t.Fatalf("re-encrypt: %v", err)
	}
	if once != twice {
		t.Fatalf("double encryption changed value: %q vs %q", once, twice)
	}
	back, err := DecryptWebhookURL(twice)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if back != plain {
		t.Fatalf("idempotent round-trip mismatch: got %q want %q", back, plain)
	}
}

func TestDecryptWebhookURLLegacyPlaintext(t *testing.T) {
	setTestEncryptKey(t)
	// A legacy row stores the URL without the encryption prefix; it must still
	// be returned verbatim so delivery works before the migration runs.
	plain := "https://open.feishu.cn/open-apis/bot/v2/hook/xyz"
	back, err := DecryptWebhookURL(plain)
	if err != nil {
		t.Fatalf("decrypt legacy: %v", err)
	}
	if back != plain {
		t.Fatalf("legacy passthrough mismatch: got %q want %q", back, plain)
	}
	if IsEncryptedWebhookURL(plain) {
		t.Fatalf("plaintext URL wrongly detected as encrypted")
	}
}

func TestDecryptWebhookURLCorruptCiphertext(t *testing.T) {
	setTestEncryptKey(t)
	// Prefixed but non-decryptable payload must error, never silently pass a
	// wrong value to the sender.
	if _, err := DecryptWebhookURL(webhookURLEncPrefix + "!!!not-base64!!!"); err == nil {
		t.Fatalf("expected error decrypting corrupt ciphertext")
	}
}
