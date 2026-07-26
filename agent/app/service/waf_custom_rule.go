package service

import (
	"encoding/json"
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

// IWafCustomRuleService manages one website's operator-authored rules.
type IWafCustomRuleService interface {
	List(websiteID uint) ([]response.WafCustomRule, error)
	Save(websiteID uint, req request.WafCustomRuleSave) ([]response.WafCustomRule, error)
	Delete(websiteID uint, ids []uint) ([]response.WafCustomRule, error)
	Reorder(websiteID uint, ids []uint) ([]response.WafCustomRule, error)
}

type WafCustomRuleService struct {
	control *WafControlService
}

func NewIWafCustomRuleService() IWafCustomRuleService {
	return &WafCustomRuleService{control: NewIWafControlService().(*WafControlService)}
}

func (s *WafCustomRuleService) List(websiteID uint) ([]response.WafCustomRule, error) {
	if global.WafDB == nil {
		return nil, errors.New("WAF database is not available")
	}
	rows, err := repo.NewIWafRepo().ListCustomRules(websiteID)
	if err != nil {
		return nil, err
	}
	return customRulesToResponse(rows), nil
}

func (s *WafCustomRuleService) Save(websiteID uint, req request.WafCustomRuleSave) ([]response.WafCustomRule, error) {
	wafControlMu.Lock()
	defer wafControlMu.Unlock()

	if global.WafDB == nil {
		return nil, errors.New("WAF database is not available")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errors.New("rule name cannot be empty")
	}
	// Validated against the same rules the data plane applies, so an unusable
	// rule is refused while the operator is still looking at the form rather
	// than silently failing the next config generation for everything else.
	candidate := wafconfig.CustomRule{
		Name:       name,
		Action:     strings.TrimSpace(req.Action),
		Conditions: conditionsFromRequest(req.Conditions),
	}
	normalized, err := wafconfig.NormalizeCustomRules([]wafconfig.CustomRule{candidate})
	if err != nil {
		return nil, fmt.Errorf("invalid custom rule: %w", err)
	}
	conditions, err := json.Marshal(normalized[0].Conditions)
	if err != nil {
		return nil, err
	}

	wafRepo := repo.NewIWafRepo()
	existing, err := wafRepo.ListCustomRules(websiteID)
	if err != nil {
		return nil, err
	}
	row := model.WafCustomRule{
		BaseModel:  model.BaseModel{ID: req.ID},
		WebsiteID:  websiteID,
		Name:       normalized[0].Name,
		Action:     normalized[0].Action,
		Conditions: string(conditions),
		Remark:     strings.TrimSpace(req.Remark),
		Enabled:    req.Enabled,
	}
	if req.ID == 0 {
		// Two rules sharing a name would be indistinguishable in an enforcement
		// record, which is the one place the name has to mean something.
		for _, e := range existing {
			if e.Name == row.Name {
				return nil, fmt.Errorf("a rule named %q already exists for this website", row.Name)
			}
		}
		// A new rule goes to the end of the evaluation order, so creating one can
		// never change how an existing rule resolves.
		row.Priority = len(existing)
	} else {
		current, found := model.WafCustomRule{}, false
		for _, e := range existing {
			if e.ID == req.ID {
				current, found = e, true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("custom rule %d does not belong to this website", req.ID)
		}
		row.Priority = current.Priority
		// The name is fixed after creation: an enforcement record already written
		// under the old one would otherwise name a rule that no longer answers to
		// it. The column is excluded from the update either way; refusing here
		// tells the operator instead of silently ignoring what they typed.
		if row.Name != current.Name {
			return nil, fmt.Errorf("a rule's name cannot be changed after creation; delete %q and create a new one", current.Name)
		}
	}
	if err := wafRepo.SaveCustomRule(&row); err != nil {
		return nil, err
	}
	return s.applyAndList(websiteID)
}

func (s *WafCustomRuleService) Delete(websiteID uint, ids []uint) ([]response.WafCustomRule, error) {
	wafControlMu.Lock()
	defer wafControlMu.Unlock()

	if global.WafDB == nil {
		return nil, errors.New("WAF database is not available")
	}
	if err := repo.NewIWafRepo().DeleteCustomRules(websiteID, ids); err != nil {
		return nil, err
	}
	return s.applyAndList(websiteID)
}

func (s *WafCustomRuleService) Reorder(websiteID uint, ids []uint) ([]response.WafCustomRule, error) {
	wafControlMu.Lock()
	defer wafControlMu.Unlock()

	if global.WafDB == nil {
		return nil, errors.New("WAF database is not available")
	}
	wafRepo := repo.NewIWafRepo()
	current, err := wafRepo.ListCustomRules(websiteID)
	if err != nil {
		return nil, err
	}
	// The submitted sequence must cover exactly this site's rules. A partial
	// sequence would leave the rules it omits at their old priority, silently
	// interleaving them into an order the operator never chose.
	if len(ids) != len(current) {
		return nil, fmt.Errorf("the new order lists %d rules but %d are stored; reload the page and try again", len(ids), len(current))
	}
	stored := make(map[uint]struct{}, len(current))
	for _, c := range current {
		stored[c.ID] = struct{}{}
	}
	seen := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := stored[id]; !ok {
			return nil, fmt.Errorf("rule %d does not belong to this website; reload the page and try again", id)
		}
		if _, dup := seen[id]; dup {
			return nil, fmt.Errorf("rule %d appears twice in the new order", id)
		}
		seen[id] = struct{}{}
	}
	if err := wafRepo.ReorderCustomRules(websiteID, ids); err != nil {
		return nil, err
	}
	return s.applyAndList(websiteID)
}

// applyAndList regenerates the gateway config so a rule change takes effect
// immediately, then returns the stored rules. A failure to apply is reported;
// the rows stay saved and the gateway keeps enforcing its previous config.
func (s *WafCustomRuleService) applyAndList(websiteID uint) ([]response.WafCustomRule, error) {
	composeChanged, err := EnsureWafRuntimeFiles()
	if err != nil {
		return nil, err
	}
	generation, err := s.control.writeGatewayConfig()
	if err != nil {
		return nil, err
	}
	if err := s.control.applyGatewayConfig(generation, composeChanged); err != nil {
		return nil, err
	}
	return s.List(websiteID)
}

func conditionsFromRequest(in []request.WafRuleCondition) []wafconfig.CustomCondition {
	out := make([]wafconfig.CustomCondition, 0, len(in))
	for _, c := range in {
		out = append(out, wafconfig.CustomCondition{
			Field:   strings.TrimSpace(c.Field),
			Name:    strings.TrimSpace(c.Name),
			Match:   strings.TrimSpace(c.Match),
			Pattern: strings.TrimSpace(c.Pattern),
			Negate:  c.Negate,
		})
	}
	return out
}

func customRulesToResponse(rows []model.WafCustomRule) []response.WafCustomRule {
	out := make([]response.WafCustomRule, 0, len(rows))
	for _, r := range rows {
		item := response.WafCustomRule{
			ID:         r.ID,
			Name:       r.Name,
			Action:     r.Action,
			Remark:     r.Remark,
			Enabled:    r.Enabled,
			Conditions: []response.WafRuleCondition{},
		}
		conditions, err := parseCustomConditions(r.Conditions)
		if err != nil {
			// A row whose conditions cannot be read is shown as broken rather than
			// as an empty rule: an empty condition list reads as "matches nothing",
			// which is a different and much more reassuring claim than the truth.
			item.Invalid = err.Error()
		} else {
			for _, c := range conditions {
				item.Conditions = append(item.Conditions, response.WafRuleCondition{
					Field:   c.Field,
					Name:    c.Name,
					Match:   c.Match,
					Pattern: c.Pattern,
					Negate:  c.Negate,
				})
			}
		}
		out = append(out, item)
	}
	return out
}

func parseCustomConditions(raw string) ([]wafconfig.CustomCondition, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("stored conditions are empty")
	}
	var out []wafconfig.CustomCondition
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("stored conditions are unreadable: %w", err)
	}
	return out, nil
}

// enabledCustomRules converts one site's stored rows into what the gateway
// should enforce, in evaluation order. A row switched off is omitted entirely
// rather than sent with a disabled flag, so the data plane never holds a rule it
// is not meant to apply.
//
// A row whose conditions cannot be parsed FAILS config generation rather than
// being skipped: silently dropping a deny rule would leave the panel showing
// protection that is not in force.
func enabledCustomRules(rows []model.WafCustomRule, websiteID uint) ([]wafconfig.CustomRule, error) {
	out := make([]wafconfig.CustomRule, 0, len(rows))
	for _, r := range rows {
		if r.WebsiteID != websiteID || !r.Enabled {
			continue
		}
		conditions, err := parseCustomConditions(r.Conditions)
		if err != nil {
			return nil, fmt.Errorf("custom rule %q (id %d): %w", r.Name, r.ID, err)
		}
		out = append(out, wafconfig.CustomRule{
			Name:       r.Name,
			Action:     r.Action,
			Conditions: conditions,
		})
	}
	return out, nil
}
