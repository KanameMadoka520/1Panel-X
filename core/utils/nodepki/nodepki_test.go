package nodepki

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testSecret() []byte {
	return []byte("0123456789abcdef0123456789abcdef") // 32 bytes
}

// issueLeaf makes a fresh keypair+CSR and signs it, returning key+cert PEM + fp.
func issueLeaf(t *testing.T, ca *CA, cn string, ips []net.IP, o LeafOptions) (keyPEM, certPEM []byte, fp string) {
	t.Helper()
	keyPEM, csrPEM, err := GenerateKeyAndCSR(cn, nil, ips)
	if err != nil {
		t.Fatalf("GenerateKeyAndCSR: %v", err)
	}
	o.CommonName = cn
	o.IPAddresses = ips
	certPEM, err = ca.SignLeaf(csrPEM, o)
	if err != nil {
		t.Fatalf("SignLeaf: %v", err)
	}
	fp, err = FingerprintPEM(certPEM)
	if err != nil {
		t.Fatalf("FingerprintPEM: %v", err)
	}
	return keyPEM, certPEM, fp
}

func newCA(t *testing.T) *CA {
	t.Helper()
	caCertPEM, caKeyPEM, err := GenerateCA("1Panel-X-Node-CA")
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	ca, err := LoadCA(caCertPEM, caKeyPEM)
	if err != nil {
		t.Fatalf("LoadCA: %v", err)
	}
	return ca
}

func TestCASignsChainableLeaf(t *testing.T) {
	ca := newCA(t)
	_, certPEM, _ := issueLeaf(t, ca, "node-1", []net.IP{net.ParseIP("127.0.0.1")}, LeafOptions{ForServer: true})

	// leaf must chain to the CA
	roots := newPool(t, ca.CertPEM)
	leaf := firstCert(t, certPEM)
	if _, err := leaf.Verify(x509VerifyOpts(roots)); err != nil {
		t.Fatalf("leaf does not chain to CA: %v", err)
	}
	if leaf.IsCA {
		t.Fatal("leaf must not be a CA")
	}
	if leaf.NotAfter.Sub(leaf.NotBefore) > LeafValidity+2*clockSkew {
		t.Fatal("leaf validity exceeds bound")
	}
}

// N13: the issued cert's CN is imposed by core, never taken from the CSR.
func TestSignLeafImposesCommonName(t *testing.T) {
	ca := newCA(t)
	_, csrPEM, err := GenerateKeyAndCSR("attacker-controlled", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	certPEM, err := ca.SignLeaf(csrPEM, LeafOptions{CommonName: "node-5", ForServer: true})
	if err != nil {
		t.Fatal(err)
	}
	if cn := firstCert(t, certPEM).Subject.CommonName; cn != "node-5" {
		t.Fatalf("CN not imposed by core: got %q, want node-5", cn)
	}
}

func TestSignLeafRejectsBadCSR(t *testing.T) {
	ca := newCA(t)
	if _, err := ca.SignLeaf([]byte("not a csr"), LeafOptions{CommonName: "x"}); !errors.Is(err, ErrCSRInvalid) {
		t.Fatalf("want ErrCSRInvalid, got %v", err)
	}
	// valid PEM framing but a tampered DER body: flip a byte in the middle
	// (lands in the signed TBS or the signature) so CheckSignature/parse rejects it
	_, csrPEM, _ := GenerateKeyAndCSR("n", nil, nil)
	block, _ := pem.Decode(csrPEM)
	der := append([]byte(nil), block.Bytes...)
	der[len(der)/2] ^= 0xFF
	corrupt := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})
	if _, err := ca.SignLeaf(corrupt, LeafOptions{CommonName: "x"}); !errors.Is(err, ErrCSRInvalid) {
		t.Fatalf("want ErrCSRInvalid for corrupt CSR, got %v", err)
	}
}

func TestFingerprintStableAndDistinct(t *testing.T) {
	ca := newCA(t)
	_, certA, fpA := issueLeaf(t, ca, "a", nil, LeafOptions{ForServer: true})
	_, _, fpB := issueLeaf(t, ca, "b", nil, LeafOptions{ForServer: true})
	fpA2, _ := FingerprintPEM(certA)
	if fpA != fpA2 {
		t.Fatal("fingerprint not stable for the same cert")
	}
	if fpA == fpB {
		t.Fatal("distinct certs share a fingerprint")
	}
	if len(fpA) != 64 {
		t.Fatalf("fingerprint not sha-256 hex: len=%d", len(fpA))
	}
}

func TestTokenRoundTrip(t *testing.T) {
	nonce, _ := NewNonce()
	claims := TokenClaims{NodeID: 7, Nonce: nonce, Exp: time.Now().Add(MaxTokenTTL).Unix(), MasterFingerprint: "abc"}
	tok, err := IssueToken(testSecret(), claims)
	if err != nil {
		t.Fatal(err)
	}
	got, err := VerifyToken(testSecret(), tok)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.NodeID != 7 || got.Nonce != nonce || got.MasterFingerprint != "abc" {
		t.Fatalf("claims round-trip mismatch: %+v", got)
	}
}

