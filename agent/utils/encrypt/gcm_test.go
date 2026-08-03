package encrypt

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/1Panel-dev/1Panel/agent/global"
)

func TestStringGCMRoundTripAndRandomNonce(t *testing.T) {
	const (
		plaintext = "synthetic-oauth-secret"
		key       = "unit-test-install-key"
		domain    = "unit-test/oauth-secret"
	)

	first, err := StringEncryptGCMWithKey(plaintext, key, domain)
	if err != nil {
		t.Fatalf("encrypt first value: %v", err)
	}
	second, err := StringEncryptGCMWithKey(plaintext, key, domain)
	if err != nil {
		t.Fatalf("encrypt second value: %v", err)
	}
	if first == second {
		t.Fatal("ciphertexts are identical; nonce must be random")
	}
	if strings.Contains(first, plaintext) || strings.Contains(second, plaintext) {
		t.Fatal("ciphertext contains plaintext")
	}
	if !strings.HasPrefix(first, stringGCMVersionPrefix) {
		t.Fatalf("ciphertext prefix = %q", first)
	}

	decrypted, err := StringDecryptGCMWithKey(first, key, domain)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("decrypted value = %q, want %q", decrypted, plaintext)
	}
}

func TestStringGCMRejectsInvalidInputs(t *testing.T) {
	const (
		plaintext = "synthetic-oauth-secret"
		key       = "unit-test-install-key"
		domain    = "unit-test/oauth-secret"
	)

	ciphertext, err := StringEncryptGCMWithKey(plaintext, key, domain)
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	payload, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(ciphertext, stringGCMVersionPrefix))
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}

	tampered := append([]byte(nil), payload...)
	tampered[len(tampered)-1] ^= 0x01
	tamperedCiphertext := stringGCMVersionPrefix + base64.RawStdEncoding.EncodeToString(tampered)
	truncatedCiphertext := stringGCMVersionPrefix + base64.RawStdEncoding.EncodeToString(payload[:8])

	tests := []struct {
		name       string
		ciphertext string
		key        string
		domain     string
	}{
		{name: "wrong domain", ciphertext: ciphertext, key: key, domain: "unit-test/other-domain"},
		{name: "wrong key", ciphertext: ciphertext, key: "unit-test-other-key", domain: domain},
		{name: "tampered", ciphertext: tamperedCiphertext, key: key, domain: domain},
		{name: "truncated", ciphertext: truncatedCiphertext, key: key, domain: domain},
		{name: "unsupported version", ciphertext: "enc:gcm:v2:" + strings.TrimPrefix(ciphertext, stringGCMVersionPrefix), key: key, domain: domain},
		{name: "empty key", ciphertext: ciphertext, key: "", domain: domain},
		{name: "empty domain", ciphertext: ciphertext, key: key, domain: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decrypted, err := StringDecryptGCMWithKey(tt.ciphertext, tt.key, tt.domain)
			if err == nil {
				t.Fatalf("decrypt unexpectedly succeeded with %q", decrypted)
			}
			if strings.Contains(err.Error(), plaintext) {
				t.Fatalf("error exposed plaintext: %v", err)
			}
		})
	}

	if _, err := StringEncryptGCMWithKey(plaintext, "", domain); err == nil {
		t.Fatal("encrypt unexpectedly accepted an empty key")
	}
	if _, err := StringEncryptGCMWithKey(plaintext, key, ""); err == nil {
		t.Fatal("encrypt unexpectedly accepted an empty domain")
	}
}

func TestStringGCMUsesConfiguredInstallKey(t *testing.T) {
	oldKey := global.CONF.Base.EncryptKey
	global.CONF.Base.EncryptKey = "configured-agent-install-key"
	t.Cleanup(func() {
		global.CONF.Base.EncryptKey = oldKey
	})

	ciphertext, err := StringEncryptGCM("configured-secret", "unit-test/configured-domain")
	if err != nil {
		t.Fatalf("encrypt with configured key: %v", err)
	}
	plaintext, err := StringDecryptGCM(ciphertext, "unit-test/configured-domain")
	if err != nil {
		t.Fatalf("decrypt with configured key: %v", err)
	}
	if plaintext != "configured-secret" {
		t.Fatalf("decrypted value = %q", plaintext)
	}
}
