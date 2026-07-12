// Package nodepki is the clean-room PKI + enrollment-token core for 1Panel-X
// secure multi-node. Core owns a private CA; nodes join by presenting a CSR
// together with a single-use, HMAC-authenticated enrollment token. The core
// then signs a per-node leaf certificate and both sides speak mutual TLS,
// pinning each other's certificate fingerprint (CA-chain validity alone is not
// enough — a CA that signs many nodes must not let node A impersonate node B).
//
// This package is deliberately dependency-free (stdlib crypto only) and pure,
// so every control in the N1-N15 threat model (see
// .planning/research/NODE-ENROLLMENT-DESIGN.md) is unit-testable without a host.
package nodepki

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
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
	"fmt"
	"math/big"
	"net"
	"strings"
	"time"
)

const (
	// MaxTokenTTL bounds how long an enrollment token may be valid (N3).
	MaxTokenTTL = 10 * time.Minute
	// LeafValidity bounds an issued node leaf certificate's lifetime (N10).
	LeafValidity = 397 * 24 * time.Hour
	caValidity   = 10 * 365 * 24 * time.Hour
	clockSkew    = 5 * time.Minute
)

var (
	ErrTokenMalformed = errors.New("enrollment token malformed")
	ErrTokenSignature = errors.New("enrollment token signature invalid")
	ErrTokenExpired   = errors.New("enrollment token expired")
	ErrCSRInvalid     = errors.New("certificate signing request invalid")
	ErrPinMismatch    = errors.New("peer certificate fingerprint mismatch")
)

// ---------------------------------------------------------------------------
// Certificate authority
// ---------------------------------------------------------------------------

// CA is a loaded signing authority (parsed cert + private key + the CA cert PEM).
type CA struct {
	Cert    *x509.Certificate
	Key     crypto.Signer
	CertPEM []byte
}

// GenerateCA creates a fresh self-signed ECDSA P-256 CA. The returned PEM blobs
// are what the caller persists (key encrypted at rest, cert may be public).
func GenerateCA(commonName string) (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             now.Add(-clockSkew),
		NotAfter:              now.Add(caValidity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

// LoadCA parses a CA cert+key PEM pair produced by GenerateCA.
func LoadCA(certPEM, keyPEM []byte) (*CA, error) {
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil || certBlock.Type != "CERTIFICATE" {
		return nil, errors.New("invalid CA certificate PEM")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, err
	}
	if !cert.IsCA {
		return nil, errors.New("certificate is not a CA")
	}
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, errors.New("invalid CA key PEM")
	}
	signer, err := parsePrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, err
	}
	return &CA{Cert: cert, Key: signer, CertPEM: certPEM}, nil
}

// LeafOptions controls how a node leaf certificate is issued. The CommonName is
// imposed by the core (bound to the enrollment token's node id) — the CSR's own
// subject is ignored; only its public key is trusted (N13).
type LeafOptions struct {
	CommonName  string
	DNSNames    []string
	IPAddresses []net.IP
	Validity    time.Duration
	ForServer   bool // node listener presents this to core
	ForClient   bool // core presents this to node
}

// SignLeaf verifies a CSR's self-signature, then issues a leaf certificate that
// carries ONLY the CSR's public key under a core-imposed subject and SANs.
func (ca *CA) SignLeaf(csrPEM []byte, o LeafOptions) (certPEM []byte, err error) {
	block, _ := pem.Decode(csrPEM)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return nil, ErrCSRInvalid
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCSRInvalid, err)
	}
	if err := csr.CheckSignature(); err != nil {
		return nil, fmt.Errorf("%w: bad self-signature: %v", ErrCSRInvalid, err)
	}
	validity := o.Validity
	if validity <= 0 || validity > LeafValidity {
		validity = LeafValidity
	}
	var eku []x509.ExtKeyUsage
	if o.ForServer {
		eku = append(eku, x509.ExtKeyUsageServerAuth)
	}
	if o.ForClient {
		eku = append(eku, x509.ExtKeyUsageClientAuth)
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: o.CommonName},
		NotBefore:             now.Add(-clockSkew),
		NotAfter:              now.Add(validity),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           eku,
		BasicConstraintsValid: true,
		IsCA:                  false,
		DNSNames:              o.DNSNames,
		IPAddresses:           o.IPAddresses,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.Cert, csr.PublicKey, ca.Key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), nil
}

// GenerateKeyAndCSR produces a fresh ECDSA P-256 keypair and a CSR for it. Used
// by the node bootstrap (the private key never leaves the node) and by tests.
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

// ---------------------------------------------------------------------------
// Fingerprints (SHA-256 of DER, lowercase hex)
// ---------------------------------------------------------------------------

// FingerprintDER returns the pinned fingerprint of a raw DER certificate.
func FingerprintDER(der []byte) string {
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:])
}

