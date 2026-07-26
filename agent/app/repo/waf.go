package repo

import (
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/global"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WafRepo struct{}

type IWafRepo interface {
	BatchCreate(list []model.WafAttackEvent) error
	// List returns a page of events for a website within [start, end), optionally
	// filtered by category, newest first, plus the total match count.
	List(websiteID uint, start, end time.Time, category string, limit, offset int) ([]model.WafAttackEvent, int64, error)

	GetCursor(path string) (model.WafAuditCursor, error)
	SaveCursor(cursor model.WafAuditCursor) error

	ListPolicies() ([]model.WafSitePolicy, error)
	GetPolicy(websiteID uint) (model.WafSitePolicy, error)
	SavePolicy(policy model.WafSitePolicy) error
	DeletePolicy(websiteID uint) error

	GetGlobalPolicy() (model.WafGlobalPolicy, error)
	SaveGlobalPolicy(policy model.WafGlobalPolicy) error

	ListEntries() ([]model.WafListEntry, error)
	GetEntry(id uint) (model.WafListEntry, error)
	SaveEntry(entry *model.WafListEntry) error
	DeleteEntries(ids []uint) error

	ListIPGroups() ([]model.WafIPGroup, error)
	GetIPGroup(id uint) (model.WafIPGroup, error)
	SaveIPGroup(group *model.WafIPGroup) error
	DeleteIPGroups(ids []uint) error

	ListCustomRules() ([]model.WafCustomRule, error)
	SaveCustomRule(rule *model.WafCustomRule) error
	DeleteCustomRules(ids []uint) error
	ReorderCustomRules(ids []uint) error

	PruneBefore(t time.Time) error
}

func NewIWafRepo() IWafRepo {
	return &WafRepo{}
}

// BatchCreate inserts events, skipping any whose Coraza transaction id is already
// stored (ON CONFLICT DO NOTHING on tx_id). This makes ingestion idempotent: if
// the tailer crashes after inserting but before advancing its cursor, the same
// audit lines re-read next run are dropped as duplicates instead of double-counted.
func (r *WafRepo) BatchCreate(list []model.WafAttackEvent) error {
	if len(list) == 0 {
		return nil
	}
	return global.WafDB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tx_id"}},
		DoNothing: true,
	}).CreateInBatches(&list, 200).Error
}

func (r *WafRepo) List(websiteID uint, start, end time.Time, category string, limit, offset int) ([]model.WafAttackEvent, int64, error) {
	q := global.WafDB.Model(&model.WafAttackEvent{}).
		Where("website_id = ? AND `time` >= ? AND `time` < ?", websiteID, start, end)
	if category != "" {
		q = q.Where("category = ?", category)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []model.WafAttackEvent
	q = q.Order("`time` desc")
	if limit > 0 {
		q = q.Limit(limit).Offset(offset)
	}
	err := q.Find(&out).Error
	return out, total, err
}

func (r *WafRepo) GetCursor(path string) (model.WafAuditCursor, error) {
	var cursor model.WafAuditCursor
	err := global.WafDB.Where("path = ?", path).First(&cursor).Error
	return cursor, err
}

func (r *WafRepo) SaveCursor(cursor model.WafAuditCursor) error {
	return global.WafDB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "path"}},
		DoUpdates: clause.AssignmentColumns([]string{"offset", "updated_at"}),
	}).Create(&cursor).Error
}

func (r *WafRepo) ListPolicies() ([]model.WafSitePolicy, error) {
	var policies []model.WafSitePolicy
	err := global.WafDB.Find(&policies).Error
	return policies, err
}

func (r *WafRepo) GetPolicy(websiteID uint) (model.WafSitePolicy, error) {
	var policy model.WafSitePolicy
	err := global.WafDB.Where("website_id = ?", websiteID).First(&policy).Error
	return policy, err
}

func (r *WafRepo) SavePolicy(policy model.WafSitePolicy) error {
	return global.WafDB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "website_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"enabled", "mode", "allow_ips", "deny_ips", "rate_limits", "rule_options", "region_rules", "real_ip_rules", "last_error", "updated_at"}),
	}).Create(&policy).Error
}

func (r *WafRepo) DeletePolicy(websiteID uint) error {
	return global.WafDB.Where("website_id = ?", websiteID).Delete(&model.WafSitePolicy{}).Error
}

