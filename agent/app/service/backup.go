package service

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/app/repo"
	"github.com/1Panel-dev/1Panel/agent/app/task"
	"github.com/1Panel-dev/1Panel/agent/buserr"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/i18n"
	backupcoord "github.com/1Panel-dev/1Panel/agent/utils/backupsync"
	"github.com/1Panel-dev/1Panel/agent/utils/cloud_storage"
	"github.com/1Panel-dev/1Panel/agent/utils/cloud_storage/client"
	"github.com/1Panel-dev/1Panel/agent/utils/encrypt"
	"github.com/1Panel-dev/1Panel/agent/utils/files"
	"github.com/1Panel-dev/1Panel/agent/utils/oauthflow"
	"github.com/jinzhu/copier"
	"gorm.io/gorm"
)

type BackupService struct{}

type IBackupService interface {
	CheckUsed(name string, isPublic bool) error

	LoadBackupOptions() ([]dto.BackupOption, error)
	SearchWithPage(search dto.SearchPageWithType) (int64, interface{}, error)
	BeginOAuth(req dto.OAuthBegin) (dto.OAuthBeginResponse, error)
	CompleteOAuth(req dto.OAuthComplete) (dto.OAuthCompleteResponse, error)
	GetOAuthCredential(id uint) (dto.OAuthCredentialInfo, error)
	ClearOAuthCredential(id uint) error
	SyncPublicAccounts(req dto.BackupPublicSync) (dto.BackupPublicSyncResult, error)
	Create(backupDto dto.BackupOperate) error
	CheckConn(req dto.BackupOperate) dto.BackupCheckRes
	GetBuckets(backupDto dto.ForBuckets) ([]interface{}, error)
	Update(req dto.BackupOperate) error
	Delete(id uint) error
	RefreshToken(req dto.OperateByID) error
	GetLocalDir() (string, error)
	UploadForRecover(req dto.UploadForRecover) error

	MysqlBackup(db dto.CommonBackup) error
	PostgresqlBackup(db dto.CommonBackup) error
	MongodbBackup(db dto.CommonBackup) error
	MysqlRecover(db dto.CommonRecover) error
	PostgresqlRecover(db dto.CommonRecover) error
	MongodbRecover(db dto.CommonRecover) error
	MysqlRecoverByUpload(req dto.CommonRecover) error
	PostgresqlRecoverByUpload(req dto.CommonRecover) error
	MongodbRecoverByUpload(req dto.CommonRecover) error

	RedisBackup(db dto.CommonBackup) error
	RedisRecover(db dto.CommonRecover) error

	WebsiteBackup(db dto.CommonBackup) error
	WebsiteRecover(req dto.CommonRecover) error

	AppBackup(db dto.CommonBackup) (*model.BackupRecord, error)
	AppRecover(req dto.CommonRecover) error

	ContainerBackup(req dto.CommonBackup) error
	ContainerRecover(req dto.CommonRecover) error
	ComposeBackup(req dto.CommonBackup) error
	ComposeRecover(req dto.CommonRecover) error
}

func NewIBackupService() IBackupService {
	return &BackupService{}
}

func (u *BackupService) GetLocalDir() (string, error) {
	account, err := backupRepo.Get(repo.WithByType(constant.Local))
	if err != nil {
		return "", err
	}
	return account.BackupPath, nil
}

func (u *BackupService) SearchWithPage(req dto.SearchPageWithType) (int64, interface{}, error) {
	options := []repo.DBOption{repo.WithOrderDesc("created_at")}
	if len(req.Type) != 0 {
		options = append(options, repo.WithByType(req.Type))
	}
	if len(req.Info) != 0 {
		options = append(options, repo.WithByType(req.Info))
	}
	count, accounts, err := backupRepo.Page(req.Page, req.PageSize, options...)
	if err != nil {
		return 0, nil, err
	}
	var data []dto.BackupInfo
	for _, account := range accounts {
		var item dto.BackupInfo
		if err := copier.Copy(&item, &account); err != nil {
			global.LOG.Errorf("copy backup account to dto backup info failed, err: %v", err)
		}
		if item.Type != constant.Sftp && item.Type != constant.Local {
			item.BackupPath = path.Join("/", strings.TrimPrefix(item.BackupPath, "/"))
		}
		if !item.RememberAuth {
			item.AccessKey = ""
			item.Credential = ""
			if account.Type == constant.Sftp {
				varMap := make(map[string]interface{})
				if err := json.Unmarshal([]byte(item.Vars), &varMap); err != nil {
					continue
				}
				delete(varMap, "passPhrase")
				itemVars, _ := json.Marshal(varMap)
				item.Vars = string(itemVars)
			}
		} else {
			item.AccessKey, _ = encrypt.StringDecryptWithBase64(item.AccessKey)
			item.Credential, _ = encrypt.StringDecryptWithBase64(item.Credential)
		}

		if isBackupOAuthType(account.Type) {
			credential, _ := backupOAuthRepo.GetByBackupAccountID(account.ID)
			info := buildOAuthCredentialInfo(account.Type, credential)
			item.OAuth = &info
			item.Vars = injectBackupOAuthInfo(item.Vars, info)
		}
		data = append(data, item)
	}
	return count, data, nil
}

