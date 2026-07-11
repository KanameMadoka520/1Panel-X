package service

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/app/repo"
	"github.com/1Panel-dev/1Panel/agent/app/task"
	"github.com/1Panel-dev/1Panel/agent/buserr"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/utils/alert_push"
	clamUtil "github.com/1Panel-dev/1Panel/agent/utils/clam"
	"github.com/1Panel-dev/1Panel/agent/utils/cmd"
	"github.com/1Panel-dev/1Panel/agent/utils/common"
	"github.com/1Panel-dev/1Panel/agent/utils/controller"
	"github.com/1Panel-dev/1Panel/agent/utils/xpack"
	"github.com/jinzhu/copier"
)

type ClamService struct {
	serviceName      string
	freshClamService string
}

type IClamService interface {
	LoadBaseInfo() (dto.ClamBaseInfo, error)
	Operate(operate string) error
	SearchWithPage(search dto.SearchClamWithPage) (int64, interface{}, error)
	Create(req dto.ClamCreate, operator string) error
	Update(req dto.ClamUpdate, operator string) error
	UpdateStatus(id uint, status string) error
	Delete(req dto.ClamDelete) error
	HandleOnce(id uint) error

	LoadFile(req dto.ClamFileReq) (string, error)
	UpdateFile(req dto.UpdateByNameAndFile) error

	SearchRecords(req dto.ClamLogSearch) (int64, interface{}, error)
	CleanRecord(id uint) error
}

func NewIClamService() IClamService {
	service := &ClamService{}
	clamUtil.RegisterScheduleHandler(service.HandleOnce)
	return service
}

func RestoreClamSchedules() error {
	NewIClamService()
	clams, err := clamRepo.List(repo.WithByStatus(constant.StatusEnable))
	if err != nil {
		return fmt.Errorf("load enabled clam schedules failed: %w", err)
	}

	var restoreErrs []error
	for i := range clams {
		item := &clams[i]
		normalizedName, err := clamUtil.NormalizeRuleName(item.Name)
		if err != nil {
			if updateErr := clamRepo.Update(item.ID, map[string]interface{}{
				"status":   constant.StatusDisable,
				"entry_id": 0,
			}); updateErr != nil {
				restoreErrs = append(restoreErrs, fmt.Errorf("disable invalid clam rule %d failed: %w", item.ID, updateErr))
			}
			restoreErrs = append(restoreErrs, fmt.Errorf("restore clam rule %d failed: %w", item.ID, err))
			continue
		}
		item.Name = normalizedName
		normalizedPath, normalizedInfectedDir, err := normalizeClamRulePaths(item.Path, item.InfectedStrategy, item.InfectedDir)
		if err != nil {
			if updateErr := clamRepo.Update(item.ID, map[string]interface{}{
				"status":   constant.StatusDisable,
				"entry_id": 0,
			}); updateErr != nil {
				restoreErrs = append(restoreErrs, fmt.Errorf("disable invalid clam rule %d failed: %w", item.ID, updateErr))
			}
			restoreErrs = append(restoreErrs, fmt.Errorf("restore clam rule %d failed: %w", item.ID, err))
			continue
		}
		item.Path = normalizedPath
		item.InfectedDir = normalizedInfectedDir
		if err := clamUtil.ValidateSchedule(item.Spec); err != nil {
			if updateErr := clamRepo.Update(item.ID, map[string]interface{}{
				"status":   constant.StatusDisable,
				"entry_id": 0,
			}); updateErr != nil {
				restoreErrs = append(restoreErrs, fmt.Errorf("disable invalid clam schedule %d failed: %w", item.ID, updateErr))
			}
			restoreErrs = append(restoreErrs, fmt.Errorf("restore clam schedule %d failed: %w", item.ID, err))
			continue
		}

		entryID, err := xpack.MultiNodeProvider.StartClam(item, true)
		if err != nil {
			_ = clamRepo.Update(item.ID, map[string]interface{}{"status": constant.StatusDisable, "entry_id": 0})
			restoreErrs = append(restoreErrs, fmt.Errorf("restore clam schedule %d failed: %w", item.ID, err))
			continue
		}
		if err := clamRepo.Update(item.ID, map[string]interface{}{
			"name":         item.Name,
			"path":         item.Path,
			"infected_dir": item.InfectedDir,
			"entry_id":     entryID,
		}); err != nil {
			clamUtil.RemoveSchedule(entryID)
			restoreErrs = append(restoreErrs, fmt.Errorf("persist restored clam schedule %d failed: %w", item.ID, err))
		}
	}
	return errors.Join(restoreErrs...)
}

