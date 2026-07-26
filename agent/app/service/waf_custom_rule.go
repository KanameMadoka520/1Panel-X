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

// IWafCustomRuleService manages the panel-wide operator-authored rules.
type IWafCustomRuleService interface {
	List() ([]response.WafCustomRule, error)
	Save(req request.WafCustomRuleSave) ([]response.WafCustomRule, error)
	Delete(ids []uint) ([]response.WafCustomRule, error)
	Reorder(ids []uint) ([]response.WafCustomRule, error)
}

type WafCustomRuleService struct {
	control *WafControlService
}

func NewIWafCustomRuleService() IWafCustomRuleService {
	return &WafCustomRuleService{control: NewIWafControlService().(*WafControlService)}
}

func (s *WafCustomRuleService) List() ([]response.WafCustomRule, error) {
	if global.WafDB == nil {
		return nil, errors.New("WAF database is not available")
	}
	rows, err := repo.NewIWafRepo().ListCustomRules()
	if err != nil {
		return nil, err
	}
	return customRulesToResponse(rows), nil
}

func (s *WafCustomRuleService) Save(req request.WafCustomRuleSave) ([]response.WafCustomRule, error) {
	wafControlMu.Lock()
	defer wafControlMu.Unlock()

	if global.WafDB == nil {
		return nil, errors.New("WAF database is not available")
	}
	// Validated against the same rules the data plane applies, so an unusable
	// rule is refused while the operator is still looking at the form rather
	// than silently failing the next config generation for everything else.
	candidate := wafconfig.CustomRule{
		Name:       strings.TrimSpace(req.Name),
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
	row := model.WafCustomRule{
		BaseModel:  model.BaseModel{ID: req.ID},
		Name:       normalized[0].Name,
		Action:     normalized[0].Action,
		Conditions: string(conditions),
		Remark:     strings.TrimSpace(req.Remark),
		Enabled:    req.Enabled,
	}
	if req.ID == 0 {
		// A new rule goes to the end of the evaluation order, so creating one can
		// never change how an existing rule resolves.
		existing, err := wafRepo.ListCustomRules()
		if err != nil {
			return nil, err
		}
		row.Priority = len(existing)
	} else {
		current, err := wafRepo.ListCustomRules()
		if err != nil {
			return nil, err
		}
		for _, c := range current {
			if c.ID == req.ID {
				row.Priority = c.Priority
				break
			}
		}
	}
	if err := wafRepo.SaveCustomRule(&row); err != nil {
		return nil, err
	}
	return s.applyAndList()
}

func (s *WafCustomRuleService) Delete(ids []uint) ([]response.WafCustomRule, error) {
	wafControlMu.Lock()
	defer wafControlMu.Unlock()

	if global.WafDB == nil {
		return nil, errors.New("WAF database is not available")
	}
	if err := repo.NewIWafRepo().DeleteCustomRules(ids); err != nil {
		return nil, err
	}
	return s.applyAndList()
}

func (s *WafCustomRuleService) Reorder(ids []uint) ([]response.WafCustomRule, error) {
	wafControlMu.Lock()
	defer wafControlMu.Unlock()

	if global.WafDB == nil {
		return nil, errors.New("WAF database is not available")
	}
	wafRepo := repo.NewIWafRepo()
	current, err := wafRepo.ListCustomRules()
	if err != nil {
		return nil, err
	}
	// The submitted sequence must cover exactly the stored rules. A partial
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
			return nil, fmt.Errorf("rule %d no longer exists; reload the page and try again", id)
		}
		if _, dup := seen[id]; dup {
			return nil, fmt.Errorf("rule %d appears twice in the new order", id)
		}
		seen[id] = struct{}{}
	}
	if err := wafRepo.ReorderCustomRules(ids); err != nil {
		return nil, err
	}
	return s.applyAndList()
}

// applyAndList regenerates the gateway config so a rule change takes effect
// immediately, then returns the stored rules. A failure to apply is reported;
// the rows stay saved and the gateway keeps enforcing its previous config.
func (s *WafCustomRuleService) applyAndList() ([]response.WafCustomRule, error) {
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
	return s.List()
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

// enabledCustomRules converts the stored rows the gateway should enforce, in
// evaluation order. A row switched off is omitted entirely rather than sent with
// a disabled flag, so the data plane never holds a rule it is not meant to apply.
//
// A row whose conditions cannot be parsed FAILS config generation rather than
// being skipped: silently dropping a deny rule would leave the panel showing
// protection that is not in force.
func enabledCustomRules(rows []model.WafCustomRule) ([]wafconfig.CustomRule, error) {
	out := make([]wafconfig.CustomRule, 0, len(rows))
	for _, r := range rows {
		if !r.Enabled {
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
