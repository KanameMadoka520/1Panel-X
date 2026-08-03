package migrations

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/utils/encrypt"
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

type legacyOAuthFingerprint struct {
	clientID     string
	clientSecret string
}

// Only irreversible SHA-256 fingerprints are retained. A known leaked secret
// is sufficient to require reconfiguration; a client ID alone is never used
// to classify an administrator-owned credential as legacy.
var legacyOAuthFingerprints = map[string][]legacyOAuthFingerprint{
	model.BackupOAuthProviderMicrosoft: {
		{clientID: "4b3c3a8afcec84f87db1e3724644a4fec642c84c51bad64aaedcb830e31aee2f", clientSecret: "48a587c68922d335bad096f0b3709c99fec7276f6c5761ae1b8823d71ae9d9bf"},
		{clientID: "2670cd5bcbd01460b3d00d9e0536badbd440d39ce09c453a011aef0778df73a0", clientSecret: "6c1a08b8cc018d2b7a64ea4c47b27774ae3a667fdc45fa5d6e6e26ce538b1445"},
		{clientSecret: "28b84a3bc2cc2ca6c3b0d9fa1e4645fd24496f5a07004b6ddd9b25e46776b004"},
	},
	model.BackupOAuthProviderGoogle: {
		{clientID: "d7a0bf99c6e815954f7d7277c2726b34d10fa605ebeaa989ca53a4e033752bd8", clientSecret: "7e2c9543c4685ffc10d0a1a9865a226f98fca0bc26fc71335a18d2d0227df286"},
		{clientID: "ef9c2fa0fff77985f63fc4410ff8fe2f39bdbf397a7e6ef408d9dfb7761e51c1", clientSecret: "9d85d30b0500877f862ca98d3cecf087415427a8f003b25f7829e8198144a1cf"},
	},
}

var MigrateBackupOAuthCredentials = &gormigrate.Migration{
	ID: "20260803-migrate-backup-oauth-credentials",
	Migrate: func(tx *gorm.DB) error {
		if err := tx.AutoMigrate(&model.BackupOAuthCredential{}); err != nil {
			return err
		}
		return migrateBackupOAuthAccounts(tx)
	},
}

func migrateBackupOAuthAccounts(tx *gorm.DB) error {
	encryptionKey, err := loadBackupOAuthEncryptionKey(tx)
	if err != nil {
		return err
	}
	var accounts []model.BackupAccount
	if err := tx.Where("type IN ?", []string{constant.OneDrive, constant.GoogleDrive, constant.ALIYUN}).Order("id ASC").Find(&accounts).Error; err != nil {
		return err
	}
	for _, account := range accounts {
		provider, ok := backupOAuthProvider(account.Type)
		if !ok {
			continue
		}
		vars, err := decodeBackupOAuthVars(account)
		if err != nil {
			return err
		}
		clientID := oauthFirstString(vars, "client_id", "clientId")
		clientSecret := oauthFirstString(vars, "client_secret", "clientSecret")
		redirectURI := oauthFirstString(vars, "redirect_uri", "redirectUri")
		refreshToken := oauthFirstString(vars, "refresh_token", "refreshToken")
		if provider == model.BackupOAuthProviderAliyun {
			refreshToken, err = extractAliyunRefreshToken(vars, refreshToken)
			if err != nil {
				return fmt.Errorf("parse OAuth token metadata for backup account %d: %w", account.ID, err)
			}
		}

		var existing model.BackupOAuthCredential
		existingErr := tx.Where("backup_account_id = ?", account.ID).First(&existing).Error
		if existingErr != nil && !errors.Is(existingErr, gorm.ErrRecordNotFound) {
			return existingErr
		}
		if errors.Is(existingErr, gorm.ErrRecordNotFound) {
			status := backupOAuthStatus(provider, clientID, clientSecret, refreshToken)
			if status == model.BackupOAuthStatusLegacyReconfigurationRequired {
				clientSecret = ""
			}
			clientSecretCipher, err := encrypt.StringEncryptGCMWithKey(clientSecret, encryptionKey, model.BackupOAuthClientSecretEncryptionDomain)
			if err != nil {
				return fmt.Errorf("encrypt client secret for backup account %d: %w", account.ID, err)
			}
			refreshTokenCipher, err := encrypt.StringEncryptGCMWithKey(refreshToken, encryptionKey, model.BackupOAuthRefreshTokenEncryptionDomain)
			if err != nil {
				return fmt.Errorf("encrypt refresh token for backup account %d: %w", account.ID, err)
			}
			credential := model.BackupOAuthCredential{
				BackupAccountID: account.ID,
				Provider:        provider,
				ClientID:        clientID,
				ClientSecret:    clientSecretCipher,
				RedirectURI:     redirectURI,
				RefreshToken:    refreshTokenCipher,
				IsCN:            oauthBool(vars, "isCN"),
				Status:          status,
				AuthorizedAt:    backupOAuthAuthorizedAt(account, vars, refreshToken),
			}
			if err := tx.Create(&credential).Error; err != nil {
				return err
			}
		}
		if err := removeSensitiveOAuthVars(tx, account, vars); err != nil {
			return err
		}
	}
	return nil
}

