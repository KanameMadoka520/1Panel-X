package job

import (
	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/app/service"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
)

type backup struct{}

func NewBackupJob() *backup {
	return &backup{}
}

func (b *backup) Run() {
	var backups []model.BackupAccount
	_ = global.DB.Where("`type` in (?) AND is_public = 0", []string{constant.OneDrive, constant.ALIYUN, constant.GoogleDrive}).Find(&backups)
	if len(backups) == 0 {
		return
	}
	for _, backupItem := range backups {
		if backupItem.ID == 0 {
			continue
		}
		global.LOG.Infof("Start to refresh %s-%s OAuth token ...", backupItem.Type, backupItem.Name)
		if err := service.NewIBackupService().RefreshToken(dto.OperateByID{ID: backupItem.ID}); err != nil {
			global.LOG.Errorf("failed to refresh %s-%s OAuth token: %v", backupItem.Type, backupItem.Name, err)
			continue
		}
		global.LOG.Infof("Refresh %s-%s OAuth token successful!", backupItem.Type, backupItem.Name)
	}
}