func (u *BackupService) CheckConn(req dto.BackupOperate) dto.BackupCheckRes {
	var res dto.BackupCheckRes
	var backup model.BackupAccount
	if err := copier.Copy(&backup, &req); err != nil {
		res.Msg = i18n.GetMsgWithDetail("ErrStructTransform", err.Error())
		return res
	}
	itemAccessKey, err := base64.StdEncoding.DecodeString(backup.AccessKey)
	if err != nil {
		res.Msg = err.Error()
		return res
	}
	backup.AccessKey = string(itemAccessKey)
	itemCredential, err := base64.StdEncoding.DecodeString(backup.Credential)
	if err != nil {
		res.Msg = err.Error()
		return res
	}
	backup.Credential = string(itemCredential)

	if req.Type == constant.OneDrive || req.Type == constant.GoogleDrive {
		if strings.TrimSpace(req.OAuthSession) != "" {
			stored, err := peekBackupOAuthSession(req.OAuthSession, req.ID, req.Name, req.Type)
			if err != nil {
				res.Msg = err.Error()
				return res
			}
			if err := applyStoredOAuthToAccount(&backup, stored); err != nil {
				res.Msg = err.Error()
				return res
			}
			backup.ID = 0
		} else if req.ID == 0 {
			res.Msg = errOAuthAuthorizationNeeded.Error()
			return res
		}
	}
	isOk, err := u.checkBackupConn(&backup)
	if err != nil {
		res.Msg = err.Error()
		return res
	}
	res.IsOk = isOk
	return res
}

func (u *BackupService) Create(req dto.BackupOperate) error {
	if req.Type == constant.Local {
		return buserr.New("ErrBackupLocalCreate")
	}
	if req.IsPublic {
		return errPublicBackupManagedByCore
	}
	if req.Type != constant.Sftp {
		req.BackupPath = strings.TrimPrefix(req.BackupPath, "/")
	}
	backup, _ := backupRepo.Get(repo.WithByName(req.Name))
	if backup.ID != 0 {
		return buserr.New("ErrRecordExist")
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
	if isBackupOAuthType(req.Type) {
		if err := global.DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&backup).Error; err != nil {
				return err
			}
			if req.Type == constant.ALIYUN {
				return saveAliyunCredentialTx(tx, backup.ID, aliyunRefreshToken, false)
			}
			return saveBackupOAuthCredentialTx(tx, backup.ID, storedOAuth)
		}); err != nil {
			return err
		}
	} else if err := backupRepo.Create(&backup); err != nil {
		return err
	}
	return nil
}

func (u *BackupService) GetBuckets(req dto.ForBuckets) ([]interface{}, error) {
	itemAccessKey, err := base64.StdEncoding.DecodeString(req.AccessKey)
	if err != nil {
		return nil, err
	}
	req.AccessKey = string(itemAccessKey)
	itemCredential, err := base64.StdEncoding.DecodeString(req.Credential)
	if err != nil {
		return nil, err
	}
	req.Credential = string(itemCredential)

	varMap := make(map[string]interface{})
	if err := json.Unmarshal([]byte(req.Vars), &varMap); err != nil {
		return nil, err
	}
	switch req.Type {
	case constant.Sftp, constant.WebDAV:
		varMap["username"] = req.AccessKey
		varMap["password"] = req.Credential
	case constant.OSS, constant.S3, constant.MinIo, constant.Cos, constant.Kodo:
		varMap["accessKey"] = req.AccessKey
		varMap["secretKey"] = req.Credential
	}
	client, err := cloud_storage.NewCloudStorageClient(req.Type, varMap)
	if err != nil {
		return nil, err
	}
	return client.ListBuckets()
}

