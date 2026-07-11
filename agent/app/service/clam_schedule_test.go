package service

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/app/repo"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/glebarez/sqlite"
	"github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func setupClamScheduleTest(t *testing.T) {
	t.Helper()

	oldDB := global.DB
	oldAlertDB := global.AlertDB
	oldCron := global.Cron
	oldLog := global.LOG

	dbName := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	mainDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s-main?mode=memory&cache=shared", dbName)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open main database: %v", err)
	}
	if err := mainDB.AutoMigrate(&model.Clam{}, &model.ClamRecord{}); err != nil {
		t.Fatalf("migrate main database: %v", err)
	}

	alertDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s-alert?mode=memory&cache=shared", dbName)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open alert database: %v", err)
	}
	if err := alertDB.AutoMigrate(&model.Alert{}); err != nil {
		t.Fatalf("migrate alert database: %v", err)
	}

	logger := logrus.New()
	logger.SetOutput(io.Discard)
	global.DB = mainDB
	global.AlertDB = alertDB
	global.Cron = cron.New()
	global.LOG = logger
	NewIClamService()

	t.Cleanup(func() {
		global.Cron.Stop()
		if sqlDB, err := mainDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
		if sqlDB, err := alertDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
		global.DB = oldDB
		global.AlertDB = oldAlertDB
		global.Cron = oldCron
		global.LOG = oldLog
	})
}

func closeClamAlertDB(t *testing.T) {
	t.Helper()
	sqlDB, err := global.AlertDB.DB()
	if err != nil {
		t.Fatalf("load alert database handle: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close alert database: %v", err)
	}
	if err := global.AlertDB.Exec("SELECT 1").Error; err == nil {
		t.Fatal("closed alert database still accepts queries")
	}
}

func TestClamScheduleLifecycle(t *testing.T) {
	setupClamScheduleTest(t)
	service := NewIClamService()
	scanDir := t.TempDir()

	if err := service.Create(dto.ClamCreate{
		Name:             "scheduled-scan",
		Path:             scanDir,
		InfectedStrategy: "none",
		Spec:             "0 1 * * *",
		Timeout:          60,
	}, "tester"); err != nil {
		t.Fatalf("create scheduled scan: %v", err)
	}

	item, err := clamRepo.Get(repo.WithByName("scheduled-scan"))
	if err != nil {
		t.Fatalf("load created scan: %v", err)
	}
	if item.Status != constant.StatusEnable || item.EntryID == 0 {
		t.Fatalf("unexpected created schedule state: status=%q entry_id=%d", item.Status, item.EntryID)
	}
	if got := len(global.Cron.Entries()); got != 1 {
		t.Fatalf("expected one cron entry after create, got %d", got)
	}
	createdEntryID := item.EntryID

	if err := service.Update(dto.ClamUpdate{
		ID:               item.ID,
		Name:             item.Name,
		Path:             item.Path,
		InfectedStrategy: "none",
		Spec:             "30 2 * * *",
		Timeout:          120,
	}, "tester"); err != nil {
		t.Fatalf("update scheduled scan: %v", err)
	}
	item, err = clamRepo.Get(repo.WithByID(item.ID))
	if err != nil {
		t.Fatalf("load updated scan: %v", err)
	}
	if item.EntryID == 0 || item.EntryID == createdEntryID {
		t.Fatalf("expected a replacement entry id, old=%d new=%d", createdEntryID, item.EntryID)
	}
	if got := len(global.Cron.Entries()); got != 1 {
		t.Fatalf("expected one cron entry after update, got %d", got)
	}

	if err := service.UpdateStatus(item.ID, constant.StatusDisable); err != nil {
		t.Fatalf("disable scheduled scan: %v", err)
	}
	item, _ = clamRepo.Get(repo.WithByID(item.ID))
	if item.Status != constant.StatusDisable || item.EntryID != 0 || len(global.Cron.Entries()) != 0 {
		t.Fatalf("unexpected disabled schedule state: status=%q entry_id=%d entries=%d", item.Status, item.EntryID, len(global.Cron.Entries()))
	}

	if err := service.UpdateStatus(item.ID, constant.StatusEnable); err != nil {
		t.Fatalf("enable scheduled scan: %v", err)
	}
	item, _ = clamRepo.Get(repo.WithByID(item.ID))
	if item.Status != constant.StatusEnable || item.EntryID == 0 || len(global.Cron.Entries()) != 1 {
		t.Fatalf("unexpected enabled schedule state: status=%q entry_id=%d entries=%d", item.Status, item.EntryID, len(global.Cron.Entries()))
	}

	if err := service.Delete(dto.ClamDelete{Ids: []uint{item.ID}}); err != nil {
		t.Fatalf("delete scheduled scan: %v", err)
	}
	if got := len(global.Cron.Entries()); got != 0 {
		t.Fatalf("expected no cron entries after delete, got %d", got)
	}
	if deleted, _ := clamRepo.Get(repo.WithByID(item.ID)); deleted.ID != 0 {
		t.Fatalf("expected scheduled scan to be deleted, got id %d", deleted.ID)
	}
}