func normalizeClamRulePaths(scanPath, infectedStrategy, infectedDir string) (string, string, error) {
	normalizedScanPath, err := normalizeClamDirectory(scanPath, "scan")
	if err != nil {
		return "", "", err
	}

	switch infectedStrategy {
	case "none", "remove":
		return normalizedScanPath, "", nil
	case "move", "copy":
	default:
		return "", "", fmt.Errorf("unsupported infected file strategy %q", infectedStrategy)
	}

	normalizedInfectedDir, err := normalizeClamDirectory(infectedDir, "infected")
	if err != nil {
		return "", "", err
	}
	quarantineRoot := filepath.Join(normalizedInfectedDir, "1panel-infected")
	if pathWithin(normalizedScanPath, quarantineRoot) || pathWithin(quarantineRoot, normalizedScanPath) {
		return "", "", fmt.Errorf("clam scan and infected directories must not overlap")
	}
	return normalizedScanPath, normalizedInfectedDir, nil
}

func normalizeClamDirectory(rawPath, label string) (string, error) {
	cleanedPath := filepath.Clean(strings.TrimSpace(rawPath))
	if cleanedPath == "." || !filepath.IsAbs(cleanedPath) {
		return "", fmt.Errorf("clam %s path must be absolute", label)
	}
	absPath, err := filepath.Abs(cleanedPath)
	if err != nil {
		return "", fmt.Errorf("resolve clam %s path %q failed: %w", label, rawPath, err)
	}
	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", fmt.Errorf("resolve clam %s path %q failed: %w", label, rawPath, err)
	}
	resolvedPath = filepath.Clean(resolvedPath)
	if isFilesystemRoot(resolvedPath) {
		return "", fmt.Errorf("clam %s path cannot be a filesystem root", label)
	}
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return "", fmt.Errorf("stat clam %s path %q failed: %w", label, resolvedPath, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("clam %s path %q is not a directory", label, resolvedPath)
	}
	return resolvedPath, nil
}

func isFilesystemRoot(path string) bool {
	cleanedPath := filepath.Clean(path)
	volume := filepath.VolumeName(cleanedPath)
	root := volume + string(filepath.Separator)
	if volume == "" {
		root = string(filepath.Separator)
	}
	return cleanedPath == root
}

func pathWithin(parent, candidate string) bool {
	relativePath, err := filepath.Rel(parent, candidate)
	if err != nil {
		return false
	}
	return relativePath == "." || (relativePath != ".." && !strings.HasPrefix(relativePath, ".."+string(filepath.Separator)))
}

func (c *ClamService) LoadBaseInfo() (dto.ClamBaseInfo, error) {
	var baseInfo dto.ClamBaseInfo
	baseInfo.Version = "-"
	baseInfo.FreshVersion = "-"

	clamSvc, err := controller.LoadServiceName("clam")
	if err != nil {
		baseInfo.IsExist = false
		return baseInfo, nil
	}
	c.serviceName = clamSvc
	exist, _ := controller.CheckExist(clamSvc)
	if exist {
		baseInfo.IsExist = true
		baseInfo.IsActive, _ = controller.CheckActive(clamSvc)
	}

	freshSvc, err := controller.LoadServiceName("freshclam")
	if err != nil {
		baseInfo.FreshIsExist = false
		return baseInfo, nil
	}
	c.freshClamService = freshSvc
	freshExist, _ := controller.CheckExist(freshSvc)
	if freshExist {
		baseInfo.FreshIsExist = true
		baseInfo.FreshIsActive, _ = controller.CheckActive(freshSvc)
	}

	if !cmd.Which("clamdscan") {
		baseInfo.IsActive = false
	}

	cmdMgr := cmd.NewCommandMgr(cmd.WithTimeout(20 * time.Second))
	if baseInfo.IsActive {
		version, err := cmdMgr.RunWithStdout("clamdscan", "--version")
		if err == nil {
			if strings.Contains(version, "/") {
				baseInfo.Version = strings.TrimPrefix(strings.Split(version, "/")[0], "ClamAV ")
			} else {
				baseInfo.Version = strings.TrimPrefix(version, "ClamAV ")
			}
		}
	} else {
		_ = clamUtil.CheckWithStopAll(false, clamRepo)
	}
	if baseInfo.FreshIsActive {
		version, err := cmdMgr.RunWithStdout("freshclam", "--version")
		if err == nil {
			if strings.Contains(version, "/") {
				baseInfo.FreshVersion = strings.TrimPrefix(strings.Split(version, "/")[0], "ClamAV ")
			} else {
				baseInfo.FreshVersion = strings.TrimPrefix(version, "ClamAV ")
			}
		}
	}
	return baseInfo, nil
}

