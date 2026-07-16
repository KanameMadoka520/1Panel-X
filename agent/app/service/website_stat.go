package service

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/dto/request"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/app/repo"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/utils/geo"
	"github.com/1Panel-dev/1Panel/agent/utils/weblog"
)

// statBucket is the granularity of both the time-series buckets and the ranking
// windows. Hourly keeps row counts bounded (24/site/day) and, combined with the
// tailer's hold-back of the still-open bucket, makes per-bucket Uv exact.
const statBucket = time.Hour

// rankStoreTopN is how many entries we keep per (bucket, kind). Query-time
// overall rankings sum these per key, so keeping a generous N makes the overall
// top-20 accurate.
const rankStoreTopN = 50

// maxBytesPerRun bounds how much of one site's access log we ingest per run so
// a huge backlog cannot stall the collector or exhaust memory; the remainder is
// picked up on the next run (the cursor only advances over finalized lines).
const maxBytesPerRun = 32 << 20 // 32 MiB

// defaultRetentionDays is used when the WebsiteStatRetentionDays setting is
// absent or invalid.
const defaultRetentionDays = 30

type WebsiteStatService struct{}

type IWebsiteStatService interface {
	Collect()
	LoadStat(websiteID uint, req request.WebsiteMonitorSearch) ([]model.WebsiteAccessStat, error)
	LoadRank(websiteID uint, req request.WebsiteMonitorSearch) ([]model.WebsiteAccessRank, error)
}

func NewIWebsiteStatService() IWebsiteStatService {
	return &WebsiteStatService{}
}

// Collect tails every access-logging website's log, finalizes closed buckets,
// and prunes old data. Invoked on a schedule by the website-stat cron job. It
// is best-effort: a failure on one site is logged and does not abort the rest.
func (s *WebsiteStatService) Collect() {
	if global.WebsiteStatDB == nil {
		return
	}
	websites, err := repo.NewIWebsiteRepo().List()
	if err != nil {
		global.LOG.Errorf("website-stat: list websites failed: %v", err)
		return
	}

	// One GeoIP reader for the whole pass; nil (skipped) when the mmdb is absent
	// so region data degrades gracefully instead of erroring.
	var resolve func(ip string) string
	if reader, gerr := geo.NewGeo(); gerr == nil {
		defer func() { _ = reader.Close() }()
		resolve = func(ip string) string {
			loc, lerr := geo.GetIPLocation(reader, ip, "en")
			if lerr != nil {
				return ""
			}
			return strings.TrimSpace(loc)
		}
	}

	statRepo := repo.NewIWebsiteStatRepo()
	for i := range websites {
		w := websites[i]
		// Honest gate: only sites that actually have access logging on, and only
		// http(s) sites (stream logs use a different, non-"main" format).
		if !w.AccessLog || w.Type == constant.Stream {
			continue
		}
		if err := s.collectSite(statRepo, w, resolve); err != nil {
			global.LOG.Errorf("website-stat: collect site %s failed: %v", w.PrimaryDomain, err)
		}
	}

	s.prune(statRepo)
}

