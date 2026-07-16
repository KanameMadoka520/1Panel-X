package repo

import (
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/global"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WebsiteStatRepo struct{}

type IWebsiteStatRepo interface {
	// SaveFinalized upsert-accumulates the finalized stat/rank rows and advances
	// the cursor in ONE transaction. Rows are added (+=) on conflict so a closed
	// hour split across runs (by the per-run byte cap) accumulates correctly, and
	// a crash rolls back both the rows and the cursor so the same lines re-apply
	// cleanly next run.
	SaveFinalized(stats []model.WebsiteAccessStat, ranks []model.WebsiteAccessRank, cursor model.WebsiteAccessCursor) error

	// SumStats returns one summed row per bucket time in [start, end), ascending.
	SumStats(websiteID uint, start, end time.Time) ([]model.WebsiteAccessStat, error)
	// SumRanks returns the overall top-N for a kind across [start, end), summed per
	// key and ordered by count desc.
	SumRanks(websiteID uint, kind string, start, end time.Time, top int) ([]model.WebsiteAccessRank, error)

	GetCursor(websiteID uint) (model.WebsiteAccessCursor, error)
	PruneBefore(t time.Time) error
}

func NewIWebsiteStatRepo() IWebsiteStatRepo {
	return &WebsiteStatRepo{}
}

func (r *WebsiteStatRepo) SaveFinalized(stats []model.WebsiteAccessStat, ranks []model.WebsiteAccessRank, cursor model.WebsiteAccessCursor) error {
	return global.WebsiteStatDB.Transaction(func(tx *gorm.DB) error {
		for i := range stats {
			s := stats[i]
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "website_id"}, {Name: "time"}},
				DoUpdates: clause.Assignments(map[string]interface{}{
					"pv":        gorm.Expr("pv + ?", s.Pv),
					"uv":        gorm.Expr("uv + ?", s.Uv),
					"bytes":     gorm.Expr("bytes + ?", s.Bytes),
					"status2xx": gorm.Expr("status2xx + ?", s.Status2xx),
					"status3xx": gorm.Expr("status3xx + ?", s.Status3xx),
					"status4xx": gorm.Expr("status4xx + ?", s.Status4xx),
					"status5xx": gorm.Expr("status5xx + ?", s.Status5xx),
				}),
			}).Create(&s).Error; err != nil {
				return err
			}
		}
		for i := range ranks {
			rk := ranks[i]
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "website_id"}, {Name: "time"}, {Name: "kind"}, {Name: "rank_key"}},
				DoUpdates: clause.Assignments(map[string]interface{}{
					"count": gorm.Expr("`count` + ?", rk.Count),
				}),
			}).Create(&rk).Error; err != nil {
				return err
			}
		}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "website_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"path", "offset", "updated_at"}),
		}).Create(&cursor).Error
	})
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

func (r *WebsiteStatRepo) PruneBefore(t time.Time) error {
	if err := global.WebsiteStatDB.Where("`time` < ?", t).Delete(&model.WebsiteAccessStat{}).Error; err != nil {
		return err
	}
	return global.WebsiteStatDB.Where("`time` < ?", t).Delete(&model.WebsiteAccessRank{}).Error
}
