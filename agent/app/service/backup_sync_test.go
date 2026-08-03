package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/utils/encrypt"
	"github.com/1Panel-dev/1Panel/agent/utils/re"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const agentBackupSyncTestKey = "agent-backup-sync-test-key"

func TestSyncPublicAccountsEncryptsSecretsAndClearPreservesAccount(t *testing.T) {
	db := setupAgentBackupSyncTestDB(t)
	private := model.BackupAccount{Name: "private-account", Type: constant.S3, IsPublic: false, Vars: "{}"}
	if err := db.Create(&private).Error; err != nil {
		t.Fatalf("seed private account: %v", err)
	}

	req := dto.BackupPublicSync{Accounts: []dto.BackupPublicSyncAccount{{
		Name:         "shared-drive",
		Type:         constant.OneDrive,
		IsPublic:     true,
		AccessKey:    "synthetic-access-key",
		Credential:   "synthetic-account-credential",
		BackupPath:   "backups",
		Vars:         `{"directory":"safe","client_secret":"must-be-removed","refreshToken":"must-be-removed","codeVerifier":"must-be-removed"}`,
		RememberAuth: true,
		OAuth: &dto.BackupOAuthSecretSync{
			Provider:     model.BackupOAuthProviderMicrosoft,
			ClientID:     "administrator-client-id",
			ClientSecret: "administrator-client-secret",
			RedirectURI:  "http://localhost/login/authorized",
			RefreshToken: "administrator-refresh-token",
			Status:       model.BackupOAuthStatusConfigured,
		},
	}}}
	service := &BackupService{}
	if err := service.SyncPublicAccounts(req); err != nil {
		t.Fatalf("sync public account: %v", err)
	}

	var account model.BackupAccount
	if err := db.Where("name = ?", "shared-drive").First(&account).Error; err != nil {
		t.Fatalf("load synced account: %v", err)
	}
	if account.AccessKey == "synthetic-access-key" || account.Credential == "synthetic-account-credential" {
		t.Fatal("public account credentials were stored in plaintext")
	}
	if got, err := encrypt.StringDecrypt(account.AccessKey); err != nil || got != "synthetic-access-key" {
		t.Fatalf("decrypt synced access key: got %q, err=%v", got, err)
	}
	if got, err := encrypt.StringDecrypt(account.Credential); err != nil || got != "synthetic-account-credential" {
		t.Fatalf("decrypt synced credential: got %q, err=%v", got, err)
	}
	for _, forbidden := range []string{"client_secret", "refreshToken", "codeVerifier", "must-be-removed"} {
		if strings.Contains(account.Vars, forbidden) {
			t.Fatalf("sanitized Vars contains %q", forbidden)
		}
	}

	var credential model.BackupOAuthCredential
	if err := db.Where("backup_account_id = ?", account.ID).First(&credential).Error; err != nil {
		t.Fatalf("load synced OAuth credential: %v", err)
	}
	if credential.ClientSecret == "administrator-client-secret" || credential.RefreshToken == "administrator-refresh-token" {
		t.Fatal("OAuth material was stored in plaintext")
	}
	assertAgentBackupSyncEncrypted(t, credential.ClientSecret, "administrator-client-secret", model.BackupOAuthClientSecretEncryptionDomain)
	assertAgentBackupSyncEncrypted(t, credential.RefreshToken, "administrator-refresh-token", model.BackupOAuthRefreshTokenEncryptionDomain)

	info, err := service.GetOAuthCredential(account.ID)
	if err != nil {
		t.Fatalf("load safe OAuth info: %v", err)
	}
	encodedInfo, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal safe OAuth info: %v", err)
	}
	for _, forbidden := range []string{
		"administrator-client-secret", "administrator-refresh-token", credential.ClientSecret, credential.RefreshToken,
		"clientSecret", "refreshToken",
	} {
		if strings.Contains(string(encodedInfo), forbidden) {
			t.Fatalf("safe OAuth response contains forbidden material %q", forbidden)
		}
	}

	if err := db.Model(&model.BackupAccount{}).Where("id = ?", account.ID).Update(
		"vars",
		`{"directory":"safe","client_secret":"read-back-secret","refreshToken":"read-back-refresh","authorizationResponse":"read-back-code"}`,
	).Error; err != nil {
		t.Fatalf("seed legacy read-path Vars: %v", err)
	}
	_, result, err := service.SearchWithPage(dto.SearchPageWithType{PageInfo: dto.PageInfo{Page: 1, PageSize: 20}})
	if err != nil {
		t.Fatalf("search backup accounts: %v", err)
	}
	items, ok := result.([]dto.BackupInfo)
	if !ok || len(items) != 2 {
		t.Fatalf("unexpected backup search result: %#v", result)
	}
	encodedSearch, err := json.Marshal(items)
	if err != nil {
		t.Fatalf("marshal backup search response: %v", err)
	}
	for _, forbidden := range []string{
		"read-back-secret", "read-back-refresh", "read-back-code",
		"administrator-client-secret", "administrator-refresh-token",
		credential.ClientSecret, credential.RefreshToken,
		"client_secret", "refreshToken", "authorizationResponse",
	} {
		if strings.Contains(string(encodedSearch), forbidden) {
			t.Fatalf("backup search response contains forbidden material %q", forbidden)
		}
	}

	clearReq := req
	clearReq.Accounts[0].OAuth = nil
	if err := service.SyncPublicAccounts(clearReq); err != nil {
		t.Fatalf("clear public OAuth credential: %v", err)
	}
	if err := db.Where("name = ?", "shared-drive").First(&account).Error; err != nil {
		t.Fatalf("account was removed by credential clear: %v", err)
	}
	var credentialCount int64
	if err := db.Model(&model.BackupOAuthCredential{}).Where("backup_account_id = ?", account.ID).Count(&credentialCount).Error; err != nil {
		t.Fatalf("count cleared OAuth credentials: %v", err)
	}
	if credentialCount != 0 {
		t.Fatalf("credential clear left %d rows", credentialCount)
	}
	if err := injectPersistedOAuthCredential(&account, map[string]interface{}{}); err != errOAuthNotConfigured {
		t.Fatalf("cleared account execution error = %v, want %v", err, errOAuthNotConfigured)
	}

	if err := service.SyncPublicAccounts(dto.BackupPublicSync{}); err != nil {
		t.Fatalf("delete stale public account: %v", err)
	}
	var publicCount, privateCount int64
	_ = db.Model(&model.BackupAccount{}).Where("is_public = ?", true).Count(&publicCount).Error
	_ = db.Model(&model.BackupAccount{}).Where("id = ?", private.ID).Count(&privateCount).Error
	if publicCount != 0 || privateCount != 1 {
		t.Fatalf("post-delete counts public=%d private=%d", publicCount, privateCount)
	}
}

