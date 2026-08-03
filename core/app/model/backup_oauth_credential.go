package model

import "time"

const (
	BackupOAuthProviderMicrosoft = "microsoft"
	BackupOAuthProviderGoogle    = "google"
	BackupOAuthProviderAliyun    = "aliyun"

	BackupOAuthStatusUnconfigured                  = "unconfigured"
	BackupOAuthStatusConfigured                    = "configured"
	BackupOAuthStatusReauthorizationRequired       = "reauthorization_required"
	BackupOAuthStatusLegacyReconfigurationRequired = "legacy_reconfiguration_required"

	BackupOAuthClientSecretEncryptionDomain  = "backup-oauth/client-secret/v1"
	BackupOAuthRefreshTokenEncryptionDomain  = "backup-oauth/refresh-token/v1"
	BackupOAuthLegacySettingEncryptionDomain = "backup-oauth/legacy-setting-secret/v1"
)

// BackupOAuthCredential owns the server-side OAuth material for one backup
// account. Secret fields are deliberately excluded from JSON serialization.
type BackupOAuthCredential struct {
	BaseModel
	BackupAccountID uint       `gorm:"not null;uniqueIndex" json:"backupAccountID"`
	Provider        string     `gorm:"type:varchar(32);not null" json:"provider"`
	ClientID        string     `gorm:"type:text" json:"-"`
	ClientSecret    string     `gorm:"type:text" json:"-"`
	RedirectURI     string     `gorm:"type:text" json:"redirectURI"`
	RefreshToken    string     `gorm:"type:text" json:"-"`
	IsCN            bool       `gorm:"not null;default:false" json:"isCN"`
	Status          string     `gorm:"type:varchar(64);not null" json:"status"`
	AuthorizedAt    *time.Time `json:"authorizedAt"`
}

func (BackupOAuthCredential) TableName() string {
	return "backup_oauth_credentials"
}
