package migrations

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/utils/encrypt"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const agentOAuthMigrationTestKey = "agent-unit-test-install-key"

func TestMigrateBackupOAuthCredentialsOnNewInstall(t *testing.T) {
	db := openAgentOAuthMigrationDB(t)
	if err := MigrateBackupOAuthCredentials.Migrate(db); err != nil {
		t.Fatalf("run OAuth migration on new install: %v", err)
	}
	var count int64
	if err := db.Model(&model.BackupOAuthCredential{}).Count(&count).Error; err != nil {
		t.Fatalf("count OAuth credentials: %v", err)
	}
	if count != 0 {
		t.Fatalf("new install created %d OAuth credential rows, want 0", count)
	}
}

func TestMigrateBackupOAuthCredentials(t *testing.T) {
	db := openAgentOAuthMigrationDB(t)
	const (
		legacyMicrosoftID     = "synthetic-legacy-microsoft-client"
		legacyMicrosoftSecret = "synthetic-legacy-microsoft-secret"
		legacyGoogleID        = "synthetic-legacy-google-client"
		legacyGoogleSecret    = "synthetic-legacy-google-secret"
	)
	restoreLegacyFingerprints := replaceAgentLegacyOAuthFingerprints(legacyMicrosoftID, legacyMicrosoftSecret, legacyGoogleID, legacyGoogleSecret)
	t.Cleanup(restoreLegacyFingerprints)

	aliyunToken := mustAgentJSON(t, map[string]interface{}{
		"refresh_token":    "aliyun-refresh-token",
		"default_drive_id": "aliyun-default-drive",
	})
	accounts := []model.BackupAccount{
		{
			Name: "legacy-microsoft",
			Type: constant.OneDrive,
			Vars: mustAgentJSON(t, map[string]interface{}{
				"client_id":     legacyMicrosoftID,
				"client_secret": legacyMicrosoftSecret,
				"redirect_uri":  "https://panel.example/oauth/microsoft",
				"refresh_token": "legacy-microsoft-refresh",
				"code":          "one-time-code",
				"directory":     "legacy-safe-directory",
			}),
		},
		{
			Name: "legacy-google",
			Type: constant.GoogleDrive,
			Vars: mustAgentJSON(t, map[string]interface{}{
				"client_id":     legacyGoogleID,
				"client_secret": legacyGoogleSecret,
				"redirect_uri":  "https://panel.example/oauth/google",
				"refresh_token": "legacy-google-refresh",
				"folder":        "legacy-google-folder",
			}),
		},
		{
			Name: "custom-microsoft",
			Type: constant.OneDrive,
			Vars: mustAgentJSON(t, map[string]interface{}{
				"client_id":     "administrator-microsoft-client",
				"client_secret": "administrator-microsoft-secret",
				"redirect_uri":  "https://panel.example/oauth/custom-microsoft",
				"refresh_token": "administrator-microsoft-refresh",
				"isCN":          true,
				"directory":     "custom-safe-directory",
			}),
		},
		{
			Name: "custom-google-needs-authorization",
			Type: constant.GoogleDrive,
			Vars: mustAgentJSON(t, map[string]interface{}{
				"accessToken":           "short-lived-access-token",
				"authorizationResponse": "transient-callback",
				"authorization-url":     "transient-authorization-url",
				"clientId":              "administrator-google-client",
				"clientSecret":          "administrator-google-secret",
				"codeChallenge":         "transient-code-challenge",
				"codeVerifier":          "transient-pkce-verifier",
				"flow-id":               "transient-flow-id",
				"pkce-verifier":         "transient-pkce-verifier-variant",
				"redirectUri":           "https://panel.example/oauth/custom-google",
				"state":                 "transient-oauth-state",
				"folder":                "custom-google-folder",
			}),
		},
		{
			Name: "aliyun",
			Type: constant.ALIYUN,
			Vars: mustAgentJSON(t, map[string]interface{}{
				"client_id":     "aliyun-client",
				"client_secret": "aliyun-secret",
				"redirect_uri":  "https://panel.example/oauth/aliyun",
				"token":         aliyunToken,
				"region":        "cn-test",
			}),
		},
	}
	if err := db.Create(&accounts).Error; err != nil {
		t.Fatalf("seed backup accounts: %v", err)
	}

	if err := MigrateBackupOAuthCredentials.Migrate(db); err != nil {
		t.Fatalf("migrate OAuth credentials: %v", err)
	}

	credentials := loadAgentOAuthCredentials(t, db)
	if len(credentials) != len(accounts) {
		t.Fatalf("credential count = %d, want %d", len(credentials), len(accounts))
	}
	byAccountID := make(map[uint]model.BackupOAuthCredential, len(credentials))
	for _, credential := range credentials {
		if _, exists := byAccountID[credential.BackupAccountID]; exists {
			t.Fatalf("duplicate credential for backup account %d", credential.BackupAccountID)
		}
		byAccountID[credential.BackupAccountID] = credential
	}

	assertAgentOAuthStatus(t, byAccountID[accounts[0].ID], model.BackupOAuthStatusLegacyReconfigurationRequired)
	assertAgentOAuthStatus(t, byAccountID[accounts[1].ID], model.BackupOAuthStatusLegacyReconfigurationRequired)
	assertAgentOAuthStatus(t, byAccountID[accounts[2].ID], model.BackupOAuthStatusConfigured)
	assertAgentOAuthStatus(t, byAccountID[accounts[3].ID], model.BackupOAuthStatusReauthorizationRequired)
	assertAgentOAuthStatus(t, byAccountID[accounts[4].ID], model.BackupOAuthStatusConfigured)
	if byAccountID[accounts[0].ID].ClientSecret != "" || byAccountID[accounts[1].ID].ClientSecret != "" {
		t.Fatal("retired shared OAuth client secret was copied into the credential table")
	}
	assertAgentEncryptedValue(t, byAccountID[accounts[0].ID].RefreshToken, "legacy-microsoft-refresh", model.BackupOAuthRefreshTokenEncryptionDomain)
	assertAgentEncryptedValue(t, byAccountID[accounts[1].ID].RefreshToken, "legacy-google-refresh", model.BackupOAuthRefreshTokenEncryptionDomain)

	customMicrosoft := byAccountID[accounts[2].ID]
	assertAgentEncryptedValue(t, customMicrosoft.ClientSecret, "administrator-microsoft-secret", model.BackupOAuthClientSecretEncryptionDomain)
	assertAgentEncryptedValue(t, customMicrosoft.RefreshToken, "administrator-microsoft-refresh", model.BackupOAuthRefreshTokenEncryptionDomain)
	if customMicrosoft.ClientID != "administrator-microsoft-client" || customMicrosoft.RedirectURI != "https://panel.example/oauth/custom-microsoft" || !customMicrosoft.IsCN {
		t.Fatalf("custom Microsoft metadata was not preserved: %#v", customMicrosoft)
	}
	if customMicrosoft.AuthorizedAt == nil {
		t.Fatal("authorized custom Microsoft credential has no authorization timestamp")
	}

	customGoogle := byAccountID[accounts[3].ID]
	assertAgentEncryptedValue(t, customGoogle.ClientSecret, "administrator-google-secret", model.BackupOAuthClientSecretEncryptionDomain)
	if customGoogle.ClientID != "administrator-google-client" || customGoogle.RedirectURI != "https://panel.example/oauth/custom-google" {
		t.Fatalf("camelCase Google metadata was not migrated: %#v", customGoogle)
	}
	if customGoogle.RefreshToken != "" {
		t.Fatal("missing Google refresh token was replaced with a value")
	}
	if customGoogle.AuthorizedAt != nil {
		t.Fatal("unauthorized Google credential has an authorization timestamp")
	}

	aliyun := byAccountID[accounts[4].ID]
	assertAgentEncryptedValue(t, aliyun.RefreshToken, "aliyun-refresh-token", model.BackupOAuthRefreshTokenEncryptionDomain)

	for _, account := range accounts {
		vars := loadAgentBackupVars(t, db, account.ID)
		for _, key := range []string{
			"access_token", "accessToken", "client_id", "clientId", "client_secret", "clientSecret",
			"code", "authorization_code", "authorizationCode", "authorizationResponse", "authorization-url",
			"code_challenge", "codeChallenge", "code_verifier", "codeVerifier", "flow-id", "pkce_verifier", "pkce-verifier",
			"redirect_uri", "redirectUri", "refresh_token", "refreshToken", "state", "token",
		} {
			if _, exists := vars[key]; exists {
				t.Fatalf("backup account %d still contains sensitive Vars key %q", account.ID, key)
			}
		}
	}
	legacyVars := loadAgentBackupVars(t, db, accounts[0].ID)
	if legacyVars["directory"] != "legacy-safe-directory" {
		t.Fatalf("non-sensitive legacy Vars changed: %#v", legacyVars)
	}
	aliyunVars := loadAgentBackupVars(t, db, accounts[4].ID)
	if aliyunVars["drive_id"] != "aliyun-default-drive" || aliyunVars["region"] != "cn-test" {
		t.Fatalf("Aliyun non-sensitive Vars not preserved: %#v", aliyunVars)
	}

	encoded, err := json.Marshal(customMicrosoft)
	if err != nil {
		t.Fatalf("marshal credential: %v", err)
	}
	jsonText := strings.ToLower(string(encoded))
	for _, forbidden := range []string{"clientid", "clientsecret", "refreshtoken", "administrator-microsoft-client", "administrator-microsoft-secret", "administrator-microsoft-refresh"} {
		if strings.Contains(jsonText, strings.ToLower(forbidden)) {
			t.Fatalf("credential JSON contains forbidden value %q: %s", forbidden, encoded)
		}
	}

	firstCiphertexts := make(map[uint]string, len(credentials))
	for _, credential := range credentials {
		firstCiphertexts[credential.BackupAccountID] = credential.ClientSecret + "\x00" + credential.RefreshToken
	}
	if err := MigrateBackupOAuthCredentials.Migrate(db); err != nil {
		t.Fatalf("rerun OAuth migration: %v", err)
	}
	afterRerun := loadAgentOAuthCredentials(t, db)
	if len(afterRerun) != len(credentials) {
		t.Fatalf("credential count after rerun = %d, want %d", len(afterRerun), len(credentials))
	}
	for _, credential := range afterRerun {
		if got := credential.ClientSecret + "\x00" + credential.RefreshToken; got != firstCiphertexts[credential.BackupAccountID] {
			t.Fatalf("credential %d was re-encrypted or overwritten on rerun", credential.BackupAccountID)
		}
	}
}

