package gateway

import (
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
