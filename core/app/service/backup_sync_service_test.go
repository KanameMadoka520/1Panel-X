package service

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/constant"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/utils/backupsync"
	"github.com/1Panel-dev/1Panel/core/utils/xpack"
	xpackhelper "github.com/1Panel-dev/1Panel/core/utils/xpack/helper"
	"github.com/1Panel-dev/1Panel/core/utils/xpack/providers"
	"github.com/glebarez/sqlite"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type failingBackupSyncProvider struct {
	providers.MultiNodeProvider
	calls int
}

func (p *failingBackupSyncProvider) Sync(string) error {
	p.calls++
	return errors.New("synthetic transport failure")
}

func TestBackupCreateCommitsOutboxWhenDeliveryFails(t *testing.T) {
	db := setupBackupSyncServiceTestDB(t)
	baselineRevision, err := backupsync.CurrentRevisionTx(db)
	if err != nil {
		t.Fatalf("load baseline revision: %v", err)
	}
	oldProvider := xpack.MultiNodeProvider
	provider := &failingBackupSyncProvider{}
	xpack.MultiNodeProvider = provider
	t.Cleanup(func() { xpack.MultiNodeProvider = oldProvider })

	service := &BackupService{}
	if err := service.Create(testPublicBackupOperate("durable-public")); err != nil {
		t.Fatalf("create returned a transport failure after commit: %v", err)
	}

	var account model.BackupAccount
	if err := db.Where("name = ?", "durable-public").First(&account).Error; err != nil {
		t.Fatalf("load created account: %v", err)
	}
	var event model.BackupSyncOutbox
	if err := db.Where("account_name = ?", account.Name).First(&event).Error; err != nil {
		t.Fatalf("load persisted outbox event: %v", err)
	}
	if event.Generation == "" || event.Revision != baselineRevision+1 || event.Operation != model.BackupSyncOperationCreate {
		t.Fatalf("unexpected outbox event: %#v", event)
	}
	status, err := backupsync.GetStatus(account.Name)
	if err != nil {
		t.Fatalf("load synchronization status: %v", err)
	}
	if status.Status != model.BackupSyncStatusPending || status.Pending != 1 || status.Total != 1 {
		t.Fatalf("unexpected pending status: %#v", status)
	}
	reconcilePublicBackupSync()
	if provider.calls != 1 {
		t.Fatalf("queued reconciliation provider calls = %d, want 1", provider.calls)
	}
	if err := db.Where("account_name = ?", account.Name).First(&event).Error; err != nil {
		t.Fatalf("reload durable outbox event after delivery failure: %v", err)
	}
	if event.Status != model.BackupSyncOutboxStatusPending {
		t.Fatalf("delivery failure changed durable outbox status to %q", event.Status)
	}
}

func TestBackupCreateRollsBackWhenOutboxCannotPersist(t *testing.T) {
	db := setupBackupSyncServiceTestDB(t)
	baselineRevision, err := backupsync.CurrentRevisionTx(db)
	if err != nil {
		t.Fatalf("load baseline revision: %v", err)
	}
	if err := db.Migrator().DropTable(&model.BackupSyncOutbox{}); err != nil {
		t.Fatalf("drop outbox table: %v", err)
	}

	err = (&BackupService{}).Create(testPublicBackupOperate("atomic-public"))
	if err == nil {
		t.Fatal("create succeeded without a writable outbox")
	}
	var count int64
	if err := db.Model(&model.BackupAccount{}).Where("name = ?", "atomic-public").Count(&count).Error; err != nil {
		t.Fatalf("count rolled-back account: %v", err)
	}
	if count != 0 {
		t.Fatalf("account committed without its outbox event: count=%d", count)
	}
	revision, err := backupsync.CurrentRevisionTx(db)
	if err != nil {
		t.Fatalf("load sequence after rollback: %v", err)
	}
	if revision != baselineRevision {
		t.Fatalf("revision after rollback = %d, want baseline %d", revision, baselineRevision)
	}
}

