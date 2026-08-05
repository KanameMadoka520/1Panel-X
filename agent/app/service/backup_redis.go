package service

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/app/repo"
	"github.com/1Panel-dev/1Panel/agent/app/task"
	"github.com/1Panel-dev/1Panel/agent/buserr"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/i18n"
	"github.com/1Panel-dev/1Panel/agent/utils/cmd"
	"github.com/1Panel-dev/1Panel/agent/utils/common"
	"github.com/1Panel-dev/1Panel/agent/utils/compose"
	"github.com/1Panel-dev/1Panel/agent/utils/files"
)

var errRedisRecoveryRollbackFailed = errors.New("Redis recovery failed and the pre-recovery backup could not be restored; manual recovery is required")

func (u *BackupService) RedisBackup(req dto.CommonBackup) error {
	redisInfo, err := appInstallRepo.LoadBaseInfo(req.Type, req.Name)
	if err != nil {
		return err
	}
	appendonly, err := configGetStr(redisInfo.ContainerName, redisInfo.Password, "appendonly")
	if err != nil {
		return err
	}
	global.LOG.Infof("appendonly in redis conf is %s", appendonly)

	timeNow := time.Now().Format(constant.DateTimeSlimLayout) + common.RandStrAndNum(5)
	fileName := fmt.Sprintf("%s.rdb", timeNow)
	if appendonly == "yes" {
		if strings.HasPrefix(redisInfo.Version, "6.") {
			fileName = fmt.Sprintf("%s.aof", timeNow)
		} else {
			fileName = fmt.Sprintf("%s.tar.gz", timeNow)
		}
	}
	itemDir := fmt.Sprintf("database/redis/%s", redisInfo.Name)
	backupDir := path.Join(global.Dir.LocalBackupDir, itemDir)
	record := &model.BackupRecord{
		Type:              req.Type,
		Name:              req.Name,
		SourceAccountIDs:  "1",
		DownloadAccountID: 1,
		FileDir:           itemDir,
		FileName:          fileName,
		TaskID:            req.TaskID,
		Status:            constant.StatusWaiting,
		Description:       req.Description,
	}
	if err := backupRepo.CreateRecord(record); err != nil {
		global.LOG.Errorf("save backup record failed, err: %v", err)
	}

	if err := handleRedisBackup(redisInfo, nil, record.ID, backupDir, fileName, req.Secret, req.TaskID); err != nil {
		markBackupFailed(record.ID, err)
		return err
	}
	return nil
}

func (u *BackupService) RedisRecover(req dto.CommonRecover) error {
	redisInfo, err := appInstallRepo.LoadBaseInfo(req.Type, req.Name)
	if err != nil {
		return err
	}
	global.LOG.Infof("recover redis from backup file %s", req.File)
	if err := handleRedisRecover(redisInfo, nil, req.File, req.Secret, req.TaskID); err != nil {
		return err
	}
	return nil
}

func handleRedisBackup(redisInfo *repo.RootInfo, parentTask *task.Task, recordID uint, backupDir, fileName, secret, taskID string) error {
	var (
		err      error
		itemTask *task.Task
	)
	itemTask = parentTask
	if parentTask == nil {
		itemTask, err = task.NewTaskWithOps("Redis", task.TaskBackup, task.TaskScopeBackup, taskID, redisInfo.ID)
		if err != nil {
			return err
		}
	}

	backupDatabase := func(_ *task.Task) error {
		return backupRedisDatabase(redisInfo, backupDir, fileName, secret)
	}
	return scheduleRedisBackupTask(itemTask, parentTask, backupDatabase, itemTask.ExecuteToCompletion, func(err error) {
		if err != nil {
			markBackupFailed(recordID, err)
			return
		}
		backupRepo.UpdateRecordByMap(recordID, map[string]interface{}{"status": constant.StatusSuccess})
	})
}