func TestKnownLegacyOAuthPairUsesRetiredSecretFingerprint(t *testing.T) {
	const (
		legacyMicrosoftID     = "synthetic-pair-microsoft-client"
		legacyMicrosoftSecret = "synthetic-pair-microsoft-secret"
		legacyGoogleID        = "synthetic-pair-google-client"
		legacyGoogleSecret    = "synthetic-pair-google-secret"
	)
	restore := replaceAgentLegacyOAuthFingerprints(legacyMicrosoftID, legacyMicrosoftSecret, legacyGoogleID, legacyGoogleSecret)
	t.Cleanup(restore)

	if !isKnownLegacyOAuthPair(model.BackupOAuthProviderMicrosoft, legacyMicrosoftID, legacyMicrosoftSecret) {
		t.Fatal("synthetic Microsoft legacy pair was not recognized")
	}
	if !isKnownLegacyOAuthPair(model.BackupOAuthProviderGoogle, legacyGoogleID, legacyGoogleSecret) {
		t.Fatal("synthetic Google legacy pair was not recognized")
	}
	if isKnownLegacyOAuthPair(model.BackupOAuthProviderMicrosoft, legacyMicrosoftID, "administrator-secret") {
		t.Fatal("legacy client ID with an administrator secret was misclassified as legacy")
	}
	if !isKnownLegacyOAuthPair(model.BackupOAuthProviderGoogle, "administrator-client", legacyGoogleSecret) {
		t.Fatal("known retired secret was not classified as legacy after the client ID changed")
	}
	if !isKnownLegacyOAuthPair(
		model.BackupOAuthProviderMicrosoft,
		"administrator-client",
		base64.StdEncoding.EncodeToString([]byte(legacyMicrosoftSecret)),
	) {
		t.Fatal("base64-wrapped retired secret was not classified as legacy")
	}
}

