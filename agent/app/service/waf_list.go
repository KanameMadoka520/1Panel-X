package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/1Panel-dev/1Panel/agent/app/dto/request"
	"github.com/1Panel-dev/1Panel/agent/app/dto/response"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/app/repo"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/utils/wafconfig"
)

// IWafListService manages the panel-wide black/white lists and the named IP
// groups they reference.
type IWafListService interface {
	List() (response.WafLists, error)
	SaveEntry(req request.WafListEntrySave) (response.WafLists, error)
	DeleteEntries(ids []uint) (response.WafLists, error)
	SaveIPGroup(req request.WafIPGroupSave) (response.WafLists, error)
	DeleteIPGroups(ids []uint) (response.WafLists, error)
}

type WafListService struct {
	control *WafControlService
}

func NewIWafListService() IWafListService {
	return &WafListService{control: NewIWafControlService().(*WafControlService)}
}

func (s *WafListService) List() (response.WafLists, error) {
	if global.WafDB == nil {
		return response.WafLists{}, errors.New("WAF database is not available")
	}
	wafRepo := repo.NewIWafRepo()
	entries, err := wafRepo.ListEntries()
	if err != nil {
		return response.WafLists{}, err
	}
	groups, err := wafRepo.ListIPGroups()
	if err != nil {
		return response.WafLists{}, err
	}
	return response.WafLists{
		Entries:  listEntriesToResponse(entries),
		IPGroups: ipGroupsToResponse(groups),
	}, nil
}

func (s *WafListService) SaveEntry(req request.WafListEntrySave) (response.WafLists, error) {
	wafControlMu.Lock()
	defer wafControlMu.Unlock()

	if global.WafDB == nil {
		return response.WafLists{}, errors.New("WAF database is not available")
	}
	wafRepo := repo.NewIWafRepo()
	groups, err := wafRepo.ListIPGroups()
	if err != nil {
		return response.WafLists{}, err
	}
	// Validate the single incoming row against the same rules the gateway
	// applies, so an unusable entry is refused while the operator is still
	// looking at the form rather than at config-generation time.
	candidate := wafconfig.ListRule{
		List:    strings.TrimSpace(req.List),
		Target:  strings.TrimSpace(req.Target),
		Match:   strings.TrimSpace(req.Match),
		Pattern: strings.TrimSpace(req.Pattern),
		Remark:  strings.TrimSpace(req.Remark),
	}
	normalized, err := wafconfig.NormalizeListRules([]wafconfig.ListRule{candidate}, ipGroupsToConfig(groups))
	if err != nil {
		return response.WafLists{}, fmt.Errorf("invalid list entry: %w", err)
	}

	entry := model.WafListEntry{
		BaseModel: model.BaseModel{ID: req.ID},
		List:      normalized[0].List,
		Target:    normalized[0].Target,
		Match:     normalized[0].Match,
		Pattern:   normalized[0].Pattern,
		Remark:    normalized[0].Remark,
		Enabled:   req.Enabled,
	}
	if err := wafRepo.SaveEntry(&entry); err != nil {
		return response.WafLists{}, err
	}
	return s.applyAndList()
}

func (s *WafListService) DeleteEntries(ids []uint) (response.WafLists, error) {
	wafControlMu.Lock()
	defer wafControlMu.Unlock()

	if global.WafDB == nil {
		return response.WafLists{}, errors.New("WAF database is not available")
	}
	if err := repo.NewIWafRepo().DeleteEntries(ids); err != nil {
		return response.WafLists{}, err
	}
	return s.applyAndList()
}

func (s *WafListService) SaveIPGroup(req request.WafIPGroupSave) (response.WafLists, error) {
	wafControlMu.Lock()
	defer wafControlMu.Unlock()

	if global.WafDB == nil {
		return response.WafLists{}, errors.New("WAF database is not available")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return response.WafLists{}, errors.New("IP group name cannot be empty")
	}
	entries, err := wafconfig.NormalizeIPList(req.Entries)
	if err != nil {
		return response.WafLists{}, fmt.Errorf("invalid IP group members: %w", err)
	}
	if len(entries) > wafconfig.MaxIPGroupEntries {
		return response.WafLists{}, fmt.Errorf("IP group has %d members, limit is %d", len(entries), wafconfig.MaxIPGroupEntries)
	}
	group := model.WafIPGroup{
		BaseModel: model.BaseModel{ID: req.ID},
		Name:      name,
		Entries:   strings.Join(entries, "\n"),
		Remark:    strings.TrimSpace(req.Remark),
	}
	if err := repo.NewIWafRepo().SaveIPGroup(&group); err != nil {
		return response.WafLists{}, err
	}
	return s.applyAndList()
}

