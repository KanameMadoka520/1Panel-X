package request

import "time"

// WafEventSearch queries a website's WAF attack events. An empty range defaults
// to the last 7 days; Category optionally filters (e.g. "sqli", "xss").
type WafEventSearch struct {
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`
	Category  string    `json:"category" validate:"omitempty,max=32"`
	Page      int       `json:"page"`
	PageSize  int       `json:"pageSize"`
}