func TestBackupOAuthStatusDistinguishesUnconfiguredFromReauthorization(t *testing.T) {
	if got := backupOAuthStatus(model.BackupOAuthProviderMicrosoft, "", "", ""); got != model.BackupOAuthStatusUnconfigured {
		t.Fatalf("empty Microsoft credential status = %q", got)
	}
	if got := backupOAuthStatus(model.BackupOAuthProviderAliyun, "", "", ""); got != model.BackupOAuthStatusUnconfigured {
		t.Fatalf("empty Aliyun credential status = %q", got)
	}
	if got := backupOAuthStatus(model.BackupOAuthProviderGoogle, "administrator-client", "administrator-secret", ""); got != model.BackupOAuthStatusReauthorizationRequired {
		t.Fatalf("configured Google app without token status = %q", got)
	}
}

func TestKnownLegacySecretDoesNotRequireMatchingClientID(t *testing.T) {
	old := legacyOAuthFingerprints
	legacyOAuthFingerprints = map[string][]legacyOAuthFingerprint{
		model.BackupOAuthProviderMicrosoft: {{clientSecret: sha256Hex("synthetic-retired-secret")}},
	}
	t.Cleanup(func() { legacyOAuthFingerprints = old })

	if !isKnownLegacyOAuthPair(model.BackupOAuthProviderMicrosoft, "different-client-id", "synthetic-retired-secret") {
		t.Fatal("known retired secret was not classified as legacy")
	}
	if isKnownLegacyOAuthPair(model.BackupOAuthProviderMicrosoft, "matching-client-is-insufficient", "administrator-secret") {
		t.Fatal("administrator secret was misclassified as legacy")
	}
}

