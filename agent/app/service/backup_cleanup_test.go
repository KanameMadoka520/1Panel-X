package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/app/task"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/i18n"
	"github.com/1Panel-dev/1Panel/agent/utils/cloud_storage"
	"github.com/1Panel-dev/1Panel/agent/utils/re"
	"github.com/glebarez/sqlite"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const backupCleanupSensitiveMarker = "provider-secret-must-not-leak"

func TestUploadCleanupResetsDeletedTargetsBeforeRetry(t *testing.T) {
	i18n.Init()
	setupBackupCleanupLogger(t)
	first := &backupCleanupClient{uploadOK: true, deleteOK: true}
	second := &backupCleanupClient{uploadErr: errors.New("upload failed"), deleteOK: true}
	accountMap := map[string]backupClientHelper{
		"1": {id: 1, isOk: true, backupPath: "primary", client: first},
		"2": {id: 2, isOk: true, backupPath: "secondary", client: second},
	}
	taskItem := task.Task{Logger: logrus.New()}
	taskItem.Logger.SetOutput(io.Discard)

	err := uploadWithMapWithContext(context.Background(), taskItem, accountMap, "source.tar.gz", "archive.tar.gz", "1,2", 2, 0, true, false)
	if err == nil || task.IsNonRetryable(err) {
		t.Fatalf("first upload error = %v, want retryable failure", err)
	}
	if accountMap["1"].hasBackup || accountMap["2"].hasBackup {
		t.Fatalf("cleanup did not reset upload state: primary=%+v secondary=%+v", accountMap["1"], accountMap["2"])
	}
	if len(first.deletedPaths) != 1 || len(second.deletedPaths) != 0 {
		t.Fatalf("cleanup delete calls = (%d, %d), want (1, 0)", len(first.deletedPaths), len(second.deletedPaths))
	}

	second.uploadErr = nil
	second.uploadOK = true
	if err := uploadWithMapWithContext(context.Background(), taskItem, accountMap, "source.tar.gz", "archive.tar.gz", "1,2", 2, 0, true, false); err != nil {
		t.Fatalf("retry upload failed: %v", err)
	}
	if first.uploadCalls != 2 || second.uploadCalls != 2 {
		t.Fatalf("retry upload calls = (%d, %d), want (2, 2)", first.uploadCalls, second.uploadCalls)
	}
}

func TestUploadCleanupFailureStopsRetryWithoutLeakingProviderError(t *testing.T) {
	i18n.Init()
	setupBackupCleanupLogger(t)
	created := &backupCleanupClient{
		uploadOK:  true,
		deleteErr: errors.New(backupCleanupSensitiveMarker),
		exist:     true,
	}
	failed := &backupCleanupClient{uploadErr: errors.New(backupCleanupSensitiveMarker)}
	accountMap := map[string]backupClientHelper{
		"1": {id: 1, isOk: true, backupPath: "primary", client: created},
		"2": {id: 2, isOk: true, backupPath: "secondary", client: failed},
	}
	taskItem := task.Task{Logger: logrus.New()}
	taskItem.Logger.SetOutput(io.Discard)

	err := uploadWithMapWithContext(context.Background(), taskItem, accountMap, "source.tar.gz", "archive.tar.gz", "1,2", 2, 0, true, false)
	if err == nil || !task.IsNonRetryable(err) {
		t.Fatalf("upload cleanup error = %v, want non-retryable failure", err)
	}
	if strings.Contains(err.Error(), backupCleanupSensitiveMarker) {
		t.Fatalf("upload cleanup error leaked provider detail: %v", err)
	}
}