func TestClamCreateRollsBackRuleAndScheduleWhenAlertCreateFails(t *testing.T) {
	setupClamScheduleTest(t)
	service := NewIClamService()
	closeClamAlertDB(t)

	err := service.Create(dto.ClamCreate{
		Name:             "alert-create-failure",
		Path:             t.TempDir(),
		InfectedStrategy: "none",
		Spec:             "0 1 * * *",
		Timeout:          60,
		AlertCount:       1,
		AlertTitle:       "Clam alert",
		AlertMethod:      constant.Email,
	}, "tester")
	if err == nil {
		t.Fatal("expected alert create failure")
	}
	if item, _ := clamRepo.Get(repo.WithByName("alert-create-failure")); item.ID != 0 {
		t.Fatalf("alert failure left clam rule %d behind", item.ID)
	}
	if got := len(global.Cron.Entries()); got != 0 {
		t.Fatalf("alert failure left %d cron entries behind", got)
	}
}

func TestClamUpdateRollsBackRuleAndScheduleWhenAlertUpdateFails(t *testing.T) {
	setupClamScheduleTest(t)
	service := NewIClamService()
	if err := service.Create(dto.ClamCreate{
		Name:             "alert-update-failure",
		Path:             t.TempDir(),
		InfectedStrategy: "none",
		Spec:             "0 2 * * *",
		Timeout:          60,
		Description:      "original",
	}, "tester"); err != nil {
		t.Fatalf("create clam rule fixture: %v", err)
	}
	original, _ := clamRepo.Get(repo.WithByName("alert-update-failure"))
	changedScanPath := t.TempDir()
	changedInfectedDir := t.TempDir()
	closeClamAlertDB(t)

	err := service.Update(dto.ClamUpdate{
		ID:               original.ID,
		Name:             "alert-update-failure-renamed",
		Path:             changedScanPath,
		InfectedStrategy: "move",
		InfectedDir:      changedInfectedDir,
		Spec:             "30 3 * * *",
		Timeout:          120,
		Description:      "changed",
		AlertCount:       1,
		AlertTitle:       "Clam alert",
		AlertMethod:      constant.Email,
	}, "tester")
	if err == nil {
		t.Fatal("expected alert update failure")
	}
	persisted, _ := clamRepo.Get(repo.WithByID(original.ID))
	if persisted.Name != original.Name || persisted.Path != original.Path ||
		persisted.InfectedStrategy != original.InfectedStrategy || persisted.InfectedDir != original.InfectedDir ||
		persisted.Spec != original.Spec || persisted.Timeout != original.Timeout ||
		persisted.Description != original.Description || persisted.Status != original.Status || persisted.EntryID != original.EntryID {
		t.Fatalf("alert failure did not restore original rule: original=%+v persisted=%+v", original, persisted)
	}
	entries := global.Cron.Entries()
	if len(entries) != 1 || int(entries[0].ID) != original.EntryID {
		t.Fatalf("alert failure did not preserve original schedule: original=%d entries=%+v", original.EntryID, entries)
	}
}