func TestSyncPublicAccountsLegacyStatusBlocksCredentialUse(t *testing.T) {
	setupAgentBackupSyncTestDB(t)
	service := &BackupService{}
	req := dto.BackupPublicSync{Accounts: []dto.BackupPublicSyncAccount{{
		Name:     "legacy-drive",
		Type:     constant.GoogleDrive,
		IsPublic: true,
		Vars:     "{}",
		OAuth: &dto.BackupOAuthSecretSync{
			Provider: model.BackupOAuthProviderGoogle,
			ClientID: "retired-client-id",
			Status:   model.BackupOAuthStatusLegacyReconfigurationRequired,
		},
	}}}
	if err := service.SyncPublicAccounts(req); err != nil {
		t.Fatalf("sync legacy account status: %v", err)
	}
	var account model.BackupAccount
	if err := global.DB.Where("name = ?", "legacy-drive").First(&account).Error; err != nil {
		t.Fatalf("load legacy account: %v", err)
	}
	if err := injectPersistedOAuthCredential(&account, map[string]interface{}{}); err != errOAuthReconfiguration {
		t.Fatalf("legacy credential execution error = %v, want %v", err, errOAuthReconfiguration)
	}
}

func TestPublicBackupMutationsRequireCore(t *testing.T) {
	db := setupAgentBackupSyncTestDB(t)
	account := model.BackupAccount{Name: "public-owned", Type: constant.OneDrive, IsPublic: true, Vars: `{}`}
	if err := db.Create(&account).Error; err != nil {
		t.Fatalf("seed public account: %v", err)
	}
	credential := model.BackupOAuthCredential{
		BackupAccountID: account.ID,
		Provider:        model.BackupOAuthProviderMicrosoft,
		ClientID:        "administrator-client-id",
		ClientSecret:    "encrypted-client-secret",
		RefreshToken:    "encrypted-refresh-token",
		Status:          model.BackupOAuthStatusConfigured,
	}
	if err := db.Create(&credential).Error; err != nil {
		t.Fatalf("seed public credential: %v", err)
	}

	service := &BackupService{}
	operations := map[string]func() error{
		"clear":   func() error { return service.ClearOAuthCredential(account.ID) },
		"delete":  func() error { return service.Delete(account.ID) },
		"refresh": func() error { return service.RefreshToken(dto.OperateByID{ID: account.ID}) },
		"update": func() error {
			return service.Update(dto.BackupOperate{ID: account.ID, Name: account.Name, Type: account.Type, IsPublic: true, Vars: `{}`})
		},
	}
	for name, operation := range operations {
		if err := operation(); !errors.Is(err, errPublicBackupManagedByCore) {
			t.Errorf("%s public account error = %v, want %v", name, err, errPublicBackupManagedByCore)
		}
	}

	var accountCount, credentialCount int64
	_ = db.Model(&model.BackupAccount{}).Where("id = ?", account.ID).Count(&accountCount).Error
	_ = db.Model(&model.BackupOAuthCredential{}).Where("id = ?", credential.ID).Count(&credentialCount).Error
	if accountCount != 1 || credentialCount != 1 {
		t.Fatalf("public mutation changed protected rows: accounts=%d credentials=%d", accountCount, credentialCount)
	}
}

