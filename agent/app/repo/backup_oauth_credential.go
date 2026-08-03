package repo

import (
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/global"
	"gorm.io/gorm/clause"
)

type BackupOAuthCredentialRepo struct{}

type IBackupOAuthCredentialRepo interface {
	GetByBackupAccountID(backupAccountID uint) (model.BackupOAuthCredential, error)
	List() ([]model.BackupOAuthCredential, error)
	Upsert(credential *model.BackupOAuthCredential) error
	DeleteByBackupAccountID(backupAccountID uint) error
}

func NewIBackupOAuthCredentialRepo() IBackupOAuthCredentialRepo {
	return &BackupOAuthCredentialRepo{}
}

func (r *BackupOAuthCredentialRepo) GetByBackupAccountID(backupAccountID uint) (model.BackupOAuthCredential, error) {
	var credential model.BackupOAuthCredential
	err := global.DB.Where("backup_account_id = ?", backupAccountID).First(&credential).Error
	return credential, err
}

func (r *BackupOAuthCredentialRepo) List() ([]model.BackupOAuthCredential, error) {
	var credentials []model.BackupOAuthCredential
	err := global.DB.Order("id ASC").Find(&credentials).Error
	return credentials, err
}

func (r *BackupOAuthCredentialRepo) Upsert(credential *model.BackupOAuthCredential) error {
	return global.DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "backup_account_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"provider", "client_id", "client_secret", "redirect_uri", "refresh_token", "is_cn", "status", "authorized_at", "updated_at",
		}),
	}).Create(credential).Error
}

func (r *BackupOAuthCredentialRepo) DeleteByBackupAccountID(backupAccountID uint) error {
	return global.DB.Where("backup_account_id = ?", backupAccountID).Delete(&model.BackupOAuthCredential{}).Error
}
