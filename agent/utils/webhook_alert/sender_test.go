package webhook_alert

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBuildPayload(t *testing.T) {
	tests := []struct {
		name       string
		platform   Platform
		assertBody func(t *testing.T, body map[string]any)
	}{
		{
			name:     "wecom",
			platform: PlatformWeCom,
			assertBody: func(t *testing.T, body map[string]any) {
				t.Helper()
				assertTextPayload(t, body, "msgtype", "text")
			},
		},
		{
			name:     "dingtalk",
			platform: PlatformDingTalk,
			assertBody: func(t *testing.T, body map[string]any) {
				t.Helper()
				assertTextPayload(t, body, "msgtype", "text")
			},
		},
		{
			name:     "feishu",
			platform: PlatformFeiShu,
			assertBody: func(t *testing.T, body map[string]any) {
				t.Helper()
				assertTextPayload(t, body, "msg_type", "content")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := buildPayload(tt.platform, "alert content")
			if err != nil {
				t.Fatalf("buildPayload() error = %v", err)
			}
			var body map[string]any
			if err := json.Unmarshal(payload, &body); err != nil {
				t.Fatalf("decode payload: %v", err)
			}
			tt.assertBody(t, body)
		})
	}
}

func TestValidateURLAllowlist(t *testing.T) {
	tests := []struct {
		platform Platform
		url      string
	}{
		{platform: PlatformWeCom, url: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=test"},
		{platform: PlatformWeCom, url: "https://QYAPI.WEIXIN.QQ.COM:443/cgi-bin/webhook/send?key=test"},
		{platform: PlatformDingTalk, url: "https://oapi.dingtalk.com/robot/send?access_token=test"},
		{platform: PlatformFeiShu, url: "https://open.feishu.cn/open-apis/bot/v2/hook/test"},
		{platform: PlatformFeiShu, url: "https://open.larksuite.com/open-apis/bot/v2/hook/test"},
	}

	for _, tt := range tests {
		t.Run(string(tt.platform)+tt.url, func(t *testing.T) {
			if _, err := validateURL(tt.platform, tt.url); err != nil {
				t.Fatalf("validateURL() error = %v", err)
			}
		})
	}
}

func TestValidateURLRejectsUnsafeTargets(t *testing.T) {
	tests := []struct {
		name     string
		platform Platform
		url      string
	}{
		{name: "empty", platform: PlatformWeCom, url: ""},
		{name: "http", platform: PlatformWeCom, url: "http://qyapi.weixin.qq.com/hook"},
		{name: "wrong platform host", platform: PlatformWeCom, url: "https://oapi.dingtalk.com/robot/send"},
		{name: "suffix confusion", platform: PlatformWeCom, url: "https://qyapi.weixin.qq.com.evil.example/hook"},
		{name: "prefix confusion", platform: PlatformWeCom, url: "https://evil.qyapi.weixin.qq.com/hook"},
		{name: "userinfo confusion", platform: PlatformWeCom, url: "https://qyapi.weixin.qq.com@evil.example/hook"},
		{name: "ip loopback", platform: PlatformWeCom, url: "https://127.0.0.1/hook"},
		{name: "ip private", platform: PlatformWeCom, url: "https://10.0.0.1/hook"},
		{name: "localhost", platform: PlatformWeCom, url: "https://localhost/hook"},
		{name: "non standard port", platform: PlatformWeCom, url: "https://qyapi.weixin.qq.com:8443/hook"},
		{name: "trailing dot", platform: PlatformWeCom, url: "https://qyapi.weixin.qq.com./hook"},
		{name: "unsupported platform", platform: Platform("unknown"), url: "https://qyapi.weixin.qq.com/hook"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := validateURL(tt.platform, tt.url); err == nil {
				t.Fatal("validateURL() error = nil, want rejection")
			}
		})
	}
}

func TestSendOverValidatedTLS(t *testing.T) {
	server, roots := newOfficialTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "qyapi.weixin.qq.com" {
			t.Errorf("Host = %q, want qyapi.weixin.qq.com", r.Host)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		assertTextPayload(t, body, "msgtype", "text")
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer server.Close()

	transport := transportForServer(server, roots)
	defer transport.CloseIdleConnections()
	url := "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=test"
	if err := Send(context.Background(), PlatformWeCom, url, "alert content", transport); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
}

func TestSendRejectsUntrustedTLSWithoutMutatingTransport(t *testing.T) {
	server, _ := newOfficialTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer server.Close()

	transport := transportForServer(server, nil)
	transport.TLSClientConfig.InsecureSkipVerify = true
	defer transport.CloseIdleConnections()

	url := "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=test"
	err := Send(context.Background(), PlatformWeCom, url, "alert", transport)
	if err == nil {
		t.Fatal("Send() error = nil, want certificate validation failure")
	}
	if strings.Contains(err.Error(), "key=test") {
		t.Fatalf("Send() leaked the webhook secret in error: %v", err)
	}
	if !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("Send() mutated the caller transport TLS configuration")
	}
}