func backupRedisDatabase(redisInfo *repo.RootInfo, backupDir, fileName, secret string) error {
	fileOp := files.NewFileOp()
	if !fileOp.Stat(backupDir) {
		if err := os.MkdirAll(backupDir, os.ModePerm); err != nil {
			return fmt.Errorf("mkdir %s failed, err: %v", backupDir, err)
		}
	}

	cmdMgr := cmd.NewCommandMgr(cmd.WithTimeout(30 * time.Minute))
	if err := cmdMgr.Run("docker", "exec", redisInfo.ContainerName, "redis-cli", "-a", redisInfo.Password, "--no-auth-warning", "save"); err != nil {
		return err
	}

	if strings.HasSuffix(fileName, ".tar.gz") {
		redisDataDir := fmt.Sprintf("%s/%s/%s/data/appendonlydir", global.Dir.AppInstallDir, redisInfo.Key, redisInfo.Name)
		if err := fileOp.TarGzCompressPro(true, redisDataDir, path.Join(backupDir, fileName), secret, ""); err != nil {
			return err
		}
		return nil
	}
	if strings.HasSuffix(fileName, ".aof") {
		if err := cmdMgr.Run("docker", "cp", redisInfo.ContainerName+":/data/appendonly.aof", path.Join(backupDir, fileName)); err != nil {
			return err
		}
		return nil
	}

	if err := cmdMgr.Run("docker", "cp", redisInfo.ContainerName+":/data/dump.rdb", path.Join(backupDir, fileName)); err != nil {
		return err
	}
	return nil
}

func scheduleRedisBackupTask(itemTask, parentTask *task.Task, action task.ActionFunc, execute func() error, complete func(error)) error {
	itemTask.AddSubTaskWithOps(i18n.GetMsgByKey("TaskBackup"), action, nil, 3, time.Hour)
	if parentTask != nil {
		return nil
	}
	go func() {
		complete(execute())
	}()
	return nil
}

func handleRedisRecover(redisInfo *repo.RootInfo, parentTask *task.Task, recoverFile, secret, taskID string) error {
	var (
		err      error
		itemTask *task.Task
	)
	itemTask = parentTask
	if parentTask == nil {
		itemTask, err = task.NewTaskWithOps("Redis", task.TaskRecover, task.TaskScopeBackup, taskID, redisInfo.ID)
		if err != nil {
			return err
		}
	}

	recoverDatabase := func(_ *task.Task) error {
		return recoverRedisDatabase(redisInfo, recoverFile, secret)
	}
	return scheduleRedisRecoverTask(itemTask, parentTask, recoverDatabase, itemTask.ExecuteToCompletion)
}

func scheduleRedisRecoverTask(itemTask, parentTask *task.Task, action task.ActionFunc, execute func() error) error {
	itemTask.AddSubTask(i18n.GetMsgByKey("TaskRecover"), action, nil)
	if parentTask != nil {
		return nil
	}
	go func() {
		_ = execute()
	}()
	return nil
}