func loadBackupOAuthEncryptionKey(tx *gorm.DB) (string, error) {
	var setting model.Setting
	if err := tx.Where("key = ?", "EncryptKey").First(&setting).Error; err != nil {
		return "", err
	}
	if setting.Value == "" {
		return "", errors.New("backup OAuth encryption key is empty")
	}
	return setting.Value, nil
}

func backupOAuthProvider(backupType string) (string, bool) {
	switch backupType {
	case constant.OneDrive:
		return model.BackupOAuthProviderMicrosoft, true
	case constant.GoogleDrive:
		return model.BackupOAuthProviderGoogle, true
	case constant.ALIYUN:
		return model.BackupOAuthProviderAliyun, true
	default:
		return "", false
	}
}

func decodeBackupOAuthVars(account model.BackupAccount) (map[string]interface{}, error) {
	vars := make(map[string]interface{})
	if strings.TrimSpace(account.Vars) == "" {
		return vars, nil
	}
	if err := json.Unmarshal([]byte(account.Vars), &vars); err != nil {
		return nil, fmt.Errorf("parse OAuth metadata for backup account %d: %w", account.ID, err)
	}
	if vars == nil {
		vars = make(map[string]interface{})
	}
	return vars, nil
}

func oauthString(vars map[string]interface{}, key string) string {
	value, ok := vars[key]
	if !ok || value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprintf("%v", value)
}

func oauthFirstString(vars map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value := oauthString(vars, key); value != "" {
			return value
		}
	}
	return ""
}

func oauthBool(vars map[string]interface{}, key string) bool {
	value, ok := vars[key]
	if !ok || value == nil {
		return false
	}
	if flag, ok := value.(bool); ok {
		return flag
	}
	return strings.EqualFold(fmt.Sprintf("%v", value), "true")
}

func extractAliyunRefreshToken(vars map[string]interface{}, current string) (string, error) {
	tokenJSON := oauthString(vars, "token")
	if tokenJSON == "" {
		return current, nil
	}
	var token map[string]interface{}
	if err := json.Unmarshal([]byte(tokenJSON), &token); err != nil {
		return "", err
	}
	if current == "" {
		current = oauthFirstString(token, "refresh_token", "refreshToken")
	}
	if oauthFirstString(vars, "drive_id", "driveId") == "" {
		if driveID := oauthFirstString(token, "default_drive_id", "defaultDriveId"); driveID != "" {
			vars["drive_id"] = driveID
		}
	}
	return current, nil
}

func backupOAuthStatus(provider, clientID, clientSecret, refreshToken string) string {
	if isKnownLegacyOAuthPair(provider, clientID, clientSecret) {
		return model.BackupOAuthStatusLegacyReconfigurationRequired
	}
	if clientID == "" && clientSecret == "" && refreshToken == "" {
		return model.BackupOAuthStatusUnconfigured
	}
	if provider == model.BackupOAuthProviderAliyun {
		if refreshToken != "" {
			return model.BackupOAuthStatusConfigured
		}
		return model.BackupOAuthStatusUnconfigured
	}
	if clientID != "" && clientSecret != "" && refreshToken != "" {
		return model.BackupOAuthStatusConfigured
	}
	return model.BackupOAuthStatusReauthorizationRequired
}

func backupOAuthAuthorizedAt(account model.BackupAccount, vars map[string]interface{}, refreshToken string) *time.Time {
	if refreshToken == "" {
		return nil
	}
	if value := oauthString(vars, "refresh_time"); value != "" {
		for _, layout := range []string{constant.DateTimeLayout, time.RFC3339} {
			if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
				return &parsed
			}
		}
	}
	if !account.UpdatedAt.IsZero() {
		value := account.UpdatedAt
		return &value
	}
	if !account.CreatedAt.IsZero() {
		value := account.CreatedAt
		return &value
	}
	return nil
}

func removeSensitiveOAuthVars(tx *gorm.DB, account model.BackupAccount, vars map[string]interface{}) error {
	changed := strings.EqualFold(strings.TrimSpace(account.Vars), "null")
	if vars == nil {
		vars = make(map[string]interface{})
	}
	for key := range vars {
		if isSensitiveMigrationOAuthVarKey(key) {
			delete(vars, key)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	encoded, err := json.Marshal(vars)
	if err != nil {
		return fmt.Errorf("encode sanitized OAuth metadata for backup account %d: %w", account.ID, err)
	}
	return tx.Model(&model.BackupAccount{}).Where("id = ?", account.ID).Update("vars", string(encoded)).Error
}

func isSensitiveMigrationOAuthVarKey(key string) bool {
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
		"oauthsession",
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

func isKnownLegacyOAuthPair(provider, _ string, clientSecret string) bool {
	if clientSecret == "" {
		return false
	}
	for _, fingerprint := range legacyOAuthFingerprints[provider] {
		if valueMatchesFingerprint(clientSecret, fingerprint.clientSecret) {
			return true
		}
	}
	return false
}

func valueMatchesFingerprint(value, expected string) bool {
	if fingerprintsEqual(sha256Hex(value), expected) {
		return true
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return false
	}
	return fingerprintsEqual(sha256Hex(string(decoded)), expected)
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", sum[:])
}

func fingerprintsEqual(left, right string) bool {
	if len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}
