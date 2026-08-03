package service

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/buserr"
	"github.com/1Panel-dev/1Panel/core/constant"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/utils/encrypt"
	"github.com/1Panel-dev/1Panel/core/utils/oauthflow"
	"github.com/1Panel-dev/1Panel/core/utils/xpack"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var backupOAuthFlowManager = oauthflow.NewManager(oauthflow.Options{})

var (
	errOAuthCredentialsRequired = errors.New("OAuth client ID, client secret, and redirect URI are required")
	errOAuthAuthorizationNeeded = errors.New("OAuth authorization is required before saving this backup account")
	errOAuthReconfiguration     = errors.New("this backup account uses a retired shared OAuth application; configure your own application and authorize again")
	errOAuthNotConfigured       = errors.New("OAuth credentials are not configured; configure the application and authorize this backup account")
	errOAuthCredentialChanged   = errors.New("OAuth credential changed while the refresh was in progress; retry the operation")
)

func (u *BackupService) BeginOAuth(req dto.OAuthBegin) (dto.OAuthBeginResponse, error) {
	var response dto.OAuthBeginResponse
	provider, _, err := backupOAuthProvider(req.Provider)
	if err != nil {
		return response, err
	}
	if strings.TrimSpace(req.AccountName) == "" {
		return response, errors.New("backup account name is required")
	}

	account, _ := backupRepo.Get(repo.WithByName(req.AccountName))
	if account.ID != 0 {
		if !account.IsPublic {
			return response, buserr.New("ErrBackupPublic")
		}
		if account.Type != req.Provider {
			return response, errors.New("OAuth provider does not match the backup account")
		}
	}

	clientID := strings.TrimSpace(req.ClientID)
	clientSecret := req.ClientSecret
	redirectURI := strings.TrimSpace(req.RedirectURI)
	var existing model.BackupOAuthCredential
	if account.ID != 0 {
		existing, _ = backupOAuthRepo.GetByBackupAccountID(account.ID)
	}
	if existing.ID != 0 {
		if existing.Status == model.BackupOAuthStatusLegacyReconfigurationRequired && (clientID == "" || clientSecret == "") {
			return response, errOAuthReconfiguration
		}
		if clientID == "" {
			clientID = existing.ClientID
		}
		if clientSecret == "" && existing.ClientSecret != "" {
			clientSecret, err = encrypt.StringDecryptGCM(existing.ClientSecret, model.BackupOAuthClientSecretEncryptionDomain)
			if err != nil {
				return response, errors.New("stored OAuth client secret cannot be decrypted; replace the credential")
			}
		}
		if redirectURI == "" {
			redirectURI = existing.RedirectURI
		}
	}
	if redirectURI == "" {
		redirectURI = constant.OneDriveRedirectURI
	}
	if clientID == "" || clientSecret == "" || redirectURI == "" {
		return response, errOAuthCredentialsRequired
	}

	result, err := backupOAuthFlowManager.Begin(oauthflow.BeginInput{
		Provider:        provider,
		ClientID:        clientID,
		ClientSecret:    clientSecret,
		RedirectURI:     redirectURI,
		IsCN:            req.IsCN,
		AccountIdentity: backupOAuthAccountIdentity(account.ID, req.AccountName, req.Provider),
	})
	if err != nil {
		return response, err
	}
	response.FlowID = result.FlowID
	response.AuthorizationURL = result.AuthorizationURL
	response.ClientIDDisplay = result.MaskedClientID
	response.ExpiresAt = result.ExpiresAt.UTC().Format(time.RFC3339)
	return response, nil
}

func (u *BackupService) CompleteOAuth(req dto.OAuthComplete) (dto.OAuthCompleteResponse, error) {
	var response dto.OAuthCompleteResponse
	result, err := backupOAuthFlowManager.Complete(req.AuthorizationResponse)
	if err != nil {
		return response, err
	}
	if result.FlowID != strings.TrimSpace(req.FlowID) {
		backupOAuthFlowManager.Delete(result.FlowID)
		return response, errors.New("OAuth flow does not match the authorization response")
	}
	stored, err := backupOAuthFlowManager.Peek(result.FlowID)
	if err != nil {
		return response, err
	}
	response.SessionID = result.FlowID
	response.Provider = string(stored.Provider)
	response.ClientIDDisplay = oauthflow.MaskClientID(stored.ClientID)
	response.ExpiresAt = result.ExpiresAt.UTC().Format(time.RFC3339)
	return response, nil
}

func (u *BackupService) GetOAuthCredential(name string) (dto.OAuthCredentialInfo, error) {
	account, _ := backupRepo.Get(repo.WithByName(name))
	if account.ID == 0 {
		return dto.OAuthCredentialInfo{}, buserr.New("ErrRecordNotFound")
	}
	if !isBackupOAuthType(account.Type) {
		return dto.OAuthCredentialInfo{}, errors.New("backup account does not use OAuth")
	}
	credential, _ := backupOAuthRepo.GetByBackupAccountID(account.ID)
	return buildOAuthCredentialInfo(account.Type, credential), nil
}