func (s *WebsiteStatService) collectSite(statRepo repo.IWebsiteStatRepo, w model.Website, resolve func(string) string) error {
	logPath := GetSitePath(w, SiteAccessLog)
	info, err := os.Stat(logPath)
	if err != nil {
		return nil // no log yet (site never received traffic) — not an error
	}

	offset := int64(0)
	if cursor, cerr := statRepo.GetCursor(w.ID); cerr == nil {
		offset = cursor.Offset
	}
	if info.Size() < offset { // rotated/truncated → re-read from the start
		// Known limitation: the still-open (held-back) hour that lives only in the
		// rotated-away file/backup is not recovered — up to ~1 hour of stats can be
		// lost per rotation. The tailer reads only the live access.log; reading the
		// rotated sibling/gz backup is a future enhancement.
		offset = 0
	}
	if info.Size() == offset {
		return nil // nothing new
	}

	f, err := os.Open(logPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Seek(offset, 0); err != nil {
		return err
	}

	reader := bufio.NewReader(f)
	pos := offset
	var read int64
	var lines []weblog.LineAt
	for {
		raw, rerr := reader.ReadString('\n')
		if rerr != nil {
			// EOF or a partial trailing line (still being written): stop without
			// consuming it so it is re-read complete next run.
			break
		}
		pos += int64(len(raw))
		read += int64(len(raw))
		if e, ok := weblog.ParseLine(strings.TrimRight(raw, "\r\n")); ok {
			lines = append(lines, weblog.LineAt{Entry: e, End: pos})
		}
		if read >= maxBytesPerRun {
			break
		}
	}

	closed, advanceTo, hasClosed := weblog.PartitionClosed(lines, time.Now().UTC(), statBucket)
	if !hasClosed {
		return nil // no bucket has closed yet; keep the cursor, persist nothing
	}

	statRows, rankRows := buildRows(w.ID, closed, resolve)

	// Additive finalize in ONE transaction: each bucket row is upsert-accumulated
	// (+=) and the cursor is advanced atomically. This is both crash-safe (a
	// rollback leaves the cursor un-advanced, so the same lines simply
	// re-accumulate correctly next run) and correct when the per-run byte cap
	// (maxBytesPerRun) splits a single closed hour across runs: the later run
	// accumulates onto the earlier row instead of replacing it, so PV/bytes/status
	// stay exact. (Uv is per-flush distinct — exact for the common single-flush
	// hour, a small over-count only for a hour split across runs.)
	return statRepo.SaveFinalized(statRows, rankRows,
		model.WebsiteAccessCursor{WebsiteID: w.ID, Path: logPath, Offset: advanceTo})
}

// buildRows groups finalized entries by hour bucket and, per bucket, produces
// one stat row plus its top-N rank rows (uri/ip/referer/status via the pure
// aggregator, region via the injected geo resolver).
func buildRows(websiteID uint, entries []weblog.AccessEntry, resolve func(string) string) (stats []model.WebsiteAccessStat, ranks []model.WebsiteAccessRank) {
	groups := map[int64][]weblog.AccessEntry{}
	for _, e := range entries {
		k := e.Time.Truncate(statBucket).Unix()
		groups[k] = append(groups[k], e)
	}
	for k, group := range groups {
		bt := time.Unix(k, 0).UTC()

		buckets, rk := weblog.Aggregate(group, statBucket, rankStoreTopN)
		for _, b := range buckets { // exactly one bucket per single-hour group
			stats = append(stats, model.WebsiteAccessStat{
				WebsiteID: websiteID, Time: bt,
				Pv: b.Pv, Uv: b.Uv, Bytes: b.Bytes,
				Status2xx: b.Status2xx, Status3xx: b.Status3xx,
				Status4xx: b.Status4xx, Status5xx: b.Status5xx,
			})
		}
		rk = append(rk, weblog.RegionRanks(group, resolve, rankStoreTopN)...)
		for _, it := range rk {
			ranks = append(ranks, model.WebsiteAccessRank{
				WebsiteID: websiteID, Time: bt,
				Kind: it.Kind, RankKey: it.Key, Count: it.Count,
			})
		}
	}
	return stats, ranks
}

func (s *WebsiteStatService) prune(statRepo repo.IWebsiteStatRepo) {
	days := defaultRetentionDays
	if setting, err := repo.NewISettingRepo().Get(repo.NewISettingRepo().WithByKey("WebsiteStatRetentionDays")); err == nil {
		if n, e := strconv.Atoi(setting.Value); e == nil && n > 0 {
			days = n
		}
	}
	if err := statRepo.PruneBefore(time.Now().UTC().AddDate(0, 0, -days)); err != nil {
		global.LOG.Errorf("website-stat: prune failed: %v", err)
	}
}

func (s *WebsiteStatService) LoadStat(websiteID uint, req request.WebsiteMonitorSearch) ([]model.WebsiteAccessStat, error) {
	start, end := normalizeRange(req.StartTime, req.EndTime)
	return repo.NewIWebsiteStatRepo().SumStats(websiteID, start, end)
}

func (s *WebsiteStatService) LoadRank(websiteID uint, req request.WebsiteMonitorSearch) ([]model.WebsiteAccessRank, error) {
	start, end := normalizeRange(req.StartTime, req.EndTime)
	kind := req.Kind
	if kind == "" {
		kind = weblog.RankURI
	}
	top := req.Top
	if top <= 0 || top > 100 {
		top = 20
	}
	return repo.NewIWebsiteStatRepo().SumRanks(websiteID, kind, start, end, top)
}

func normalizeRange(start, end time.Time) (time.Time, time.Time) {
	if end.IsZero() {
		end = time.Now().UTC()
	}
	if start.IsZero() {
		start = end.AddDate(0, 0, -7)
	}
	return start.UTC(), end.UTC()
}