func (s *WafListService) DeleteIPGroups(ids []uint) (response.WafLists, error) {
	wafControlMu.Lock()
	defer wafControlMu.Unlock()

	if global.WafDB == nil {
		return response.WafLists{}, errors.New("WAF database is not available")
	}
	wafRepo := repo.NewIWafRepo()
	groups, err := wafRepo.ListIPGroups()
	if err != nil {
		return response.WafLists{}, err
	}
	doomed := make(map[uint]string, len(ids))
	for _, id := range ids {
		for _, g := range groups {
			if g.ID == id {
				doomed[id] = g.Name
			}
		}
	}
	entries, err := wafRepo.ListEntries()
	if err != nil {
		return response.WafLists{}, err
	}
	// Deleting a group that entries still point at would leave those entries
	// unloadable and take the whole gateway config down with them, so the
	// conflict is reported instead.
	for _, e := range entries {
		if e.Target != wafconfig.ListTargetIPGroup {
			continue
		}
		for _, name := range doomed {
			if e.Pattern == name {
				return response.WafLists{}, fmt.Errorf("IP group %q is still used by a list entry; remove the entry first", name)
			}
		}
	}
	if err := wafRepo.DeleteIPGroups(ids); err != nil {
		return response.WafLists{}, err
	}
	return s.applyAndList()
}

// applyAndList regenerates the gateway config so a list change takes effect
// immediately, then returns the stored lists. A failure to apply is reported;
// the rows stay saved, and the gateway keeps enforcing its previous config.
func (s *WafListService) applyAndList() (response.WafLists, error) {
	composeChanged, err := EnsureWafRuntimeFiles()
	if err != nil {
		return response.WafLists{}, err
	}
	generation, err := s.control.writeGatewayConfig()
	if err != nil {
		return response.WafLists{}, err
	}
	if err := s.control.applyGatewayConfig(generation, composeChanged); err != nil {
		return response.WafLists{}, err
	}
	return s.List()
}

func listEntriesToResponse(entries []model.WafListEntry) []response.WafListEntry {
	out := make([]response.WafListEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, response.WafListEntry{
			ID:      e.ID,
			List:    e.List,
			Target:  e.Target,
			Match:   e.Match,
			Pattern: e.Pattern,
			Remark:  e.Remark,
			Enabled: e.Enabled,
		})
	}
	return out
}

func ipGroupsToResponse(groups []model.WafIPGroup) []response.WafIPGroup {
	out := make([]response.WafIPGroup, 0, len(groups))
	for _, g := range groups {
		out = append(out, response.WafIPGroup{
			ID:      g.ID,
			Name:    g.Name,
			Entries: splitIPLines(g.Entries),
			Remark:  g.Remark,
		})
	}
	return out
}

func ipGroupsToConfig(groups []model.WafIPGroup) []wafconfig.IPGroup {
	out := make([]wafconfig.IPGroup, 0, len(groups))
	for _, g := range groups {
		out = append(out, wafconfig.IPGroup{Name: g.Name, Entries: splitIPLines(g.Entries)})
	}
	return out
}

// enabledListRules converts the stored rows the gateway should enforce. A row
// switched off is omitted entirely rather than sent with a disabled flag, so the
// data plane never holds a rule it is not meant to apply.
func enabledListRules(entries []model.WafListEntry) []wafconfig.ListRule {
	out := make([]wafconfig.ListRule, 0, len(entries))
	for _, e := range entries {
		if !e.Enabled {
			continue
		}
		out = append(out, wafconfig.ListRule{
			List:    e.List,
			Target:  e.Target,
			Match:   e.Match,
			Pattern: e.Pattern,
			Remark:  e.Remark,
		})
	}
	return out
}