func TestMigrateBackupOAuthCredentialsRedactsMalformedVars(t *testing.T) {
	db := openAgentOAuthMigrationDB(t)
	account := model.BackupAccount{
		Name: "malformed",
		Type: constant.OneDrive,
		Vars: `{"client_secret":"must-not-appear-in-error"`,
	}
	if err := db.Create(&account).Error; err != nil {
		t.Fatalf("seed malformed account: %v", err)
	}

	err := MigrateBackupOAuthCredentials.Migrate(db)
	if err == nil {
		t.Fatal("migration unexpectedly accepted malformed Vars")
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("backup account %d", account.ID)) {
		t.Fatalf("error lacks actionable account ID: %v", err)
	}
	if strings.Contains(err.Error(), "must-not-appear-in-error") {
		t.Fatalf("error exposed sensitive Vars content: %v", err)
	}
}

func openAgentOAuthMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsnName := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open("file:"+dsnName+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	if err := db.AutoMigrate(&model.Setting{}, &model.BackupAccount{}); err != nil {
		t.Fatalf("create migration fixture tables: %v", err)
	}
	if err := db.Create(&model.Setting{Key: "EncryptKey", Value: agentOAuthMigrationTestKey}).Error; err != nil {
		t.Fatalf("seed encryption key: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func replaceAgentLegacyOAuthFingerprints(microsoftID, microsoftSecret, googleID, googleSecret string) func() {
	old := legacyOAuthFingerprints
	legacyOAuthFingerprints = map[string][]legacyOAuthFingerprint{
		model.BackupOAuthProviderMicrosoft: {
			{clientID: sha256Hex(microsoftID), clientSecret: sha256Hex(microsoftSecret)},
		},
		model.BackupOAuthProviderGoogle: {
			{clientID: sha256Hex(googleID), clientSecret: sha256Hex(googleSecret)},
		},
	}
	return func() {
		legacyOAuthFingerprints = old
	}
}

func mustAgentJSON(t *testing.T, value interface{}) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode JSON fixture: %v", err)
	}
	return string(encoded)
}

func loadAgentOAuthCredentials(t *testing.T, db *gorm.DB) []model.BackupOAuthCredential {
	t.Helper()
	var credentials []model.BackupOAuthCredential
	if err := db.Order("backup_account_id ASC").Find(&credentials).Error; err != nil {
		t.Fatalf("load OAuth credentials: %v", err)
	}
	return credentials
}

func loadAgentBackupVars(t *testing.T, db *gorm.DB, accountID uint) map[string]interface{} {
	t.Helper()
	var account model.BackupAccount
	if err := db.First(&account, accountID).Error; err != nil {
		t.Fatalf("load backup account %d: %v", accountID, err)
	}
	var vars map[string]interface{}
	if err := json.Unmarshal([]byte(account.Vars), &vars); err != nil {
		t.Fatalf("decode backup account %d Vars: %v", accountID, err)
	}
	return vars
}

func assertAgentOAuthStatus(t *testing.T, credential model.BackupOAuthCredential, want string) {
	t.Helper()
	if credential.Status != want {
		t.Fatalf("backup account %d status = %q, want %q", credential.BackupAccountID, credential.Status, want)
	}
}

func assertAgentEncryptedValue(t *testing.T, ciphertext, plaintext, domain string) {
	t.Helper()
	if ciphertext == "" {
		t.Fatal("encrypted value is empty")
	}
	if strings.Contains(ciphertext, plaintext) {
		t.Fatal("ciphertext contains plaintext")
	}
	decrypted, err := encrypt.StringDecryptGCMWithKey(ciphertext, agentOAuthMigrationTestKey, domain)
	if err != nil {
		t.Fatalf("decrypt migrated value: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("decrypted migrated value = %q, want %q", decrypted, plaintext)
	}
}
