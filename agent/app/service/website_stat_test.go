package service

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/app/repo"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/utils/common"
	"github.com/1Panel-dev/1Panel/agent/utils/weblog"
)

func setupStatTestDB(t *testing.T) {
	t.Helper()
	db, err := common.LoadDBConnByPathWithErr(filepath.Join(t.TempDir(), "website_stat.db"), "website_stat")
	if err != nil {
		t.Fatalf("open temp stat db: %v", err)
	}
	if err := db.AutoMigrate(&model.WebsiteAccessStat{}, &model.WebsiteAccessRank{}, &model.WebsiteAccessCursor{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	global.WebsiteStatDB = db
}

// The split-bucket fix: when one closed hour is finalized across two runs (e.g.
// the per-run byte cap cuts it), the second run must ACCUMULATE onto the first
// row, never replace/drop it. Proves PV/bytes/status stay exact and the rank
// counts and cursor accumulate/advance.
func TestSaveFinalizedAccumulatesSplitBucket(t *testing.T) {
	setupStatTestDB(t)
	r := repo.NewIWebsiteStatRepo()
	h := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)

	head := []model.WebsiteAccessStat{{WebsiteID: 1, Time: h, Pv: 10, Uv: 3, Bytes: 1000, Status2xx: 8, Status4xx: 2}}
	headRanks := []model.WebsiteAccessRank{{WebsiteID: 1, Time: h, Kind: "uri", RankKey: "/a", Count: 6}}
	if err := r.SaveFinalized(head, headRanks, model.WebsiteAccessCursor{WebsiteID: 1, Path: "/p", Offset: 100}); err != nil {
		t.Fatalf("run1: %v", err)
	}

	tail := []model.WebsiteAccessStat{{WebsiteID: 1, Time: h, Pv: 5, Uv: 2, Bytes: 500, Status2xx: 5}}
	tailRanks := []model.WebsiteAccessRank{{WebsiteID: 1, Time: h, Kind: "uri", RankKey: "/a", Count: 4}}
	if err := r.SaveFinalized(tail, tailRanks, model.WebsiteAccessCursor{WebsiteID: 1, Path: "/p", Offset: 200}); err != nil {
		t.Fatalf("run2: %v", err)
	}

	stats, err := r.SumStats(1, h.Add(-time.Hour), h.Add(time.Hour))
	if err != nil {
		t.Fatalf("sum: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("want 1 bucket, got %d", len(stats))
	}
	if stats[0].Pv != 15 || stats[0].Bytes != 1500 || stats[0].Status2xx != 13 || stats[0].Status4xx != 2 {
		t.Fatalf("split bucket must accumulate, got %+v", stats[0])
	}

	ranks, _ := r.SumRanks(1, "uri", h.Add(-time.Hour), h.Add(time.Hour), 10)
	if len(ranks) != 1 || ranks[0].Count != 10 {
		t.Fatalf("rank count must accumulate to 10: %+v", ranks)
	}

	c, err := r.GetCursor(1)
	if err != nil || c.Offset != 200 {
		t.Fatalf("cursor should advance to 200: %+v err=%v", c, err)
	}
}

func TestBuildRowsGroupsByHour(t *testing.T) {
	resolve := func(string) string { return "" }
	lines := []string{
		`1.1.1.1 - - [15/Jul/2026:10:00:05 +0000] "GET /a HTTP/1.1" 200 100 "-" "UA" "-"`,
		`2.2.2.2 - - [15/Jul/2026:10:30:00 +0000] "GET /a HTTP/1.1" 200 100 "-" "UA" "-"`,
		`3.3.3.3 - - [15/Jul/2026:11:05:00 +0000] "GET /b HTTP/1.1" 404 50 "-" "UA" "-"`,
	}
	var entries []weblog.AccessEntry
	for _, l := range lines {
		e, ok := weblog.ParseLine(l)
		if !ok {
			t.Fatalf("fixture parse failed: %s", l)
		}
		entries = append(entries, e)
	}
	stats, _ := buildRows(1, entries, resolve)
	if len(stats) != 2 {
		t.Fatalf("want 2 hourly buckets, got %d", len(stats))
	}
	byPv := map[int64]int64{}
	for _, s := range stats {
		byPv[s.Time.Unix()] = s.Pv
	}
	h10 := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC).Unix()
	h11 := time.Date(2026, 7, 15, 11, 0, 0, 0, time.UTC).Unix()
	if byPv[h10] != 2 || byPv[h11] != 1 {
		t.Fatalf("hourly bucket PVs wrong: %+v", byPv)
	}
}