func (u *BackupService) ClearOAuthCredential(name string) error {
	account, _ := backupRepo.Get(repo.WithByName(name))
	if account.ID == 0 {
		return buserr.New("ErrRecordNotFound")
	}
	if !account.IsPublic {
		return buserr.New("ErrBackupPublic")
	}
	if err := global.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("backup_account_id = ?", account.ID).Delete(&model.BackupOAuthCredential{}).Error; err != nil {
			return err
		}
		vars, _, err := sanitizeBackupOAuthVars(account.Vars)
		if err != nil {
			return err
		}
		vars["oauth_status"] = model.BackupOAuthStatusUnconfigured
		encoded, err := json.Marshal(vars)
		if err != nil {
			return err
		}
		return tx.Model(&model.BackupAccount{}).Where("id = ?", account.ID).Update("vars", string(encoded)).Error
	}); err != nil {
		return err
	}
	if err := xpack.MultiNodeProvider.Sync(constant.SyncBackupAccounts); err != nil {
		return fmt.Errorf("OAuth credential was cleared in core but could not be cleared from the execution agent: %w", err)
	}
	return nil
}

func backupOAuthProvider(backupType string) (oauthflow.Provider, string, error) {
	switch backupType {
	case constant.OneDrive:
		return oauthflow.ProviderOneDrive, model.BackupOAuthProviderMicrosoft, nil
	case constant.GoogleDrive:
		return oauthflow.ProviderGoogleDrive, model.BackupOAuthProviderGoogle, nil
	default:
		return "", "", errors.New("unsupported OAuth backup provider")
	}
}

func isBackupOAuthType(backupType string) bool {
	return backupType == constant.OneDrive || backupType == constant.GoogleDrive || backupType == constant.ALIYUN
}

func backupOAuthAccountIdentity(id uint, name, backupType string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(name)))
	return fmt.Sprintf("%s:%d:%x", backupType, id, sum[:])
}

func buildOAuthCredentialInfo(backupType string, credential model.BackupOAuthCredential) dto.OAuthCredentialInfo {
	status := model.BackupOAuthStatusUnconfigured
	if credential.ID != 0 && credential.Status != "" {
		status = credential.Status
	}
	info := dto.OAuthCredentialInfo{
		Provider:                backupType,
		Configured:              credential.ID != 0 && credential.ClientID != "" && credential.ClientSecret != "",
		Authorized:              credential.ID != 0 && credential.RefreshToken != "" && status == model.BackupOAuthStatusConfigured,
		ClientIDDisplay:         oauthflow.MaskClientID(credential.ClientID),
		RedirectURI:             credential.RedirectURI,
		Status:                  status,
		RequiresReauthorization: status != model.BackupOAuthStatusConfigured,
	}
	if credential.ID != 0 {
		info.UpdatedAt = credential.UpdatedAt.UTC().Format(time.RFC3339)
	}
	if credential.ClientID == "" {
		info.ClientIDDisplay = ""
	}
	if credential.Provider == model.BackupOAuthProviderAliyun {
		info.Configured = credential.RefreshToken != ""
		info.Authorized = credential.RefreshToken != "" && status == model.BackupOAuthStatusConfigured
	}
	return info
}

func consumeBackupOAuthSession(sessionID string, accountID uint, accountName, backupType string) (oauthflow.StoredResult, error) {
	if strings.TrimSpace(sessionID) == "" {
		return oauthflow.StoredResult{}, errOAuthAuthorizationNeeded
	}
	stored, err := backupOAuthFlowManager.Consume(sessionID)
	if err != nil {
		return oauthflow.StoredResult{}, err
	}
	if stored.AccountIdentity != backupOAuthAccountIdentity(accountID, accountName, backupType) || string(stored.Provider) != backupType {
		return oauthflow.StoredResult{}, errors.New("OAuth session does not belong to this backup account")
	}
	return stored, nil
}

func saveBackupOAuthCredentialTx(tx *gorm.DB, accountID uint, stored oauthflow.StoredResult) error {
	provider, providerName, err := backupOAuthProvider(string(stored.Provider))
	if err != nil {
		return err
	}
	_ = provider
	clientSecret, err := encrypt.StringEncryptGCM(stored.ClientSecret, model.BackupOAuthClientSecretEncryptionDomain)
	if err != nil {
		return err
	}
	refreshToken, err := encrypt.StringEncryptGCM(stored.RefreshToken, model.BackupOAuthRefreshTokenEncryptionDomain)
	if err != nil {
		return err
	}
	now := time.Now()
	credential := model.BackupOAuthCredential{
		BackupAccountID: accountID,
		Provider:        providerName,
		ClientID:        stored.ClientID,
		ClientSecret:    clientSecret,
		RedirectURI:     stored.RedirectURI,
		RefreshToken:    refreshToken,
		IsCN:            stored.IsCN,
		Status:          model.BackupOAuthStatusConfigured,
		AuthorizedAt:    &now,
	}
	return upsertBackupOAuthCredentialTx(tx, &credential)
}