func (c *ClamService) Operate(operate string) error {
	switch operate {
	case "start", "restart", "stop":
		if err := controller.Handle(operate, c.serviceName); err != nil {
			return fmt.Errorf("%s the %s failed, err: %s", operate, c.serviceName, err)
		}
		return nil
	case "fresh-start", "fresh-restart", "fresh-stop":
		if err := controller.Handle(strings.TrimPrefix(operate, "fresh-"), c.freshClamService); err != nil {
			return fmt.Errorf("%s the %s failed, err: %s", operate, c.serviceName, err)
		}
		return nil
	default:
		return fmt.Errorf("not support such operation: %v", operate)
	}
}

func (c *ClamService) SearchWithPage(req dto.SearchClamWithPage) (int64, interface{}, error) {
	total, clams, err := clamRepo.Page(req.Page, req.PageSize, repo.WithByLikeName(req.Info), repo.WithOrderRuleBy(req.OrderBy, req.Order))
	if err != nil {
		return 0, nil, err
	}
	var datas []dto.ClamInfo
	for _, clam := range clams {
		var item dto.ClamInfo
		if err := copier.Copy(&item, &clam); err != nil {
			return 0, nil, buserr.WithDetail("ErrStructTransform", err.Error(), nil)
		}
		datas = append(datas, item)
	}
	for i := 0; i < len(datas); i++ {
		record, _ := clamRepo.RecordFirst(datas[i].ID)
		if record.ID != 0 {
			datas[i].LastRecordStatus = record.Status
			datas[i].LastRecordTime = record.StartTime.Format(constant.DateTimeLayout)
		} else {
			datas[i].LastRecordTime = "-"
		}
		alertBase := dto.AlertBase{
			AlertType: "clams",
			EntryID:   datas[i].ID,
		}
		alertInfo, _ := alertRepo.Get(alertRepo.WithByType(alertBase.AlertType), alertRepo.WithByProject(strconv.Itoa(int(alertBase.EntryID))), repo.WithByStatus(constant.AlertEnable))
		datas[i].AlertMethod = alertInfo.Method
		if alertInfo.SendCount != 0 {
			datas[i].AlertCount = alertInfo.SendCount
		} else {
			datas[i].AlertCount = 0
		}
	}
	return total, datas, err
}