func TestBackupDeleteWithOfflineNodePersistsPendingTombstone(t *testing.T) {
	db := setupBackupSyncServiceTestDB(t)
	oldProvider := xpack.MultiNodeProvider
	xpack.MultiNodeProvider = xpackhelper.NewIMultiNodeProvider()
	t.Cleanup(func() {
		xpack.MultiNodeProvider = oldProvider
	})

	account := model.BackupAccount{
		Name:     "offline-delete",
		Type:     constant.S3,
		IsPublic: true,
		Vars:     "{}",
	}
	if err := db.Create(&account).Error; err != nil {
		t.Fatalf("create public backup account: %v", err)
	}
	offlineNode := model.Node{
		Name:     "offline-delete-node",
		Status:   constant.NodeStatusOffline,
		Enrolled: true,
	}
	if err := db.Create(&offlineNode).Error; err != nil {
		t.Fatalf("create offline node: %v", err)
	}

	service := &BackupService{localBackupUsageChecker: func(string) error { return nil }}
	if err := service.Delete(account.Name); err != nil {
		t.Fatalf("delete public backup account with offline node: %v", err)
	}
	var accountCount int64
	if err := db.Model(&model.BackupAccount{}).Where("id = ?", account.ID).Count(&accountCount).Error; err != nil {
		t.Fatalf("count deleted backup account: %v", err)
	}
	if accountCount != 0 {
		t.Fatalf("deleted backup account count = %d, want 0", accountCount)
	}

	var event model.BackupSyncOutbox
	if err := db.Where("account_name = ? AND operation = ?", account.Name, model.BackupSyncOperationDelete).First(&event).Error; err != nil {
		t.Fatalf("load delete outbox event: %v", err)
	}
	if event.Status != model.BackupSyncOutboxStatusPending {
		t.Fatalf("delete outbox status = %q, want pending", event.Status)
	}
	var tombstone model.BackupSyncTombstone
	if err := db.Where("account_name = ?", account.Name).First(&tombstone).Error; err != nil {
		t.Fatalf("load delete tombstone: %v", err)
	}
	if !tombstone.Active || tombstone.Revision != event.Revision || tombstone.Generation != event.Generation {
		t.Fatalf("unexpected delete tombstone: %#v, event=%#v", tombstone, event)
	}
	var target model.BackupSyncTarget
	if err := db.Where("target_key = ?", backupsync.NodeTargetKey(offlineNode.ID)).First(&target).Error; err != nil {
		t.Fatalf("load offline node synchronization target: %v", err)
	}
	if target.Status != model.BackupSyncTargetStatusPending || target.DesiredRevision != event.Revision || target.AppliedRevision >= event.Revision {
		t.Fatalf("unexpected offline node synchronization target: %#v", target)
	}
}

func testPublicBackupOperate(name string) dto.BackupOperate {
	return dto.BackupOperate{
		Name:       name,
		Type:       constant.S3,
		IsPublic:   true,
		AccessKey:  base64.StdEncoding.EncodeToString([]byte("synthetic-access-key")),
		Credential: base64.StdEncoding.EncodeToString([]byte("synthetic-credential")),
		BackupPath: "/",
		Vars:       "{}",
	}
}

func setupBackupSyncServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := global.DB
	oldKey := global.CONF.Base.EncryptKey
	oldLog := global.LOG
	dsn := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", dsn)), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open backup sync service test database: %v", err)
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
		t.Fatalf("migrate backup sync service test database: %v", err)
	}
	if err := backupsync.InitializeTx(db); err != nil {
		t.Fatalf("initialize backup sync state: %v", err)
	}
	log := logrus.New()
	log.SetOutput(io.Discard)
	global.DB = db
	global.LOG = log
	global.CONF.Base.EncryptKey = "1234567890abcdef"
	t.Cleanup(func() {
		global.DB = oldDB
		global.CONF.Base.EncryptKey = oldKey
		global.LOG = oldLog
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}
