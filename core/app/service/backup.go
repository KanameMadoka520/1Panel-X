package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/buserr"
	"github.com/1Panel-dev/1Panel/core/constant"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/utils/backupsync"
	"github.com/1Panel-dev/1Panel/core/utils/cloud_storage"
	"github.com/1Panel-dev/1Panel/core/utils/encrypt"
	"github.com/1Panel-dev/1Panel/core/utils/oauthflow"
	"github.com/1Panel-dev/1Panel/core/utils/req_helper/proxy_local"
	"github.com/1Panel-dev/1Panel/core/utils/xpack"
	"github.com/jinzhu/copier"
	"gorm.io/gorm"
)

type BackupService struct {
	localBackupUsageChecker func(string) error
}

var (
	publicBackupSyncWake       = make(chan struct{}, 1)
	publicBackupSyncWorkerOnce sync.Once
)

type IBackupService interface {
	LoadBackupClientInfo(clientType string) (dto.BackupClientInfo, error)
	BeginOAuth(req dto.OAuthBegin) (dto.OAuthBeginResponse, error)
	CompleteOAuth(req dto.OAuthComplete) (dto.OAuthCompleteResponse, error)
	GetOAuthCredential(name string) (dto.OAuthCredentialInfo, error)
	ClearOAuthCredential(name string) error
	GetSyncStatus(name string) (dto.BackupSyncStatus, error)
	ListSyncStatuses() ([]dto.BackupSyncStatus, error)
	RetrySync(name string) (dto.BackupSyncStatus, error)
	Create(backupDto dto.BackupOperate) error
	Update(req dto.BackupOperate) error
	Delete(name string) error
	RefreshToken(req dto.OperateByName) error
}

func NewIBackupService() IBackupService {
	return &BackupService{}
}

func (u *BackupService) LoadBackupClientInfo(clientType string) (dto.BackupClientInfo, error) {
	if clientType != constant.OneDrive && clientType != constant.GoogleDrive {
		return dto.BackupClientInfo{}, fmt.Errorf("unsupported OAuth backup provider")
	}
	return dto.BackupClientInfo{
		Provider:    clientType,
		Configured:  false,
		RedirectURI: constant.OneDriveRedirectURI,
		Status:      model.BackupOAuthStatusUnconfigured,
	}, nil
}

func (u *BackupService) Create(req dto.BackupOperate) error {
	if !req.IsPublic {
		return buserr.New("ErrBackupPublic")
	}
	backup, _ := backupRepo.Get(repo.WithByName(req.Name))
	if backup.ID != 0 {
		return buserr.New("ErrRecordExist")
	}
	if req.Type != constant.Sftp && req.BackupPath != "/" {
		req.BackupPath = strings.TrimPrefix(req.BackupPath, "/")
	}
	if err := copier.Copy(&backup, &req); err != nil {
		return buserr.WithDetail("ErrStructTransform", err.Error(), nil)
	}
	var err error
	var storedOAuth oauthflow.StoredResult
	var aliyunRefreshToken string
	if req.Type == constant.OneDrive || req.Type == constant.GoogleDrive {
		storedOAuth, err = consumeBackupOAuthSession(req.OAuthSession, 0, req.Name, req.Type)
		if err != nil {
			return err
		}
		_, backup.Vars, err = sanitizeBackupOAuthVars(backup.Vars)
		if err != nil {
			return err
		}
	} else if req.Type == constant.ALIYUN {
		vars := make(map[string]interface{})
		if err := json.Unmarshal([]byte(backup.Vars), &vars); err != nil {
			return errors.New("Aliyun backup metadata is invalid")
		}
		aliyunRefreshToken, err = extractAliyunRefreshToken(vars)
		if err != nil {
			return err
		}
		backup.Vars, err = sanitizeBackupOAuthVarsMap(vars)
		if err != nil {
			return err
		}
	}
	itemAccessKey, err := base64.StdEncoding.DecodeString(backup.AccessKey)
	if err != nil {
		return err
	}
	backup.AccessKey = string(itemAccessKey)
	itemCredential, err := base64.StdEncoding.DecodeString(backup.Credential)
	if err != nil {
		return err
	}
	backup.Credential = string(itemCredential)

	backup.AccessKey, err = encrypt.StringEncrypt(backup.AccessKey)
	if err != nil {
		return err
	}
	backup.Credential, err = encrypt.StringEncrypt(backup.Credential)
	if err != nil {
		return err
	}
	releaseDesiredState := backupsync.AcquireDesiredStateMutation()
	defer releaseDesiredState()
	if err := global.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&backup).Error; err != nil {
			return err
		}
		if isBackupOAuthType(req.Type) {
			if req.Type == constant.ALIYUN {
				if err := saveAliyunCredentialTx(tx, backup.ID, aliyunRefreshToken, false); err != nil {
					return err
				}
			} else if err := saveBackupOAuthCredentialTx(tx, backup.ID, storedOAuth); err != nil {
				return err
			}
		}
		_, err := backupsync.EnqueueTx(tx, backup.Name, model.BackupSyncOperationCreate)
		return err
	}); err != nil {
		return err
	}
	triggerPublicBackupSync()
	return nil
}