func TestUploadConflictDoesNotDeletePreExistingTarget(t *testing.T) {
	i18n.Init()
	setupBackupCleanupLogger(t)
	client := &backupCleanupClient{
		uploadErr: errors.New("target already exists"),
		exist:     true,
	}
	accountMap := map[string]backupClientHelper{
		"1": {id: 1, isOk: true, backupPath: "primary", client: client},
	}
	taskItem := task.Task{Logger: logrus.New()}
	taskItem.Logger.SetOutput(io.Discard)

	err := uploadWithMapWithContext(context.Background(), taskItem, accountMap, "source.tar.gz", "archive.tar.gz", "1", 1, 0, true, false)
	if err == nil || task.IsNonRetryable(err) {
		t.Fatalf("upload conflict error = %v, want retryable upload failure", err)
	}
	if len(client.deletedPaths) != 0 {
		t.Fatalf("cleanup deleted pre-existing target after failed upload: %#v", client.deletedPaths)
	}
	if !client.exist {
		t.Fatal("pre-existing target was unexpectedly removed")
	}
}

func TestUploadFailureKeepsSourceWhenCallerOwnsCleanup(t *testing.T) {
	i18n.Init()
	setupBackupCleanupLogger(t)
	client := &backupCleanupClient{uploadErr: errors.New("upload failed")}
	accountMap := map[string]backupClientHelper{
		"1": {id: 1, isOk: true, backupPath: "primary", client: client},
	}
	taskItem := task.Task{Logger: logrus.New()}
	taskItem.Logger.SetOutput(io.Discard)
	source := filepath.Join(t.TempDir(), "source.tar.gz")
	if err := os.WriteFile(source, []byte("backup"), 0o600); err != nil {
		t.Fatalf("write source artifact: %v", err)
	}

	err := uploadWithMapWithContext(context.Background(), taskItem, accountMap, source, "archive.tar.gz", "1", 1, 0, false, true)
	if err == nil {
		t.Fatal("upload failure was ignored")
	}
	if _, statErr := os.Stat(source); statErr != nil {
		t.Fatalf("retry source was removed after upload failure: %v", statErr)
	}
}

func TestCronjobUploadUsesOneProviderAttemptPerSubtaskAttempt(t *testing.T) {
	i18n.Init()
	setupBackupCleanupLogger(t)
	client := &backupCleanupClient{uploadErr: errors.New("upload failed")}
	accountMap := map[string]backupClientHelper{
		"1": {id: 1, isOk: true, backupPath: "primary", client: client},
	}
	taskItem := &task.Task{Logger: logrus.New()}
	taskItem.Logger.SetOutput(io.Discard)

	err := uploadCronjobArtifact(taskItem, accountMap, "source.tar.gz", "archive.tar.gz", "1", 1)
	if err == nil {
		t.Fatal("cronjob upload failure was ignored")
	}
	if client.uploadCalls != 1 {
		t.Fatalf("provider upload calls = %d, want exactly 1 per subtask attempt", client.uploadCalls)
	}
}

func TestCronjobMetadataFailureRollsBackUploadBeforeRetry(t *testing.T) {
	i18n.Init()
	setupBackupCleanupLogger(t)
	client := &backupCleanupClient{uploadOK: true, deleteOK: true, exist: false}
	accountMap := map[string]backupClientHelper{
		"1": {id: 1, isOk: true, hasBackup: true, backupPath: "primary", client: client},
	}
	metadataErr := errors.New("metadata insert failed")

	err := rollbackCronjobUploadAfterMetadataFailure(accountMap, "source.tar.gz", "archive.tar.gz", metadataErr)
	if !errors.Is(err, metadataErr) || task.IsNonRetryable(err) {
		t.Fatalf("metadata rollback error = %v, want retryable metadata error", err)
	}
	if len(client.deletedPaths) != 1 || client.deletedPaths[0] != "primary/archive.tar.gz" {
		t.Fatalf("metadata rollback deleted paths = %#v", client.deletedPaths)
	}
	if accountMap["1"].hasBackup {
		t.Fatal("metadata rollback did not reset the uploaded state before retry")
	}
}

