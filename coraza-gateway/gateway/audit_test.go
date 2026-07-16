package gateway

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A blocked attack must leave a JSON audit record the agent can tail into the
// attack-event store (Phase 20).
func TestAuditLogEmitsRecordOnBlock(t *testing.T) {
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.log")

	eng, err := NewEngineWithAudit(ModeBlock, 1<<20, auditPath)
	if err != nil {
		t.Fatalf("engine: %v", err)
	}
	reached := false
	h := NewHandler(eng, recordingUpstream(&reached), ModeBlock)

	rr := serve(h, "GET", sqliTarget, "")
	if rr.Code != 403 {
		t.Fatalf("expected block 403, got %d", rr.Code)
	}
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("audit log empty after a blocked attack")
	}
	t.Logf("AUDIT-RECORD-SCHEMA:\n%s", string(data))
	if !strings.Contains(string(data), `"id"`) {
		t.Errorf("audit json missing rule id: %s", data)
	}
}

// The real client IP behind the proxy is recovered from the trusted header and
// used for evaluation/logging, not the loopback address the proxy connects from.
func TestRealIPHeaderRecoversClientIP(t *testing.T) {
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.log")
	eng, err := NewEngineWithAudit(ModeBlock, 1<<20, auditPath)
	if err != nil {
		t.Fatalf("engine: %v", err)
	}
	reached := false
	h := NewHandler(eng, recordingUpstream(&reached), ModeBlock).WithRealIPHeader("X-Real-IP")

	req := httptest.NewRequest("GET", sqliTarget, nil)
	req.RemoteAddr = "127.0.0.1:5555" // the nginx loopback connection
	req.Header.Set("X-Real-IP", "203.0.113.7")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 403 {
		t.Fatalf("attack should still block: %d", rr.Code)
	}
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if !strings.Contains(string(data), `"client_ip":"203.0.113.7"`) {
		t.Fatalf("audit must record the real client IP, not the proxy: %s", data)
	}
	if strings.Contains(string(data), "127.0.0.1") {
		t.Fatalf("audit must not record the proxy loopback IP: %s", data)
	}
}
