package repo

import (
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/global"
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
		DoUpdates: clause.AssignmentColumns([]string{"enabled", "mode", "allow_ips", "deny_ips", "rate_limits", "last_error", "updated_at"}),
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
		DoUpdates: clause.AssignmentColumns([]string{"default_mode", "allow_ips", "deny_ips", "rate_limits", "updated_at"}),
	}).Create(&policy).Error
}

func (r *WafRepo) PruneBefore(t time.Time) error {
	return global.WafDB.Where("`time` < ?", t).Delete(&model.WafAttackEvent{}).Error
}