func TestCloneSecureTransport(t *testing.T) {
	dialTLS := func(context.Context, string, string) (net.Conn, error) {
		return nil, nil
	}
	original := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS10,
			ServerName:         "bypass.example",
		},
		DialTLSContext: dialTLS,
	}

	secured := cloneSecureTransport(original)
	if secured == original || secured.TLSClientConfig == original.TLSClientConfig {
		t.Fatal("cloneSecureTransport() did not clone transport and TLS config")
	}
	if secured.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("InsecureSkipVerify = true, want false")
	}
	if secured.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("MinVersion = %x, want TLS 1.2", secured.TLSClientConfig.MinVersion)
	}
	if secured.TLSClientConfig.ServerName != "" {
		t.Fatalf("ServerName = %q, want request-host validation", secured.TLSClientConfig.ServerName)
	}
	if secured.DialTLSContext != nil || secured.DialTLS != nil {
		t.Fatal("custom TLS dialers must be cleared")
	}
	if !original.TLSClientConfig.InsecureSkipVerify || original.TLSClientConfig.ServerName != "bypass.example" {
		t.Fatal("cloneSecureTransport() mutated the original transport")
	}
}

func TestSendErrors(t *testing.T) {
	tests := []struct {
		name      string
		platform  Platform
		response  string
		wantError string
	}{
		{name: "wecom code", platform: PlatformWeCom, response: `{"errcode":93000,"errmsg":"invalid webhook"}`, wantError: "errcode 93000"},
		{name: "dingtalk code", platform: PlatformDingTalk, response: `{"errcode":310000,"errmsg":"keywords not in content"}`, wantError: "errcode 310000"},
		{name: "feishu code", platform: PlatformFeiShu, response: `{"code":19024,"msg":"Key Words Not Found"}`, wantError: "code 19024"},
		{name: "invalid json", platform: PlatformWeCom, response: `{`, wantError: "decode"},
		{name: "missing code", platform: PlatformFeiShu, response: `{"msg":"unknown"}`, wantError: "missing code"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePlatformResponse(tt.platform, []byte(tt.response))
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("validatePlatformResponse() error = %v, want %q", err, tt.wantError)
			}
		})
	}
}

func TestSendHTTPStatusAndTransportTimeout(t *testing.T) {
	t.Run("http status", func(t *testing.T) {
		server, roots := newOfficialTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("upstream failed: access_token=super-secret"))
		}))
		defer server.Close()
		transport := transportForServer(server, roots)
		defer transport.CloseIdleConnections()

		err := Send(context.Background(), PlatformWeCom, "https://qyapi.weixin.qq.com/hook", "alert", transport)
		if err == nil || !strings.Contains(err.Error(), "HTTP status 502") {
			t.Fatalf("Send() error = %v, want HTTP status error", err)
		}
		if strings.Contains(err.Error(), "super-secret") {
			t.Fatalf("Send() leaked response content in error: %v", err)
		}
	})

	t.Run("transport timeout", func(t *testing.T) {
		server, roots := newOfficialTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(100 * time.Millisecond)
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
		}))
		defer server.Close()
		transport := transportForServer(server, roots)
		transport.ResponseHeaderTimeout = 20 * time.Millisecond
		defer transport.CloseIdleConnections()

		err := Send(context.Background(), PlatformWeCom, "https://qyapi.weixin.qq.com/hook", "alert", transport)
		if err == nil {
			t.Fatal("Send() error = nil, want transport timeout")
		}
	})
}

func TestFeiShuLegacyResponse(t *testing.T) {
	if err := validatePlatformResponse(PlatformFeiShu, []byte(`{"StatusCode":0,"StatusMessage":"success"}`)); err != nil {
		t.Fatalf("validatePlatformResponse() error = %v", err)
	}
}

func assertTextPayload(t *testing.T, body map[string]any, typeKey, contentKey string) {
	t.Helper()
	if got := body[typeKey]; got != "text" {
		t.Fatalf("%s = %#v, want text", typeKey, got)
	}
	content, ok := body[contentKey].(map[string]any)
	if !ok {
		t.Fatalf("%s = %#v, want object", contentKey, body[contentKey])
	}
	if got := content["content"]; contentKey == "text" && got != "alert content" {
		t.Fatalf("text.content = %#v, want alert content", got)
	}
	if got := content["text"]; contentKey == "content" && got != "alert content" {
		t.Fatalf("content.text = %#v, want alert content", got)
	}
}

func newOfficialTLSServer(t *testing.T, handler http.Handler) (*httptest.Server, *x509.CertPool) {
	t.Helper()
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	now := time.Now()
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Webhook Test CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse CA certificate: %v", err)
	}

	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate server key: %v", err)
	}
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "Webhook Test Server"},
		DNSNames: []string{
			"qyapi.weixin.qq.com",
			"oapi.dingtalk.com",
			"open.feishu.cn",
			"open.larksuite.com",
		},
		NotBefore:   now.Add(-time.Hour),
		NotAfter:    now.Add(time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create server certificate: %v", err)
	}
	certificate := tls.Certificate{
		Certificate: [][]byte{serverDER, caDER},
		PrivateKey:  serverKey,
	}

	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{certificate},
		MinVersion:   tls.VersionTLS12,
	}
	server.StartTLS()

	roots := x509.NewCertPool()
	roots.AddCert(caCert)
	return server, roots
}

func transportForServer(server *httptest.Server, roots *x509.CertPool) *http.Transport {
	serverAddress := server.Listener.Addr().String()
	return &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, serverAddress)
		},
		TLSClientConfig: &tls.Config{RootCAs: roots},
	}
}