func TestClamDeleteReturnsRetryableErrorWhenAlertCleanupFails(t *testing.T) {
	setupClamScheduleTest(t)
	service := NewIClamService()
	if err := service.Create(dto.ClamCreate{
		Name:             "alert-delete-failure",
		Path:             t.TempDir(),
		InfectedStrategy: "none",
		Spec:             "0 4 * * *",
		Timeout:          60,
	}, "tester"); err != nil {
		t.Fatalf("create clam rule fixture: %v", err)
	}
	item, _ := clamRepo.Get(repo.WithByName("alert-delete-failure"))
	const callbackName = "test:fail-clam-alert-delete"
	if err := global.AlertDB.Callback().Delete().Before("gorm:delete").Register(callbackName, func(db *gorm.DB) {
		db.AddError(errors.New("simulated alert cleanup failure"))
	}); err != nil {
		t.Fatalf("register alert delete failure callback: %v", err)
	}

	if err := service.Delete(dto.ClamDelete{Ids: []uint{item.ID}}); err == nil {
		t.Fatal("expected alert cleanup failure after primary deletion")
	}
	if persisted, _ := clamRepo.Get(repo.WithByID(item.ID)); persisted.ID != 0 {
		t.Fatalf("clam rule %d remained after delete", persisted.ID)
	}
	if got := len(global.Cron.Entries()); got != 0 {
		t.Fatalf("delete left %d cron entries behind", got)
	}
	global.AlertDB.Callback().Delete().Remove(callbackName)
	if err := service.Delete(dto.ClamDelete{Ids: []uint{item.ID}}); err != nil {
		t.Fatalf("retry should clean an orphan alert even after the clam rule is gone: %v", err)
	}
}

func TestClamDeleteRejectsLegacyFilesystemRootQuarantine(t *testing.T) {
	setupClamScheduleTest(t)
	service := NewIClamService()
	volume := filepath.VolumeName(t.TempDir())
	filesystemRoot := volume + string(filepath.Separator)
	if volume == "" {
		filesystemRoot = string(filepath.Separator)
	}
	legacy := model.Clam{
		Name:             "legacy-root-quarantine",
		Path:             t.TempDir(),
		InfectedStrategy: "move",
		InfectedDir:      filesystemRoot,
		Timeout:          60,
	}
	if err := global.DB.Create(&legacy).Error; err != nil {
		t.Fatalf("create legacy clam rule: %v", err)
	}
	entryID, err := global.Cron.AddFunc("0 6 * * *", func() {})
	if err != nil {
		t.Fatalf("register legacy clam schedule: %v", err)
	}
	legacy.EntryID = int(entryID)
	if err := clamRepo.Update(legacy.ID, map[string]interface{}{"entry_id": legacy.EntryID}); err != nil {
		t.Fatalf("persist legacy clam schedule: %v", err)
	}

	err = service.Delete(dto.ClamDelete{Ids: []uint{legacy.ID}, RemoveInfected: true})
	if err == nil || !strings.Contains(err.Error(), "filesystem root") {
		t.Fatalf("expected legacy filesystem-root quarantine to be rejected, got %v", err)
	}
	if persisted, _ := clamRepo.Get(repo.WithByID(legacy.ID)); persisted.ID == 0 {
		t.Fatal("legacy clam rule was deleted despite unsafe quarantine base")
	}
	entries := global.Cron.Entries()
	if len(entries) != 1 || int(entries[0].ID) != legacy.EntryID {
		t.Fatalf("legacy schedule changed despite unsafe quarantine base: entry=%d entries=%+v", legacy.EntryID, entries)
	}
}

