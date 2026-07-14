package repo

import (
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/global"
	"gorm.io/gorm/clause"
)

type WebsiteStatRepo struct{}

type IWebsiteStatRepo interface {
	BatchCreateStats(list []model.WebsiteAccessStat) error
	BatchCreateRanks(list []model.WebsiteAccessRank) error

	// SumStats returns one summed row per bucket time in [start, end), ordered
	// ascending. Grouping collapses the rare case of a bucket finalized in more
	// than one flush into a single point.
	SumStats(websiteID uint, start, end time.Time) ([]model.WebsiteAccessStat, error)
	// SumRanks returns the overall top-N for a kind across [start, end), summed
	// per key and ordered by count desc.
	SumRanks(websiteID uint, kind string, start, end time.Time, top int) ([]model.WebsiteAccessRank, error)

	GetCursor(websiteID uint) (model.WebsiteAccessCursor, error)
	SaveCursor(cursor model.WebsiteAccessCursor) error

	// DeleteHours removes any stat/rank rows for the given bucket times so a
	// finalized bucket can be re-written idempotently (crash-retry safe).
	DeleteHours(websiteID uint, times []time.Time) error
	PruneBefore(t time.Time) error
}

func NewIWebsiteStatRepo() IWebsiteStatRepo {
	return &WebsiteStatRepo{}
}

func (r *WebsiteStatRepo) BatchCreateStats(list []model.WebsiteAccessStat) error {
	if len(list) == 0 {
		return nil
	}
	return global.WebsiteStatDB.CreateInBatches(&list, 200).Error
}

func (r *WebsiteStatRepo) BatchCreateRanks(list []model.WebsiteAccessRank) error {
	if len(list) == 0 {
		return nil
	}
	return global.WebsiteStatDB.CreateInBatches(&list, 200).Error
}

func (r *WebsiteStatRepo) SumStats(websiteID uint, start, end time.Time) ([]model.WebsiteAccessStat, error) {
	var out []model.WebsiteAccessStat
	err := global.WebsiteStatDB.Model(&model.WebsiteAccessStat{}).
		Select("`time`, "+
			"sum(pv) as pv, sum(uv) as uv, sum(bytes) as bytes, "+
			"sum(status2xx) as status2xx, sum(status3xx) as status3xx, "+
			"sum(status4xx) as status4xx, sum(status5xx) as status5xx").
		Where("website_id = ? AND `time` >= ? AND `time` < ?", websiteID, start, end).
		Group("`time`").
		Order("`time` asc").
		Scan(&out).Error
	return out, err
}

func (r *WebsiteStatRepo) SumRanks(websiteID uint, kind string, start, end time.Time, top int) ([]model.WebsiteAccessRank, error) {
	var out []model.WebsiteAccessRank
	db := global.WebsiteStatDB.Model(&model.WebsiteAccessRank{}).
		Select("kind, rank_key, sum(`count`) as `count`").
		Where("website_id = ? AND kind = ? AND `time` >= ? AND `time` < ?", websiteID, kind, start, end).
		Group("kind, rank_key").
		Order("`count` desc")
	if top > 0 {
		db = db.Limit(top)
	}
	err := db.Scan(&out).Error
	return out, err
}

func (r *WebsiteStatRepo) GetCursor(websiteID uint) (model.WebsiteAccessCursor, error) {
	var cursor model.WebsiteAccessCursor
	err := global.WebsiteStatDB.Where("website_id = ?", websiteID).First(&cursor).Error
	return cursor, err
}

// SaveCursor upserts the per-site cursor on website_id.
func (r *WebsiteStatRepo) SaveCursor(cursor model.WebsiteAccessCursor) error {
	// gorm stamps created_at/updated_at before the upsert, so the DoUpdates set
	// writes a fresh updated_at on conflict.
	return global.WebsiteStatDB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "website_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"path", "offset", "updated_at"}),
	}).Create(&cursor).Error
}

func (r *WebsiteStatRepo) DeleteHours(websiteID uint, times []time.Time) error {
	if len(times) == 0 {
		return nil
	}
	if err := global.WebsiteStatDB.Where("website_id = ? AND `time` IN ?", websiteID, times).
		Delete(&model.WebsiteAccessStat{}).Error; err != nil {
		return err
	}
	return global.WebsiteStatDB.Where("website_id = ? AND `time` IN ?", websiteID, times).
		Delete(&model.WebsiteAccessRank{}).Error
}

func (r *WebsiteStatRepo) PruneBefore(t time.Time) error {
	if err := global.WebsiteStatDB.Where("`time` < ?", t).Delete(&model.WebsiteAccessStat{}).Error; err != nil {
		return err
	}
	return global.WebsiteStatDB.Where("`time` < ?", t).Delete(&model.WebsiteAccessRank{}).Error
}