func (c *ClamService) Create(req dto.ClamCreate, operator string) error {
	normalizedName, err := clamUtil.NormalizeRuleName(req.Name)
	if err != nil {
		return err
	}
	req.Name = normalizedName
	req.Spec = strings.TrimSpace(req.Spec)
	if req.Spec != "" {
		if err := clamUtil.ValidateSchedule(req.Spec); err != nil {
			return err
		}
	}
	clam, _ := clamRepo.Get(repo.WithByName(req.Name))
	if clam.ID != 0 {
		return buserr.New("ErrRecordExist")
	}
	if cmd.CheckIllegal(req.Path) {
		return buserr.New("ErrCmdIllegal")
	}
	normalizedPath, normalizedInfectedDir, err := normalizeClamRulePaths(req.Path, req.InfectedStrategy, req.InfectedDir)
	if err != nil {
		return err
	}
	req.Path = normalizedPath
	req.InfectedDir = normalizedInfectedDir
	if err := copier.Copy(&clam, &req); err != nil {
		return buserr.WithDetail("ErrStructTransform", err.Error(), nil)
	}
	if clam.InfectedStrategy == "none" || clam.InfectedStrategy == "remove" {
		clam.InfectedDir = ""
	}
	if req.Spec != "" {
		clam.Status = constant.StatusEnable
	}
	if err := clamRepo.Create(&clam); err != nil {
		return err
	}
	if req.Spec != "" {
		entryID, err := xpack.MultiNodeProvider.StartClam(&clam, false)
		if err != nil {
			rollbackErr := clamRepo.Delete(repo.WithByID(clam.ID))
			return errors.Join(err, rollbackErr)
		}
		if err := clamRepo.Update(clam.ID, map[string]interface{}{"entry_id": entryID}); err != nil {
			clamUtil.RemoveSchedule(entryID)
			rollbackErr := clamRepo.Delete(repo.WithByID(clam.ID))
			return errors.Join(err, rollbackErr)
		}
		clam.EntryID = entryID
	}
	if req.AlertCount != 0 && req.AlertTitle != "" && req.AlertMethod != "" {
		createAlert := dto.AlertCreate{
			Title:     req.AlertTitle,
			SendCount: req.AlertCount,
			Method:    req.AlertMethod,
			Type:      "clams",
			Project:   strconv.Itoa(int(clam.ID)),
			Status:    constant.AlertEnable,
		}
		if err := NewIAlertService().CreateAlert(createAlert, operator); err != nil {
			if rollbackErr := rollbackCreatedClam(clam); rollbackErr != nil {
				return clamAlertRollbackError("create", clam.ID, err, rollbackErr)
			}
			return err
		}
	}
	return nil
}

func rollbackCreatedClam(clam model.Clam) error {
	if err := clamRepo.Delete(repo.WithByID(clam.ID)); err != nil {
		return err
	}
	clamUtil.RemoveSchedule(clam.EntryID)
	return nil
}

func (c *ClamService) Update(req dto.ClamUpdate, operator string) error {
	normalizedName, err := clamUtil.NormalizeRuleName(req.Name)
	if err != nil {
		return err
	}
	req.Name = normalizedName
	req.Spec = strings.TrimSpace(req.Spec)
	if req.Spec != "" {
		if err := clamUtil.ValidateSchedule(req.Spec); err != nil {
			return err
		}
	}
	if cmd.CheckIllegal(req.Path) {
		return buserr.New("ErrCmdIllegal")
	}
	clam, _ := clamRepo.Get(repo.WithByID(req.ID))
	if clam.ID == 0 {
		return buserr.New("ErrRecordNotFound")
	}
	if clam.IsExecuting {
		return buserr.New("TaskIsExecuting")
	}
	if sameName, _ := clamRepo.Get(repo.WithByName(req.Name)); sameName.ID != 0 && sameName.ID != clam.ID {
		return buserr.New("ErrRecordExist")
	}
	if req.InfectedStrategy == "none" || req.InfectedStrategy == "remove" {
		req.InfectedDir = ""
	}
	normalizedPath, normalizedInfectedDir, err := normalizeClamRulePaths(req.Path, req.InfectedStrategy, req.InfectedDir)
	if err != nil {
		return err
	}
	req.Path = normalizedPath
	req.InfectedDir = normalizedInfectedDir
	var clamItem model.Clam
	if err := copier.Copy(&clamItem, &req); err != nil {
		return buserr.WithDetail("ErrStructTransform", err.Error(), nil)
	}
	clamItem.ID = clam.ID
	targetStatus := clam.Status
	if req.Spec == "" {
		targetStatus = ""
	} else if clam.Spec == "" || targetStatus == "" {
		targetStatus = constant.StatusEnable
	}
	clamItem.Status = targetStatus

	newEntryID := 0
	if req.Spec != "" && targetStatus != constant.StatusDisable {
		entryID, err := xpack.MultiNodeProvider.StartClam(&clamItem, true)
		if err != nil {
			return err
		}
		newEntryID = entryID
	}

	upMap := map[string]interface{}{
		"name":              req.Name,
		"path":              req.Path,
		"infected_dir":      req.InfectedDir,
		"infected_strategy": req.InfectedStrategy,
		"spec":              req.Spec,
		"timeout":           req.Timeout,
		"description":       req.Description,
		"status":            targetStatus,
		"entry_id":          newEntryID,
	}
	if err := clamRepo.Update(req.ID, upMap); err != nil {
		clamUtil.RemoveSchedule(newEntryID)
		return err
	}
	updateAlert := dto.AlertCreate{
		Title:     req.AlertTitle,
		SendCount: req.AlertCount,
		Method:    req.AlertMethod,
		Type:      "clams",
		Project:   strconv.Itoa(int(clam.ID)),
	}
	err = NewIAlertService().ExternalUpdateAlert(updateAlert, operator)
	if err != nil {
		if rollbackErr := rollbackUpdatedClam(clam, newEntryID); rollbackErr != nil {
			if clam.EntryID != 0 && clam.EntryID != newEntryID {
				clamUtil.RemoveSchedule(clam.EntryID)
			}
			return clamAlertRollbackError("update", clam.ID, err, rollbackErr)
		}
		return err
	}
	if clam.EntryID != 0 && clam.EntryID != newEntryID {
		clamUtil.RemoveSchedule(clam.EntryID)
	}
	return nil
}