// FingerprintPEM returns the pinned fingerprint of the first cert in a PEM blob.
func FingerprintPEM(certPEM []byte) (string, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", errors.New("invalid certificate PEM")
	}
	return FingerprintDER(block.Bytes), nil
}

// ---------------------------------------------------------------------------
// Enrollment token (HMAC-SHA256, single-use enforced by the caller/DB)
// ---------------------------------------------------------------------------

// TokenClaims is the authenticated enrollment-token payload. Single-use (N1) is
// enforced by the caller burning the token row atomically on redemption; this
// struct carries the node binding (N3), expiry (N3) and master fingerprint the
// joining node pins before sending its CSR (N4).
type TokenClaims struct {
	NodeID            uint   `json:"nid"`
	Nonce             string `json:"non"`
	Exp               int64  `json:"exp"`
	MasterFingerprint string `json:"mfp"`
}

// NewNonce returns a random 128-bit nonce (hex).
func NewNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// IssueToken serialises claims and appends an HMAC-SHA256 tag (N2). secret is a
// core-only per-install key (>=32 bytes) never sent to nodes.
func IssueToken(secret []byte, c TokenClaims) (string, error) {
	if len(secret) < 32 {
		return "", errors.New("enrollment secret too short")
	}
	payload, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	p := base64.RawURLEncoding.EncodeToString(payload)
	sig := base64.RawURLEncoding.EncodeToString(tag(secret, p))
	return p + "." + sig, nil
}

// VerifyToken checks the HMAC in constant time (N2) and the expiry (N3). It does
// NOT check single-use — the caller must burn the token's DB row atomically.
func VerifyToken(secret []byte, token string) (TokenClaims, error) {
	var claims TokenClaims
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return claims, ErrTokenMalformed
	}
	got, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims, ErrTokenMalformed
	}
	want := tag(secret, parts[0])
	if subtle.ConstantTimeCompare(want, got) != 1 {
		return claims, ErrTokenSignature
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return claims, ErrTokenMalformed
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return claims, ErrTokenMalformed
	}
	if time.Now().After(time.Unix(claims.Exp, 0)) {
		return claims, ErrTokenExpired
	}
	return claims, nil
}

func tag(secret []byte, payload string) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	return mac.Sum(nil)
}

// ---------------------------------------------------------------------------
// mTLS configs with per-peer fingerprint pinning
// ---------------------------------------------------------------------------

// ClientTLSConfig builds the core->node dialing config: it presents core's
// client cert, trusts only the CA, and additionally pins the node's exact
// server-cert fingerprint (N5/N8). Fingerprint mismatch aborts the handshake.
func ClientTLSConfig(caPEM, clientCertPEM, clientKeyPEM []byte, pinnedServerFP string) (*tls.Config, error) {
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("failed to load CA into pool")
	}
	clientCert, err := tls.X509KeyPair(clientCertPEM, clientKeyPEM)
	if err != nil {
		return nil, err
	}
	pinned := strings.ToLower(strings.TrimSpace(pinnedServerFP))
	cfg := &tls.Config{
		RootCAs:      roots,
		Certificates: []tls.Certificate{clientCert},
		MinVersion:   tls.VersionTLS12,
	}
	if pinned != "" {
		cfg.VerifyPeerCertificate = pinVerifier(pinned)
	}
	return cfg, nil
}

// ServerTLSConfig builds the node listener config: it presents the node's leaf,
// requires+verifies a client cert chaining to the CA, and (when a master
// fingerprint is known) pins core's exact client-cert fingerprint (N6).
func ServerTLSConfig(caPEM, serverCertPEM, serverKeyPEM []byte, pinnedClientFP string) (*tls.Config, error) {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("failed to load CA into pool")
	}
	serverCert, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	if err != nil {
		return nil, err
	}
	pinned := strings.ToLower(strings.TrimSpace(pinnedClientFP))
	cfg := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
	}
	if pinned != "" {
		cfg.VerifyPeerCertificate = pinVerifier(pinned)
	}
	return cfg, nil
}

// pinVerifier returns a VerifyPeerCertificate that requires the peer's leaf
// (rawCerts[0]) to match the pinned fingerprint in constant time.
func pinVerifier(pinned string) func([][]byte, [][]*x509.Certificate) error {
	want := []byte(pinned)
	return func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			return ErrPinMismatch
		}
		got := []byte(FingerprintDER(rawCerts[0]))
		if subtle.ConstantTimeCompare(want, got) != 1 {
			return ErrPinMismatch
		}
		return nil
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, limit)
}

func parsePrivateKey(der []byte) (crypto.Signer, error) {
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		if signer, ok := key.(crypto.Signer); ok {
			return signer, nil
		}
		return nil, errors.New("unsupported private key type")
	}
	if key, err := x509.ParseECPrivateKey(der); err == nil {
		return key, nil
	}
	return nil, errors.New("failed to parse private key")
}
