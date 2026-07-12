package nodepki

import (
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGenerateKeyAndCSR(t *testing.T) {
	keyPEM, csrPEM, err := GenerateKeyAndCSR("node", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(keyPEM), "PRIVATE KEY") {
		t.Fatal("key PEM missing")
	}
	block, _ := pem.Decode(csrPEM)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		t.Fatal("csr PEM invalid")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		t.Fatalf("parse csr: %v", err)
	}
	if err := csr.CheckSignature(); err != nil {
		t.Fatalf("csr self-signature invalid: %v", err)
	}
}

func TestParseTokenClaims(t *testing.T) {
	// payload is base64url(json).sig — ParseTokenClaims reads payload without verifying.
	// {"nid":7,"non":"abc","exp":123,"mfp":"deadbeef"}
	claims, err := ParseTokenClaims("eyJuaWQiOjcsIm5vbiI6ImFiYyIsImV4cCI6MTIzLCJtZnAiOiJkZWFkYmVlZiJ9.anything")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.NodeID != 7 || claims.MasterFingerprint != "deadbeef" {
		t.Fatalf("claims mismatch: %+v", claims)
	}
	for _, bad := range []string{"", "nodot", ".sig"} {
		if _, err := ParseTokenClaims(bad); err == nil {
			t.Fatalf("malformed token %q accepted", bad)
		}
	}
}

func TestFingerprintEqual(t *testing.T) {
	if !FingerprintEqual("ABCdef", "abcdef") {
		t.Fatal("case/space-insensitive compare should match")
	}
	if FingerprintEqual("abc", "abd") {
		t.Fatal("different fps must not match")
	}
}

func TestEnrollTLSConfigPins(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()
	serverFP := FingerprintDER(srv.Certificate().Raw)

	// correct pin -> connects
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: EnrollTLSConfig(serverFP)}}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("pinned request failed: %v", err)
	}
	resp.Body.Close()

	// wrong pin -> fails
	bad := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: EnrollTLSConfig(strings.Repeat("00", 32))}}
	if _, err := bad.Get(srv.URL); err == nil {
		t.Fatal("wrong master fingerprint pin was accepted")
	}
}
