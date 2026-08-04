package service

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/utils/cloud_storage"
	"github.com/1Panel-dev/1Panel/agent/utils/re"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const backupCleanupSensitiveMarker = "provider-secret-must-not-leak"

func TestDeleteRecordByNameRetainsMetadataWhenPublicLeaseUnavailable(t *testing.T) {
	db := setupBackupCleanupTestDB(t)
	account := model.BackupAccount{Name: "public-local", Type: constant.Local, IsPublic: true, Vars: "{}"}
	if err := db.Create(&account).Error; err != nil {
		t.Fatalf("seed public backup account: %v", err)
	}
	record := model.BackupRecord{
		Type:              "website",
		Name:              "example.test",
		DetailName:        "example",
		FileDir:           "website/example",
		FileName:          "backup.tar.gz",
		DownloadAccountID: account.ID,
	}
	if err := db.Create(&record).Error; err != nil {
		t.Fatalf("seed backup record: %v", err)
	}

	err := (&BackupRecordService{}).DeleteRecordByName(record.Type, record.Name, record.DetailName, true)
	if err == nil || !strings.Contains(err.Error(), "backup account is unavailable") {
		t.Fatalf("lease failure error = %v", err)
	}
	assertBackupCleanupRecordCount(t, db, 1)
}

func TestDeleteBackupRecordsWithFilesRequiresConfirmedRemoteDelete(t *testing.T) {
	tests := []struct {
		name       string
		client     backupCleanupClient
		wantRecord bool
		wantErr    bool
		wantExist  int
	}{
		{
			name:      "delete error and confirmed absent",
			client:    backupCleanupClient{deleteErr: errors.New(backupCleanupSensitiveMarker)},
			wantExist: 1,
		},
		{
			name:      "unconfirmed delete and confirmed absent",
			client:    backupCleanupClient{},
			wantExist: 1,
		},
		{
			name:       "delete error and object still exists",
			client:     backupCleanupClient{deleteErr: errors.New(backupCleanupSensitiveMarker), exist: true},
			wantRecord: true,
			wantErr:    true,
			wantExist:  1,
		},
		{
			name:       "delete error and existence check failed",
			client:     backupCleanupClient{deleteErr: errors.New(backupCleanupSensitiveMarker), existErr: errors.New(backupCleanupSensitiveMarker)},
			wantRecord: true,
			wantErr:    true,
			wantExist:  1,
		},
		{
			name:   "confirmed delete",
			client: backupCleanupClient{deleteOK: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupBackupCleanupTestDB(t)
			record := model.BackupRecord{FileDir: "archive", FileName: "record.tar.gz", DownloadAccountID: 17}
			if err := db.Create(&record).Error; err != nil {
				t.Fatalf("seed backup record: %v", err)
			}
			client := &tt.client
			account := &model.BackupAccount{BaseModel: model.BaseModel{ID: 17}, BackupPath: "backups"}
			err := deleteBackupRecordsWithFiles([]model.BackupRecord{record}, func(uint) (*model.BackupAccount, cloud_storage.CloudStorageClient, error) {
				return account, client, nil
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("cleanup error = %v, wantErr=%v", err, tt.wantErr)
			}
			if err != nil && strings.Contains(err.Error(), backupCleanupSensitiveMarker) {
				t.Fatalf("cleanup error leaked provider detail: %v", err)
			}
			wantCount := int64(0)
			if tt.wantRecord {
				wantCount = 1
			}
			assertBackupCleanupRecordCount(t, db, wantCount)
			if len(client.deletedPaths) != 1 || client.deletedPaths[0] != "backups/archive/record.tar.gz" {
				t.Fatalf("delete paths = %#v", client.deletedPaths)
			}
			if client.existCalls != tt.wantExist {
				t.Fatalf("exist calls = %d, want %d", client.existCalls, tt.wantExist)
			}
		})
	}
}

func TestDeleteSnapshotRetainsMetadataUntilRemoteDeleteSucceeds(t *testing.T) {
	tests := []struct {
		name      string
		helper    backupClientHelper
		wantRows  int64
		wantError bool
	}{
		{
			name:      "client unavailable",
			helper:    backupClientHelper{id: 23, isOk: false, message: backupCleanupSensitiveMarker},
			wantRows:  1,
			wantError: true,
		},
		{
			name: "remote delete failed",
			helper: backupClientHelper{
				id: 23, isOk: true, backupPath: "snapshots",
				client: &backupCleanupClient{deleteErr: errors.New(backupCleanupSensitiveMarker), exist: true},
			},
			wantRows:  1,
			wantError: true,
		},
		{
			name: "remote delete succeeded",
			helper: backupClientHelper{
				id: 23, isOk: true, backupPath: "snapshots",
				client: &backupCleanupClient{deleteOK: true},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupBackupCleanupTestDB(t)
			snapshot := model.Snapshot{Name: "snapshot-test", SourceAccountIDs: "23"}
			if err := db.Create(&snapshot).Error; err != nil {
				t.Fatalf("seed snapshot: %v", err)
			}
			record := model.BackupRecord{Type: "snapshot", FileName: snapshot.Name + ".tar.gz"}
			if err := db.Create(&record).Error; err != nil {
				t.Fatalf("seed snapshot backup record: %v", err)
			}

			err := deleteSnapshotWithClients(snapshot, true, map[string]backupClientHelper{"23": tt.helper})
			if (err != nil) != tt.wantError {
				t.Fatalf("snapshot cleanup error = %v, wantError=%v", err, tt.wantError)
			}
			if err != nil && strings.Contains(err.Error(), backupCleanupSensitiveMarker) {
				t.Fatalf("snapshot cleanup error leaked provider detail: %v", err)
			}
			assertBackupCleanupRecordCount(t, db, tt.wantRows)
			assertBackupCleanupSnapshotCount(t, db, tt.wantRows)
		})
	}
}

func TestRemoveExpiredBackupKeepsRecordUntilEveryRemoteDeleteSucceeds(t *testing.T) {
	db := setupBackupCleanupTestDB(t)
	cronjobID := uint(31)
	newest := model.BackupRecord{
		BaseModel:         model.BaseModel{CreatedAt: time.Now()},
		From:              "cronjob",
		CronjobID:         cronjobID,
		Type:              "directory",
		FileDir:           "archive",
		FileName:          "new.tar.gz",
		SourceAccountIDs:  "41,42",
		DownloadAccountID: 41,
	}
	expired := model.BackupRecord{
		BaseModel:         model.BaseModel{CreatedAt: time.Now().Add(-time.Hour)},
		From:              "cronjob",
		CronjobID:         cronjobID,
		Type:              "directory",
		FileDir:           "archive",
		FileName:          "old.tar.gz",
		SourceAccountIDs:  "41,42",
		DownloadAccountID: 41,
	}
	if err := db.Create(&newest).Error; err != nil {
		t.Fatalf("seed newest backup record: %v", err)
	}
	if err := db.Create(&expired).Error; err != nil {
		t.Fatalf("seed expired backup record: %v", err)
	}
	first := &backupCleanupClient{deleteOK: true}
	second := &backupCleanupClient{deleteErr: errors.New(backupCleanupSensitiveMarker), exist: true}
	accountMap := map[string]backupClientHelper{
		"41": {id: 41, isOk: true, backupPath: "primary", client: first},
		"42": {id: 42, isOk: true, backupPath: "secondary", client: second},
	}
	cronjob := model.Cronjob{
		BaseModel:        model.BaseModel{ID: cronjobID},
		Type:             "directory",
		SourceAccountIDs: "41,42",
		RetainCopies:     1,
	}

	err := (&CronjobService{}).removeExpiredBackup(cronjob, accountMap, model.BackupRecord{})
	if err == nil {
		t.Fatal("expected partial remote cleanup failure")
	}
	if strings.Contains(err.Error(), backupCleanupSensitiveMarker) {
		t.Fatalf("retention cleanup error leaked provider detail: %v", err)
	}
	assertBackupCleanupRecordCount(t, db, 2)

	first.deleteOK = false
	first.deleteErr = errors.New(backupCleanupSensitiveMarker)
	second.deleteErr = nil
	second.deleteOK = true
	if err := (&CronjobService{}).removeExpiredBackup(cronjob, accountMap, model.BackupRecord{}); err != nil {
		t.Fatalf("retry retention cleanup: %v", err)
	}
	if first.existCalls != 1 {
		t.Fatalf("already deleted object existence checks = %d, want 1", first.existCalls)
	}
	assertBackupCleanupRecordCount(t, db, 1)
	var remaining model.BackupRecord
	if err := db.First(&remaining).Error; err != nil {
		t.Fatalf("load retained backup record: %v", err)
	}
	if remaining.ID != newest.ID {
		t.Fatalf("remaining record ID = %d, want newest %d", remaining.ID, newest.ID)
	}
}

type backupCleanupClient struct {
	deleteOK     bool
	deleteErr    error
	exist        bool
	existErr     error
	existCalls   int
	deletedPaths []string
}

func (c *backupCleanupClient) ListBuckets() ([]interface{}, error)  { return nil, nil }
func (c *backupCleanupClient) ListObjects(string) ([]string, error) { return nil, nil }
func (c *backupCleanupClient) Exist(string) (bool, error) {
	c.existCalls++
	return c.exist, c.existErr
}
func (c *backupCleanupClient) Delete(target string) (bool, error) {
	c.deletedPaths = append(c.deletedPaths, target)
	return c.deleteOK, c.deleteErr
}
func (c *backupCleanupClient) Upload(string, string) (bool, error)   { return false, nil }
func (c *backupCleanupClient) Download(string, string) (bool, error) { return false, nil }
func (c *backupCleanupClient) Size(string) (int64, error)            { return 0, nil }

func setupBackupCleanupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	re.Init()
	oldDB := global.DB
	oldLocalBackupDir := global.Dir.LocalBackupDir
	dsn := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", dsn)), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open cleanup test database: %v", err)
	}
	if err := db.AutoMigrate(&model.BackupAccount{}, &model.BackupRecord{}, &model.Snapshot{}, &model.BackupPublicSyncState{}); err != nil {
		t.Fatalf("migrate cleanup test database: %v", err)
	}
	global.DB = db
	global.Dir.LocalBackupDir = t.TempDir()
	t.Cleanup(func() {
		global.DB = oldDB
		global.Dir.LocalBackupDir = oldLocalBackupDir
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func assertBackupCleanupRecordCount(t *testing.T, db *gorm.DB, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(&model.BackupRecord{}).Count(&count).Error; err != nil {
		t.Fatalf("count backup records: %v", err)
	}
	if count != want {
		t.Fatalf("backup record count = %d, want %d", count, want)
	}
}

func assertBackupCleanupSnapshotCount(t *testing.T, db *gorm.DB, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(&model.Snapshot{}).Count(&count).Error; err != nil {
		t.Fatalf("count snapshots: %v", err)
	}
	if count != want {
		t.Fatalf("snapshot count = %d, want %d", count, want)
	}
}