func TestCronjobMetadataFailureCleanupIsNonRetryableAndRedacted(t *testing.T) {
	i18n.Init()
	setupBackupCleanupLogger(t)
	client := &backupCleanupClient{
		uploadOK:  true,
		deleteErr: errors.New(backupCleanupSensitiveMarker),
		exist:     true,
	}
	accountMap := map[string]backupClientHelper{
		"1": {id: 1, isOk: true, hasBackup: true, backupPath: "primary", client: client},
	}

	err := rollbackCronjobUploadAfterMetadataFailure(accountMap, "source.tar.gz", "archive.tar.gz", errors.New("metadata insert failed"))
	if err == nil || !task.IsNonRetryable(err) {
		t.Fatalf("metadata cleanup error = %v, want non-retryable failure", err)
	}
	if strings.Contains(err.Error(), backupCleanupSensitiveMarker) {
		t.Fatalf("metadata cleanup error leaked provider detail: %v", err)
	}
	if !accountMap["1"].hasBackup {
		t.Fatal("unconfirmed metadata cleanup cleared the uploaded state")
	}
}

func TestSnapshotMetadataFailureRollsBackEveryCreatedArtifact(t *testing.T) {
	i18n.Init()
	setupBackupCleanupLogger(t)
	primary := &backupCleanupClient{uploadOK: true, deleteOK: true, exist: false}
	secondary := &backupCleanupClient{uploadOK: true, deleteOK: true, exist: false}
	accountMap := map[string]backupClientHelper{
		"1": {id: 1, isOk: true, backupPath: "primary", client: primary},
		"2": {id: 2, isOk: true, backupPath: "secondary", client: secondary},
	}
	markBackupArtifactsCreated(accountMap)

	metadataErr := errors.New("snapshot metadata insert failed")
	err := rollbackCronjobUploadAfterMetadataFailure(accountMap, "", "system_snapshot/snapshot.tar.gz", metadataErr)
	if !errors.Is(err, metadataErr) || task.IsNonRetryable(err) {
		t.Fatalf("snapshot metadata rollback error = %v, want retryable metadata error", err)
	}
	if len(primary.deletedPaths) != 1 || primary.deletedPaths[0] != "primary/system_snapshot/snapshot.tar.gz" {
		t.Fatalf("primary snapshot cleanup paths = %#v", primary.deletedPaths)
	}
	if len(secondary.deletedPaths) != 1 || secondary.deletedPaths[0] != "secondary/system_snapshot/snapshot.tar.gz" {
		t.Fatalf("secondary snapshot cleanup paths = %#v", secondary.deletedPaths)
	}
	if accountMap["1"].hasBackup || accountMap["2"].hasBackup {
		t.Fatalf("snapshot cleanup did not reset ownership: primary=%+v secondary=%+v", accountMap["1"], accountMap["2"])
	}
}

func TestSnapshotUploadRollbackPreventsPrimaryDuplicateOnRetry(t *testing.T) {
	i18n.Init()
	setupBackupCleanupLogger(t)
	primary := &snapshotRetryClient{objects: make(map[string]int)}
	optional := &snapshotRetryClient{failUploads: 1, objects: make(map[string]int)}
	taskItem := task.Task{Logger: logrus.New()}
	taskItem.Logger.SetOutput(io.Discard)
	source := filepath.Join(t.TempDir(), "snapshot.tar.gz")
	if err := os.WriteFile(source, []byte("snapshot"), 0o600); err != nil {
		t.Fatalf("write snapshot source: %v", err)
	}
	newMaps := func() (map[string]backupClientHelper, map[string]backupClientHelper) {
		return map[string]backupClientHelper{
				"1": {id: 1, isOk: true, backupPath: "primary", client: primary},
			}, map[string]backupClientHelper{
				"2": {id: 2, isOk: true, backupPath: "optional", client: optional},
			}
	}

	primaryMap, optionalMap := newMaps()
	err := uploadSnapshotArtifacts(context.Background(), taskItem, primaryMap, optionalMap, source, "system_snapshot/snapshot.tar.gz", "1", []string{"2"}, 1, 0)
	if err == nil || task.IsNonRetryable(err) {
		t.Fatalf("first snapshot upload error = %v, want retryable failure", err)
	}
	if primary.totalObjects() != 0 {
		t.Fatalf("primary snapshot object survived rollback: %v", primary.objects)
	}
	if _, statErr := os.Stat(source); statErr != nil {
		t.Fatalf("snapshot source was removed before retry: %v", statErr)
	}

	primaryMap, optionalMap = newMaps()
	if err := uploadSnapshotArtifacts(context.Background(), taskItem, primaryMap, optionalMap, source, "system_snapshot/snapshot.tar.gz", "1", []string{"2"}, 1, 0); err != nil {
		t.Fatalf("snapshot retry failed: %v", err)
	}
	if primary.totalObjects() != 1 || optional.totalObjects() != 1 {
		t.Fatalf("snapshot retry objects = primary:%v optional:%v, want exactly one each", primary.objects, optional.objects)
	}
	if _, statErr := os.Stat(source); !os.IsNotExist(statErr) {
		t.Fatalf("successful snapshot source cleanup error = %v, want source removed", statErr)
	}
}