func (u *BackupService) Delete(id uint) error {
	backup, _ := backupRepo.Get(repo.WithByID(id))
	if backup.ID == 0 {
		return buserr.New("ErrRecordNotFound")
	}
	if backup.IsPublic {
		return errPublicBackupManagedByCore
	}
	if backup.Type == constant.Local {
		return buserr.New("ErrBackupLocalDelete")
	}
	if err := u.CheckUsed(backup.Name, false); err != nil {
		return err
	}
	if !isBackupOAuthType(backup.Type) {
		return backupRepo.Delete(repo.WithByID(id))
	}
	return global.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("backup_account_id = ?", backup.ID).Delete(&model.BackupOAuthCredential{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", backup.ID).Delete(&model.BackupAccount{}).Error
	})
}

func (u *BackupService) Update(req dto.BackupOperate) error {
	backup, _ := backupRepo.Get(repo.WithByID(req.ID))
	if backup.ID == 0 {
		return buserr.New("ErrRecordNotFound")
	}
	if backup.IsPublic || req.IsPublic {
		return errPublicBackupManagedByCore
	}
	if req.Type != constant.Sftp && req.Type != constant.Local && req.BackupPath != "/" {
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
	if backup.Type == constant.Local {
		if err := changeLocalBackup(backup.BackupPath, newBackup.BackupPath); err != nil {
			return err
		}
		global.Dir.LocalBackupDir = newBackup.BackupPath
	}

	if backup.Type != constant.Local {
		newBackup.AccessKey, err = encrypt.StringEncrypt(newBackup.AccessKey)
		if err != nil {
			return err
		}
		newBackup.Credential, err = encrypt.StringEncrypt(newBackup.Credential)
		if err != nil {
			return err
		}
	}

	newBackup.ID = backup.ID
	newBackup.CreatedAt = backup.CreatedAt
	newBackup.UpdatedAt = backup.UpdatedAt
	if isBackupOAuthType(backup.Type) || isBackupOAuthType(req.Type) {
		if err := global.DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Save(&newBackup).Error; err != nil {
				return err
			}
			switch req.Type {
			case constant.OneDrive, constant.GoogleDrive:
				if hasOAuthSession {
					return saveBackupOAuthCredentialTx(tx, backup.ID, storedOAuth)
				}
				return nil
			case constant.ALIYUN:
				return saveAliyunCredentialTx(tx, backup.ID, aliyunRefreshToken, true)
			default:
				return tx.Where("backup_account_id = ?", backup.ID).Delete(&model.BackupOAuthCredential{}).Error
			}
		}); err != nil {
			return err
		}
	} else if err := backupRepo.Save(&newBackup); err != nil {
		return err
	}
	return nil
}

