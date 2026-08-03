package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/utils/encrypt"
	"gorm.io/gorm"
)

const maxPublicBackupSyncAccounts = 1000

func (u *BackupService) SyncPublicAccounts(req dto.BackupPublicSync) error {
	if len(req.Accounts) > maxPublicBackupSyncAccounts {
		return errors.New("public backup account snapshot exceeds limit")
	}
	return global.DB.Transaction(func(tx *gorm.DB) error {
		var existing []model.BackupAccount
		if err := tx.Where("is_public = ?", true).Find(&existing).Error; err != nil {
			return err
		}
		existingByName := make(map[string]model.BackupAccount, len(existing))
		for _, account := range existing {
			existingByName[account.Name] = account
		}

		seen := make(map[string]struct{}, len(req.Accounts))
		for _, incoming := range req.Accounts {
			name := strings.TrimSpace(incoming.Name)
			if name == "" || !incoming.IsPublic {
				return errors.New("invalid public backup account snapshot")
			}
			if _, exists := seen[name]; exists {
				return fmt.Errorf("duplicate public backup account %q", name)
			}
			seen[name] = struct{}{}

			var privateCount int64
			if err := tx.Model(&model.BackupAccount{}).
				Where("name = ? AND is_public = ?", name, false).
				Count(&privateCount).Error; err != nil {
				return err
			}
			if privateCount != 0 {
				return fmt.Errorf("public backup account %q conflicts with a private account", name)
			}

			accessKey, err := encrypt.StringEncrypt(incoming.AccessKey)
			if err != nil {
				return fmt.Errorf("encrypt access key for public backup account %q: %w", name, err)
			}
			credential, err := encrypt.StringEncrypt(incoming.Credential)
			if err != nil {
				return fmt.Errorf("encrypt credential for public backup account %q: %w", name, err)
			}
			vars := incoming.Vars
			if isBackupOAuthType(incoming.Type) {
				_, vars, err = sanitizeBackupOAuthVars(vars)
				if err != nil {
					return fmt.Errorf("sanitize public OAuth account %q: %w", name, err)
				}
			} else if incoming.OAuth != nil {
				return fmt.Errorf("public backup account %q has OAuth data for a non-OAuth provider", name)
			}

			account := model.BackupAccount{
				Name:         name,
				Type:         incoming.Type,
				IsPublic:     true,
				Bucket:       incoming.Bucket,
				AccessKey:    accessKey,
				Credential:   credential,
				BackupPath:   incoming.BackupPath,
				Vars:         vars,
				RememberAuth: incoming.RememberAuth,
			}
			if old, exists := existingByName[name]; exists {
				account.ID = old.ID
				account.CreatedAt = old.CreatedAt
				delete(existingByName, name)
			}
			if err := tx.Save(&account).Error; err != nil {
				return err
			}

			if incoming.OAuth == nil {
				if err := tx.Where("backup_account_id = ?", account.ID).Delete(&model.BackupOAuthCredential{}).Error; err != nil {
					return err
				}
				continue
			}
			if err := syncPublicOAuthCredential(tx, account, *incoming.OAuth); err != nil {
				return err
			}
		}

		for _, stale := range existingByName {
			if err := tx.Where("backup_account_id = ?", stale.ID).Delete(&model.BackupOAuthCredential{}).Error; err != nil {
				return err
			}
			if err := tx.Where("id = ?", stale.ID).Delete(&model.BackupAccount{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func syncPublicOAuthCredential(tx *gorm.DB, account model.BackupAccount, incoming dto.BackupOAuthSecretSync) error {
	wantProvider := ""
	switch account.Type {
	case constant.OneDrive:
		wantProvider = model.BackupOAuthProviderMicrosoft
	case constant.GoogleDrive:
		wantProvider = model.BackupOAuthProviderGoogle
	case constant.ALIYUN:
		wantProvider = model.BackupOAuthProviderAliyun
	default:
		return errors.New("unsupported public OAuth backup provider")
	}
	if incoming.Provider != wantProvider || !validBackupOAuthStatus(incoming.Status) {
		return fmt.Errorf("invalid OAuth state for public backup account %q", account.Name)
	}
	clientSecret, err := encrypt.StringEncryptGCM(incoming.ClientSecret, model.BackupOAuthClientSecretEncryptionDomain)
	if err != nil {
		return fmt.Errorf("encrypt OAuth client secret for public backup account %q: %w", account.Name, err)
	}
	refreshToken, err := encrypt.StringEncryptGCM(incoming.RefreshToken, model.BackupOAuthRefreshTokenEncryptionDomain)
	if err != nil {
		return fmt.Errorf("encrypt OAuth refresh token for public backup account %q: %w", account.Name, err)
	}
	credential := model.BackupOAuthCredential{
		BackupAccountID: account.ID,
		Provider:        incoming.Provider,
		ClientID:        incoming.ClientID,
		ClientSecret:    clientSecret,
		RedirectURI:     incoming.RedirectURI,
		RefreshToken:    refreshToken,
		IsCN:            incoming.IsCN,
		Status:          incoming.Status,
		AuthorizedAt:    incoming.AuthorizedAt,
	}
	return upsertBackupOAuthCredentialTx(tx, &credential)
}

func validBackupOAuthStatus(status string) bool {
	switch status {
	case model.BackupOAuthStatusConfigured,
		model.BackupOAuthStatusUnconfigured,
		model.BackupOAuthStatusReauthorizationRequired,
		model.BackupOAuthStatusLegacyReconfigurationRequired:
		return true
	default:
		return false
	}
}