func recoverRedisDatabase(redisInfo *repo.RootInfo, recoverFile, secret string) error {
	fileOp := files.NewFileOp()
	if !fileOp.Stat(recoverFile) {
		return buserr.WithName("ErrFileNotFound", recoverFile)
	}

	appendonly, err := configGetStr(redisInfo.ContainerName, redisInfo.Password, "appendonly")
	if err != nil {
		return err
	}

	if appendonly == "yes" {
		if strings.HasPrefix(redisInfo.Version, "6.") && !strings.HasSuffix(recoverFile, ".aof") {
			return buserr.New("ErrTypeOfRedis")
		}
		if strings.HasPrefix(redisInfo.Version, "7.") && !strings.HasSuffix(recoverFile, ".tar.gz") {
			return buserr.New("ErrTypeOfRedis")
		}
	} else if !strings.HasSuffix(recoverFile, ".rdb") {
		return buserr.New("ErrTypeOfRedis")
	}

	global.LOG.Infof("appendonly in redis conf is %s", appendonly)
	suffix := "rdb"
	if appendonly == "yes" {
		if strings.HasPrefix(redisInfo.Version, "6.") {
			suffix = "aof"
		} else {
			suffix = "tar.gz"
		}
	}
	rollbackFile := path.Join(global.Dir.TmpDir, fmt.Sprintf("database/redis/%s_%s.%s", redisInfo.Name, time.Now().Format(constant.DateTimeSlimLayout), suffix))
	if err := backupRedisDatabase(redisInfo, path.Dir(rollbackFile), path.Base(rollbackFile), secret); err != nil {
		return fmt.Errorf("backup database %s for rollback before recover failed, err: %v", redisInfo.Name, err)
	}

	recoverErr := recoverRedisWithRollback(recoverFile, rollbackFile, func(file string) error {
		return applyRedisRecovery(redisInfo, file, appendonly, secret)
	})
	if !errors.Is(recoverErr, errRedisRecoveryRollbackFailed) {
		_ = os.RemoveAll(rollbackFile)
	} else if global.LOG != nil {
		global.LOG.Errorf("Redis pre-recovery backup retained at %s for manual recovery", rollbackFile)
	}
	return recoverErr
}

func recoverRedisWithRollback(recoverFile, rollbackFile string, apply func(string) error) error {
	recoverErr := apply(recoverFile)
	if recoverErr == nil {
		return nil
	}

	if global.LOG != nil {
		global.LOG.Info("Redis recovery failed; restoring the pre-recovery backup")
	}
	if rollbackErr := apply(rollbackFile); rollbackErr != nil {
		if global.LOG != nil {
			global.LOG.Error("Redis recovery rollback failed")
		}
		return task.MarkNonRetryable(errRedisRecoveryRollbackFailed)
	}
	if global.LOG != nil {
		global.LOG.Info("Redis recovery rollback completed")
	}
	return recoverErr
}

func applyRedisRecovery(redisInfo *repo.RootInfo, recoverFile, appendonly, secret string) error {
	fileOp := files.NewFileOp()
	if !fileOp.Stat(recoverFile) {
		return buserr.WithName("ErrFileNotFound", recoverFile)
	}

	if appendonly == "yes" {
		if strings.HasPrefix(redisInfo.Version, "6.") && !strings.HasSuffix(recoverFile, ".aof") {
			return buserr.New("ErrTypeOfRedis")
		}
		if strings.HasPrefix(redisInfo.Version, "7.") && !strings.HasSuffix(recoverFile, ".tar.gz") {
			return buserr.New("ErrTypeOfRedis")
		}
	} else if !strings.HasSuffix(recoverFile, ".rdb") {
		return buserr.New("ErrTypeOfRedis")
	}

	composeDir := fmt.Sprintf("%s/%s/%s", global.Dir.AppInstallDir, redisInfo.Key, redisInfo.Name)
	if _, err := compose.Down(composeDir + "/docker-compose.yml"); err != nil {
		return err
	}
	if appendonly == "yes" && strings.HasPrefix(redisInfo.Version, "7.") {
		redisDataDir := fmt.Sprintf("%s/%s/%s/data", global.Dir.AppInstallDir, redisInfo.Key, redisInfo.Name)
		if err := fileOp.TarGzExtractPro(recoverFile, redisDataDir, secret); err != nil {
			return err
		}
	} else {
		itemName := "dump.rdb"
		if appendonly == "yes" && strings.HasPrefix(redisInfo.Version, "6.") {
			itemName = "appendonly.aof"
		}
		input, err := os.ReadFile(recoverFile)
		if err != nil {
			return err
		}
		if err = os.WriteFile(composeDir+"/data/"+itemName, input, 0640); err != nil {
			return err
		}
	}
	if _, err := compose.Up(composeDir + "/docker-compose.yml"); err != nil {
		return err
	}
	return nil
}