func (r *WafRepo) GetGlobalPolicy() (model.WafGlobalPolicy, error) {
	var policy model.WafGlobalPolicy
	err := global.WafDB.First(&policy).Error
	return policy, err
}

// SaveGlobalPolicy upserts the single panel-wide row (fixed primary key 1) so
// concurrent saves can never fork the global policy into multiple rows.
func (r *WafRepo) SaveGlobalPolicy(policy model.WafGlobalPolicy) error {
	policy.ID = 1
	return global.WafDB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"default_mode", "allow_ips", "deny_ips", "rate_limits", "rule_options", "region_rules", "block_page", "log_options", "updated_at"}),
	}).Create(&policy).Error
}

// ListEntries returns every black/white list row, newest first within a stable
// grouping so the table order does not shuffle between reads.
func (r *WafRepo) ListEntries() ([]model.WafListEntry, error) {
	var out []model.WafListEntry
	err := global.WafDB.Order("list asc, target asc, id asc").Find(&out).Error
	return out, err
}

func (r *WafRepo) GetEntry(id uint) (model.WafListEntry, error) {
	var entry model.WafListEntry
	err := global.WafDB.Where("id = ?", id).First(&entry).Error
	return entry, err
}

// SaveEntry creates or updates a row. Enabled is written through Select so a
// switch turned OFF is persisted rather than skipped as a zero value.
func (r *WafRepo) SaveEntry(entry *model.WafListEntry) error {
	if entry.ID == 0 {
		return global.WafDB.Create(entry).Error
	}
	return global.WafDB.Model(entry).
		Select("list", "target", "match", "pattern", "remark", "enabled", "updated_at").
		Updates(entry).Error
}

func (r *WafRepo) DeleteEntries(ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	return global.WafDB.Where("id in (?)", ids).Delete(&model.WafListEntry{}).Error
}

func (r *WafRepo) ListIPGroups() ([]model.WafIPGroup, error) {
	var out []model.WafIPGroup
	err := global.WafDB.Order("name asc").Find(&out).Error
	return out, err
}

func (r *WafRepo) GetIPGroup(id uint) (model.WafIPGroup, error) {
	var group model.WafIPGroup
	err := global.WafDB.Where("id = ?", id).First(&group).Error
	return group, err
}

// ListCustomRules returns the rules in the operator's evaluation order. The id
// tiebreak keeps two rules that share a priority in a stable order rather than
// letting the database choose one per read.
func (r *WafRepo) ListCustomRules() ([]model.WafCustomRule, error) {
	var out []model.WafCustomRule
	err := global.WafDB.Order("priority asc, id asc").Find(&out).Error
	return out, err
}

// SaveCustomRule creates or updates a rule. Enabled is written through Select so
// a switch turned OFF is persisted rather than skipped as a zero value.
func (r *WafRepo) SaveCustomRule(rule *model.WafCustomRule) error {
	if rule.ID == 0 {
		return global.WafDB.Create(rule).Error
	}
	return global.WafDB.Model(rule).
		Select("name", "action", "conditions", "priority", "remark", "enabled", "updated_at").
		Updates(rule).Error
}

func (r *WafRepo) DeleteCustomRules(ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	return global.WafDB.Where("id in (?)", ids).Delete(&model.WafCustomRule{}).Error
}

// ReorderCustomRules rewrites the evaluation order from the given id sequence.
// It runs in one transaction: a half-applied reorder would leave two rules
// claiming the same position, and with deny/allow rules that changes which one
// decides a request.
func (r *WafRepo) ReorderCustomRules(ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	return global.WafDB.Transaction(func(tx *gorm.DB) error {
		for i, id := range ids {
			if err := tx.Model(&model.WafCustomRule{}).Where("id = ?", id).
				Update("priority", i).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *WafRepo) SaveIPGroup(group *model.WafIPGroup) error {
	if group.ID == 0 {
		return global.WafDB.Create(group).Error
	}
	return global.WafDB.Model(group).
		Select("name", "entries", "remark", "updated_at").
		Updates(group).Error
}

func (r *WafRepo) DeleteIPGroups(ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	return global.WafDB.Where("id in (?)", ids).Delete(&model.WafIPGroup{}).Error
}

func (r *WafRepo) PruneBefore(t time.Time) error {
	return global.WafDB.Where("`time` < ?", t).Delete(&model.WafAttackEvent{}).Error
}
