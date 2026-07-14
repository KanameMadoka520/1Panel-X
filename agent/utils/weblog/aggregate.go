package weblog

import (
	"sort"
	"strconv"
	"time"
)

// Rank kinds.
const (
	RankURI     = "uri"
	RankIP      = "ip"
	RankReferer = "referer"
	RankStatus  = "status"
	RankRegion  = "region"
)

// Bucket is per-(website, time-bucket) aggregated traffic. Uv is the count of
// distinct IPs seen in this batch for the bucket (see the design doc: accurate
// cross-flush UV requires the tailer to flush on bucket boundaries).
type Bucket struct {
	Time      time.Time
	Pv        int64
	Uv        int64
	Bytes     int64
	Status2xx int64
	Status3xx int64
	Status4xx int64
	Status5xx int64
}

// RankItem is one top-N entry for a kind (uri/ip/referer/status).
type RankItem struct {
	Kind  string
	Key   string
	Count int64
}

// Aggregate turns parsed entries into time-series buckets + top-N rankings.
// Pure: no I/O, deterministic (ties broken by key), safe on empty input.
func Aggregate(entries []AccessEntry, bucketSize time.Duration, topN int) ([]Bucket, []RankItem) {
	if bucketSize <= 0 {
		bucketSize = time.Minute
	}
	buckets := map[int64]*Bucket{}
	bucketIPs := map[int64]map[string]struct{}{}
	rankCounts := map[string]map[string]int64{
		RankURI:     {},
		RankIP:      {},
		RankReferer: {},
		RankStatus:  {},
	}

	for _, e := range entries {
		key := e.Time.Truncate(bucketSize).Unix()
		b := buckets[key]
		if b == nil {
			b = &Bucket{Time: time.Unix(key, 0).UTC()}
			buckets[key] = b
			bucketIPs[key] = map[string]struct{}{}
		}
		b.Pv++
		b.Bytes += e.Bytes
		switch {
		case e.Status >= 200 && e.Status < 300:
			b.Status2xx++
		case e.Status >= 300 && e.Status < 400:
			b.Status3xx++
		case e.Status >= 400 && e.Status < 500:
			b.Status4xx++
		case e.Status >= 500:
			b.Status5xx++
		}
		if e.IP != "" {
			if _, seen := bucketIPs[key][e.IP]; !seen {
				bucketIPs[key][e.IP] = struct{}{}
				b.Uv++
			}
			rankCounts[RankIP][e.IP]++
		}
		if e.URI != "" {
			rankCounts[RankURI][e.URI]++
		}
		if e.Referer != "" {
			rankCounts[RankReferer][e.Referer]++
		}
		rankCounts[RankStatus][strconv.Itoa(e.Status)]++
	}

	out := make([]Bucket, 0, len(buckets))
	for _, b := range buckets {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })

	var ranks []RankItem
	for _, kind := range []string{RankURI, RankIP, RankReferer, RankStatus} {
		ranks = append(ranks, topRanks(kind, rankCounts[kind], topN)...)
	}
	return out, ranks
}

// RegionRanks builds a top-N region ranking from entries. The geo lookup is
// injected as resolve(ip) so this package stays pure and dependency-free (the
// caller passes an agent/utils/geo-backed resolver, or a no-op when the GeoIP
// database is absent — in which case this returns nil). A resolve that returns
// "" (unknown) is skipped, so a missing mmdb degrades gracefully to no region
// data rather than a wrong one.
func RegionRanks(entries []AccessEntry, resolve func(ip string) string, topN int) []RankItem {
	if resolve == nil {
		return nil
	}
	counts := map[string]int64{}
	cache := map[string]string{}
	for _, e := range entries {
		if e.IP == "" {
			continue
		}
		region, ok := cache[e.IP]
		if !ok {
			region = resolve(e.IP)
			cache[e.IP] = region
		}
		if region == "" {
			continue
		}
		counts[region]++
	}
	return topRanks(RankRegion, counts, topN)
}

func topRanks(kind string, counts map[string]int64, topN int) []RankItem {
	items := make([]RankItem, 0, len(counts))
	for k, c := range counts {
		items = append(items, RankItem{Kind: kind, Key: k, Count: c})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count != items[j].Count {
			return items[i].Count > items[j].Count
		}
		return items[i].Key < items[j].Key // deterministic tiebreak
	})
	if topN > 0 && len(items) > topN {
		items = items[:topN]
	}
	return items
}
