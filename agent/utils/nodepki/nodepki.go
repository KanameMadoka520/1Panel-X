// Package nodepki is the node-side (agent) half of the clean-room secure
// multi-node PKI. The agent generates its OWN keypair + CSR (the private key
// never leaves the host), reads the master fingerprint carried in the
// enrollment token to pin core before enrolling, and pins/compares certificate
// fingerprints. It is a separate module from core, so this minimal crypto is
// duplicated by necessity (no shared internal package across Go modules).
package nodepki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net"
	"strings"
)

// TokenClaims mirrors the core enrollment-token payload. The agent can READ it
// (base64 JSON) to pin the master, but cannot forge it (no HMAC secret).
type TokenClaims struct {
	NodeID            uint   `json:"nid"`
	Nonce             string `json:"non"`
	Exp               int64  `json:"exp"`
	MasterFingerprint string `json:"mfp"`
}

// GenerateKeyAndCSR produces a fresh ECDSA P-256 key + CSR. The key PEM stays on
// the node; only the CSR is sent to core for signing.
func GenerateKeyAndCSR(commonName string, dnsNames []string, ips []net.IP) (keyPEM, csrPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	tmpl := &x509.CertificateRequest{
		Subject:     pkix.Name{CommonName: commonName},
		DNSNames:    dnsNames,
		IPAddresses: ips,
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, tmpl, key)
	if err != nil {
		return nil, nil, err
	}
	csrPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return keyPEM, csrPEM, nil
}

// ParseTokenClaims decodes (but does NOT authenticate) the token payload so the
// agent can extract the master fingerprint to pin before enrolling (N4).
func ParseTokenClaims(token string) (TokenClaims, error) {
	var claims TokenClaims
	parts := strings.SplitN(strings.TrimSpace(token), ".", 2)
	if len(parts) != 2 || parts[0] == "" {
		return claims, errors.New("malformed enrollment token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return claims, err
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return claims, err
	}
	return claims, nil
}

// FingerprintDER / FingerprintPEM: SHA-256 of the DER cert, lowercase hex.
func FingerprintDER(der []byte) string {
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:])
}

func FingerprintPEM(certPEM []byte) (string, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", errors.New("invalid certificate PEM")
	}
	return FingerprintDER(block.Bytes), nil
}

// FingerprintEqual compares two fingerprints in constant time.
func FingerprintEqual(a, b string) bool {
	return subtle.ConstantTimeCompare(
		[]byte(strings.ToLower(strings.TrimSpace(a))),
		[]byte(strings.ToLower(strings.TrimSpace(b))),
	) == 1
}

// EnrollTLSConfig builds the client config for the enrollment HTTP call: it
// pins core's server-cert fingerprint carried in the token (no client cert —
// enrollment is authenticated by the token, not mTLS). If pinnedServerFP is
// empty the caller must decide whether to proceed (core served plain HTTP).
func EnrollTLSConfig(pinnedServerFP string) *tls.Config {
	pinned := strings.ToLower(strings.TrimSpace(pinnedServerFP))
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if pinned != "" {
		// pin exactly; skip default CA verification since core's web cert may be
		// self-signed. The token's HMAC integrity is what makes this pin trustworthy.
		cfg.InsecureSkipVerify = true
		cfg.VerifyConnection = func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("no server certificate")
			}
			got := FingerprintDER(cs.PeerCertificates[0].Raw)
			if subtle.ConstantTimeCompare([]byte(pinned), []byte(got)) != 1 {
				return errors.New("master certificate fingerprint mismatch")
			}
			return nil
		}
	}
	return cfg
}
