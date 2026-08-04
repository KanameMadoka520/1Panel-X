package migrations

import (
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var MigrateBackupSyncState = &gormigrate.Migration{
	ID: "20260804-migrate-backup-sync-state",
	Migrate: func(tx *gorm.DB) error {
		if err := tx.AutoMigrate(&model.BackupPublicSyncState{}); err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.BackupPublicSyncState{
			ID: model.BackupPublicSyncStateID,
		}).Error
	},
}