func TestMarkBackupFailedAfterTaskConstructionFailureDoesNotDeleteRemoteArtifact(t *testing.T) {
	db := setupBackupCleanupTestDB(t)
	setupBackupCleanupLogger(t)
	remoteRoot := t.TempDir()
	remoteArtifact := filepath.Join(remoteRoot, "app", "demo", "backup.tar.gz")
	if err := os.MkdirAll(filepath.Dir(remoteArtifact), 0o700); err != nil {
		t.Fatalf("create remote artifact directory: %v", err)
	}
	if err := os.WriteFile(remoteArtifact, []byte("existing backup"), 0o600); err != nil {
		t.Fatalf("write remote artifact: %v", err)
	}
	account := model.BackupAccount{Name: "remote-local", Type: constant.Local, BackupPath: remoteRoot, Vars: "{}"}
	if err := db.Create(&account).Error; err != nil {
		t.Fatalf("seed remote backup account: %v", err)
	}
	record := model.BackupRecord{
		SourceAccountIDs: fmt.Sprintf("%d", account.ID),
		FileDir:          "app/demo",
		FileName:         "backup.tar.gz",
		Status:           constant.StatusWaiting,
	}
	if err := db.Create(&record).Error; err != nil {
		t.Fatalf("seed failed backup record: %v", err)
	}

	markBackupFailed(record.ID, errors.New("task construction failed"))

	if _, err := os.Stat(remoteArtifact); err != nil {
		t.Fatalf("task construction failure deleted an unowned remote artifact: %v", err)
	}
	var updated model.BackupRecord
	if err := db.First(&updated, record.ID).Error; err != nil {
		t.Fatalf("load failed backup record: %v", err)
	}
	if updated.Status != constant.StatusFailed || updated.Message != "task construction failed" {
		t.Fatalf("failed backup record = status:%q message:%q", updated.Status, updated.Message)
	}
}

func TestMarkBackupFailedRejectsLocalPathTraversal(t *testing.T) {
	db := setupBackupCleanupTestDB(t)
	setupBackupCleanupLogger(t)
	outsideDir := t.TempDir()
	outsideArtifact := filepath.Join(outsideDir, "outside.tar.gz")
	if err := os.WriteFile(outsideArtifact, []byte("outside"), 0o600); err != nil {
		t.Fatalf("write outside artifact: %v", err)
	}
	relativeOutside, err := filepath.Rel(global.Dir.LocalBackupDir, outsideArtifact)
	if err != nil || !strings.HasPrefix(relativeOutside, ".."+string(filepath.Separator)) {
		t.Fatalf("construct traversal path: relative=%q err=%v", relativeOutside, err)
	}
	record := model.BackupRecord{
		FileDir:  filepath.Dir(relativeOutside),
		FileName: filepath.Base(relativeOutside),
		Status:   constant.StatusWaiting,
	}
	if err := db.Create(&record).Error; err != nil {
		t.Fatalf("seed traversal backup record: %v", err)
	}

	markBackupFailed(record.ID, errors.New("backup failed"))

	if _, err := os.Stat(outsideArtifact); err != nil {
		t.Fatalf("path traversal removed an artifact outside the backup root: %v", err)
	}
}

