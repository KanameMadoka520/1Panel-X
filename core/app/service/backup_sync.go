package service

import (
	"errors"
	"strings"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/utils/backupsync"
)

func (u *BackupService) GetSyncStatus(name string) (dto.BackupSyncStatus, error) {
	return backupsync.GetStatus(name)
}

func (u *BackupService) ListSyncStatuses() ([]dto.BackupSyncStatus, error) {
	return backupsync.ListStatuses()
}

func (u *BackupService) RetrySync(name string) (dto.BackupSyncStatus, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return dto.BackupSyncStatus{}, errors.New("backup account name is required")
	}
	if err := backupsync.RetryAccount(name); err != nil {
		return dto.BackupSyncStatus{}, err
	}
	reconcilePublicBackupSync()
	return backupsync.GetStatus(name)
}
