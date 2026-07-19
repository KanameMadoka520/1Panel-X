package wafconfig

import "testing"

func TestReplaceProxyPassPreservesLocationDirectives(t *testing.T) {
	old := `location ^~ / {
    proxy_pass http://origin.internal:8080;
    proxy_set_header Host $host;
    proxy_set_header Upgrade $http_upgrade;
    proxy_ssl_server_name on;
}`
	updated, origin, err := ReplaceProxyPass(old, "http://waf-gateway:9000")
	if err != nil {
		t.Fatal(err)
	}
	if origin != "http://origin.internal:8080" {
		t.Fatalf("origin=%q", origin)
	}
	if want := "proxy_pass http://waf-gateway:9000;"; !contains(updated, want) {
		t.Fatalf("replacement missing: %s", updated)
	}
	for _, want := range []string{"proxy_set_header Host $host;", "proxy_set_header Upgrade $http_upgrade;", "proxy_ssl_server_name on;"} {
		if !contains(updated, want) {
			t.Fatalf("unrelated directive lost: %s", want)
		}
	}
	restored, err := RestoreProxyPass(updated, origin)
	if err != nil || restored != old {
		t.Fatalf("restore mismatch: err=%v restored=%q old=%q", err, restored, old)
	}
}

func TestReplaceProxyPassRejectsAmbiguousOrUnsafeInput(t *testing.T) {
	for _, content := range []string{
		"location / { proxy_pass http://a; }\nlocation /x { proxy_pass http://b; }",
		"location / { root /www; }",
	} {
		if _, _, err := ReplaceProxyPass(content, "http://waf:9000"); err == nil {
			t.Errorf("expected ambiguous content to fail: %q", content)
		}
	}
	for _, replacement := range []string{"", "http://waf:9000; proxy_pass http://evil", "http://waf:9000\nproxy_pass http://evil"} {
		if _, _, err := ReplaceProxyPass("proxy_pass http://origin;", replacement); err == nil {
			t.Errorf("unsafe replacement should fail: %q", replacement)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