func TestMarkBackupFailedRemovesContainedLocalArtifact(t *testing.T) {
	db := setupBackupCleanupTestDB(t)
	setupBackupCleanupLogger(t)
	localArtifact := filepath.Join(global.Dir.LocalBackupDir, "app", "demo", "failed.tar.gz")
	if err := os.MkdirAll(filepath.Dir(localArtifact), 0o700); err != nil {
		t.Fatalf("create local artifact directory: %v", err)
	}
	if err := os.WriteFile(localArtifact, []byte("failed backup"), 0o600); err != nil {
		t.Fatalf("write local artifact: %v", err)
	}
	record := model.BackupRecord{FileDir: "app/demo", FileName: "failed.tar.gz", Status: constant.StatusWaiting}
	if err := db.Create(&record).Error; err != nil {
		t.Fatalf("seed local backup record: %v", err)
	}

	markBackupFailed(record.ID, errors.New("backup failed"))

	if _, err := os.Stat(localArtifact); !os.IsNotExist(err) {
		t.Fatalf("contained failed artifact cleanup error = %v, want removed", err)
	}
}

func TestFailedBackupLocalArtifactPathRejectsUnsafeTargets(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name     string
		fileDir  string
		fileName string
	}{
		{name: "absolute directory", fileDir: filepath.Join(string(filepath.Separator), "outside"), fileName: "backup.tar.gz"},
		{name: "absolute file", fileName: filepath.Join(string(filepath.Separator), "outside", "backup.tar.gz")},
		{name: "parent escape", fileDir: "..", fileName: "backup.tar.gz"},
		{name: "backup root", fileName: "."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if target, err := failedBackupLocalArtifactPath(root, tt.fileDir, tt.fileName); err == nil {
				t.Fatalf("unsafe path resolved to %q", target)
			}
		})
	}
}

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
	uploadOK     bool
	uploadErr    error
	uploadCalls  int
	deleteOK     bool
	deleteErr    error
	exist        bool
	existErr     error
	existCalls   int
	deletedPaths []string
}

type snapshotRetryClient struct {
	failUploads int
	objects     map[string]int
}

func (c *snapshotRetryClient) ListBuckets() ([]interface{}, error)  { return nil, nil }
func (c *snapshotRetryClient) ListObjects(string) ([]string, error) { return nil, nil }
func (c *snapshotRetryClient) Exist(target string) (bool, error)    { return c.objects[target] > 0, nil }
func (c *snapshotRetryClient) Delete(target string) (bool, error) {
	delete(c.objects, target)
	return true, nil
}
func (c *snapshotRetryClient) Upload(_ context.Context, _, target string) (bool, error) {
	if c.failUploads > 0 {
		c.failUploads--
		return false, errors.New("upload failed")
	}
	c.objects[target]++
	return true, nil
}
func (c *snapshotRetryClient) Download(string, string) (bool, error) { return false, nil }
func (c *snapshotRetryClient) Size(string) (int64, error)            { return 0, nil }
func (c *snapshotRetryClient) totalObjects() int {
	total := 0
	for _, count := range c.objects {
		total += count
	}
	return total
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
func (c *backupCleanupClient) Upload(context.Context, string, string) (bool, error) {
	c.uploadCalls++
	return c.uploadOK, c.uploadErr
}
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

func setupBackupCleanupLogger(t *testing.T) {
	t.Helper()
	oldLogger := global.LOG
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	global.LOG = logger
	t.Cleanup(func() {
		global.LOG = oldLogger
	})
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
