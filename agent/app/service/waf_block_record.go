package service

import (
	"bufio"
	"encoding/json"
	"os"
	"path"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/dto/request"
	"github.com/1Panel-dev/1Panel/agent/app/dto/response"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/app/repo"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/utils/weblog"
)

// The gateway decides plenty on its own — deny lists, custom rules, region
// policy, frequency limits, bans, challenges, unknown hosts, oversize bodies —
// and none of it appears in the rule set's audit log. Those decisions go to a
// separate journal, and this is the tailer that brings them into the panel.
//
// Without it every one of those controls would enforce correctly and remain
// completely invisible, which is the same as not being able to tell whether they
// work at all.

// GetWafEventLogPath is where the gateway appends its enforcement records.
func GetWafEventLogPath() string {
	return path.Join(global.Dir.DataDir, "waf", "audit", "events.log")
}

type IWafBlockRecordService interface {
	Collect()
	Load(websiteID uint, req request.WafBlockSearch) (dto.PageResult, error)
}

type WafBlockRecordService struct{}

func NewIWafBlockRecordService() IWafBlockRecordService {
	return &WafBlockRecordService{}
}

// Collect tails the enforcement journal. Best-effort: it no-ops quietly when the
// gateway is not deployed yet.
func (s *WafBlockRecordService) Collect() {
	if global.WafDB == nil {
		return
	}
	logPath := GetWafEventLogPath()
	info, err := os.Stat(logPath)
	if err != nil {
		return
	}

	wafRepo := repo.NewIWafRepo()
	offset := int64(0)
	if cursor, cerr := wafRepo.GetCursor(logPath); cerr == nil {
		offset = cursor.Offset
	}
	// The gateway stops writing rather than rotating, so a file SHORTER than the
	// cursor means it was replaced or truncated out from under us; re-reading from
	// the start is the only way not to skip whatever is there now.
	if info.Size() < offset {
		offset = 0
	}
	if info.Size() == offset {
		return
	}

	f, err := os.Open(logPath)
	if err != nil {
		global.LOG.Errorf("waf-block: open event log failed: %v", err)
		return
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Seek(offset, 0); err != nil {
		return
	}

	reader := bufio.NewReader(f)
	pos := offset
	var read int64
	var lines []string
	for {
		raw, rerr := reader.ReadString('\n')
		if rerr != nil {
			// A partial trailing line is left for the next run: the cursor does not
			// advance past it, so nothing is lost and nothing is half-parsed.
			break
		}
		pos += int64(len(raw))
		read += int64(len(raw))
		lines = append(lines, raw)
		if read >= wafMaxBytesPerRun {
			break
		}
	}

	records := buildBlockRecords(lines, hostResolver())
	if err := wafRepo.BatchCreateBlocks(records); err != nil {
		global.LOG.Errorf("waf-block: store failed: %v", err)
		return // keep the cursor so these lines are retried next run
	}
	_ = wafRepo.SaveCursor(model.WafAuditCursor{Path: logPath, Offset: pos})
	if err := wafRepo.PruneBlocksBefore(time.Now().UTC().AddDate(0, 0, -wafRetentionDays(wafRepo))); err != nil {
		global.LOG.Errorf("waf-block: prune failed: %v", err)
	}
}

// clip bounds a field to what its column can hold. weblog.Clean already strips
// control characters and applies its own generous ceiling; this is the per-column
// cut, so a long URI is truncated rather than failing the whole insert.
func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// journalLine mirrors the gateway's record shape.
type journalLine struct {
	ID        string    `json:"id"`
	Time      time.Time `json:"time"`
	Kind      string    `json:"kind"`
	WebsiteID uint      `json:"websiteId"`
	Host      string    `json:"host"`
	ClientIP  string    `json:"clientIp"`
	Method    string    `json:"method"`
	URI       string    `json:"uri"`
	Rule      string    `json:"rule"`
	Action    string    `json:"action"`
	Detail    string    `json:"detail"`
}

// buildBlockRecords parses journal lines into storable records.
//
// Every attacker-influenced field is sanitized again here even though the
// gateway truncated it: this side is what writes to the database and renders in
// a browser, so it does not get to assume the other side did its job.
func buildBlockRecords(lines []string, resolve func(host string) uint) []model.WafBlockRecord {
	out := make([]model.WafBlockRecord, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		var e journalLine
		if err := json.Unmarshal([]byte(ln), &e); err != nil {
			// One malformed line must not stop the rest from being ingested.
			continue
		}
		if e.ID == "" || e.Kind == "" {
			continue
		}
		host := clip(weblog.Clean(e.Host), 253)
		websiteID := e.WebsiteID
		if websiteID == 0 && host != "" {
			websiteID = resolve(host)
		}
		when := e.Time
		if when.IsZero() {
			when = time.Now().UTC()
		}
		out = append(out, model.WafBlockRecord{
			RecordID:  clip(weblog.Clean(e.ID), 64),
			WebsiteID: websiteID,
			Time:      when.UTC(),
			Kind:      clip(weblog.Clean(e.Kind), 32),
			Host:      host,
			SourceIP:  clip(weblog.Clean(e.ClientIP), 45),
			Method:    clip(weblog.Clean(e.Method), 16),
			URI:       clip(weblog.Clean(e.URI), 1024),
			Rule:      clip(weblog.Clean(e.Rule), 128),
			Action:    clip(weblog.Clean(e.Action), 16),
			Detail:    clip(weblog.Clean(e.Detail), 256),
		})
	}
	return out
}

func (s *WafBlockRecordService) Load(websiteID uint, req request.WafBlockSearch) (dto.PageResult, error) {
	start, end := normalizeRange(req.StartTime, req.EndTime)
	page, size := req.Page, req.PageSize
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 500 {
		size = 50
	}
	rows, total, err := repo.NewIWafRepo().ListBlocks(
		websiteID, start, end, strings.TrimSpace(req.Kind), size, (page-1)*size)
	if err != nil {
		return dto.PageResult{}, err
	}
	items := make([]response.WafBlockRecord, 0, len(rows))
	for _, r := range rows {
		items = append(items, response.WafBlockRecord{
			ID:       r.ID,
			Time:     r.Time,
			Kind:     r.Kind,
			Host:     r.Host,
			SourceIP: r.SourceIP,
			Method:   r.Method,
			URI:      r.URI,
			Rule:     r.Rule,
			Action:   r.Action,
			Detail:   r.Detail,
		})
	}
	return dto.PageResult{Total: total, Items: items}, nil
}
