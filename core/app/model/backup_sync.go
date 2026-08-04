package model

import "time"

const (
	BackupSyncSequenceID uint = 1

	BackupSyncTargetLocal = "local"

	BackupSyncStatusSynced          = "synced"
	BackupSyncStatusPending         = "sync_pending"
	BackupSyncStatusPartiallySynced = "partially_synced"

	BackupSyncTargetStatusPending = "pending"
	BackupSyncTargetStatusFailed  = "failed"
	BackupSyncTargetStatusSynced  = "synced"

	BackupSyncOutboxStatusPending   = "pending"
	BackupSyncOutboxStatusCompleted = "completed"

	BackupSyncOperationBootstrap = "bootstrap"
	BackupSyncOperationCreate    = "create"
	BackupSyncOperationUpdate    = "update"
	BackupSyncOperationDelete    = "delete"
	BackupSyncOperationClear     = "clear"
	BackupSyncOperationRefresh   = "refresh"
)

// BackupSyncSequence owns the monotonically increasing revision for the
// complete public backup-account desired state.
type BackupSyncSequence struct {
	ID             uint   `gorm:"primaryKey"`
	Authority      string `gorm:"type:varchar(64);not null;default:''"`
	Generation     string `gorm:"type:varchar(64);not null;default:''"`
	Revision       uint64 `gorm:"not null;default:0"`
	SnapshotDigest string `gorm:"type:varchar(64);not null;default:''"`
}

func (BackupSyncSequence) TableName() string {
	return "backup_sync_sequences"
}

// BackupSyncOutbox records only mutation metadata. Sensitive account material
// is read from the encrypted source tables when a delivery is attempted.
type BackupSyncOutbox struct {
	BaseModel
	Generation  string `gorm:"type:varchar(64);not null;uniqueIndex:idx_backup_sync_outbox_generation_revision"`
	Revision    uint64 `gorm:"not null;uniqueIndex:idx_backup_sync_outbox_generation_revision"`
	AccountName string `gorm:"type:varchar(255);index"`
	Operation   string `gorm:"type:varchar(32);not null"`
	Status      string `gorm:"type:varchar(32);not null;index"`
}

func (BackupSyncOutbox) TableName() string {
	return "backup_sync_outbox"
}

// BackupSyncTarget tracks the desired and acknowledged revisions for the
// local agent or one enrolled remote node.
type BackupSyncTarget struct {
	BaseModel
	TargetKey          string     `gorm:"type:varchar(64);not null;uniqueIndex"`
	NodeID             uint       `gorm:"not null;default:0;index"`
	Active             bool       `gorm:"not null;default:true;index"`
	TargetEpoch        string     `gorm:"type:varchar(64);not null;default:''"`
	DesiredGeneration  string     `gorm:"type:varchar(64);not null;default:''"`
	DesiredRevision    uint64     `gorm:"not null;default:0"`
	AppliedTargetEpoch string     `gorm:"type:varchar(64);not null;default:''"`
	AppliedAuthority   string     `gorm:"type:varchar(64);not null;default:''"`
	AppliedGeneration  string     `gorm:"type:varchar(64);not null;default:''"`
	AppliedRevision    uint64     `gorm:"not null;default:0"`
	AppliedDigest      string     `gorm:"type:varchar(64);not null;default:''"`
	Status             string     `gorm:"type:varchar(32);not null;index"`
	Attempts           uint       `gorm:"not null;default:0"`
	NextRetryAt        *time.Time `gorm:"index"`
	LastAttemptAt      *time.Time
	LastSuccessAt      *time.Time
	LastError          string `gorm:"type:text"`
}

func (BackupSyncTarget) TableName() string {
	return "backup_sync_targets"
}

// BackupSyncTombstone preserves a public account deletion until every active
// target has acknowledged a revision at or beyond the deletion.
type BackupSyncTombstone struct {
	BaseModel
	AccountName string `gorm:"type:varchar(255);not null;uniqueIndex"`
	Generation  string `gorm:"type:varchar(64);not null;default:'';index"`
	Revision    uint64 `gorm:"not null;index"`
	Active      bool   `gorm:"not null;default:true;index"`
}

func (BackupSyncTombstone) TableName() string {
	return "backup_sync_tombstones"
}
