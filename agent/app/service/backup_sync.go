package service

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	backupcoord "github.com/1Panel-dev/1Panel/agent/utils/backupsync"
	"github.com/1Panel-dev/1Panel/agent/utils/encrypt"
	"gorm.io/gorm"
)

const (
	maxPublicBackupSyncAccounts   = 1000
	publicBackupSyncLeaseDuration = 5 * time.Minute
)

type publicBackupSyncLease struct {
	Authority   string
	Generation  string
	TargetEpoch string
	Revision    uint64
	Digest      string
}

func (u *BackupService) SyncPublicAccounts(req dto.BackupPublicSync) (dto.BackupPublicSyncResult, error) {
	var result dto.BackupPublicSyncResult
	normalized, err := normalizePublicBackupSync(req)
	if err != nil {
		return result, err
	}
	req = normalized
	releaseMutation := backupcoord.AcquireMutation()
	defer releaseMutation()

	alreadyApplied := false
	err = global.DB.Transaction(func(tx *gorm.DB) error {
		var state model.BackupPublicSyncState
		stateErr := tx.Where("id = ?", model.BackupPublicSyncStateID).First(&state).Error
		if errors.Is(stateErr, gorm.ErrRecordNotFound) {
			state = model.BackupPublicSyncState{ID: model.BackupPublicSyncStateID}
			if err := tx.Create(&state).Error; err != nil {
				return err
			}
		} else if stateErr != nil {
			return stateErr
		}
		result.Authority = state.Authority
		result.Generation = state.Generation
		result.TargetEpoch = state.TargetEpoch
		result.AppliedRevision = state.AppliedRevision
		result.SnapshotDigest = state.AppliedDigest
		if state.Authority != "" && state.Authority != req.Authority {
			return errors.New("public backup synchronization authority changed; re-enroll this node before retrying")
		}
		if state.Authority != "" && state.Generation != req.Generation {
			return errors.New("public backup synchronization generation changed; explicit recovery is required")
		}
		if state.TargetEpoch != "" && state.TargetEpoch != req.TargetEpoch {
			return errors.New("public backup synchronization target epoch changed; re-enroll this node before retrying")
		}
		if state.Authority == "" && state.AppliedRevision > req.Revision {
			return errors.New("public backup synchronization state requires a newer authoritative snapshot")
		}
		if req.Revision < state.AppliedRevision {
			result.Result = "stale_ignored"
			return nil
		}
		if state.Authority != "" && req.Revision == state.AppliedRevision && state.AppliedDigest != "" {
			if !equalPublicBackupSyncDigest(req.SnapshotDigest, state.AppliedDigest) {
				return errors.New("public backup snapshot conflicts with the already applied revision")
			}
			alreadyApplied = true
		}

		tombstones := make(map[string]dto.BackupPublicSyncTombstone, len(req.Tombstones))
		for _, tombstone := range req.Tombstones {
			name := strings.TrimSpace(tombstone.Name)
			if name == "" || tombstone.Revision == 0 || (req.Revision != 0 && tombstone.Revision > req.Revision) {
				return errors.New("invalid public backup account tombstone")
			}
			if _, exists := tombstones[name]; exists {
				return fmt.Errorf("duplicate public backup account tombstone %q", name)
			}
			tombstone.Name = name
			tombstones[name] = tombstone
		}

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
			if _, deleted := tombstones[name]; deleted {
				return fmt.Errorf("public backup account %q is both present and deleted", name)
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

		for name := range tombstones {
			stale, exists := existingByName[name]
			if !exists {
				continue
			}
			if err := tx.Where("backup_account_id = ?", stale.ID).Delete(&model.BackupOAuthCredential{}).Error; err != nil {
				return err
			}
			if err := tx.Where("id = ?", stale.ID).Delete(&model.BackupAccount{}).Error; err != nil {
				return err
			}
			delete(existingByName, name)
		}

		for _, stale := range existingByName {
			if err := tx.Where("backup_account_id = ?", stale.ID).Delete(&model.BackupOAuthCredential{}).Error; err != nil {
				return err
			}
			if err := tx.Where("id = ?", stale.ID).Delete(&model.BackupAccount{}).Error; err != nil {
				return err
			}
		}
		now := time.Now()
		if err := tx.Model(&model.BackupPublicSyncState{}).
			Where("id = ?", model.BackupPublicSyncStateID).
			Updates(map[string]interface{}{
				"authority":        req.Authority,
				"generation":       req.Generation,
				"target_epoch":     req.TargetEpoch,
				"applied_revision": req.Revision,
				"applied_digest":   req.SnapshotDigest,
				"applied_at":       now,
			}).Error; err != nil {
			return err
		}
		result.Authority = req.Authority
		result.Generation = req.Generation
		result.TargetEpoch = req.TargetEpoch
		result.AppliedRevision = req.Revision
		result.SnapshotDigest = req.SnapshotDigest
		if alreadyApplied {
			result.Result = "already_applied"
		} else {
			result.Result = "applied"
		}
		return nil
	})
	return result, err
}

func ensurePublicBackupSyncLease(account *model.BackupAccount, now time.Time) error {
	if account == nil || !account.IsPublic {
		return nil
	}
	_, err := currentPublicBackupSyncLease(now)
	return err
}

func currentPublicBackupSyncLease(now time.Time) (publicBackupSyncLease, error) {
	var state model.BackupPublicSyncState
	if err := global.DB.Where("id = ?", model.BackupPublicSyncStateID).First(&state).Error; err != nil {
		return publicBackupSyncLease{}, errors.New("public backup account synchronization lease is unavailable; wait for reconciliation")
	}
	if !validPublicBackupSyncHex(state.Authority) || !validPublicBackupSyncHex(state.Generation) || !validPublicBackupSyncHex(state.TargetEpoch) ||
		state.AppliedRevision == 0 || !validPublicBackupSyncHex(state.AppliedDigest) || state.AppliedAt == nil ||
		state.AppliedAt.After(now.Add(time.Minute)) || now.Sub(*state.AppliedAt) > publicBackupSyncLeaseDuration {
		return publicBackupSyncLease{}, errors.New("public backup account synchronization lease expired; wait for reconciliation")
	}
	return publicBackupSyncLease{
		Authority:   state.Authority,
		Generation:  state.Generation,
		TargetEpoch: state.TargetEpoch,
		Revision:    state.AppliedRevision,
		Digest:      state.AppliedDigest,
	}, nil
}

func validatePublicBackupSyncLease(expected publicBackupSyncLease, now time.Time) error {
	current, err := currentPublicBackupSyncLease(now)
	if err != nil {
		return err
	}
	if current != expected {
		return errors.New("public backup account synchronization changed; retry after reconciliation")
	}
	return nil
}

func normalizePublicBackupSync(req dto.BackupPublicSync) (dto.BackupPublicSync, error) {
	if len(req.Accounts) > maxPublicBackupSyncAccounts || len(req.Tombstones) > maxPublicBackupSyncAccounts {
		return dto.BackupPublicSync{}, errors.New("public backup account snapshot exceeds limit")
	}
	req.Authority = strings.ToLower(strings.TrimSpace(req.Authority))
	req.Generation = strings.ToLower(strings.TrimSpace(req.Generation))
	req.TargetEpoch = strings.ToLower(strings.TrimSpace(req.TargetEpoch))
	req.SnapshotDigest = strings.ToLower(strings.TrimSpace(req.SnapshotDigest))
	if !validPublicBackupSyncHex(req.Authority) || !validPublicBackupSyncHex(req.Generation) || !validPublicBackupSyncHex(req.TargetEpoch) || req.Revision == 0 {
		return dto.BackupPublicSync{}, errors.New("invalid public backup synchronization identity")
	}
	if req.Accounts == nil {
		req.Accounts = []dto.BackupPublicSyncAccount{}
	}
	if req.Tombstones == nil {
		req.Tombstones = []dto.BackupPublicSyncTombstone{}
	}

	tombstones := make(map[string]struct{}, len(req.Tombstones))
	for index := range req.Tombstones {
		name := strings.TrimSpace(req.Tombstones[index].Name)
		if name == "" || req.Tombstones[index].Revision == 0 || req.Tombstones[index].Revision > req.Revision {
			return dto.BackupPublicSync{}, errors.New("invalid public backup account tombstone")
		}
		if _, exists := tombstones[name]; exists {
			return dto.BackupPublicSync{}, fmt.Errorf("duplicate public backup account tombstone %q", name)
		}
		tombstones[name] = struct{}{}
		req.Tombstones[index].Name = name
	}

	accounts := make(map[string]struct{}, len(req.Accounts))
	for index := range req.Accounts {
		name := strings.TrimSpace(req.Accounts[index].Name)
		if name == "" || !req.Accounts[index].IsPublic {
			return dto.BackupPublicSync{}, errors.New("invalid public backup account snapshot")
		}
		if _, deleted := tombstones[name]; deleted {
			return dto.BackupPublicSync{}, fmt.Errorf("public backup account %q is both present and deleted", name)
		}
		if _, exists := accounts[name]; exists {
			return dto.BackupPublicSync{}, fmt.Errorf("duplicate public backup account %q", name)
		}
		accounts[name] = struct{}{}
		req.Accounts[index].Name = name
		if req.Accounts[index].OAuth != nil {
			oauth := *req.Accounts[index].OAuth
			if oauth.AuthorizedAt != nil {
				authorizedAt := oauth.AuthorizedAt.UTC()
				oauth.AuthorizedAt = &authorizedAt
			}
			req.Accounts[index].OAuth = &oauth
		}
	}

	sort.Slice(req.Accounts, func(i, j int) bool { return req.Accounts[i].Name < req.Accounts[j].Name })
	sort.Slice(req.Tombstones, func(i, j int) bool {
		if req.Tombstones[i].Name == req.Tombstones[j].Name {
			return req.Tombstones[i].Revision < req.Tombstones[j].Revision
		}
		return req.Tombstones[i].Name < req.Tombstones[j].Name
	})
	expected, err := publicBackupSyncDigest(req)
	if err != nil {
		return dto.BackupPublicSync{}, errors.New("calculate public backup snapshot digest failed")
	}
	if !validPublicBackupSyncHex(req.SnapshotDigest) || !equalPublicBackupSyncDigest(req.SnapshotDigest, expected) {
		return dto.BackupPublicSync{}, errors.New("public backup snapshot digest verification failed")
	}
	req.SnapshotDigest = expected
	return req, nil
}

func publicBackupSyncDigest(req dto.BackupPublicSync) (string, error) {
	// TargetEpoch is checked against durable enrollment state and echoed in the
	// exact ACK. It is intentionally outside the shared snapshot-content digest.
	canonical := struct {
		Authority  string                          `json:"authority"`
		Generation string                          `json:"generation"`
		Revision   uint64                          `json:"revision"`
		Accounts   []dto.BackupPublicSyncAccount   `json:"accounts"`
		Tombstones []dto.BackupPublicSyncTombstone `json:"tombstones"`
	}{
		Authority:  req.Authority,
		Generation: req.Generation,
		Revision:   req.Revision,
		Accounts:   req.Accounts,
		Tombstones: req.Tombstones,
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func validPublicBackupSyncHex(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func equalPublicBackupSyncDigest(left, right string) bool {
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
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