func TestClamScheduleRejectsInvalidExpressions(t *testing.T) {
	setupClamScheduleTest(t)
	service := NewIClamService()
	scanDir := t.TempDir()

	err := service.Create(dto.ClamCreate{
		Name:             "invalid-scan",
		Path:             scanDir,
		InfectedStrategy: "none",
		Spec:             "not-a-cron-expression",
		Timeout:          60,
	}, "tester")
	if err == nil || !strings.Contains(err.Error(), "invalid clam schedule") {
		t.Fatalf("expected explicit invalid schedule error, got %v", err)
	}
	var count int64
	if err := global.DB.Model(&model.Clam{}).Count(&count).Error; err != nil {
		t.Fatalf("count clam rules: %v", err)
	}
	if count != 0 || len(global.Cron.Entries()) != 0 {
		t.Fatalf("invalid create left state behind: rows=%d entries=%d", count, len(global.Cron.Entries()))
	}

	if err := service.Create(dto.ClamCreate{
		Name:             "valid-scan",
		Path:             scanDir,
		InfectedStrategy: "none",
		Spec:             "0 3 * * *",
		Timeout:          60,
	}, "tester"); err != nil {
		t.Fatalf("create valid scan: %v", err)
	}
	item, _ := clamRepo.Get(repo.WithByName("valid-scan"))
	originalEntryID := item.EntryID

	err = service.Update(dto.ClamUpdate{
		ID:               item.ID,
		Name:             item.Name,
		Path:             item.Path,
		InfectedStrategy: "none",
		Spec:             "still-not-a-cron-expression",
		Timeout:          60,
	}, "tester")
	if err == nil || !strings.Contains(err.Error(), "invalid clam schedule") {
		t.Fatalf("expected explicit invalid update error, got %v", err)
	}
	item, _ = clamRepo.Get(repo.WithByID(item.ID))
	if item.Spec != "0 3 * * *" || item.EntryID != originalEntryID || len(global.Cron.Entries()) != 1 {
		t.Fatalf("invalid update changed existing schedule: spec=%q entry_id=%d entries=%d", item.Spec, item.EntryID, len(global.Cron.Entries()))
	}
}

func TestRestoreClamSchedules(t *testing.T) {
	setupClamScheduleTest(t)
	scanDir := t.TempDir()

	valid := model.Clam{
		Name:             "restore-valid",
		Path:             scanDir,
		InfectedStrategy: "none",
		Spec:             "15 4 * * *",
		Status:           constant.StatusEnable,
		EntryID:          999,
		Timeout:          60,
	}
	invalid := model.Clam{
		Name:             "restore-invalid",
		Path:             scanDir,
		InfectedStrategy: "none",
		Spec:             "invalid",
		Status:           constant.StatusEnable,
		EntryID:          1000,
		Timeout:          60,
	}
	if err := global.DB.Create(&valid).Error; err != nil {
		t.Fatalf("create valid persisted schedule: %v", err)
	}
	if err := global.DB.Create(&invalid).Error; err != nil {
		t.Fatalf("create invalid persisted schedule: %v", err)
	}

	err := RestoreClamSchedules()
	if err == nil || !strings.Contains(err.Error(), "restore clam schedule") {
		t.Fatalf("expected restore to report invalid persisted schedule, got %v", err)
	}

	valid, _ = clamRepo.Get(repo.WithByID(valid.ID))
	if valid.Status != constant.StatusEnable || valid.EntryID == 0 || valid.EntryID == 999 {
		t.Fatalf("valid schedule was not restored: status=%q entry_id=%d", valid.Status, valid.EntryID)
	}
	invalid, _ = clamRepo.Get(repo.WithByID(invalid.ID))
	if invalid.Status != constant.StatusDisable || invalid.EntryID != 0 {
		t.Fatalf("invalid schedule was not disabled: status=%q entry_id=%d", invalid.Status, invalid.EntryID)
	}
	if got := len(global.Cron.Entries()); got != 1 {
		t.Fatalf("expected one restored cron entry, got %d", got)
	}
}