func TestPersistBackupOAuthRefreshResultRejectsStaleAgentSnapshot(t *testing.T) {
	db := setupAgentBackupSyncTestDB(t)
	account := model.BackupAccount{Name: "agent-refresh-race", Type: constant.GoogleDrive, Vars: `{"marker":"original"}`}
	if err := db.Create(&account).Error; err != nil {
		t.Fatalf("seed account: %v", err)
	}
	credential := model.BackupOAuthCredential{
		BackupAccountID: account.ID,
		Provider:        model.BackupOAuthProviderGoogle,
		ClientID:        "administrator-client-id",
		ClientSecret:    "encrypted-old-client-secret",
		RefreshToken:    "encrypted-old-refresh-token",
		Status:          model.BackupOAuthStatusConfigured,
	}
	if err := db.Create(&credential).Error; err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	if err := db.Model(&model.BackupOAuthCredential{}).Where("id = ?", credential.ID).Updates(map[string]interface{}{
		"client_secret": "encrypted-new-client-secret",
		"refresh_token": "encrypted-new-refresh-token",
		"updated_at":    time.Now().Add(time.Second),
	}).Error; err != nil {
		t.Fatalf("replace credential: %v", err)
	}

	err := persistBackupOAuthRefreshResult(
		account,
		credential,
		"stale-refresh-result",
		model.BackupOAuthStatusReauthorizationRequired,
		`{"marker":"stale"}`,
	)
	if !errors.Is(err, errOAuthCredentialChanged) {
		t.Fatalf("persist stale refresh error = %v, want %v", err, errOAuthCredentialChanged)
	}
	var persisted model.BackupOAuthCredential
	if err := db.First(&persisted, credential.ID).Error; err != nil {
		t.Fatalf("reload credential: %v", err)
	}
	if persisted.ClientSecret != "encrypted-new-client-secret" || persisted.RefreshToken != "encrypted-new-refresh-token" {
		t.Fatal("stale agent refresh overwrote replacement credential")
	}
	if persisted.Status != model.BackupOAuthStatusConfigured {
		t.Fatalf("stale agent refresh changed replacement status to %s", persisted.Status)
	}
}

func TestSanitizeBackupOAuthVarsNormalizesSensitiveAgentKeysAndNull(t *testing.T) {
	vars, encoded, err := sanitizeBackupOAuthVars("null")
	if err != nil {
		t.Fatalf("sanitize null vars: %v", err)
	}
	if vars == nil || encoded != "{}" {
		t.Fatalf("null vars were not normalized: vars=%#v encoded=%q", vars, encoded)
	}

	raw := `{"safe":"kept","authorizationResponse":"sensitive","authorization-url":"sensitive","codeChallenge":"sensitive","pkce-verifier":"sensitive","flow-id":"sensitive","refreshToken":"sensitive"}`
	vars, encoded, err = sanitizeBackupOAuthVars(raw)
	if err != nil {
		t.Fatalf("sanitize variant vars: %v", err)
	}
	if len(vars) != 1 || vars["safe"] != "kept" || encoded != `{"safe":"kept"}` {
		t.Fatalf("sensitive OAuth variants were not removed: vars=%#v encoded=%s", vars, encoded)
	}
}

func setupAgentBackupSyncTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	re.Init()
	oldDB := global.DB
	oldKey := global.CONF.Base.EncryptKey
	dsn := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", dsn)), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.AutoMigrate(&model.BackupAccount{}, &model.BackupOAuthCredential{}); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	global.DB = db
	global.CONF.Base.EncryptKey = agentBackupSyncTestKey
	t.Cleanup(func() {
		global.DB = oldDB
		global.CONF.Base.EncryptKey = oldKey
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func assertAgentBackupSyncEncrypted(t *testing.T, ciphertext, plaintext, domain string) {
	t.Helper()
	if ciphertext == "" || strings.Contains(ciphertext, plaintext) {
		t.Fatal("invalid encrypted OAuth value")
	}
	decrypted, err := encrypt.StringDecryptGCMWithKey(ciphertext, agentBackupSyncTestKey, domain)
	if err != nil {
		t.Fatalf("decrypt synced OAuth value: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("decrypted OAuth value = %q, want %q", decrypted, plaintext)
	}
}
