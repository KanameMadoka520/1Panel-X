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

	PruneBefore(t time.Time) error
}

func NewIWafRepo() IWafRepo {
	return &WafRepo{}
}

func (r *WafRepo) BatchCreate(list []model.WafAttackEvent) error {
	if len(list) == 0 {
		return nil
	}
	return global.WafDB.CreateInBatches(&list, 200).Error
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

func (r *WafRepo) PruneBefore(t time.Time) error {
	return global.WafDB.Where("`time` < ?", t).Delete(&model.WafAttackEvent{}).Error
}
