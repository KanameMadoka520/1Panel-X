package model

import "time"

const BackupPublicSyncStateID uint = 1

// BackupPublicSyncState survives agent restarts and prevents an older public
// account snapshot from replacing a newer one.
type BackupPublicSyncState struct {
	ID              uint   `gorm:"primaryKey"`
	Authority       string `gorm:"type:varchar(64);not null;default:''"`
	Generation      string `gorm:"type:varchar(64);not null;default:''"`
	TargetEpoch     string `gorm:"type:varchar(64);not null;default:''"`
	AppliedRevision uint64 `gorm:"not null;default:0"`
	AppliedDigest   string `gorm:"type:varchar(64);not null;default:''"`
	AppliedAt       *time.Time
}

func (BackupPublicSyncState) TableName() string {
	return "backup_public_sync_states"
}