func rollbackUpdatedClam(clam model.Clam, newEntryID int) error {
	rollbackMap := map[string]interface{}{
		"name":              clam.Name,
		"path":              clam.Path,
		"infected_dir":      clam.InfectedDir,
		"infected_strategy": clam.InfectedStrategy,
		"spec":              clam.Spec,
		"timeout":           clam.Timeout,
		"description":       clam.Description,
		"status":            clam.Status,
		"entry_id":          clam.EntryID,
	}
	if err := clamRepo.Update(clam.ID, rollbackMap); err != nil {
		return err
	}
	clamUtil.RemoveSchedule(newEntryID)
	return nil
}

func clamAlertRollbackError(operation string, clamID uint, alertErr, rollbackErr error) error {
	partialErr := fmt.Errorf(
		"clam rule %d %s alert change failed and rollback failed; the persisted rule state remains applied: %w",
		clamID,
		operation,
		rollbackErr,
	)
	if global.LOG != nil {
		global.LOG.Errorf("%v; alert error: %v", partialErr, alertErr)
	}
	return errors.Join(alertErr, partialErr)
}

func (c *ClamService) UpdateStatus(id uint, status string) error {
	clam, _ := clamRepo.Get(repo.WithByID(id))
	if clam.ID == 0 {
		return buserr.New("ErrRecordNotFound")
	}
	if clam.IsExecuting {
		return buserr.New("TaskIsExecuting")
	}
	if status != constant.StatusEnable && status != constant.StatusDisable {
		return fmt.Errorf("unsupported clam schedule status %q", status)
	}
	if status == clam.Status && (status == constant.StatusDisable || clam.EntryID != 0) {
		return nil
	}

	entryID := 0
	if status == constant.StatusEnable {
		if err := clamUtil.ValidateSchedule(clam.Spec); err != nil {
			return err
		}
		newEntryID, err := xpack.MultiNodeProvider.StartClam(&clam, true)
		if err != nil {
			return err
		}
		entryID = newEntryID
	}

	if err := clamRepo.Update(clam.ID, map[string]interface{}{"status": status, "entry_id": entryID}); err != nil {
		clamUtil.RemoveSchedule(entryID)
		return err
	}
	if clam.EntryID != 0 && clam.EntryID != entryID {
		clamUtil.RemoveSchedule(clam.EntryID)
		global.LOG.Infof("stop clam schedule entryID: %v", clam.EntryID)
	}
	return nil
}

