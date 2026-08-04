package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/constant"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/utils/backupsync"
	"github.com/1Panel-dev/1Panel/core/utils/encrypt"
	"github.com/1Panel-dev/1Panel/core/utils/oauthflow"
	"github.com/1Panel-dev/1Panel/core/utils/xpack"
	"github.com/1Panel-dev/1Panel/core/utils/xpack/providers"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const coreBackupOAuthServiceTestKey = "core-backup-oauth-service-test-key"

type recordingBackupSyncProvider struct {
	providers.MultiNodeProvider
	calls []string
}

func (p *recordingBackupSyncProvider) Sync(dataType string) error {
	p.calls = append(p.calls, dataType)
	return nil
}

func TestBackupOAuthSecretWriteOnlyPreserveAndClear(t *testing.T) {
	db := setupCoreBackupOAuthServiceTestDB(t)
	account := model.BackupAccount{
		Name:     "shared-onedrive",
		Type:     constant.OneDrive,
		IsPublic: true,
		Vars:     `{"directory":"safe"}`,
	}
	if err := db.Create(&account).Error; err != nil {
		t.Fatalf("seed public backup account: %v", err)
	}

	stored := oauthflow.StoredResult{
		Provider:     oauthflow.ProviderOneDrive,
		ClientID:     "administrator-client-id",
		ClientSecret: "administrator-client-secret",
		RedirectURI:  "http://localhost/login/authorized",
		RefreshToken: "administrator-refresh-token",
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return saveBackupOAuthCredentialTx(tx, account.ID, stored)
	}); err != nil {
		t.Fatalf("save OAuth credential: %v", err)
	}

	var persisted model.BackupOAuthCredential
	if err := db.Where("backup_account_id = ?", account.ID).First(&persisted).Error; err != nil {
		t.Fatalf("load persisted OAuth credential: %v", err)
	}
	if persisted.ClientSecret == stored.ClientSecret || persisted.RefreshToken == stored.RefreshToken {
		t.Fatal("OAuth material was persisted in plaintext")
	}
	if got, err := encrypt.StringDecryptGCM(persisted.ClientSecret, model.BackupOAuthClientSecretEncryptionDomain); err != nil || got != stored.ClientSecret {
		t.Fatalf("decrypt persisted client secret: got %q, err=%v", got, err)
	}

	service := &BackupService{}
	info, err := service.GetOAuthCredential(account.Name)
	if err != nil {
		t.Fatalf("get OAuth credential status: %v", err)
	}
	encoded, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal OAuth credential status: %v", err)
	}
	for _, forbidden := range []string{
		stored.ClientSecret, stored.RefreshToken, persisted.ClientSecret, persisted.RefreshToken, "clientSecret", "refreshToken",
	} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("OAuth status response contains forbidden material %q", forbidden)
		}
	}

	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse token form: %v", err)
			}
			if r.Form.Get("client_id") != "replacement-client-id" || r.Form.Get("client_secret") != stored.ClientSecret {
				t.Errorf("token exchange did not preserve the stored client secret")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"synthetic-access-token","refresh_token":"replacement-refresh-token"}`))
		case "/drive":
			if r.Header.Get("Authorization") != "Bearer synthetic-access-token" {
				t.Errorf("unexpected drive authorization header")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"synthetic-drive"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(providerServer.Close)
	endpoints := oauthflow.EndpointSet{
		AuthorizationURL: providerServer.URL + "/authorize",
		TokenURL:         providerServer.URL + "/token",
		DriveURL:         providerServer.URL + "/drive",
	}
	backupOAuthFlowManager = oauthflow.NewManager(oauthflow.Options{
		HTTPClient: providerServer.Client(),
		Endpoints: oauthflow.Endpoints{
			OneDrive:      endpoints,
			OneDriveChina: endpoints,
			GoogleDrive:   endpoints,
		},
	})

	begin, err := service.BeginOAuth(dto.OAuthBegin{
		Provider:    constant.OneDrive,
		AccountID:   account.ID,
		AccountName: account.Name,
		ClientID:    "replacement-client-id",
	})
	if err != nil {
		t.Fatalf("begin OAuth while preserving empty secret: %v", err)
	}
	defer backupOAuthFlowManager.Delete(begin.FlowID)
	if strings.Contains(begin.AuthorizationURL, stored.ClientSecret) {
		t.Fatal("authorization URL contains the client secret")
	}
	authorizationURL, err := url.Parse(begin.AuthorizationURL)
	if err != nil {
		t.Fatalf("parse authorization URL: %v", err)
	}
	state := authorizationURL.Query().Get("state")
	if state == "" {
		t.Fatal("authorization URL does not contain state")
	}
	complete, err := service.CompleteOAuth(dto.OAuthComplete{
		FlowID: begin.FlowID,
		AuthorizationResponse: stored.RedirectURI + "?code=synthetic-code&state=" +
			url.QueryEscape(state),
	})
	if err != nil {
		t.Fatalf("complete OAuth with preserved secret: %v", err)
	}
	flow, err := backupOAuthFlowManager.Peek(complete.SessionID)
	if err != nil {
		t.Fatalf("peek completed OAuth flow: %v", err)
	}
	if flow.ClientSecret != stored.ClientSecret || flow.ClientID != "replacement-client-id" || flow.RefreshToken != "replacement-refresh-token" {
		t.Fatalf("completed OAuth flow did not preserve and replace the expected fields")
	}

	provider := &recordingBackupSyncProvider{}
	oldProvider := xpack.MultiNodeProvider
	xpack.MultiNodeProvider = provider
	t.Cleanup(func() { xpack.MultiNodeProvider = oldProvider })
	if err := service.ClearOAuthCredential(account.Name); err != nil {
		t.Fatalf("clear OAuth credential: %v", err)
	}
	if len(provider.calls) != 0 {
		t.Fatalf("clear blocked on synchronous delivery: calls=%#v", provider.calls)
	}
	reconcilePublicBackupSync()
	if len(provider.calls) != 1 || provider.calls[0] != constant.SyncBackupAccounts {
		t.Fatalf("reconciliation calls = %#v", provider.calls)
	}
	var accountCount, credentialCount int64
	_ = db.Model(&model.BackupAccount{}).Where("id = ?", account.ID).Count(&accountCount).Error
	_ = db.Model(&model.BackupOAuthCredential{}).Where("backup_account_id = ?", account.ID).Count(&credentialCount).Error
	if accountCount != 1 || credentialCount != 0 {
		t.Fatalf("clear counts account=%d credential=%d", accountCount, credentialCount)
	}
	var refreshed model.BackupAccount
	if err := db.First(&refreshed, account.ID).Error; err != nil {
		t.Fatalf("reload account after clear: %v", err)
	}
	if !strings.Contains(refreshed.Vars, model.BackupOAuthStatusUnconfigured) {
		t.Fatalf("clear did not persist unconfigured status: %s", refreshed.Vars)
	}
}

func TestPersistBackupOAuthRefreshResultRejectsStaleSnapshots(t *testing.T) {
	t.Run("cleared credential is not recreated", func(t *testing.T) {
		db := setupCoreBackupOAuthServiceTestDB(t)
		account := model.BackupAccount{Name: "clear-race", Type: constant.OneDrive, IsPublic: true, Vars: `{"marker":"original"}`}
		if err := db.Create(&account).Error; err != nil {
			t.Fatalf("seed account: %v", err)
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
			t.Fatalf("seed credential: %v", err)
		}
		if err := db.Delete(&model.BackupOAuthCredential{}, credential.ID).Error; err != nil {
			t.Fatalf("clear credential: %v", err)
		}

		err := persistBackupOAuthRefreshResult(
			account,
			credential,
			"stale-refresh-result",
			model.BackupOAuthStatusConfigured,
			`{"marker":"stale"}`,
		)
		if !errors.Is(err, errOAuthCredentialChanged) {
			t.Fatalf("persist stale refresh error = %v, want %v", err, errOAuthCredentialChanged)
		}
		var credentialCount int64
		if err := db.Model(&model.BackupOAuthCredential{}).Where("backup_account_id = ?", account.ID).Count(&credentialCount).Error; err != nil {
			t.Fatalf("count credentials: %v", err)
		}
		if credentialCount != 0 {
			t.Fatalf("stale refresh recreated %d credential rows", credentialCount)
		}
		var persisted model.BackupAccount
		if err := db.First(&persisted, account.ID).Error; err != nil {
			t.Fatalf("reload account: %v", err)
		}
		if persisted.Vars != account.Vars {
			t.Fatalf("stale refresh overwrote account vars: %s", persisted.Vars)
		}
	})

	t.Run("reauthorized credential is not overwritten", func(t *testing.T) {
		db := setupCoreBackupOAuthServiceTestDB(t)
		account := model.BackupAccount{Name: "reauthorize-race", Type: constant.GoogleDrive, IsPublic: true, Vars: `{"marker":"original"}`}
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
			t.Fatalf("stale refresh overwrote replacement credential")
		}
		if persisted.Status != model.BackupOAuthStatusConfigured {
			t.Fatalf("stale refresh changed replacement status to %s", persisted.Status)
		}
	})
}

func TestClearOAuthCredentialWaitsForDesiredStateExecution(t *testing.T) {
	db := setupCoreBackupOAuthServiceTestDB(t)
	account := model.BackupAccount{
		Name:     "guarded-onedrive",
		Type:     constant.OneDrive,
		IsPublic: true,
		Vars:     `{}`,
	}
	if err := db.Create(&account).Error; err != nil {
		t.Fatalf("seed guarded account: %v", err)
	}
	if err := db.Create(&model.BackupOAuthCredential{
		BackupAccountID: account.ID,
		Provider:        model.BackupOAuthProviderMicrosoft,
		ClientID:        "synthetic-client-id",
		ClientSecret:    "synthetic-encrypted-client-secret",
		RefreshToken:    "synthetic-encrypted-refresh-token",
		Status:          model.BackupOAuthStatusConfigured,
	}).Error; err != nil {
		t.Fatalf("seed guarded credential: %v", err)
	}

	releaseExecution := backupsync.AcquireDesiredStateExecution()
	result := make(chan error, 1)
	go func() {
		result <- (&BackupService{}).ClearOAuthCredential(account.Name)
	}()

	select {
	case err := <-result:
		releaseExecution()
		t.Fatalf("credential mutation crossed active execution guard: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	var credentialCount int64
	if err := db.Model(&model.BackupOAuthCredential{}).Where("backup_account_id = ?", account.ID).Count(&credentialCount).Error; err != nil {
		releaseExecution()
		t.Fatalf("count guarded credential: %v", err)
	}
	if credentialCount != 1 {
		releaseExecution()
		t.Fatalf("credential changed while execution guard was active: count=%d", credentialCount)
	}

	releaseExecution()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("clear credential after execution guard release: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("credential mutation did not resume after execution guard release")
	}
	if err := db.Model(&model.BackupOAuthCredential{}).Where("backup_account_id = ?", account.ID).Count(&credentialCount).Error; err != nil {
		t.Fatalf("count cleared credential: %v", err)
	}
	if credentialCount != 0 {
		t.Fatalf("credential remained after guarded clear: count=%d", credentialCount)
	}
	sequence, err := backupsync.CurrentSequence()
	if err != nil {
		t.Fatalf("load sequence after guarded clear: %v", err)
	}
	if sequence.Revision != 2 {
		t.Fatalf("guarded clear revision = %d, want 2", sequence.Revision)
	}
}

func TestSanitizeBackupOAuthVarsNormalizesSensitiveKeysAndNull(t *testing.T) {
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

func setupCoreBackupOAuthServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := global.DB
	oldKey := global.CONF.Base.EncryptKey
	oldManager := backupOAuthFlowManager
	dsn := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", dsn)), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.AutoMigrate(
		&model.BackupAccount{},
		&model.BackupOAuthCredential{},
		&model.Node{},
		&model.BackupSyncSequence{},
		&model.BackupSyncOutbox{},
		&model.BackupSyncTarget{},
		&model.BackupSyncTombstone{},
	); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	global.DB = db
	global.CONF.Base.EncryptKey = coreBackupOAuthServiceTestKey
	backupOAuthFlowManager = oauthflow.NewManager(oauthflow.Options{})
	if err := db.Transaction(backupsync.InitializeTx); err != nil {
		t.Fatalf("initialize backup synchronization state: %v", err)
	}
	t.Cleanup(func() {
		global.DB = oldDB
		global.CONF.Base.EncryptKey = oldKey
		backupOAuthFlowManager = oldManager
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}
