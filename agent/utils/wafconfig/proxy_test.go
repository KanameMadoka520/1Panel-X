package wafconfig

import (
	"strings"
	"testing"
)

func TestManagedDirectiveRestoresPreviousValue(t *testing.T) {
	for _, original := range []string{
		`location ^~ / {
    proxy_pass http://127.0.0.1:8080;
}`,
		`location ^~ / {
    client_max_body_size 50m;
    proxy_pass http://127.0.0.1:8080;
}`,
	} {
		managed, err := EnsureManagedDirective(original, "client_max_body_size", "13m")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(managed, "client_max_body_size 13m; "+managedDirectiveMarker) {
			t.Fatalf("managed marker missing: %s", managed)
		}
		restored, err := RestoreManagedDirective(managed, "client_max_body_size")
		if err != nil || restored != original {
			t.Fatalf("managed restore mismatch: %q err=%v", restored, err)
		}
	}
}

func TestEnsureDirectiveAddsAndUpdatesBodyLimit(t *testing.T) {
	original := `location ^~ / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
}`
	added, err := EnsureDirective(original, "client_max_body_size", "13m")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(added, "    client_max_body_size 13m;\n    proxy_pass") {
		t.Fatalf("directive was not inserted before proxy_pass: %s", added)
	}
	updated, err := EnsureDirective(strings.Replace(added, "13m", "20m", 1), "client_max_body_size", "13m")
	if err != nil {
		t.Fatal(err)
	}
	removed, err := RemoveDirective(updated, "client_max_body_size")
	if err != nil || removed != original {
		t.Fatalf("directive removal did not restore original: %q err=%v", removed, err)
	}
}

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