func (u *BackupService) Delete(name string) error {
	backup, _ := backupRepo.Get(repo.WithByName(name))
	if backup.ID == 0 {
		return buserr.New("ErrRecordNotFound")
	}
	if !backup.IsPublic {
		return buserr.New("ErrBackupPublic")
	}
	if backup.Type == constant.Local {
		return buserr.New("ErrBackupLocal")
	}
	checkLocalBackupUsed := u.localBackupUsageChecker
	if checkLocalBackupUsed == nil {
		checkLocalBackupUsed = func(name string) error {
			_, err := proxy_local.NewLocalClient(fmt.Sprintf("/api/v2/backups/check/%s", name), http.MethodGet, nil, nil)
			return err
		}
	}
	if err := checkLocalBackupUsed(name); err != nil {
		global.LOG.Errorf("check used of local cronjob failed, err: %v", err)
		return buserr.New("ErrBackupInUsed")
	}
	if err := xpack.MultiNodeProvider.CheckBackupUsed(name); err != nil {
		global.LOG.Errorf("check used of node cronjob failed, err: %v", err)
		return buserr.New("ErrBackupInUsed")
	}

	releaseDesiredState := backupsync.AcquireDesiredStateMutation()
	defer releaseDesiredState()
	if err := global.DB.Transaction(func(tx *gorm.DB) error {
		if isBackupOAuthType(backup.Type) {
			if err := tx.Where("backup_account_id = ?", backup.ID).Delete(&model.BackupOAuthCredential{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("id = ?", backup.ID).Delete(&model.BackupAccount{}).Error; err != nil {
			return err
		}
		_, err := backupsync.EnqueueTx(tx, backup.Name, model.BackupSyncOperationDelete)
		return err
	}); err != nil {
		return err
	}
	triggerPublicBackupSync()
	return nil
}

func (u *BackupService) Update(req dto.BackupOperate) error {
	backup, _ := backupRepo.Get(repo.WithByName(req.Name))
	if backup.ID == 0 {
		return buserr.New("ErrRecordNotFound")
	}
	if !backup.IsPublic {
		return buserr.New("ErrBackupPublic")
	}
	if backup.Type == constant.Local {
		return buserr.New("ErrBackupLocal")
	}
	if req.Type != constant.Sftp && req.BackupPath != "/" {
		req.BackupPath = strings.TrimPrefix(req.BackupPath, "/")
	}
	var newBackup model.BackupAccount
	if err := copier.Copy(&newBackup, &req); err != nil {
		return buserr.WithDetail("ErrStructTransform", err.Error(), nil)
	}
	var err error
	var storedOAuth oauthflow.StoredResult
	hasOAuthSession := strings.TrimSpace(req.OAuthSession) != ""
	var aliyunRefreshToken string
	if req.Type == constant.OneDrive || req.Type == constant.GoogleDrive {
		if hasOAuthSession {
			storedOAuth, err = consumeBackupOAuthSession(req.OAuthSession, backup.ID, req.Name, req.Type)
			if err != nil {
				return err
			}
		} else {
			credential, _ := backupOAuthRepo.GetByBackupAccountID(backup.ID)
			if credential.ID == 0 || credential.Provider == model.BackupOAuthProviderAliyun || backup.Type != req.Type {
				return errOAuthAuthorizationNeeded
			}
		}
		_, newBackup.Vars, err = sanitizeBackupOAuthVars(newBackup.Vars)
		if err != nil {
			return err
		}
	} else if req.Type == constant.ALIYUN {
		vars := make(map[string]interface{})
		if err := json.Unmarshal([]byte(newBackup.Vars), &vars); err != nil {
			return errors.New("Aliyun backup metadata is invalid")
		}
		aliyunRefreshToken, err = extractAliyunRefreshToken(vars)
		if err != nil {
			return err
		}
		newBackup.Vars, err = sanitizeBackupOAuthVarsMap(vars)
		if err != nil {
			return err
		}
	}
	newBackup.ID = backup.ID
	itemAccessKey, err := base64.StdEncoding.DecodeString(newBackup.AccessKey)
	if err != nil {
		return err
	}
	newBackup.AccessKey = string(itemAccessKey)
	itemCredential, err := base64.StdEncoding.DecodeString(newBackup.Credential)
	if err != nil {
		return err
	}
	newBackup.Credential = string(itemCredential)
	newBackup.AccessKey, err = encrypt.StringEncrypt(newBackup.AccessKey)
	if err != nil {
		return err
	}
	newBackup.Credential, err = encrypt.StringEncrypt(newBackup.Credential)
	if err != nil {
		return err
	}
	newBackup.ID = backup.ID
	newBackup.CreatedAt = backup.CreatedAt
	newBackup.UpdatedAt = backup.UpdatedAt
	releaseDesiredState := backupsync.AcquireDesiredStateMutation()
	defer releaseDesiredState()
	if err := global.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&newBackup).Error; err != nil {
			return err
		}
		if isBackupOAuthType(backup.Type) || isBackupOAuthType(req.Type) {
			switch req.Type {
			case constant.OneDrive, constant.GoogleDrive:
				if hasOAuthSession {
					if err := saveBackupOAuthCredentialTx(tx, backup.ID, storedOAuth); err != nil {
						return err
					}
				}
			case constant.ALIYUN:
				if err := saveAliyunCredentialTx(tx, backup.ID, aliyunRefreshToken, true); err != nil {
					return err
				}
			default:
				if err := tx.Where("backup_account_id = ?", backup.ID).Delete(&model.BackupOAuthCredential{}).Error; err != nil {
					return err
				}
			}
		}
		_, err := backupsync.EnqueueTx(tx, newBackup.Name, model.BackupSyncOperationUpdate)
		return err
	}); err != nil {
		return err
	}
	triggerPublicBackupSync()
	return nil
}

