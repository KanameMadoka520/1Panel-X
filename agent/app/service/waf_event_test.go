package service

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/app/repo"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/utils/common"
)

func setupWafTestDB(t *testing.T) {
	t.Helper()
	db, err := common.LoadDBConnByPathWithErr(filepath.Join(t.TempDir(), "waf.db"), "waf")
	if err != nil {
		t.Fatalf("open temp waf db: %v", err)
	}
	if err := db.AutoMigrate(&model.WafAttackEvent{}, &model.WafAuditCursor{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	global.WafDB = db
}

func TestBuildEventsMapsHostAndAction(t *testing.T) {
	lines := [][]byte{
		[]byte(`{"transaction":{"id":"tx-a","unix_timestamp":1,"client_ip":"1.1.1.1","request":{"method":"GET","uri":"/a","headers":{"host":["a.example"]}},"is_interrupted":true},"messages":[{"error_message":"[id \"942100\"] [msg \"SQLi\"] [severity \"critical\"] [tag \"attack-sqli\"]"}]}`),
		[]byte(`{"transaction":{"id":"tx-b","unix_timestamp":2,"client_ip":"2.2.2.2","request":{"method":"GET","uri":"/b","headers":{"host":["b.example"]}},"is_interrupted":false},"messages":[{"error_message":"[id \"941100\"] [msg \"XSS\"] [severity \"critical\"] [tag \"attack-xss\"]"}]}`),
		[]byte(`not a json record`), // must be skipped, not crash
	}
	resolve := func(host string) uint {
		if host == "a.example" {
			return 7
		}
		return 0
	}
	evs := buildEvents(lines, resolve)
	if len(evs) != 2 {
		t.Fatalf("want 2 events (garbage skipped), got %d", len(evs))
	}
	if evs[0].WebsiteID != 7 || evs[0].Action != "blocked" || evs[0].Category != "sqli" {
		t.Fatalf("event 0 wrong: %+v", evs[0])
	}
	if evs[1].WebsiteID != 0 || evs[1].Action != "detected" || evs[1].Category != "xss" {
		t.Fatalf("event 1 wrong: %+v", evs[1])
	}
	if evs[0].TxID != "tx-a" || evs[1].TxID != "tx-b" {
		t.Fatalf("transaction id not carried: %q %q", evs[0].TxID, evs[1].TxID)
	}
}

// Re-ingesting the same audit records (crash between BatchCreate and cursor save)
// must not duplicate events — deduped on the Coraza transaction id.
func TestWafIngestIdempotent(t *testing.T) {
	setupWafTestDB(t)
	wafRepo := repo.NewIWafRepo()
	now := time.Now().UTC()
	events := []model.WafAttackEvent{
		{TxID: "tx-1", WebsiteID: 1, Time: now, Category: "sqli", Action: "blocked"},
		{TxID: "tx-2", WebsiteID: 1, Time: now, Category: "xss", Action: "blocked"},
	}
	if err := wafRepo.BatchCreate(events); err != nil {
		t.Fatalf("create 1: %v", err)
	}
	if err := wafRepo.BatchCreate(events); err != nil {
		t.Fatalf("create 2 (retry): %v", err)
	}
	_, total, err := wafRepo.List(1, now.AddDate(0, 0, -1), now.Add(time.Minute), "", 0, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 2 {
		t.Fatalf("idempotent ingest must keep 2 (not 4), got %d", total)
	}
}

func TestWafStoreListFilterAndPrune(t *testing.T) {
	setupWafTestDB(t)
	wafRepo := repo.NewIWafRepo()
	now := time.Now().UTC()
	events := []model.WafAttackEvent{
		{TxID: "e1", WebsiteID: 1, Time: now.Add(-1 * time.Hour), Category: "sqli", SourceIP: "1.1.1.1", Action: "blocked"},
		{TxID: "e2", WebsiteID: 1, Time: now.Add(-2 * time.Hour), Category: "xss", SourceIP: "2.2.2.2", Action: "blocked"},
		{TxID: "e3", WebsiteID: 2, Time: now.Add(-1 * time.Hour), Category: "sqli", SourceIP: "3.3.3.3", Action: "blocked"},
		{TxID: "e4", WebsiteID: 1, Time: now.AddDate(0, 0, -60), Category: "sqli", SourceIP: "old", Action: "blocked"},
	}
	if err := wafRepo.BatchCreate(events); err != nil {
		t.Fatalf("create: %v", err)
	}

	weekAgo, soon := now.AddDate(0, 0, -7), now.Add(time.Minute)
	items, total, err := wafRepo.List(1, weekAgo, soon, "", 50, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("site1 within a week should be 2, got total=%d len=%d", total, len(items))
	}
	if !items[0].Time.After(items[1].Time) {
		t.Fatal("events must be newest-first")
	}

	if _, totalSqli, _ := wafRepo.List(1, weekAgo, soon, "sqli", 50, 0); totalSqli != 1 {
		t.Fatalf("site1 sqli should be 1, got %d", totalSqli)
	}

	if err := wafRepo.PruneBefore(now.AddDate(0, 0, -30)); err != nil {
		t.Fatalf("prune: %v", err)
	}
	if _, totalAll, _ := wafRepo.List(1, now.AddDate(0, 0, -365), soon, "", 0, 0); totalAll != 2 {
		t.Fatalf("after prune site1 should keep 2 (60-day-old gone), got %d", totalAll)
	}

	if err := wafRepo.SaveCursor(model.WafAuditCursor{Path: "/p", Offset: 100}); err != nil {
		t.Fatalf("cursor: %v", err)
	}
	if err := wafRepo.SaveCursor(model.WafAuditCursor{Path: "/p", Offset: 250}); err != nil {
		t.Fatalf("cursor upsert: %v", err)
	}
	c, err := wafRepo.GetCursor("/p")
	if err != nil || c.Offset != 250 {
		t.Fatalf("cursor should upsert to 250: %+v err=%v", c, err)
	}
}