func TestNormalizeClamRulePaths(t *testing.T) {
	scanDir := t.TempDir()
	quarantineDir := t.TempDir()
	linkParent := t.TempDir()
	linkPath := filepath.Join(linkParent, "scan-link")
	if err := os.Symlink(scanDir, linkPath); err != nil {
		t.Skipf("create symlink: %v", err)
	}

	normalizedScanPath, normalizedQuarantineDir, err := normalizeClamRulePaths(linkPath, "move", quarantineDir)
	if err != nil {
		t.Fatalf("normalize symlinked clam paths: %v", err)
	}
	resolvedScanPath, _ := filepath.EvalSymlinks(scanDir)
	resolvedQuarantineDir, _ := filepath.EvalSymlinks(quarantineDir)
	if normalizedScanPath != resolvedScanPath || normalizedQuarantineDir != resolvedQuarantineDir {
		t.Fatalf("unexpected normalized paths: scan=%q infected=%q", normalizedScanPath, normalizedQuarantineDir)
	}

	if _, _, err := normalizeClamRulePaths(string(filepath.Separator), "none", ""); err == nil {
		t.Fatal("expected filesystem root scan path to be rejected")
	}
	if _, _, err := normalizeClamRulePaths(scanDir, "move", scanDir); err == nil {
		t.Fatal("expected quarantine directory inside scan path to be rejected")
	}
	nestedScanDir := filepath.Join(quarantineDir, "1panel-infected", "existing-rule")
	if err := os.MkdirAll(nestedScanDir, 0o700); err != nil {
		t.Fatalf("create nested scan directory: %v", err)
	}
	if _, _, err := normalizeClamRulePaths(nestedScanDir, "move", quarantineDir); err == nil {
		t.Fatal("expected scan directory inside quarantine root to be rejected")
	}
}

func TestClamRuleRejectsUnsafeName(t *testing.T) {
	setupClamScheduleTest(t)
	service := NewIClamService()
	err := service.Create(dto.ClamCreate{
		Name:             "../../escape",
		Path:             t.TempDir(),
		InfectedStrategy: "none",
		Timeout:          60,
	}, "tester")
	if err == nil {
		t.Fatal("expected unsafe clam rule name to be rejected")
	}
}

func TestClamScheduleRejectsChangesWhileRunning(t *testing.T) {
	setupClamScheduleTest(t)
	service := NewIClamService()
	scanDir := t.TempDir()

	if err := service.Create(dto.ClamCreate{
		Name:             "running-scan",
		Path:             scanDir,
		InfectedStrategy: "none",
		Spec:             "0 5 * * *",
		Timeout:          60,
	}, "tester"); err != nil {
		t.Fatalf("create running scan fixture: %v", err)
	}
	item, _ := clamRepo.Get(repo.WithByName("running-scan"))
	if err := clamRepo.Update(item.ID, map[string]interface{}{"is_executing": true}); err != nil {
		t.Fatalf("mark scan running: %v", err)
	}

	if err := service.Update(dto.ClamUpdate{
		ID:               item.ID,
		Name:             item.Name,
		Path:             item.Path,
		InfectedStrategy: "none",
		Spec:             item.Spec,
		Timeout:          item.Timeout,
	}, "tester"); err == nil {
		t.Fatal("expected update of running scan to fail")
	}
	if err := service.UpdateStatus(item.ID, constant.StatusDisable); err == nil {
		t.Fatal("expected status update of running scan to fail")
	}
	if err := service.Delete(dto.ClamDelete{Ids: []uint{item.ID}}); err == nil {
		t.Fatal("expected delete of running scan to fail")
	}
	if persisted, _ := clamRepo.Get(repo.WithByID(item.ID)); persisted.ID == 0 {
		t.Fatal("running scan was deleted despite execution guard")
	}
}

func TestClaimClamExecutionIsAtomic(t *testing.T) {
	setupClamScheduleTest(t)
	item := model.Clam{Name: "atomic-claim", Path: t.TempDir(), InfectedStrategy: "none"}
	if err := global.DB.Create(&item).Error; err != nil {
		t.Fatalf("create clam rule: %v", err)
	}
	if err := claimClamExecution(item.ID); err != nil {
		t.Fatalf("first execution claim failed: %v", err)
	}
	if err := claimClamExecution(item.ID); err == nil {
		t.Fatal("expected second execution claim to be rejected")
	}
}
