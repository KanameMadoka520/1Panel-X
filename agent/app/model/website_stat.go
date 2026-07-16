package model

import "time"

// WebsiteAccessStat is one finalized time-bucket rollup of a website's access
// log (see agent/utils/weblog). One row per (website, bucket-start). The bucket
// is finalized only once its time window has closed, so Uv (distinct IPs) is
// exact for the common case (a single flush per bucket); see the tailer's
// hold-back logic in service/website_stat.go.
type WebsiteAccessStat struct {
	BaseModel
	WebsiteID uint      `gorm:"uniqueIndex:idx_stat_site_time" json:"websiteID"`
	Time      time.Time `gorm:"uniqueIndex:idx_stat_site_time" json:"time"`
	Pv        int64     `json:"pv"`
	Uv        int64     `json:"uv"`
	Bytes     int64     `json:"bytes"`
	Status2xx int64     `json:"status2xx"`
	Status3xx int64     `json:"status3xx"`
	Status4xx int64     `json:"status4xx"`
	Status5xx int64     `json:"status5xx"`
}

// WebsiteAccessRank is a top-N ranking row for a website within one bucket
// window. Kind is uri|ip|referer|status|region; Count is the hit count of Key
// within that window. Overall (multi-window) rankings are produced at query
// time by summing Count grouped by (kind, key).
type WebsiteAccessRank struct {
	BaseModel
	WebsiteID uint      `gorm:"uniqueIndex:idx_rank_key" json:"websiteID"`
	Time      time.Time `gorm:"uniqueIndex:idx_rank_key" json:"time"`
	Kind      string    `gorm:"uniqueIndex:idx_rank_key" json:"kind"`
	RankKey   string    `gorm:"uniqueIndex:idx_rank_key" json:"key"`
	Count     int64     `json:"count"`
}

// WebsiteAccessCursor tracks how far a site's access.log has been ingested so
// the tailer only reads new bytes. Offset is the byte position of the last
// finalized (closed-bucket) line; a file whose size drops below Offset is
// treated as rotated/truncated and re-read from 0.
type WebsiteAccessCursor struct {
	BaseModel
	WebsiteID uint   `gorm:"uniqueIndex" json:"websiteID"`
	Path      string `json:"path"`
	Offset    int64  `json:"offset"`
}
