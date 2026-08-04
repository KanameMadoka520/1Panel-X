package job

import (
	"github.com/1Panel-dev/1Panel/core/constant"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/utils/xpack"
)

type backupSync struct{}

func NewBackupSyncJob() *backupSync {
	return &backupSync{}
}

func (b *backupSync) Run() {
	if err := xpack.MultiNodeProvider.Sync(constant.SyncBackupAccounts); err != nil {
		global.LOG.Warn("public backup account reconciliation remains pending")
	}
}