// N2: forged / wrong-secret / tampered tokens are rejected.
func TestTokenForgeryRejected(t *testing.T) {
	claims := TokenClaims{NodeID: 1, Nonce: "n", Exp: time.Now().Add(time.Minute).Unix()}
	tok, _ := IssueToken(testSecret(), claims)

	if _, err := VerifyToken([]byte("wrong-secret-wrong-secret-wrong!"), tok); !errors.Is(err, ErrTokenSignature) {
		t.Fatalf("wrong secret: want ErrTokenSignature, got %v", err)
	}
	// flip a byte in the payload, keep the old signature
	parts := strings.SplitN(tok, ".", 2)
	tampered := flipFirst(parts[0]) + "." + parts[1]
	if _, err := VerifyToken(testSecret(), tampered); err == nil {
		t.Fatal("tampered payload accepted")
	}
}

// N3: expired tokens are rejected.
func TestTokenExpiredRejected(t *testing.T) {
	claims := TokenClaims{NodeID: 1, Nonce: "n", Exp: time.Now().Add(-time.Second).Unix()}
	tok, _ := IssueToken(testSecret(), claims)
	if _, err := VerifyToken(testSecret(), tok); !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("want ErrTokenExpired, got %v", err)
	}
}

func TestTokenMalformed(t *testing.T) {
	for _, bad := range []string{"", "nodot", ".", "a.", ".b"} {
		if _, err := VerifyToken(testSecret(), bad); err == nil {
			t.Fatalf("malformed token %q accepted", bad)
		}
	}
}

func TestIssueTokenRejectsShortSecret(t *testing.T) {
	if _, err := IssueToken([]byte("short"), TokenClaims{}); err == nil {
		t.Fatal("short secret accepted")
	}
}

// The single-box loopback proof: a real mTLS handshake with mutual
// fingerprint pinning, plus the three rejection paths (N5/N6/N8).
func TestMutualTLSWithFingerprintPinning(t *testing.T) {
	ca := newCA(t)
	loopback := []net.IP{net.ParseIP("127.0.0.1")}
	serverKey, serverCert, serverFP := issueLeaf(t, ca, "node-1", loopback, LeafOptions{ForServer: true})
	clientKey, clientCert, clientFP := issueLeaf(t, ca, "core", nil, LeafOptions{ForClient: true})

	serverCfg, err := ServerTLSConfig(ca.CertPEM, serverCert, serverKey, clientFP)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	srv.TLS = serverCfg
	srv.StartTLS()
	defer srv.Close()

	// happy path: correct pins both ways
	clientCfg, err := ClientTLSConfig(ca.CertPEM, clientCert, clientKey, serverFP)
	if err != nil {
		t.Fatal(err)
	}
	if body := doGet(t, srv.URL, clientCfg); body != "ok" {
		t.Fatalf("handshake body = %q", body)
	}

	// N5: client pins the WRONG server fingerprint -> abort
	badServerPin, _ := ClientTLSConfig(ca.CertPEM, clientCert, clientKey, strings.Repeat("00", 32))
	if _, err := tryGet(srv.URL, badServerPin); err == nil {
		t.Fatal("wrong server-fp pin was accepted")
	}

	// N8: client cert from a DIFFERENT CA -> RequireAndVerifyClientCert rejects
	otherCA := newCA(t)
	otherKey, otherCert, _ := issueLeaf(t, otherCA, "core", nil, LeafOptions{ForClient: true})
	foreignCfg, _ := ClientTLSConfig(ca.CertPEM, otherCert, otherKey, serverFP)
	if _, err := tryGet(srv.URL, foreignCfg); err == nil {
		t.Fatal("client cert from a foreign CA was accepted")
	}

	// N6: server pins the WRONG client fingerprint -> abort (rebuild server)
	strictCfg, _ := ServerTLSConfig(ca.CertPEM, serverCert, serverKey, strings.Repeat("11", 32))
	srv2 := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	srv2.TLS = strictCfg
	srv2.StartTLS()
	defer srv2.Close()
	if _, err := tryGet(srv2.URL, clientCfg); err == nil {
		t.Fatal("wrong client-fp pin was accepted")
	}
	_ = serverFP
}

// ---- test helpers ----

func doGet(t *testing.T, url string, cfg *tls.Config) string {
	t.Helper()
	body, err := tryGet(url, cfg)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return body
}

func tryGet(url string, cfg *tls.Config) (string, error) {
	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: &http.Transport{TLSClientConfig: cfg},
	}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}

func flipFirst(s string) string {
	if s == "" {
		return "A"
	}
	c := s[0]
	if c == 'A' {
		c = 'B'
	} else {
		c = 'A'
	}
	return string(c) + s[1:]
}

func newPool(t *testing.T, caPEM []byte) *x509.CertPool {
	t.Helper()
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("failed to append CA to pool")
	}
	return pool
}

func firstCert(t *testing.T, certPEM []byte) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("no PEM block")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	return cert
}

func x509VerifyOpts(roots *x509.CertPool) x509.VerifyOptions {
	return x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}}
}