func upsertBackupOAuthCredentialTx(tx *gorm.DB, credential *model.BackupOAuthCredential) error {
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "backup_account_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"provider", "client_id", "client_secret", "redirect_uri", "refresh_token", "is_cn", "status", "authorized_at", "updated_at",
		}),
	}).Create(credential).Error
}

func sanitizeBackupOAuthVars(raw string) (map[string]interface{}, string, error) {
	vars := make(map[string]interface{})
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &vars); err != nil {
			return nil, "", errors.New("backup OAuth metadata is invalid")
		}
	}
	if vars == nil {
		vars = make(map[string]interface{})
	}
	encoded, err := sanitizeBackupOAuthVarsMap(vars)
	return vars, encoded, err
}

func sanitizeBackupOAuthVarsMap(vars map[string]interface{}) (string, error) {
	if vars == nil {
		vars = make(map[string]interface{})
	}
	for key := range vars {
		if isSensitiveBackupOAuthVarKey(key) {
			delete(vars, key)
		}
	}
	encoded, err := json.Marshal(vars)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func isSensitiveBackupOAuthVarKey(key string) bool {
	normalized := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(strings.TrimSpace(key)))
	switch normalized {
	case "accesstoken",
		"authorizationcode",
		"authorizationresponse",
		"authorizationurl",
		"clientid",
		"clientsecret",
		"code",
		"codechallenge",
		"codeverifier",
		"flowid",
		"idtoken",
		"oauthauthorized",
		"oauthclientiddisplay",
		"oauthconfigured",
		"oauthexpiresat",
		"oauthsession",
		"oauthstatus",
		"oauthupdatedat",
		"pkceverifier",
		"redirecturi",
		"refreshtoken",
		"sessionid",
		"state",
		"token":
		return true
	default:
		return false
	}
}

func extractAliyunRefreshToken(vars map[string]interface{}) (string, error) {
	refreshToken, _ := vars["refresh_token"].(string)
	if rawToken, ok := vars["token"].(string); ok && strings.TrimSpace(rawToken) != "" {
		var token map[string]interface{}
		if err := json.Unmarshal([]byte(rawToken), &token); err != nil {
			return "", errors.New("Aliyun token metadata is invalid")
		}
		if refreshToken == "" {
			refreshToken, _ = token["refresh_token"].(string)
		}
		if _, ok := vars["drive_id"]; !ok {
			if driveID, ok := token["default_drive_id"].(string); ok && driveID != "" {
				vars["drive_id"] = driveID
			}
		}
	}
	return strings.TrimSpace(refreshToken), nil
}

func saveAliyunCredentialTx(tx *gorm.DB, accountID uint, refreshToken string, preserve bool) error {
	var credential model.BackupOAuthCredential
	if preserve {
		_ = tx.Where("backup_account_id = ?", accountID).First(&credential).Error
	}
	if refreshToken != "" {
		encrypted, err := encrypt.StringEncryptGCM(refreshToken, model.BackupOAuthRefreshTokenEncryptionDomain)
		if err != nil {
			return err
		}
		credential.RefreshToken = encrypted
	}
	if credential.RefreshToken == "" {
		return errOAuthNotConfigured
	}
	credential.BackupAccountID = accountID
	credential.Provider = model.BackupOAuthProviderAliyun
	credential.Status = model.BackupOAuthStatusConfigured
	now := time.Now()
	credential.AuthorizedAt = &now
	return upsertBackupOAuthCredentialTx(tx, &credential)
}

func persistBackupOAuthRefreshResult(
	account model.BackupAccount,
	credential model.BackupOAuthCredential,
	refreshToken, status, vars string,
) error {
	now := time.Now()
	return global.DB.Transaction(func(tx *gorm.DB) error {
		credentialUpdate := tx.Model(&model.BackupOAuthCredential{}).
			Where("id = ? AND backup_account_id = ? AND updated_at = ?", credential.ID, account.ID, credential.UpdatedAt).
			Updates(map[string]interface{}{
				"refresh_token": refreshToken,
				"status":        status,
				"updated_at":    now,
			})
		if credentialUpdate.Error != nil {
			return credentialUpdate.Error
		}
		if credentialUpdate.RowsAffected != 1 {
			return errOAuthCredentialChanged
		}

		accountUpdate := tx.Model(&model.BackupAccount{}).
			Where("id = ? AND updated_at = ?", account.ID, account.UpdatedAt).
			Updates(map[string]interface{}{
				"vars":       vars,
				"updated_at": now,
			})
		if accountUpdate.Error != nil {
			return accountUpdate.Error
		}
		if accountUpdate.RowsAffected != 1 {
			return errOAuthCredentialChanged
		}
		return nil
	})
}
