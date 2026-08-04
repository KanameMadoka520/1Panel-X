package migrations

import (
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/utils/backupsync"
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

var MigrateBackupSyncState = &gormigrate.Migration{
	ID: "20260804-migrate-backup-sync-state",
	Migrate: func(tx *gorm.DB) error {
		if err := tx.AutoMigrate(
			&model.BackupSyncSequence{},
			&model.BackupSyncOutbox{},
			&model.BackupSyncTarget{},
			&model.BackupSyncTombstone{},
		); err != nil {
			return err
		}
		return backupsync.InitializeTx(tx)
	},
}