func (c *ClamService) Delete(req dto.ClamDelete) error {
	clams := make([]model.Clam, 0, len(req.Ids))
	for _, id := range req.Ids {
		item, _ := clamRepo.Get(repo.WithByID(id))
		if item.ID == 0 {
			continue
		}
		if item.IsExecuting {
			return buserr.New("TaskIsExecuting")
		}
		clams = append(clams, item)
	}
	for _, clam := range clams {
		_ = c.CleanRecord(clam.ID)
		if req.RemoveInfected && clam.InfectedDir != "" &&
			(clam.InfectedStrategy == "move" || clam.InfectedStrategy == "copy") {
			if err := clamUtil.RemoveInfectedDirectory(clam.InfectedDir, clam.Name); err != nil {
				return err
			}
		}
		if err := clamRepo.Delete(repo.WithByID(clam.ID)); err != nil {
			return err
		}
		clamUtil.RemoveSchedule(clam.EntryID)
	}
	var alertCleanupErrors []error
	for _, id := range req.Ids {
		if err := alertRepo.Delete(alertRepo.WithByProject(strconv.Itoa(int(id))), alertRepo.WithByType("clams")); err != nil {
			cleanupErr := fmt.Errorf("delete alert for clam rule %d failed after the primary deletion: %w", id, err)
			if global.LOG != nil {
				global.LOG.Error(cleanupErr)
			}
			alertCleanupErrors = append(alertCleanupErrors, cleanupErr)
		}
	}
	return errors.Join(alertCleanupErrors...)
}

func (c *ClamService) HandleOnce(id uint) error {
	if active := clamUtil.CheckWithStopAll(true, clamRepo); !active {
		return buserr.New("ErrClamdscanNotFound")
	}
	clamItem, _ := clamRepo.Get(repo.WithByID(id))
	if clamItem.ID == 0 {
		return buserr.New("ErrRecordNotFound")
	}
	if clamItem.IsExecuting {
		return buserr.New("TaskIsExecuting")
	}
	if err := claimClamExecution(clamItem.ID); err != nil {
		return err
	}
	record := clamRepo.StartRecords(clamItem.ID)
	taskItem, err := task.NewTaskWithOps("clam-"+clamItem.Name, task.TaskScan, task.TaskScopeClam, record.TaskID, clamItem.ID)
	if err != nil {
		_ = clamRepo.Update(clamItem.ID, map[string]interface{}{"is_executing": false})
		return fmt.Errorf("new task for exec shell failed, err: %v", err)
	}
	clamUtil.AddScanTask(taskItem, clamItem, record.StartTime.Format(constant.DateTimeSlimLayout))
	go func() {
		err := taskItem.Execute()
		taskRepo := repo.NewITaskRepo()
		taskItem, _ := taskRepo.GetFirst(taskRepo.WithByID(record.TaskID))
		if len(taskItem.ID) == 0 {
			record.TaskID = ""
		}
		if err != nil {
			clamRepo.EndRecords(record, constant.StatusFailed, err.Error())
			return
		}
		clamUtil.AnalysisFromLog(taskItem.LogFile, &record)
		clamRepo.EndRecords(record, constant.StatusDone, "")
		handleAlert(record.InfectedFiles, clamItem.Name, clamItem.ID)
	}()
	return nil
}

func claimClamExecution(id uint) error {
	claim := global.DB.Model(&model.Clam{}).
		Where("id = ? AND is_executing = ?", id, false).
		Update("is_executing", true)
	if claim.Error != nil {
		return claim.Error
	}
	if claim.RowsAffected == 0 {
		return buserr.New("TaskIsExecuting")
	}
	return nil
}

func (c *ClamService) SearchRecords(req dto.ClamLogSearch) (int64, interface{}, error) {
	clam, _ := clamRepo.Get(repo.WithByID(req.ClamID))
	if clam.ID == 0 {
		return 0, nil, buserr.New("ErrRecordNotFound")
	}
	loc, _ := time.LoadLocation(common.LoadTimeZoneByCmd())
	req.StartTime = req.StartTime.In(loc)
	req.EndTime = req.EndTime.In(loc)

	total, records, err := clamRepo.PageRecords(req.Page, req.PageSize, clamRepo.WithByClamID(req.ClamID), repo.WithByStatus(req.Status), repo.WithByCreatedAt(req.StartTime, req.EndTime))
	if err != nil {
		return 0, nil, err
	}
	var datas []dto.ClamRecord
	for _, record := range records {
		var item dto.ClamRecord
		if err := copier.Copy(&item, &record); err != nil {
			return 0, nil, buserr.WithDetail("ErrStructTransform", err.Error(), nil)
		}
		datas = append(datas, item)
	}
	return int64(total), datas, nil
}

