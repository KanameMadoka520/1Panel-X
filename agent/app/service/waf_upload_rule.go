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
	"gorm.io/gorm"
)

// IWafUploadRuleService manages one website's upload restriction rules.
type IWafUploadRuleService interface {
	List(websiteID uint) (response.WafUploadRules, error)
	Save(websiteID uint, req request.WafUploadRuleSave) (response.WafUploadRules, error)
	Delete(websiteID uint, ids []uint) (response.WafUploadRules, error)
	SetEnabled(websiteID uint, enabled bool) (response.WafUploadRules, error)
}

type WafUploadRuleService struct {
	control *WafControlService
}

func NewIWafUploadRuleService() IWafUploadRuleService {
	return &WafUploadRuleService{control: NewIWafControlService().(*WafControlService)}
}

func (s *WafUploadRuleService) List(websiteID uint) (response.WafUploadRules, error) {
	if global.WafDB == nil {
		return response.WafUploadRules{}, errors.New("WAF database is not available")
	}
	wafRepo := repo.NewIWafRepo()
	if err := seedUploadRules(wafRepo, websiteID); err != nil {
		return response.WafUploadRules{}, err
	}
	return loadUploadRules(wafRepo, websiteID)
}

// seedUploadRules writes the default rule set once, so a new site's list looks
// like the upstream product's. The master switch is left OFF, so seeding can
// never change what is enforced; and the fact that it has run is recorded, so
// deleting every rule does not make them reappear on the next read.
func seedUploadRules(wafRepo repo.IWafRepo, websiteID uint) error {
	policy, err := wafRepo.GetPolicy(websiteID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// No WAF policy row yet: nothing to record the seed against, and the site
		// is not protected anyway. Seeding waits until the operator saves.
		return nil
	}
	if err != nil {
		return err
	}
	if policy.UploadSeeded {
		return nil
	}
	for _, rule := range wafconfig.DefaultUploadRules {
		row := model.WafUploadRule{WebsiteID: websiteID, Rule: rule, Enabled: true}
		if err := wafRepo.SaveUploadRule(&row); err != nil {
			return err
		}
	}
	policy.UploadSeeded = true
	return wafRepo.SavePolicy(policy)
}

func (s *WafUploadRuleService) Save(websiteID uint, req request.WafUploadRuleSave) (response.WafUploadRules, error) {
	wafControlMu.Lock()
	defer wafControlMu.Unlock()

	if global.WafDB == nil {
		return response.WafUploadRules{}, errors.New("WAF database is not available")
	}
	// Validated against the same rules the data plane applies, so an unusable
	// rule is refused while the operator is still looking at the form.
	normalized, err := wafconfig.NormalizeUploadRules([]string{req.Rule})
	if err != nil {
		return response.WafUploadRules{}, fmt.Errorf("invalid upload rule: %w", err)
	}
	if len(normalized) == 0 {
		return response.WafUploadRules{}, errors.New("upload rule cannot be empty")
	}
	wafRepo := repo.NewIWafRepo()
	existing, err := wafRepo.ListUploadRules(websiteID)
	if err != nil {
		return response.WafUploadRules{}, err
	}
	// Two rows with the same rule would be indistinguishable in the table while
	// only one of them appeared to do anything.
	for _, e := range existing {
		if e.Rule == normalized[0] && e.ID != req.ID {
			return response.WafUploadRules{}, fmt.Errorf("rule %q already exists for this website", normalized[0])
		}
	}
	row := model.WafUploadRule{
		BaseModel: model.BaseModel{ID: req.ID},
		WebsiteID: websiteID,
		Rule:      normalized[0],
		Remark:    strings.TrimSpace(req.Remark),
		Enabled:   req.Enabled,
	}
	if req.ID != 0 {
		// An update must not be able to move a rule to another website.
		owned := false
		for _, e := range existing {
			if e.ID == req.ID {
				owned = true
				break
			}
		}
		if !owned {
			return response.WafUploadRules{}, fmt.Errorf("upload rule %d does not belong to this website", req.ID)
		}
	}
	if err := wafRepo.SaveUploadRule(&row); err != nil {
		return response.WafUploadRules{}, err
	}
	return s.applyAndList(websiteID)
}

func (s *WafUploadRuleService) Delete(websiteID uint, ids []uint) (response.WafUploadRules, error) {
	wafControlMu.Lock()
	defer wafControlMu.Unlock()

	if global.WafDB == nil {
		return response.WafUploadRules{}, errors.New("WAF database is not available")
	}
	if err := repo.NewIWafRepo().DeleteUploadRules(websiteID, ids); err != nil {
		return response.WafUploadRules{}, err
	}
	return s.applyAndList(websiteID)
}

// SetEnabled flips the master switch. Turning it on with an empty rule list is
// allowed and enforces nothing, which is honest: the switch says the control is
// armed, the empty list says there is nothing to catch.
func (s *WafUploadRuleService) SetEnabled(websiteID uint, enabled bool) (response.WafUploadRules, error) {
	wafControlMu.Lock()
	defer wafControlMu.Unlock()

	if global.WafDB == nil {
		return response.WafUploadRules{}, errors.New("WAF database is not available")
	}
	wafRepo := repo.NewIWafRepo()
	policy, err := wafRepo.GetPolicy(websiteID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return response.WafUploadRules{}, errors.New("enable the WAF for this website first")
	}
	if err != nil {
		return response.WafUploadRules{}, err
	}
	policy.UploadLimit = enabled
	if err := wafRepo.SavePolicy(policy); err != nil {
		return response.WafUploadRules{}, err
	}
	return s.applyAndList(websiteID)
}

func (s *WafUploadRuleService) applyAndList(websiteID uint) (response.WafUploadRules, error) {
	composeChanged, err := EnsureWafRuntimeFiles()
	if err != nil {
		return response.WafUploadRules{}, err
	}
	generation, err := s.control.writeGatewayConfig()
	if err != nil {
		return response.WafUploadRules{}, err
	}
	if err := s.control.applyGatewayConfig(generation, composeChanged); err != nil {
		return response.WafUploadRules{}, err
	}
	return loadUploadRules(repo.NewIWafRepo(), websiteID)
}

func loadUploadRules(wafRepo repo.IWafRepo, websiteID uint) (response.WafUploadRules, error) {
	rows, err := wafRepo.ListUploadRules(websiteID)
	if err != nil {
		return response.WafUploadRules{}, err
	}
	out := response.WafUploadRules{Rules: make([]response.WafUploadRule, 0, len(rows))}
	for _, r := range rows {
		out.Rules = append(out.Rules, response.WafUploadRule{
			ID:      r.ID,
			Rule:    r.Rule,
			Remark:  r.Remark,
			Enabled: r.Enabled,
		})
	}
	policy, err := wafRepo.GetPolicy(websiteID)
	if err == nil {
		out.Enabled = policy.UploadLimit
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return response.WafUploadRules{}, err
	}
	return out, nil
}

// enabledUploadRules returns the rules the gateway should enforce for one site.
//
// The master switch being off yields NOTHING, not a disabled flag: the data
// plane never holds a rule it is not meant to apply. A row switched off
// individually is omitted for the same reason.
func enabledUploadRules(rows []model.WafUploadRule, websiteID uint, limitOn bool) []string {
	if !limitOn {
		return nil
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.WebsiteID != websiteID || !r.Enabled {
			continue
		}
		out = append(out, r.Rule)
	}
	return out
}