func (u *BackupService) RefreshToken(req dto.OperateByID) error {
	backup, _ := backupRepo.Get(repo.WithByID(req.ID))
	if backup.ID == 0 {
		return buserr.New("ErrRecordNotFound")
	}
	if backup.IsPublic {
		return errPublicBackupManagedByCore
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
		newRefreshToken, err = client.RefreshToken("refresh_token", "refreshToken", varMap)
	case constant.GoogleDrive:
		newRefreshToken, err = client.RefreshGoogleToken("refresh_token", "refreshToken", varMap)
	case constant.ALIYUN:
		newRefreshToken, err = client.RefreshALIToken(varMap)
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
	return persistBackupOAuthRefreshResult(
		backup,
		credential,
		encryptedRefreshToken,
		model.BackupOAuthStatusConfigured,
		string(varsItem),
	)
}

func (u *BackupService) UploadForRecover(req dto.UploadForRecover) error {
	fileOp := files.NewFileOp()
	if !fileOp.Stat(req.TargetDir) {
		if err := fileOp.CreateDir(req.TargetDir, constant.DirPerm); err != nil {
			return err
		}
	}
	return fileOp.Copy(req.FilePath, req.TargetDir)
}

func (u *BackupService) checkBackupConn(backup *model.BackupAccount) (bool, error) {
	client, err := newClient(backup, false)
	if err != nil {
		return false, err
	}
	fileItem := path.Join(global.Dir.BaseDir, "1panel/tmp/test/1panel")
	if _, err := os.Stat(path.Dir(fileItem)); err != nil && os.IsNotExist(err) {
		if err = os.MkdirAll(path.Dir(fileItem), os.ModePerm); err != nil {
			return false, err
		}
	}
	file, err := os.OpenFile(fileItem, os.O_WRONLY|os.O_CREATE, constant.FilePerm)
	if err != nil {
		return false, err
	}
	defer file.Close()
	write := bufio.NewWriter(file)
	_, _ = write.WriteString("1Panel 备份账号测试文件。\n")
	_, _ = write.WriteString("1Panel 備份賬號測試文件。\n")
	_, _ = write.WriteString("1Panel Backs up account test files.\n")
	_, _ = write.WriteString("1Panelアカウントのテストファイルをバックアップします。\n")
	write.Flush()

	targetPath := path.Join(backup.BackupPath, "test/1panel")
	if backup.Type != constant.Sftp && backup.Type != constant.Local && targetPath != "/" {
		targetPath = strings.TrimPrefix(targetPath, "/")
	}

	if _, err := client.Upload(fileItem, targetPath); err != nil {
		return false, err
	}
	_, _ = client.Delete(path.Join(backup.BackupPath, "test/1panel"))
	return true, nil
}

func (u *BackupService) LoadBackupOptions() ([]dto.BackupOption, error) {
	accounts, err := backupRepo.List(repo.WithOrderDesc("created_at"))
	if err != nil {
		return nil, err
	}
	var data []dto.BackupOption
	for _, account := range accounts {
		var item dto.BackupOption
		if err := copier.Copy(&item, &account); err != nil {
			global.LOG.Errorf("copy backup account to dto backup info failed, err: %v", err)
		}
		data = append(data, item)
	}
	return data, nil
}

func (u *BackupService) CheckUsed(name string, isPublic bool) error {
	account, _ := backupRepo.Get(repo.WithByName(name), backupRepo.WithByPublic(isPublic))
	if account.ID == 0 {
		return nil
	}
	cronjobs, _ := cronjobRepo.List()
	for _, job := range cronjobs {
		if job.DownloadAccountID == account.ID {
			return buserr.New("ErrBackupInUsed")
		}
		ids := strings.Split(job.SourceAccountIDs, ",")
		for _, idItem := range ids {
			if idItem == fmt.Sprintf("%v", account.ID) {
				return buserr.New("ErrBackupInUsed")
			}
		}
	}
	return nil
}

func NewBackupClientWithID(id uint) (*model.BackupAccount, cloud_storage.CloudStorageClient, error) {
	account, _ := backupRepo.Get(repo.WithByID(id))
	backClient, err := newClient(&account, true)
	if err != nil {
		return nil, nil, err
	}
	return &account, backClient, nil
}

type backupClientHelper struct {
	id          uint
	accountType string
	name        string
	backupPath  string
	client      cloud_storage.CloudStorageClient

	isOk      bool
	hasBackup bool
	message   string
}

func NewBackupClientMap(ids []string) map[string]backupClientHelper {
	var accounts []model.BackupAccount
	var idItems []uint
	for i := 0; i < len(ids); i++ {
		item, _ := strconv.Atoi(ids[i])
		idItems = append(idItems, uint(item))
	}
	accounts, _ = backupRepo.List(repo.WithByIDs(idItems))
	clientMap := make(map[string]backupClientHelper)
	for _, item := range accounts {
		backClient, err := newClient(&item, true)
		itemHelper := backupClientHelper{
			client:      backClient,
			name:        item.Name,
			backupPath:  item.BackupPath,
			accountType: item.Type,
			id:          item.ID,
			isOk:        err == nil,
		}
		if err != nil {
			itemHelper.message = err.Error()
		}
		clientMap[fmt.Sprintf("%v", item.ID)] = itemHelper
	}
	return clientMap
}

func uploadWithMap(taskItem task.Task, accountMap map[string]backupClientHelper, src, dst, accountIDs string, downloadAccountID, retry uint) error {
	accounts := strings.Split(accountIDs, ",")
	var firstErr error
	for _, account := range accounts {
		if len(account) == 0 {
			continue
		}
		itemBackup, ok := accountMap[account]
		if !ok {
			if firstErr == nil {
				firstErr = fmt.Errorf("backup account %s is unavailable", account)
			}
			continue
		}
		if itemBackup.hasBackup {
			continue
		}
		if !itemBackup.isOk {
			taskItem.LogFailed(i18n.GetMsgWithDetail("LoadBackupFailed", itemBackup.message))
			if firstErr == nil {
				firstErr = errors.New(itemBackup.message)
			}
			continue
		}
		name := itemBackup.name
		if itemBackup.name == "localhost" {
			name = i18n.GetMsgByKey("Localhost")
		}
		taskItem.LogStart(i18n.GetMsgWithMap("UploadFile", map[string]interface{}{
			"file":   path.Join(itemBackup.backupPath, dst),
			"backup": name,
		}))
		var uploadErr error
		for i := 0; i < int(retry)+1; i++ {
			_, uploadErr = itemBackup.client.Upload(src, path.Join(itemBackup.backupPath, dst))
			taskItem.LogWithStatus(i18n.GetMsgByKey("Upload"), uploadErr)
			if uploadErr == nil {
				break
			}
		}
		if uploadErr != nil {
			if firstErr == nil {
				firstErr = uploadErr
			}
			if account == fmt.Sprintf("%d", downloadAccountID) {
				return uploadErr
			}
		}
		itemBackup.hasBackup = uploadErr == nil
		accountMap[account] = itemBackup
	}
	os.RemoveAll(src)
	return firstErr
}

func newClient(account *model.BackupAccount, isEncrypt bool) (cloud_storage.CloudStorageClient, error) {
	if account == nil {
		return nil, errors.New("backup account is unavailable")
	}
	workingAccount := *account
	var publicLease *publicBackupSyncLease
	if isEncrypt && workingAccount.IsPublic {
		releaseExecution := backupcoord.AcquireExecution()
		defer releaseExecution()
		if workingAccount.ID == 0 {
			return nil, errors.New("public backup account synchronization is unavailable; wait for reconciliation")
		}
		var current model.BackupAccount
		if err := global.DB.Where("id = ? AND is_public = ?", workingAccount.ID, true).First(&current).Error; err != nil {
			return nil, errors.New("public backup account changed during synchronization; retry after reconciliation")
		}
		if !samePublicBackupAccountSnapshot(workingAccount, current) {
			return nil, errors.New("public backup account changed during synchronization; retry after reconciliation")
		}
		lease, err := currentPublicBackupSyncLease(time.Now())
		if err != nil {
			return nil, err
		}
		workingAccount = current
		publicLease = &lease
	}
	account = &workingAccount
	varMap := make(map[string]interface{})
	if len(account.Vars) != 0 {
		if err := json.Unmarshal([]byte(account.Vars), &varMap); err != nil {
			return nil, err
		}
	}
	if err := injectPersistedOAuthCredential(account, varMap); err != nil {
		return nil, err
	}
	varMap["bucket"] = account.Bucket
	varMap["backupPath"] = account.BackupPath
	if isEncrypt {
		accessKey, accessKeyErr := encrypt.StringDecrypt(account.AccessKey)
		credential, credentialErr := encrypt.StringDecrypt(account.Credential)
		if account.IsPublic && (accessKeyErr != nil || credentialErr != nil) {
			return nil, errors.New("public backup account credential is unavailable; wait for reconciliation")
		}
		account.AccessKey = accessKey
		account.Credential = credential
	}
	switch account.Type {
	case constant.Sftp, constant.WebDAV:
		varMap["username"] = account.AccessKey
		varMap["password"] = account.Credential
	case constant.OSS, constant.S3, constant.MinIo, constant.Cos, constant.Kodo:
		varMap["accessKey"] = account.AccessKey
		varMap["secretKey"] = account.Credential
	case constant.UPYUN:
		varMap["operator"] = account.AccessKey
		varMap["password"] = account.Credential
	}

	client, err := cloud_storage.NewCloudStorageClient(account.Type, varMap)
	if err != nil {
		return nil, err
	}
	if publicLease != nil {
		return &publicBackupLeaseClient{client: client, lease: *publicLease}, nil
	}
	return client, nil
}

func samePublicBackupAccountSnapshot(left, right model.BackupAccount) bool {
	return left.ID == right.ID && left.Name == right.Name && left.Type == right.Type &&
		left.IsPublic == right.IsPublic && left.Bucket == right.Bucket &&
		left.AccessKey == right.AccessKey && left.Credential == right.Credential &&
		left.BackupPath == right.BackupPath && left.Vars == right.Vars &&
		left.RememberAuth == right.RememberAuth
}

type publicBackupLeaseClient struct {
	client cloud_storage.CloudStorageClient
	lease  publicBackupSyncLease
}

func (c *publicBackupLeaseClient) acquire() (func(), error) {
	releaseExecution := backupcoord.AcquireExecution()
	if err := validatePublicBackupSyncLease(c.lease, time.Now()); err != nil {
		releaseExecution()
		return nil, err
	}
	return releaseExecution, nil
}

func (c *publicBackupLeaseClient) ListBuckets() ([]interface{}, error) {
	release, err := c.acquire()
	if err != nil {
		return nil, err
	}
	defer release()
	return c.client.ListBuckets()
}

func (c *publicBackupLeaseClient) ListObjects(prefix string) ([]string, error) {
	release, err := c.acquire()
	if err != nil {
		return nil, err
	}
	defer release()
	return c.client.ListObjects(prefix)
}

func (c *publicBackupLeaseClient) Exist(target string) (bool, error) {
	release, err := c.acquire()
	if err != nil {
		return false, err
	}
	defer release()
	return c.client.Exist(target)
}

func (c *publicBackupLeaseClient) Delete(target string) (bool, error) {
	release, err := c.acquire()
	if err != nil {
		return false, err
	}
	defer release()
	return c.client.Delete(target)
}

func (c *publicBackupLeaseClient) Upload(source, target string) (bool, error) {
	release, err := c.acquire()
	if err != nil {
		return false, err
	}
	defer release()
	return c.client.Upload(source, target)
}

func (c *publicBackupLeaseClient) Download(source, target string) (bool, error) {
	release, err := c.acquire()
	if err != nil {
		return false, err
	}
	defer release()
	return c.client.Download(source, target)
}

func (c *publicBackupLeaseClient) Size(target string) (int64, error) {
	release, err := c.acquire()
	if err != nil {
		return 0, err
	}
	defer release()
	return c.client.Size(target)
}

func loadBackupNamesByID(accountIDs string, downloadID uint) ([]string, string, error) {
	accountIDList := strings.Split(accountIDs, ",")
	var ids []uint
	for _, item := range accountIDList {
		if len(item) != 0 {
			itemID, _ := strconv.Atoi(item)
			ids = append(ids, uint(itemID))
		}
	}
	list, err := backupRepo.List(repo.WithByIDs(ids))
	if err != nil {
		return nil, "", err
	}
	var accounts []string
	var downloadAccount string
	for _, item := range list {
		accounts = append(accounts, item.Name)
		if item.ID == downloadID {
			downloadAccount = item.Name
		}
	}
	return accounts, downloadAccount, nil
}

func changeLocalBackup(oldPath, newPath string) error {
	fileOp := files.NewFileOp()
	if fileOp.Stat(path.Join(oldPath, "app")) {
		if err := fileOp.CopyDir(path.Join(oldPath, "app"), newPath); err != nil {
			return err
		}
	}
	if fileOp.Stat(path.Join(oldPath, "database")) {
		if err := fileOp.CopyDir(path.Join(oldPath, "database"), newPath); err != nil {
			return err
		}
	}
	if fileOp.Stat(path.Join(oldPath, "directory")) {
		if err := fileOp.CopyDir(path.Join(oldPath, "directory"), newPath); err != nil {
			return err
		}
	}
	if fileOp.Stat(path.Join(oldPath, "system_snapshot")) {
		if err := fileOp.CopyDir(path.Join(oldPath, "system_snapshot"), newPath); err != nil {
			return err
		}
	}
	if fileOp.Stat(path.Join(oldPath, "website")) {
		if err := fileOp.CopyDir(path.Join(oldPath, "website"), newPath); err != nil {
			return err
		}
	}
	if fileOp.Stat(path.Join(oldPath, "log")) {
		if err := fileOp.CopyDir(path.Join(oldPath, "log"), newPath); err != nil {
			return err
		}
	}
	if fileOp.Stat(path.Join(oldPath, "master")) {
		if err := fileOp.CopyDir(path.Join(oldPath, "master"), newPath); err != nil {
			return err
		}
	}
	_ = fileOp.RmRf(path.Join(oldPath, "app"))
	_ = fileOp.RmRf(path.Join(oldPath, "database"))
	_ = fileOp.RmRf(path.Join(oldPath, "directory"))
	_ = fileOp.RmRf(path.Join(oldPath, "system_snapshot"))
	_ = fileOp.RmRf(path.Join(oldPath, "website"))
	_ = fileOp.RmRf(path.Join(oldPath, "log"))
	_ = fileOp.RmRf(path.Join(oldPath, "master"))
	return nil
}
