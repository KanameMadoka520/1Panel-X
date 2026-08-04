package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/app/task"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/i18n"
	"github.com/1Panel-dev/1Panel/agent/utils/encrypt"
	"github.com/1Panel-dev/1Panel/agent/utils/re"
	xpackhelper "github.com/1Panel-dev/1Panel/agent/utils/xpack/helper"
	"github.com/glebarez/sqlite"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	agentBackupSyncTestKey         = "agent-backup-sync-test-key"
	agentBackupSyncTestAuthority   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	agentBackupSyncTestGeneration  = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	agentBackupSyncTestTargetEpoch = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
)

func TestSyncPublicAccountsEncryptsSecretsAndClearPreservesAccount(t *testing.T) {
	db := setupAgentBackupSyncTestDB(t)
	private := model.BackupAccount{Name: "private-account", Type: constant.S3, IsPublic: false, Vars: "{}"}
	if err := db.Create(&private).Error; err != nil {
		t.Fatalf("seed private account: %v", err)
	}

	req := sealAgentBackupSyncRequest(t, dto.BackupPublicSync{Revision: 1, Accounts: []dto.BackupPublicSyncAccount{{
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
	}}})
	service := &BackupService{}
	if _, err := service.SyncPublicAccounts(req); err != nil {
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
	clearReq.Revision = 2
	clearReq.Accounts[0].OAuth = nil
	clearReq = sealAgentBackupSyncRequest(t, clearReq)
	if _, err := service.SyncPublicAccounts(clearReq); err != nil {
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

	if _, err := service.SyncPublicAccounts(sealAgentBackupSyncRequest(t, dto.BackupPublicSync{Revision: 3})); err != nil {
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
	req := sealAgentBackupSyncRequest(t, dto.BackupPublicSync{Revision: 1, Accounts: []dto.BackupPublicSyncAccount{{
		Name:     "legacy-drive",
		Type:     constant.GoogleDrive,
		IsPublic: true,
		Vars:     "{}",
		OAuth: &dto.BackupOAuthSecretSync{
			Provider: model.BackupOAuthProviderGoogle,
			ClientID: "retired-client-id",
			Status:   model.BackupOAuthStatusLegacyReconfigurationRequired,
		},
	}}})
	if _, err := service.SyncPublicAccounts(req); err != nil {
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

func TestSyncPublicAccountsAppliesFirstRevision(t *testing.T) {
	db := setupAgentBackupSyncTestDB(t)
	service := &BackupService{}

	result, err := service.SyncPublicAccounts(sealAgentBackupSyncRequest(t, dto.BackupPublicSync{
		Revision: 1,
		Accounts: []dto.BackupPublicSyncAccount{
			newAgentBackupSyncAccount("first-revision", "revision-1"),
		},
	}))
	if err != nil {
		t.Fatalf("apply first revision: %v", err)
	}
	assertAgentBackupSyncResult(t, result, 1, "applied")

	state := loadAgentBackupSyncState(t, db)
	if state.Authority != agentBackupSyncTestAuthority || state.Generation != agentBackupSyncTestGeneration || state.TargetEpoch != agentBackupSyncTestTargetEpoch || state.AppliedRevision != 1 || state.AppliedDigest == "" || state.AppliedAt == nil {
		t.Fatalf("persisted state = %#v, want applied revision 1 with timestamp", state)
	}
	var account model.BackupAccount
	if err := db.Where("name = ? AND is_public = ?", "first-revision", true).First(&account).Error; err != nil {
		t.Fatalf("load first revision account: %v", err)
	}
}

func TestPublicBackupSyncCanonicalDigestGoldenVector(t *testing.T) {
	authorizedAt := time.Date(2026, 8, 4, 12, 0, 0, 123000000, time.FixedZone("test-offset", 8*60*60))
	req := dto.BackupPublicSync{
		Authority:   agentBackupSyncTestAuthority,
		Generation:  agentBackupSyncTestGeneration,
		TargetEpoch: agentBackupSyncTestTargetEpoch,
		Revision:    42,
		Accounts: []dto.BackupPublicSyncAccount{
			{Name: " zeta ", Type: constant.S3, IsPublic: true, Vars: `{"z":1}`},
			{Name: "alpha", Type: constant.GoogleDrive, IsPublic: true, Vars: `{"a":1}`, OAuth: &dto.BackupOAuthSecretSync{
				Provider:     model.BackupOAuthProviderGoogle,
				ClientID:     "synthetic-client-id",
				ClientSecret: "synthetic-client-secret",
				RefreshToken: "synthetic-refresh-token",
				Status:       model.BackupOAuthStatusConfigured,
				AuthorizedAt: &authorizedAt,
			}},
		},
		Tombstones: []dto.BackupPublicSyncTombstone{
			{Name: " old-zeta ", Revision: 41},
			{Name: "old-alpha", Revision: 40},
		},
	}
	canonical := req
	canonical.Accounts = []dto.BackupPublicSyncAccount{req.Accounts[1], req.Accounts[0]}
	canonical.Accounts[0].Name = "alpha"
	canonical.Accounts[1].Name = "zeta"
	oauth := *canonical.Accounts[0].OAuth
	utc := authorizedAt.UTC()
	oauth.AuthorizedAt = &utc
	canonical.Accounts[0].OAuth = &oauth
	canonical.Tombstones = []dto.BackupPublicSyncTombstone{
		{Name: "old-alpha", Revision: 40},
		{Name: "old-zeta", Revision: 41},
	}
	digest, err := publicBackupSyncDigest(canonical)
	if err != nil {
		t.Fatalf("calculate canonical digest: %v", err)
	}
	req.SnapshotDigest = digest
	normalized, err := normalizePublicBackupSync(req)
	if err != nil {
		t.Fatalf("normalize canonical payload: %v", err)
	}
	const expectedDigest = "56ba03b9c591e0a6c67153568e75923b195e95e87142c550bee4fddc51d07c45"
	if normalized.SnapshotDigest != expectedDigest {
		t.Fatalf("canonical digest = %s, want %s", normalized.SnapshotDigest, expectedDigest)
	}
	if normalized.SnapshotDigest != digest {
		t.Fatalf("normalized digest = %s, want %s", normalized.SnapshotDigest, digest)
	}
}

func TestSyncPublicAccountsSameRevisionIsIdempotent(t *testing.T) {
	db := setupAgentBackupSyncTestDB(t)
	service := &BackupService{}
	first := sealAgentBackupSyncRequest(t, dto.BackupPublicSync{
		Revision: 7,
		Accounts: []dto.BackupPublicSyncAccount{
			newAgentBackupSyncAccount("idempotent-account", "original"),
		},
	})
	if result, err := service.SyncPublicAccounts(first); err != nil {
		t.Fatalf("apply initial revision: %v", err)
	} else {
		assertAgentBackupSyncResult(t, result, 7, "applied")
	}
	expiredAt := time.Now().Add(-publicBackupSyncLeaseDuration - time.Minute)
	if err := db.Model(&model.BackupPublicSyncState{}).
		Where("id = ?", model.BackupPublicSyncStateID).
		Update("applied_at", expiredAt).Error; err != nil {
		t.Fatalf("expire synchronization lease before idempotent heartbeat: %v", err)
	}

	result, err := service.SyncPublicAccounts(first)
	if err != nil {
		t.Fatalf("reapply same revision: %v", err)
	}
	assertAgentBackupSyncResult(t, result, 7, "already_applied")
	refreshedState := loadAgentBackupSyncState(t, db)
	if refreshedState.AppliedAt == nil || !refreshedState.AppliedAt.After(expiredAt) || time.Since(*refreshedState.AppliedAt) > time.Minute {
		t.Fatalf("idempotent snapshot did not renew the execution lease: %#v", refreshedState)
	}

	var account model.BackupAccount
	if err := db.Where("name = ?", "idempotent-account").First(&account).Error; err != nil {
		t.Fatalf("same revision removed existing account: %v", err)
	}
	if !strings.Contains(account.Vars, "original") {
		t.Fatalf("same revision mutated account vars: %s", account.Vars)
	}

	conflicting := sealAgentBackupSyncRequest(t, dto.BackupPublicSync{Revision: 7})
	if _, err := service.SyncPublicAccounts(conflicting); err == nil || !strings.Contains(err.Error(), "conflicts with the already applied revision") {
		t.Fatalf("same revision with different content error = %v", err)
	}
}

func TestSyncPublicAccountsSameRevisionRepairsLocalSnapshotDrift(t *testing.T) {
	db := setupAgentBackupSyncTestDB(t)
	service := &BackupService{}
	req := sealAgentBackupSyncRequest(t, dto.BackupPublicSync{
		Revision: 9,
		Accounts: []dto.BackupPublicSyncAccount{
			newAgentBackupSyncAccount("repair-drift", "authoritative"),
		},
	})
	if _, err := service.SyncPublicAccounts(req); err != nil {
		t.Fatalf("apply authoritative snapshot: %v", err)
	}
	if err := db.Where("name = ?", "repair-drift").Delete(&model.BackupAccount{}).Error; err != nil {
		t.Fatalf("simulate local snapshot drift: %v", err)
	}

	result, err := service.SyncPublicAccounts(req)
	if err != nil {
		t.Fatalf("reapply authoritative snapshot after drift: %v", err)
	}
	assertAgentBackupSyncResult(t, result, 9, "already_applied")
	var repaired model.BackupAccount
	if err := db.Where("name = ? AND is_public = ?", "repair-drift", true).First(&repaired).Error; err != nil {
		t.Fatalf("same revision did not repair missing public account: %v", err)
	}
	if !strings.Contains(repaired.Vars, "authoritative") {
		t.Fatalf("repaired account does not match authoritative snapshot: %s", repaired.Vars)
	}
}

func TestStaleAndConflictingSnapshotsDoNotRenewLease(t *testing.T) {
	db := setupAgentBackupSyncTestDB(t)
	service := &BackupService{}
	current := sealAgentBackupSyncRequest(t, dto.BackupPublicSync{
		Revision: 7,
		Accounts: []dto.BackupPublicSyncAccount{
			newAgentBackupSyncAccount("lease-stability", "current"),
		},
	})
	if _, err := service.SyncPublicAccounts(current); err != nil {
		t.Fatalf("apply current snapshot: %v", err)
	}
	expiredAt := time.Now().Add(-publicBackupSyncLeaseDuration - time.Minute)
	if err := db.Model(&model.BackupPublicSyncState{}).
		Where("id = ?", model.BackupPublicSyncStateID).
		Update("applied_at", expiredAt).Error; err != nil {
		t.Fatalf("expire synchronization lease: %v", err)
	}
	before := loadAgentBackupSyncState(t, db)

	stale := sealAgentBackupSyncRequest(t, dto.BackupPublicSync{
		Revision: 6,
		Accounts: []dto.BackupPublicSyncAccount{
			newAgentBackupSyncAccount("lease-stability", "stale"),
		},
	})
	result, err := service.SyncPublicAccounts(stale)
	if err != nil {
		t.Fatalf("deliver stale snapshot: %v", err)
	}
	assertAgentBackupSyncResult(t, result, 7, "stale_ignored")
	afterStale := loadAgentBackupSyncState(t, db)
	if before.AppliedAt == nil || afterStale.AppliedAt == nil || !afterStale.AppliedAt.Equal(*before.AppliedAt) {
		t.Fatalf("stale snapshot renewed lease: before=%v after=%v", before.AppliedAt, afterStale.AppliedAt)
	}

	conflicting := sealAgentBackupSyncRequest(t, dto.BackupPublicSync{
		Revision: 7,
		Accounts: []dto.BackupPublicSyncAccount{
			newAgentBackupSyncAccount("lease-stability", "conflicting"),
		},
	})
	if _, err := service.SyncPublicAccounts(conflicting); err == nil || !strings.Contains(err.Error(), "conflicts with the already applied revision") {
		t.Fatalf("conflicting snapshot error = %v", err)
	}
	afterConflict := loadAgentBackupSyncState(t, db)
	if afterConflict.AppliedAt == nil || !afterConflict.AppliedAt.Equal(*before.AppliedAt) {
		t.Fatalf("conflicting snapshot renewed lease: before=%v after=%v", before.AppliedAt, afterConflict.AppliedAt)
	}
}

func TestSyncPublicAccountsRejectsTamperedDigestWithoutMutation(t *testing.T) {
	db := setupAgentBackupSyncTestDB(t)
	service := &BackupService{}
	req := sealAgentBackupSyncRequest(t, dto.BackupPublicSync{
		Revision: 1,
		Accounts: []dto.BackupPublicSyncAccount{
			newAgentBackupSyncAccount("tampered-account", "must-not-persist"),
		},
	})
	req.SnapshotDigest = strings.Repeat("0", 64)
	if _, err := service.SyncPublicAccounts(req); err == nil || !strings.Contains(err.Error(), "digest verification failed") {
		t.Fatalf("tampered digest error = %v", err)
	}
	var count int64
	if err := db.Model(&model.BackupAccount{}).Where("name = ?", "tampered-account").Count(&count).Error; err != nil {
		t.Fatalf("count tampered account: %v", err)
	}
	if count != 0 {
		t.Fatalf("tampered snapshot persisted %d accounts", count)
	}
}

func TestSyncPublicAccountsRejectsAuthorityChangeUntilReenrollment(t *testing.T) {
	db := setupAgentBackupSyncTestDB(t)
	service := &BackupService{}
	if _, err := service.SyncPublicAccounts(sealAgentBackupSyncRequest(t, dto.BackupPublicSync{
		Revision: 1,
		Accounts: []dto.BackupPublicSyncAccount{
			newAgentBackupSyncAccount("bound-account", "original-authority"),
		},
	})); err != nil {
		t.Fatalf("bind initial authority: %v", err)
	}

	foreign := sealAgentBackupSyncRequestWithIdentity(t, dto.BackupPublicSync{
		Revision: 2,
		Accounts: []dto.BackupPublicSyncAccount{
			newAgentBackupSyncAccount("foreign-account", "must-not-persist"),
		},
	}, strings.Repeat("c", 64), agentBackupSyncTestGeneration)
	if _, err := service.SyncPublicAccounts(foreign); err == nil || !strings.Contains(err.Error(), "authority changed") {
		t.Fatalf("foreign authority error = %v", err)
	}
	state := loadAgentBackupSyncState(t, db)
	if state.Authority != agentBackupSyncTestAuthority || state.AppliedRevision != 1 {
		t.Fatalf("foreign authority changed state: %#v", state)
	}
	var foreignCount int64
	if err := db.Model(&model.BackupAccount{}).Where("name = ?", "foreign-account").Count(&foreignCount).Error; err != nil {
		t.Fatalf("count foreign account: %v", err)
	}
	if foreignCount != 0 {
		t.Fatalf("foreign authority persisted %d accounts", foreignCount)
	}
}

func TestSyncPublicAccountsRejectsGenerationChangeWithoutRecovery(t *testing.T) {
	db := setupAgentBackupSyncTestDB(t)
	service := &BackupService{}
	if _, err := service.SyncPublicAccounts(sealAgentBackupSyncRequest(t, dto.BackupPublicSync{
		Revision: 1,
		Accounts: []dto.BackupPublicSyncAccount{
			newAgentBackupSyncAccount("generation-bound", "original-generation"),
		},
	})); err != nil {
		t.Fatalf("bind initial generation: %v", err)
	}

	changed := sealAgentBackupSyncRequestWithIdentity(t, dto.BackupPublicSync{
		Revision: 2,
		Accounts: []dto.BackupPublicSyncAccount{
			newAgentBackupSyncAccount("changed-generation", "must-not-persist"),
		},
	}, agentBackupSyncTestAuthority, strings.Repeat("d", 64))
	if _, err := service.SyncPublicAccounts(changed); err == nil || !strings.Contains(err.Error(), "generation changed") {
		t.Fatalf("changed generation error = %v", err)
	}
	state := loadAgentBackupSyncState(t, db)
	if state.Generation != agentBackupSyncTestGeneration || state.AppliedRevision != 1 {
		t.Fatalf("generation change modified state: %#v", state)
	}
}

func TestSyncPublicAccountsBindsLegacyRevisionStateWithAuthoritativeSnapshot(t *testing.T) {
	db := setupAgentBackupSyncTestDB(t)
	legacyState := model.BackupPublicSyncState{ID: model.BackupPublicSyncStateID, AppliedRevision: 9}
	if err := db.Create(&legacyState).Error; err != nil {
		t.Fatalf("seed legacy revision state: %v", err)
	}
	service := &BackupService{}
	result, err := service.SyncPublicAccounts(sealAgentBackupSyncRequest(t, dto.BackupPublicSync{
		Revision: 9,
		Accounts: []dto.BackupPublicSyncAccount{
			newAgentBackupSyncAccount("legacy-bound-account", "authoritative"),
		},
	}))
	if err != nil {
		t.Fatalf("bind legacy revision state: %v", err)
	}
	assertAgentBackupSyncResult(t, result, 9, "applied")
	state := loadAgentBackupSyncState(t, db)
	if state.Authority != agentBackupSyncTestAuthority || state.Generation != agentBackupSyncTestGeneration || state.TargetEpoch != agentBackupSyncTestTargetEpoch || state.AppliedDigest == "" {
		t.Fatalf("legacy state was not bound to authority: %#v", state)
	}
}

func TestSyncPublicAccountsRejectsOlderSnapshotForLegacyRevisionState(t *testing.T) {
	db := setupAgentBackupSyncTestDB(t)
	legacyState := model.BackupPublicSyncState{ID: model.BackupPublicSyncStateID, AppliedRevision: 9}
	if err := db.Create(&legacyState).Error; err != nil {
		t.Fatalf("seed legacy revision state: %v", err)
	}
	service := &BackupService{}
	_, err := service.SyncPublicAccounts(sealAgentBackupSyncRequest(t, dto.BackupPublicSync{
		Revision: 8,
		Accounts: []dto.BackupPublicSyncAccount{
			newAgentBackupSyncAccount("too-old-for-legacy-state", "must-not-persist"),
		},
	}))
	if err == nil || !strings.Contains(err.Error(), "requires a newer authoritative snapshot") {
		t.Fatalf("older legacy snapshot error = %v", err)
	}
	state := loadAgentBackupSyncState(t, db)
	if state.Authority != "" || state.AppliedRevision != 9 {
		t.Fatalf("older legacy snapshot changed state: %#v", state)
	}
}

func TestSyncPublicAccountsOlderRevisionIsIgnored(t *testing.T) {
	db := setupAgentBackupSyncTestDB(t)
	service := &BackupService{}
	if result, err := service.SyncPublicAccounts(sealAgentBackupSyncRequest(t, dto.BackupPublicSync{
		Revision: 12,
		Accounts: []dto.BackupPublicSyncAccount{
			newAgentBackupSyncAccount("newer-account", "revision-12"),
		},
	})); err != nil {
		t.Fatalf("apply newer revision: %v", err)
	} else {
		assertAgentBackupSyncResult(t, result, 12, "applied")
	}

	result, err := service.SyncPublicAccounts(sealAgentBackupSyncRequest(t, dto.BackupPublicSync{
		Revision: 11,
		Accounts: []dto.BackupPublicSyncAccount{
			newAgentBackupSyncAccount("older-account", "revision-11"),
		},
	}))
	if err != nil {
		t.Fatalf("send older revision: %v", err)
	}
	assertAgentBackupSyncResult(t, result, 12, "stale_ignored")

	var newerCount, olderCount int64
	if err := db.Model(&model.BackupAccount{}).Where("name = ?", "newer-account").Count(&newerCount).Error; err != nil {
		t.Fatalf("count newer account: %v", err)
	}
	if err := db.Model(&model.BackupAccount{}).Where("name = ?", "older-account").Count(&olderCount).Error; err != nil {
		t.Fatalf("count older account: %v", err)
	}
	if newerCount != 1 || olderCount != 0 {
		t.Fatalf("older revision changed snapshot: newer=%d older=%d", newerCount, olderCount)
	}
}

func TestSyncPublicAccountsAppliesTombstone(t *testing.T) {
	db := setupAgentBackupSyncTestDB(t)
	service := &BackupService{}
	if _, err := service.SyncPublicAccounts(sealAgentBackupSyncRequest(t, dto.BackupPublicSync{
		Revision: 3,
		Accounts: []dto.BackupPublicSyncAccount{{
			Name:     "removed-oauth-account",
			Type:     constant.OneDrive,
			IsPublic: true,
			Vars:     "{}",
			OAuth: &dto.BackupOAuthSecretSync{
				Provider:     model.BackupOAuthProviderMicrosoft,
				ClientID:     "synthetic-client-id",
				ClientSecret: "synthetic-client-secret",
				RefreshToken: "synthetic-refresh-token",
				Status:       model.BackupOAuthStatusConfigured,
			},
		}},
	})); err != nil {
		t.Fatalf("seed tombstoned account: %v", err)
	}

	result, err := service.SyncPublicAccounts(sealAgentBackupSyncRequest(t, dto.BackupPublicSync{
		Revision: 4,
		Tombstones: []dto.BackupPublicSyncTombstone{{
			Name:     "removed-oauth-account",
			Revision: 4,
		}},
	}))
	if err != nil {
		t.Fatalf("apply tombstone: %v", err)
	}
	assertAgentBackupSyncResult(t, result, 4, "applied")

	var accountCount, credentialCount int64
	if err := db.Model(&model.BackupAccount{}).Where("name = ?", "removed-oauth-account").Count(&accountCount).Error; err != nil {
		t.Fatalf("count tombstoned account: %v", err)
	}
	if err := db.Model(&model.BackupOAuthCredential{}).Count(&credentialCount).Error; err != nil {
		t.Fatalf("count tombstoned credential: %v", err)
	}
	if accountCount != 0 || credentialCount != 0 {
		t.Fatalf("tombstone left rows: accounts=%d credentials=%d", accountCount, credentialCount)
	}
}

func TestSyncPublicAccountsTransactionFailureDoesNotAdvanceRevision(t *testing.T) {
	db := setupAgentBackupSyncTestDB(t)
	service := &BackupService{}
	if _, err := service.SyncPublicAccounts(sealAgentBackupSyncRequest(t, dto.BackupPublicSync{
		Revision: 20,
		Accounts: []dto.BackupPublicSyncAccount{
			newAgentBackupSyncAccount("stable-account", "revision-20"),
		},
	})); err != nil {
		t.Fatalf("seed stable revision: %v", err)
	}
	private := model.BackupAccount{Name: "private-conflict", Type: constant.S3, IsPublic: false, Vars: "{}"}
	if err := db.Create(&private).Error; err != nil {
		t.Fatalf("seed private conflict: %v", err)
	}

	_, err := service.SyncPublicAccounts(sealAgentBackupSyncRequest(t, dto.BackupPublicSync{
		Revision: 21,
		Accounts: []dto.BackupPublicSyncAccount{
			newAgentBackupSyncAccount("created-before-failure", "must-roll-back"),
			newAgentBackupSyncAccount("private-conflict", "must-fail"),
		},
	}))
	if err == nil || !strings.Contains(err.Error(), "conflicts with a private account") {
		t.Fatalf("transaction failure error = %v", err)
	}

	state := loadAgentBackupSyncState(t, db)
	if state.AppliedRevision != 20 {
		t.Fatalf("failed transaction advanced revision to %d", state.AppliedRevision)
	}
	var stableCount, rolledBackCount, privateCount int64
	_ = db.Model(&model.BackupAccount{}).Where("name = ? AND is_public = ?", "stable-account", true).Count(&stableCount).Error
	_ = db.Model(&model.BackupAccount{}).Where("name = ? AND is_public = ?", "created-before-failure", true).Count(&rolledBackCount).Error
	_ = db.Model(&model.BackupAccount{}).Where("id = ?", private.ID).Count(&privateCount).Error
	if stableCount != 1 || rolledBackCount != 0 || privateCount != 1 {
		t.Fatalf("failed transaction changed rows: stable=%d rolledBack=%d private=%d", stableCount, rolledBackCount, privateCount)
	}
}

func TestSyncPublicAccountsRevisionSurvivesDatabaseRestart(t *testing.T) {
	_, restart := setupAgentBackupSyncRestartableDB(t)
	service := &BackupService{}
	if _, err := service.SyncPublicAccounts(sealAgentBackupSyncRequest(t, dto.BackupPublicSync{
		Revision: 31,
		Accounts: []dto.BackupPublicSyncAccount{{
			Name:     "restart-account",
			Type:     constant.Local,
			IsPublic: true,
			Vars:     `{"marker":"revision-31"}`,
		}},
	})); err != nil {
		t.Fatalf("apply revision before restart: %v", err)
	}

	db := restart()
	service = &BackupService{}
	result, err := service.SyncPublicAccounts(sealAgentBackupSyncRequest(t, dto.BackupPublicSync{
		Revision: 30,
		Accounts: []dto.BackupPublicSyncAccount{
			newAgentBackupSyncAccount("stale-after-restart", "revision-30"),
		},
	}))
	if err != nil {
		t.Fatalf("send stale revision after restart: %v", err)
	}
	assertAgentBackupSyncResult(t, result, 31, "stale_ignored")

	state := loadAgentBackupSyncState(t, db)
	if state.Authority != agentBackupSyncTestAuthority || state.Generation != agentBackupSyncTestGeneration || state.TargetEpoch != agentBackupSyncTestTargetEpoch || state.AppliedRevision != 31 || state.AppliedDigest == "" || state.AppliedAt == nil {
		t.Fatalf("sync state after restart = %#v, want persisted authority/generation/revision/digest", state)
	}
	var account model.BackupAccount
	if err := db.Where("name = ?", "restart-account").First(&account).Error; err != nil {
		t.Fatalf("load public account after restart: %v", err)
	}
	if err := ensurePublicBackupSyncLease(&account, time.Now()); err != nil {
		t.Fatalf("fresh synchronization lease did not survive restart: %v", err)
	}
	if _, client, err := NewBackupClientWithID(account.ID); err != nil || client == nil {
		t.Fatalf("real public client entry point after restart: client=%T err=%v", client, err)
	}
	var currentCount, staleCount int64
	_ = db.Model(&model.BackupAccount{}).Where("name = ?", "restart-account").Count(&currentCount).Error
	_ = db.Model(&model.BackupAccount{}).Where("name = ?", "stale-after-restart").Count(&staleCount).Error
	if currentCount != 1 || staleCount != 0 {
		t.Fatalf("restart lost revision guard: current=%d stale=%d", currentCount, staleCount)
	}

	expiredAt := time.Now().Add(-publicBackupSyncLeaseDuration - time.Second)
	if err := db.Model(&model.BackupPublicSyncState{}).
		Where("id = ?", model.BackupPublicSyncStateID).
		Update("applied_at", expiredAt).Error; err != nil {
		t.Fatalf("expire persisted synchronization lease: %v", err)
	}
	db = restart()
	if err := db.Where("name = ?", "restart-account").First(&account).Error; err != nil {
		t.Fatalf("reload public account after expired-lease restart: %v", err)
	}
	if err := ensurePublicBackupSyncLease(&account, time.Now()); err == nil || !strings.Contains(err.Error(), "lease expired") {
		t.Fatalf("expired synchronization lease after restart error = %v", err)
	}
	if _, _, err := NewBackupClientWithID(account.ID); err == nil || !strings.Contains(err.Error(), "lease expired") {
		t.Fatalf("real public client entry point with expired restarted lease error = %v", err)
	}
}

func TestPublicBackupClientRequiresFreshSynchronizationLease(t *testing.T) {
	db := setupAgentBackupSyncTestDB(t)
	service := &BackupService{}
	if _, err := service.SyncPublicAccounts(sealAgentBackupSyncRequest(t, dto.BackupPublicSync{
		Revision: 1,
		Accounts: []dto.BackupPublicSyncAccount{{
			Name:     "public-local",
			Type:     constant.Local,
			IsPublic: true,
			Vars:     "{}",
		}},
	})); err != nil {
		t.Fatalf("seed synchronized public account: %v", err)
	}

	var publicAccount model.BackupAccount
	if err := db.Where("name = ?", "public-local").First(&publicAccount).Error; err != nil {
		t.Fatalf("load synchronized public account: %v", err)
	}
	if _, err := newClient(&publicAccount, true); err != nil {
		t.Fatalf("fresh public-account lease rejected: %v", err)
	}

	state := loadAgentBackupSyncState(t, db)
	invalidStates := []struct {
		name    string
		updates map[string]interface{}
	}{
		{name: "empty authority", updates: map[string]interface{}{"authority": ""}},
		{name: "invalid authority", updates: map[string]interface{}{"authority": "invalid"}},
		{name: "empty generation", updates: map[string]interface{}{"generation": ""}},
		{name: "invalid generation", updates: map[string]interface{}{"generation": "invalid"}},
		{name: "empty target epoch", updates: map[string]interface{}{"target_epoch": ""}},
		{name: "invalid target epoch", updates: map[string]interface{}{"target_epoch": "invalid"}},
		{name: "zero revision", updates: map[string]interface{}{"applied_revision": 0}},
		{name: "missing applied time", updates: map[string]interface{}{"applied_at": nil}},
		{name: "future applied time", updates: map[string]interface{}{"applied_at": time.Now().Add(2 * time.Minute)}},
	}
	for _, test := range invalidStates {
		if err := db.Model(&model.BackupPublicSyncState{}).
			Where("id = ?", model.BackupPublicSyncStateID).
			Updates(test.updates).Error; err != nil {
			t.Fatalf("corrupt persisted synchronization state %s: %v", test.name, err)
		}
		if _, err := newClient(&publicAccount, true); err == nil || !strings.Contains(err.Error(), "lease expired") {
			t.Fatalf("invalid synchronization state %s error = %v", test.name, err)
		}
		if err := db.Model(&model.BackupPublicSyncState{}).
			Where("id = ?", model.BackupPublicSyncStateID).
			Updates(map[string]interface{}{
				"authority":        state.Authority,
				"generation":       state.Generation,
				"target_epoch":     state.TargetEpoch,
				"applied_revision": state.AppliedRevision,
				"applied_digest":   state.AppliedDigest,
				"applied_at":       state.AppliedAt,
			}).Error; err != nil {
			t.Fatalf("restore persisted synchronization state after %s: %v", test.name, err)
		}
	}
	if err := db.Model(&model.BackupPublicSyncState{}).
		Where("id = ?", model.BackupPublicSyncStateID).
		Update("applied_digest", "invalid").Error; err != nil {
		t.Fatalf("corrupt persisted synchronization digest: %v", err)
	}
	if _, err := newClient(&publicAccount, true); err == nil || !strings.Contains(err.Error(), "lease expired") {
		t.Fatalf("invalid synchronization state error = %v", err)
	}

	expiredAt := time.Now().Add(-publicBackupSyncLeaseDuration - time.Second)
	if err := db.Model(&model.BackupPublicSyncState{}).
		Where("id = ?", model.BackupPublicSyncStateID).
		Updates(map[string]interface{}{
			"applied_digest": state.AppliedDigest,
			"applied_at":     expiredAt,
		}).Error; err != nil {
		t.Fatalf("expire persisted synchronization lease: %v", err)
	}
	if _, err := newClient(&publicAccount, true); err == nil || !strings.Contains(err.Error(), "lease expired") {
		t.Fatalf("expired synchronization state error = %v", err)
	}

	if err := db.Where("id = ?", model.BackupPublicSyncStateID).Delete(&model.BackupPublicSyncState{}).Error; err != nil {
		t.Fatalf("delete persisted synchronization lease: %v", err)
	}
	if _, err := newClient(&publicAccount, true); err == nil || !strings.Contains(err.Error(), "lease is unavailable") {
		t.Fatalf("missing synchronization state error = %v", err)
	}

	privateAccount := model.BackupAccount{Name: "private-local", Type: constant.Local, Vars: "{}"}
	if err := db.Create(&privateAccount).Error; err != nil {
		t.Fatalf("seed private account without public synchronization state: %v", err)
	}
	if _, err := newClient(&privateAccount, true); err != nil {
		t.Fatalf("private account was incorrectly subject to public synchronization lease: %v", err)
	}
	if _, client, err := NewBackupClientWithID(privateAccount.ID); err != nil || client == nil {
		t.Fatalf("real private client entry point required public synchronization lease: client=%T err=%v", client, err)
	}
}

func TestHandleDownloadSnapshotReturnsExpiredLeaseErrorWithoutPanic(t *testing.T) {
	db := setupAgentBackupSyncTestDB(t)
	i18n.Init()
	service := &BackupService{}
	if _, err := service.SyncPublicAccounts(sealAgentBackupSyncRequest(t, dto.BackupPublicSync{
		Revision: 1,
		Accounts: []dto.BackupPublicSyncAccount{{
			Name:     "expired-snapshot-source",
			Type:     constant.Local,
			IsPublic: true,
			Vars:     "{}",
		}},
	})); err != nil {
		t.Fatal(err)
	}
	var account model.BackupAccount
	if err := db.Where("name = ?", "expired-snapshot-source").First(&account).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.BackupPublicSyncState{}).
		Where("id = ?", model.BackupPublicSyncStateID).
		Update("applied_at", time.Now().Add(-publicBackupSyncLeaseDuration-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	helper := &snapRecoverHelper{Task: &task.Task{Logger: logger}}
	err := handleDownloadSnapshot(helper, model.Snapshot{DownloadAccountID: account.ID}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "lease expired") {
		t.Fatalf("expired snapshot source error = %v", err)
	}
}

func TestPublicBackupClientRejectsStaleAccountAndLeaseAfterRevisionChange(t *testing.T) {
	db := setupAgentBackupSyncTestDB(t)
	service := &BackupService{}
	first := sealAgentBackupSyncRequest(t, dto.BackupPublicSync{
		Revision: 1,
		Accounts: []dto.BackupPublicSyncAccount{{
			Name:      "rotating-public-local",
			Type:      constant.Local,
			IsPublic:  true,
			AccessKey: "revision-one-key",
			Vars:      `{"marker":"revision-one"}`,
		}},
	})
	if _, err := service.SyncPublicAccounts(first); err != nil {
		t.Fatalf("apply first public-account revision: %v", err)
	}

	var stale model.BackupAccount
	if err := db.Where("name = ?", "rotating-public-local").First(&stale).Error; err != nil {
		t.Fatalf("load first-revision public account: %v", err)
	}
	oldClient, err := newClient(&stale, true)
	if err != nil {
		t.Fatalf("create first-revision guarded client: %v", err)
	}

	second := sealAgentBackupSyncRequest(t, dto.BackupPublicSync{
		Revision: 2,
		Accounts: []dto.BackupPublicSyncAccount{{
			Name:      "rotating-public-local",
			Type:      constant.Local,
			IsPublic:  true,
			AccessKey: "revision-two-key",
			Vars:      `{"marker":"revision-two"}`,
		}},
	})
	if _, err := service.SyncPublicAccounts(second); err != nil {
		t.Fatalf("apply rotated public-account revision: %v", err)
	}
	if _, err := oldClient.ListBuckets(); err == nil || !strings.Contains(err.Error(), "synchronization changed") {
		t.Fatalf("first-revision client after rotation error = %v", err)
	}
	if _, err := newClient(&stale, true); err == nil || !strings.Contains(err.Error(), "account changed") {
		t.Fatalf("stale first-revision account after rotation error = %v", err)
	}

	var current model.BackupAccount
	if err := db.Where("name = ?", "rotating-public-local").First(&current).Error; err != nil {
		t.Fatalf("load rotated public account: %v", err)
	}
	if _, err := newClient(&current, true); err != nil {
		t.Fatalf("current public account was rejected after rotation: %v", err)
	}

	deleted := sealAgentBackupSyncRequest(t, dto.BackupPublicSync{
		Revision: 3,
		Tombstones: []dto.BackupPublicSyncTombstone{{
			Name:     "rotating-public-local",
			Revision: 3,
		}},
	})
	if _, err := service.SyncPublicAccounts(deleted); err != nil {
		t.Fatalf("apply public-account tombstone: %v", err)
	}
	if _, err := newClient(&current, true); err == nil || !strings.Contains(err.Error(), "account changed") {
		t.Fatalf("deleted public account object error = %v", err)
	}
}

func TestPublicBackupClientSerializesOperationWithSnapshotApply(t *testing.T) {
	setupAgentBackupSyncTestDB(t)
	service := &BackupService{}
	first := sealAgentBackupSyncRequest(t, dto.BackupPublicSync{
		Revision: 1,
		Accounts: []dto.BackupPublicSyncAccount{{Name: "serialized-public-local", Type: constant.Local, IsPublic: true, Vars: "{}"}},
	})
	if _, err := service.SyncPublicAccounts(first); err != nil {
		t.Fatalf("apply initial serialized snapshot: %v", err)
	}
	lease, err := currentPublicBackupSyncLease(time.Now())
	if err != nil {
		t.Fatalf("load initial execution lease: %v", err)
	}
	blocker := &blockingBackupClient{started: make(chan struct{}), release: make(chan struct{})}
	guarded := &publicBackupLeaseClient{client: blocker, lease: lease}
	operationDone := make(chan error, 1)
	go func() {
		_, operationErr := guarded.ListBuckets()
		operationDone <- operationErr
	}()
	<-blocker.started

	second := sealAgentBackupSyncRequest(t, dto.BackupPublicSync{
		Revision: 2,
		Accounts: []dto.BackupPublicSyncAccount{{Name: "serialized-public-local", Type: constant.Local, IsPublic: true, Vars: "{}"}},
	})
	syncStarted := make(chan struct{})
	syncDone := make(chan error, 1)
	go func() {
		close(syncStarted)
		_, syncErr := service.SyncPublicAccounts(second)
		syncDone <- syncErr
	}()
	<-syncStarted
	select {
	case syncErr := <-syncDone:
		t.Fatalf("snapshot apply completed while a guarded operation was active: %v", syncErr)
	case <-time.After(100 * time.Millisecond):
	}

	close(blocker.release)
	if operationErr := <-operationDone; operationErr != nil {
		t.Fatalf("guarded operation failed before revision change: %v", operationErr)
	}
	if syncErr := <-syncDone; syncErr != nil {
		t.Fatalf("snapshot apply after guarded operation: %v", syncErr)
	}
	if _, err := guarded.ListBuckets(); err == nil || !strings.Contains(err.Error(), "synchronization changed") {
		t.Fatalf("old guarded client after serialized revision change error = %v", err)
	}
}

func TestPublicBackupClientRejectsCredentialDecryptFailure(t *testing.T) {
	db := setupAgentBackupSyncTestDB(t)
	service := &BackupService{}
	if _, err := service.SyncPublicAccounts(sealAgentBackupSyncRequest(t, dto.BackupPublicSync{
		Revision: 1,
		Accounts: []dto.BackupPublicSyncAccount{{
			Name:      "corrupt-public-local",
			Type:      constant.Local,
			IsPublic:  true,
			AccessKey: "synthetic-key",
			Vars:      "{}",
		}},
	})); err != nil {
		t.Fatalf("apply public account before ciphertext corruption: %v", err)
	}
	const corruptCiphertext = "not-valid-panel-ciphertext"
	if err := db.Model(&model.BackupAccount{}).
		Where("name = ?", "corrupt-public-local").
		Update("access_key", corruptCiphertext).Error; err != nil {
		t.Fatalf("corrupt public account ciphertext: %v", err)
	}
	var account model.BackupAccount
	if err := db.Where("name = ?", "corrupt-public-local").First(&account).Error; err != nil {
		t.Fatalf("load corrupted public account: %v", err)
	}
	_, err := newClient(&account, true)
	if err == nil || !strings.Contains(err.Error(), "credential is unavailable") {
		t.Fatalf("corrupted public credential error = %v", err)
	}
	if strings.Contains(err.Error(), corruptCiphertext) || strings.Contains(err.Error(), account.Name) {
		t.Fatalf("corrupted public credential error leaked stored data: %v", err)
	}
}

func TestReenrollmentRejectsDelayedSnapshotFromOldTargetEpoch(t *testing.T) {
	db := setupAgentBackupSyncTestDB(t)
	if err := db.AutoMigrate(&model.Setting{}); err != nil {
		t.Fatalf("migrate enrollment settings: %v", err)
	}
	service := &BackupService{}
	oldSnapshot := sealAgentBackupSyncRequest(t, dto.BackupPublicSync{
		Revision: 1,
		Accounts: []dto.BackupPublicSyncAccount{
			newAgentBackupSyncAccount("old-authority-account", "old"),
		},
	})
	if _, err := service.SyncPublicAccounts(oldSnapshot); err != nil {
		t.Fatalf("apply old-authority snapshot: %v", err)
	}

	newAuthority := agentBackupSyncTestAuthority
	newGeneration := agentBackupSyncTestGeneration
	newTargetEpoch := strings.Repeat("e", 64)
	if err := xpackhelper.ApplyEnrollment(xpackhelper.EnrollResponse{
		ServerCert:            "-----NEW SERVER CERT-----",
		CACert:                "-----NEW CA CERT-----",
		ProxyID:               "new-proxy-id",
		CoreClientFingerprint: "synthetic-core-fingerprint",
		BackupSyncAuthority:   newAuthority,
		BackupSyncGeneration:  newGeneration,
		BackupSyncTargetEpoch: newTargetEpoch,
	}, []byte("-----NEW NODE KEY-----"), 9101, filepath.Join(t.TempDir(), ".nodeProxyID")); err != nil {
		t.Fatalf("apply re-enrollment namespace: %v", err)
	}
	if _, err := service.SyncPublicAccounts(oldSnapshot); err == nil || !strings.Contains(err.Error(), "target epoch changed") {
		t.Fatalf("delayed old-target-epoch snapshot error = %v", err)
	}
	state := loadAgentBackupSyncState(t, db)
	if state.Authority != newAuthority || state.Generation != newGeneration || state.TargetEpoch != newTargetEpoch || state.AppliedRevision != 0 {
		t.Fatalf("delayed old snapshot changed re-enrollment target epoch: %#v", state)
	}

	newSnapshot := sealAgentBackupSyncRequestWithIdentity(t, dto.BackupPublicSync{
		TargetEpoch: newTargetEpoch,
		Revision:    2,
		Accounts: []dto.BackupPublicSyncAccount{
			newAgentBackupSyncAccount("new-authority-account", "new"),
		},
	}, newAuthority, newGeneration)
	if _, err := service.SyncPublicAccounts(newSnapshot); err != nil {
		t.Fatalf("apply snapshot from re-enrolled authority: %v", err)
	}
}

func TestSyncPublicAccountsConcurrentRevisionsConvergeToNewest(t *testing.T) {
	db := setupAgentBackupSyncTestDB(t)
	service := &BackupService{}
	revisions := []uint64{43, 41, 46, 42, 45, 44}
	type syncOutcome struct {
		revision uint64
		result   dto.BackupPublicSyncResult
		err      error
	}
	start := make(chan struct{})
	outcomes := make(chan syncOutcome, len(revisions))
	var wg sync.WaitGroup
	for _, revision := range revisions {
		revision := revision
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, err := service.SyncPublicAccounts(sealAgentBackupSyncRequest(t, dto.BackupPublicSync{
				Revision: revision,
				Accounts: []dto.BackupPublicSyncAccount{
					newAgentBackupSyncAccount("concurrent-account", fmt.Sprintf("revision-%d", revision)),
				},
			}))
			outcomes <- syncOutcome{revision: revision, result: result, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(outcomes)

	newestApplied := false
	for outcome := range outcomes {
		if outcome.err != nil {
			t.Fatalf("revision %d failed: %v", outcome.revision, outcome.err)
		}
		if outcome.revision == 46 {
			assertAgentBackupSyncResult(t, outcome.result, 46, "applied")
			newestApplied = true
		}
	}
	if !newestApplied {
		t.Fatal("newest concurrent revision did not complete")
	}

	state := loadAgentBackupSyncState(t, db)
	if state.AppliedRevision != 46 {
		t.Fatalf("concurrent applied revision = %d, want 46", state.AppliedRevision)
	}
	var account model.BackupAccount
	if err := db.Where("name = ?", "concurrent-account").First(&account).Error; err != nil {
		t.Fatalf("load concurrent account: %v", err)
	}
	if !strings.Contains(account.Vars, "revision-46") {
		t.Fatalf("concurrent snapshot ended with stale data: %s", account.Vars)
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
	if err := db.AutoMigrate(&model.BackupAccount{}, &model.BackupOAuthCredential{}, &model.BackupPublicSyncState{}, &model.Setting{}); err != nil {
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

func setupAgentBackupSyncRestartableDB(t *testing.T) (*gorm.DB, func() *gorm.DB) {
	t.Helper()
	re.Init()
	oldDB := global.DB
	oldKey := global.CONF.Base.EncryptKey
	databasePath := filepath.Join(t.TempDir(), "backup-sync.db")
	var current *gorm.DB

	open := func() *gorm.DB {
		db, err := gorm.Open(sqlite.Open(databasePath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			t.Fatalf("open restartable test database: %v", err)
		}
		if err := db.AutoMigrate(&model.BackupAccount{}, &model.BackupOAuthCredential{}, &model.BackupPublicSyncState{}, &model.Setting{}); err != nil {
			t.Fatalf("migrate restartable test database: %v", err)
		}
		return db
	}
	closeCurrent := func() {
		if current == nil {
			return
		}
		if sqlDB, err := current.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}

	current = open()
	global.DB = current
	global.CONF.Base.EncryptKey = agentBackupSyncTestKey
	t.Cleanup(func() {
		closeCurrent()
		global.DB = oldDB
		global.CONF.Base.EncryptKey = oldKey
	})
	restart := func() *gorm.DB {
		closeCurrent()
		current = open()
		global.DB = current
		return current
	}
	return current, restart
}

func newAgentBackupSyncAccount(name, marker string) dto.BackupPublicSyncAccount {
	return dto.BackupPublicSyncAccount{
		Name:     name,
		Type:     constant.S3,
		IsPublic: true,
		Vars:     fmt.Sprintf(`{"marker":%q}`, marker),
	}
}

func sealAgentBackupSyncRequest(t *testing.T, req dto.BackupPublicSync) dto.BackupPublicSync {
	return sealAgentBackupSyncRequestWithIdentity(t, req, agentBackupSyncTestAuthority, agentBackupSyncTestGeneration)
}

func sealAgentBackupSyncRequestWithIdentity(t *testing.T, req dto.BackupPublicSync, authority, generation string) dto.BackupPublicSync {
	t.Helper()
	req.Authority = authority
	req.Generation = generation
	if req.TargetEpoch == "" {
		req.TargetEpoch = agentBackupSyncTestTargetEpoch
	}
	if req.Accounts == nil {
		req.Accounts = []dto.BackupPublicSyncAccount{}
	}
	if req.Tombstones == nil {
		req.Tombstones = []dto.BackupPublicSyncTombstone{}
	}
	digest, err := publicBackupSyncDigest(req)
	if err != nil {
		t.Fatalf("seal backup sync request: %v", err)
	}
	req.SnapshotDigest = digest
	return req
}

func loadAgentBackupSyncState(t *testing.T, db *gorm.DB) model.BackupPublicSyncState {
	t.Helper()
	var state model.BackupPublicSyncState
	if err := db.Where("id = ?", model.BackupPublicSyncStateID).First(&state).Error; err != nil {
		t.Fatalf("load backup sync state: %v", err)
	}
	return state
}

func assertAgentBackupSyncResult(t *testing.T, result dto.BackupPublicSyncResult, revision uint64, want string) {
	t.Helper()
	if result.Authority != agentBackupSyncTestAuthority || result.Generation != agentBackupSyncTestGeneration || result.TargetEpoch != agentBackupSyncTestTargetEpoch || result.AppliedRevision != revision || result.SnapshotDigest == "" || result.Result != want {
		t.Fatalf("sync result = %#v, want revision=%d result=%q", result, revision, want)
	}
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

type blockingBackupClient struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (c *blockingBackupClient) ListBuckets() ([]interface{}, error) {
	c.once.Do(func() { close(c.started) })
	<-c.release
	return nil, nil
}

func (c *blockingBackupClient) ListObjects(string) ([]string, error) { return nil, nil }
func (c *blockingBackupClient) Exist(string) (bool, error)           { return false, nil }
func (c *blockingBackupClient) Delete(string) (bool, error)          { return false, nil }
func (c *blockingBackupClient) Upload(string, string) (bool, error)  { return false, nil }
func (c *blockingBackupClient) Download(string, string) (bool, error) {
	return false, nil
}
func (c *blockingBackupClient) Size(string) (int64, error) { return 0, nil }