func (c *ClamService) CleanRecord(id uint) error {
	record, _ := clamRepo.ListRecord()
	for _, item := range record {
		if len(item.TaskID) != 0 {
			continue
		}
		taskItem, _ := taskRepo.GetFirst(taskRepo.WithByID(item.TaskID))
		if len(taskItem.LogFile) != 0 {
			_ = os.Remove(taskItem.LogFile)
		}
	}
	return clamRepo.DeleteRecord(clamRepo.WithByClamID(id))
}

func (c *ClamService) LoadFile(req dto.ClamFileReq) (string, error) {
	filePath := ""
	switch req.Name {
	case "clamd":
		filePath = c.loadConfigPath("clamd")
	case "clamd-log":
		filePath = c.loadLogPath("clamd-log")
	case "freshclam":
		filePath = c.loadConfigPath("freshclam")
	case "freshclam-log":
		filePath = c.loadLogPath("freshclam-log")
	default:
		return "", fmt.Errorf("not support such type")
	}
	if _, err := os.Stat(filePath); err != nil {
		return "", buserr.New("ErrHttpReqNotFound")
	}
	var tail string
	if req.Tail != "0" {
		tail = req.Tail
	} else {
		tail = "+1"
	}
	cmd := exec.Command("tail", "-n", tail, filePath)
	stdout, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tail -n %v failed, err: %v", req.Tail, err)
	}
	return string(stdout), nil
}

func (c *ClamService) UpdateFile(req dto.UpdateByNameAndFile) error {
	filePath := ""
	switch req.Name {
	case "clamd":
		filePath = c.loadConfigPath("clamd")
	case "freshclam":
		filePath = c.loadConfigPath("freshclam")
	default:
		return fmt.Errorf("not support such type")
	}
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_TRUNC, 0640)
	if err != nil {
		return err
	}
	defer file.Close()
	write := bufio.NewWriter(file)
	_, _ = write.WriteString(req.File)
	write.Flush()

	_ = controller.HandleRestart(c.serviceName)
	return nil
}

func (c *ClamService) loadLogPath(name string) string {
	configKey := "clamd"
	searchPrefix := "LogFile "
	if name != "clamd-log" {
		configKey = "freshclam"
		searchPrefix = "UpdateLogFile "
	}
	confPath := c.loadConfigPath(configKey)
	content, err := os.ReadFile(confPath)
	if err != nil {
		global.LOG.Debugf("read config of %s failed, err: %v", configKey, err)
		return ""
	}
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, searchPrefix) {
			return strings.Trim(strings.ReplaceAll(line, searchPrefix, ""), " ")
		}
	}
	if configKey == "clamd" {
		if _, err := os.Stat("/var/log/clamav/clamav.log"); err == nil {
			return "/var/log/clamav/clamav.log"
		}
		if _, err := os.Stat("/var/log/clamd.scan"); err == nil {
			return "/var/log/clamd.scan"
		}
	}
	if configKey == "freshclam" {
		if _, err := os.Stat("/var/log/clamav/freshclam.log"); err == nil {
			return "/var/log/clamav/freshclam.log"
		}
		if _, err := os.Stat("/var/log/freshclam.log"); err == nil {
			return "/var/log/freshclam.log"
		}
	}
	return ""
}

func (c *ClamService) loadConfigPath(confType string) string {
	switch confType {
	case "clamd":
		if _, err := os.Stat("/etc/clamav/clamd.conf"); err == nil {
			return "/etc/clamav/clamd.conf"
		}
		return "/etc/clamd.d/scan.conf"
	case "freshclam":
		if _, err := os.Stat("/etc/clamav/freshclam.conf"); err == nil {
			return "/etc/clamav/freshclam.conf"
		}
		return "/etc/freshclam.conf"
	default:
		return ""
	}
}

func handleAlert(infectedFiles, clamName string, clamId uint) {
	itemInfected, _ := strconv.Atoi(strings.TrimSpace(infectedFiles))
	if itemInfected <= 0 {
		return
	}
	pushAlert := dto.PushAlert{
		TaskName:  clamName,
		AlertType: "clams",
		EntryID:   clamId,
		Param:     strconv.Itoa(itemInfected),
	}
	_ = alert_push.PushAlert(pushAlert)
}
