package request

import "time"

// WebsiteMonitorSearch is the query for a website's access-monitoring data.
// All fields are optional: an empty range defaults to the last 7 days; Kind
// defaults to "uri" and Top to 20 for rank queries.
type WebsiteMonitorSearch struct {
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`
	Kind      string    `json:"kind" validate:"omitempty,oneof=uri ip referer status region"`
	Top       int       `json:"top"`
}
