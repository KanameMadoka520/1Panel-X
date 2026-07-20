package service

import (
	"bufio"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/dto/request"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/app/repo"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/utils/wafaudit"
)

// wafMaxBytesPerRun bounds how much of the audit log we ingest per run.
const wafMaxBytesPerRun = 32 << 20

// wafDefaultRetentionDays is used when the WafRetentionDays setting is absent.
const wafDefaultRetentionDays = 30

type WafEventService struct{}

type IWafEventService interface {
	Collect()
	LoadEvents(websiteID uint, req request.WafEventSearch) (dto.PageResult, error)
}

func NewIWafEventService() IWafEventService {
	return &WafEventService{}
}

// GetWafAuditLogPath is the shared path the coraza-gateway appends JSON audit
// records to (mapped into the gateway container by the Phase 21 packaging) and
// the agent tails here.
func GetWafAuditLogPath() string {
	return path.Join(global.Dir.DataDir, "waf", "audit", "audit.log")
}

// Collect tails the gateway audit log, ingests new attack events, and prunes old
// ones. Invoked on a schedule by the waf cron job. Best-effort: it no-ops
// quietly when the gateway is not deployed yet (no audit file).
func (s *WafEventService) Collect() {
	if global.WafDB == nil {
		return
	}
	logPath := GetWafAuditLogPath()
	info, err := os.Stat(logPath)
	if err != nil {
		return // gateway not deployed / no traffic yet
	}

	wafRepo := repo.NewIWafRepo()
	offset := int64(0)
	if cursor, cerr := wafRepo.GetCursor(logPath); cerr == nil {
		offset = cursor.Offset
	}
	if info.Size() < offset { // rotated/truncated
		offset = 0
	}
	if info.Size() == offset {
		s.prune(wafRepo)
		return
	}

	f, err := os.Open(logPath)
	if err != nil {
		global.LOG.Errorf("waf-event: open audit log failed: %v", err)
		return
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Seek(offset, 0); err != nil {
		return
	}

	reader := bufio.NewReader(f)
	pos := offset
	var read int64
	var lines [][]byte
	for {
		raw, rerr := reader.ReadString('\n')
		if rerr != nil {
			break // EOF / partial trailing line: leave it for next run
		}
		pos += int64(len(raw))
		read += int64(len(raw))
		lines = append(lines, []byte(raw))
		if read >= wafMaxBytesPerRun {
			break
		}
	}

	events := buildEvents(lines, hostResolver())
	if err := wafRepo.BatchCreate(events); err != nil {
		global.LOG.Errorf("waf-event: store failed: %v", err)
		return // keep the cursor so we retry these lines next run
	}
	_ = wafRepo.SaveCursor(model.WafAuditCursor{Path: logPath, Offset: pos})
	s.prune(wafRepo)
}

// buildEvents parses audit lines into storable events, mapping each event's Host
// to a website id via resolve. Pure aside from resolve; unit-tested.
func buildEvents(lines [][]byte, resolve func(host string) uint) []model.WafAttackEvent {
	out := make([]model.WafAttackEvent, 0, len(lines))
	for _, ln := range lines {
		ev, ok := wafaudit.ParseRecord(ln)
		if !ok {
			continue
		}
		action := "detected"
		if ev.Interrupted {
			action = "blocked"
		}
		out = append(out, model.WafAttackEvent{
			TxID:        ev.TxID,
			WebsiteID:   resolve(ev.Host),
			Time:        ev.Time,
			Host:        ev.Host,
			SourceIP:    ev.SourceIP,
			Method:      ev.Method,
			URI:         ev.URI,
			RuleID:      ev.RuleID,
			RuleMsg:     ev.RuleMsg,
			Category:    ev.Category,
			Severity:    ev.Severity,
			MatchedData: ev.MatchedData,
			HitCount:    ev.HitCount,
			Action:      action,
		})
	}
	return out
}

// hostResolver builds a host→websiteID map from the site registry — the primary
// domain AND every additional/aliased domain (WebsiteRepo.List preloads Domains)
// — so an attack on any of a site's hostnames is attributed to it, not lost to
// websiteID 0. Limitation: hostnames are matched without port, so two managed
// sites sharing the same hostname on different ports would collide (last wins);
// this is rare and acceptable for the first slice.
func hostResolver() func(string) uint {
	byHost := map[string]uint{}
	if sites, err := repo.NewIWebsiteRepo().List(); err == nil {
		for i := range sites {
			byHost[normalizeHost(sites[i].PrimaryDomain)] = sites[i].ID
			for _, d := range sites[i].Domains {
				byHost[normalizeHost(d.Domain)] = sites[i].ID
			}
		}
	}
	return func(host string) uint {
		return byHost[normalizeHost(host)]
	}
}

func normalizeHost(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	if i := strings.IndexByte(h, ':'); i >= 0 {
		h = h[:i]
	}
	return h
}

func (s *WafEventService) prune(wafRepo repo.IWafRepo) {
	days := wafDefaultRetentionDays
	if setting, err := repo.NewISettingRepo().Get(repo.NewISettingRepo().WithByKey("WafRetentionDays")); err == nil {
		if n, e := strconv.Atoi(setting.Value); e == nil && n > 0 {
			days = n
		}
	}
	if err := wafRepo.PruneBefore(time.Now().UTC().AddDate(0, 0, -days)); err != nil {
		global.LOG.Errorf("waf-event: prune failed: %v", err)
	}
}

func (s *WafEventService) LoadEvents(websiteID uint, req request.WafEventSearch) (dto.PageResult, error) {
	start, end := normalizeRange(req.StartTime, req.EndTime)
	page, size := req.Page, req.PageSize
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 500 {
		size = 50
	}
	items, total, err := repo.NewIWafRepo().List(websiteID, start, end, req.Category, size, (page-1)*size)
	if err != nil {
		return dto.PageResult{}, err
	}
	return dto.PageResult{Total: total, Items: items}, nil
}