func (u *BackupService) RefreshToken(req dto.OperateByName) error {
	backup, _ := backupRepo.Get(repo.WithByName(req.Name))
	if backup.ID == 0 {
		return buserr.New("ErrRecordNotFound")
	}
	if !backup.IsPublic {
		return buserr.New("ErrBackupPublic")
	}
	credential, _ := backupOAuthRepo.GetByBackupAccountID(backup.ID)
	if credential.ID == 0 {
		return errOAuthNotConfigured
	}
	if credential.Status == model.BackupOAuthStatusLegacyReconfigurationRequired {
		return errOAuthReconfiguration
	}
	if credential.Status != model.BackupOAuthStatusConfigured || credential.RefreshToken == "" {
		return errOAuthNotConfigured
	}
	refreshToken, err := encrypt.StringDecryptGCM(credential.RefreshToken, model.BackupOAuthRefreshTokenEncryptionDomain)
	if err != nil || refreshToken == "" {
		return errors.New("stored OAuth refresh token cannot be decrypted; authorize the backup account again")
	}
	varMap, _, err := sanitizeBackupOAuthVars(backup.Vars)
	if err != nil {
		return err
	}
	varMap["refresh_token"] = refreshToken
	varMap["client_id"] = credential.ClientID
	varMap["redirect_uri"] = credential.RedirectURI
	varMap["isCN"] = credential.IsCN
	if credential.ClientSecret != "" {
		clientSecret, decryptErr := encrypt.StringDecryptGCM(credential.ClientSecret, model.BackupOAuthClientSecretEncryptionDomain)
		if decryptErr != nil {
			return errors.New("stored OAuth client secret cannot be decrypted; replace the credential")
		}
		varMap["client_secret"] = clientSecret
	}
	newRefreshToken := refreshToken
	switch backup.Type {
	case constant.OneDrive:
		newRefreshToken, err = cloud_storage.RefreshToken("refresh_token", "refreshToken", varMap)
	case constant.GoogleDrive:
		newRefreshToken, err = cloud_storage.RefreshGoogleToken("refresh_token", "refreshToken", varMap)
	case constant.ALIYUN:
		newRefreshToken, err = cloud_storage.RefreshALIToken(varMap)
	default:
		return errors.New("backup account does not use a refreshable OAuth token")
	}
	delete(varMap, "refresh_token")
	delete(varMap, "client_id")
	delete(varMap, "client_secret")
	delete(varMap, "redirect_uri")
	if err != nil {
		varMap["refresh_status"] = constant.StatusFailed
		varMap["refresh_msg"] = "OAuth refresh failed; authorize the backup account again"
		varsItem, marshalErr := json.Marshal(varMap)
		if marshalErr != nil {
			return marshalErr
		}
		if persistErr := persistBackupOAuthRefreshResult(
			backup,
			credential,
			credential.RefreshToken,
			model.BackupOAuthStatusReauthorizationRequired,
			string(varsItem),
		); persistErr != nil {
			return persistErr
		}
		triggerPublicBackupSync()
		return errors.New("OAuth token refresh failed; authorize the backup account again")
	}
	varMap["refresh_status"] = constant.StatusSuccess
	varMap["refresh_time"] = time.Now().Format(constant.DateTimeLayout)
	delete(varMap, "refresh_msg")
	encryptedRefreshToken, err := encrypt.StringEncryptGCM(newRefreshToken, model.BackupOAuthRefreshTokenEncryptionDomain)
	if err != nil {
		return err
	}
	varsItem, err := json.Marshal(varMap)
	if err != nil {
		return err
	}
	if err := persistBackupOAuthRefreshResult(
		backup,
		credential,
		encryptedRefreshToken,
		model.BackupOAuthStatusConfigured,
		string(varsItem),
	); err != nil {
		return err
	}
	triggerPublicBackupSync()
	return nil
}

func triggerPublicBackupSync() {
	select {
	case publicBackupSyncWake <- struct{}{}:
	default:
	}
}

// StartPublicBackupSyncWorker starts one coalescing process-local wake loop.
// Durable retry state remains in the database; the channel is only a prompt to
// attempt delivery without holding the administrator's request open.
func StartPublicBackupSyncWorker() {
	publicBackupSyncWorkerOnce.Do(func() {
		if err := backupsync.EnqueueStartupReconciliation(); err != nil {
			global.LOG.Warn("public backup account startup reconciliation could not be queued")
		}
		go func() {
			for range publicBackupSyncWake {
				reconcilePublicBackupSync()
			}
		}()
	})
	triggerPublicBackupSync()
}

func reconcilePublicBackupSync() {
	if err := xpack.MultiNodeProvider.Sync(constant.SyncBackupAccounts); err != nil {
		global.LOG.Warn("public backup account synchronization remains pending")
	}
}
