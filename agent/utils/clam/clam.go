package clam

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/app/repo"
	"github.com/1Panel-dev/1Panel/agent/app/task"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/i18n"
	"github.com/1Panel-dev/1Panel/agent/utils/cmd"
	"github.com/1Panel-dev/1Panel/agent/utils/controller"
	"github.com/robfig/cron/v3"
)

var (
	scheduleHandlerMu sync.RWMutex
	scheduleHandler   func(uint) error
)

func RegisterScheduleHandler(handler func(uint) error) {
	scheduleHandlerMu.Lock()
	scheduleHandler = handler
	scheduleHandlerMu.Unlock()
}

func ValidateSchedule(spec string) error {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return fmt.Errorf("clam schedule is empty")
	}
	if _, err := cron.ParseStandard(spec); err != nil {
		return fmt.Errorf("invalid clam schedule %q: %w", spec, err)
	}
	return nil
}

func StartSchedule(startClam *model.Clam) (cron.EntryID, error) {
	if startClam == nil || startClam.ID == 0 {
		return 0, fmt.Errorf("clam schedule requires a persisted rule")
	}
	if err := ValidateSchedule(startClam.Spec); err != nil {
		return 0, err
	}
	if global.Cron == nil {
		return 0, fmt.Errorf("clam scheduler is not initialized")
	}

	scheduleHandlerMu.RLock()
	handler := scheduleHandler
	scheduleHandlerMu.RUnlock()
	if handler == nil {
		return 0, fmt.Errorf("clam schedule handler is not initialized")
	}

	clamID := startClam.ID
	entryID, err := global.Cron.AddFunc(strings.TrimSpace(startClam.Spec), func() {
		if err := handler(clamID); err != nil && global.LOG != nil {
			global.LOG.Errorf("run scheduled clam scan %d failed, err: %v", clamID, err)
		}
	})
	if err != nil {
		return 0, fmt.Errorf("register clam schedule %q failed: %w", startClam.Spec, err)
	}
	return entryID, nil
}

func RemoveSchedule(entryID int) {
	if global.Cron == nil || entryID <= 0 {
		return
	}
	global.Cron.Remove(cron.EntryID(entryID))
}

func NormalizeRuleName(rawName string) (string, error) {
	name := strings.TrimSpace(rawName)
	if name == "" || len([]rune(name)) > 128 {
		return "", fmt.Errorf("clam rule name must contain 1 to 128 characters")
	}
	if name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, `/\\`) {
		return "", fmt.Errorf("clam rule name must be a single path component")
	}
	for _, char := range name {
		if unicode.IsControl(char) {
			return "", fmt.Errorf("clam rule name contains control characters")
		}
	}
	return name, nil
}

func PrepareInfectedDirectory(baseDir, ruleName, runName string) (string, error) {
	baseDir, err := resolveQuarantineBase(baseDir)
	if err != nil {
		return "", err
	}
	ruleName, err = NormalizeRuleName(ruleName)
	if err != nil {
		return "", err
	}
	runName, err = NormalizeRuleName(runName)
	if err != nil {
		return "", err
	}

	root := filepath.Join(baseDir, "1panel-infected")
	if err := ensureSecureDirectory(root, true); err != nil {
		return "", err
	}
	ruleDir := filepath.Join(root, ruleName)
	if err := ensureSecureDirectory(ruleDir, true); err != nil {
		return "", err
	}
	runDir := filepath.Join(ruleDir, runName)
	if err := ensureSecureDirectory(runDir, true); err != nil {
		return "", err
	}
	return runDir, nil
}

func RemoveInfectedDirectory(baseDir, ruleName string) error {
	baseDir, err := resolveQuarantineBase(baseDir)
	if err != nil {
		return err
	}
	ruleName, err = NormalizeRuleName(ruleName)
	if err != nil {
		return err
	}

	root := filepath.Join(baseDir, "1panel-infected")
	if err := ensureSecureDirectory(root, false); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	ruleDir := filepath.Join(root, ruleName)
	if err := ensureSecureDirectory(ruleDir, false); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.RemoveAll(ruleDir); err != nil {
		return fmt.Errorf("remove clam infected directory %s failed: %w", ruleDir, err)
	}
	return nil
}

func resolveQuarantineBase(rawPath string) (string, error) {
	cleanedPath := filepath.Clean(strings.TrimSpace(rawPath))
	if cleanedPath == "." || !filepath.IsAbs(cleanedPath) {
		return "", fmt.Errorf("clam infected path must be absolute")
	}
	pathInfo, err := os.Lstat(cleanedPath)
	if err != nil {
		return "", fmt.Errorf("stat clam infected path %q failed: %w", cleanedPath, err)
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("clam infected path %q must not be a symbolic link", cleanedPath)
	}
	resolvedPath, err := filepath.EvalSymlinks(cleanedPath)
	if err != nil {
		return "", fmt.Errorf("resolve clam infected path %q failed: %w", rawPath, err)
	}
	resolvedPath = filepath.Clean(resolvedPath)
	if isFilesystemRoot(resolvedPath) {
		return "", fmt.Errorf("clam infected path cannot be a filesystem root")
	}
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return "", fmt.Errorf("stat clam infected path %q failed: %w", resolvedPath, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("clam infected path %q is not a directory", resolvedPath)
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

func ensureSecureDirectory(directory string, create bool) error {
	info, err := os.Lstat(directory)
	if err != nil {
		if !os.IsNotExist(err) || !create {
			return err
		}
		if err := os.Mkdir(directory, 0o700); err != nil {
			return fmt.Errorf("create clam infected directory %s failed: %w", directory, err)
		}
		info, err = os.Lstat(directory)
		if err != nil {
			return err
		}
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("clam infected directory %s must be a real directory", directory)
	}
	if create {
		if err := os.Chmod(directory, 0o700); err != nil {
			return fmt.Errorf("secure clam infected directory %s failed: %w", directory, err)
		}
	}
	return nil
}

func AddScanTask(taskItem *task.Task, clam model.Clam, timeNow string) {
	taskItem.AddSubTask(i18n.GetWithName("Clamscan", clam.Path), func(t *task.Task) error {
		strategy := ""
		switch clam.InfectedStrategy {
		case "remove":
			strategy = "--remove"
		case "move", "copy":
			dir, err := PrepareInfectedDirectory(clam.InfectedDir, clam.Name, timeNow)
			if err != nil {
				return err
			}
			taskItem.Log("infected dir: " + dir)
			strategy = fmt.Sprintf("--%s=%s", clam.InfectedStrategy, dir)
		}
		args := []string{"--fdpass"}
		if strategy != "" {
			args = append(args, strategy)
		}
		args = append(args, clam.Path)
		taskItem.Logf("clamdscan %s", strings.Join(args, " "))
		mgr := cmd.NewCommandMgr(cmd.WithIgnoreExist1(), cmd.WithTimeout(time.Duration(clam.Timeout)*time.Second), cmd.WithTask(*taskItem))
		if err := mgr.Run("clamdscan", args...); err != nil {
			return fmt.Errorf("clamdscan failed, %v", err)
		}
		return nil
	}, nil)
}

func AnalysisFromLog(pathItem string, record *model.ClamRecord) {
	file, err := os.ReadFile(pathItem)
	if err != nil {
		return
	}
	lines := strings.Split(string(file), "\n")
	for _, line := range lines {
		if len(line) < 20 {
			continue
		}
		line = line[20:]
		switch {
		case strings.HasPrefix(line, "Infected files: "):
			record.InfectedFiles = strings.TrimPrefix(line, "Infected files: ")
		case strings.HasPrefix(line, "Total errors: "):
			record.TotalError = strings.TrimPrefix(line, "Total errors: ")
		case strings.HasPrefix(line, "Time: "):
			record.ScanTime = strings.TrimPrefix(line, "Time: ")
		}
	}
}

func CheckWithStopAll(withCheck bool, clamRepo repo.IClamRepo) bool {
	if withCheck {
		isExist, _ := controller.CheckExist("clam")
		if !isExist {
			return false
		}
		isActive, _ := controller.CheckActive("clam")
		if isActive {
			return true
		}
	}
	clams, _ := clamRepo.List(repo.WithByStatus(constant.StatusEnable))
	for i := 0; i < len(clams); i++ {
		RemoveSchedule(clams[i].EntryID)
		_ = clamRepo.Update(clams[i].ID, map[string]interface{}{"status": constant.StatusDisable, "entry_id": 0})
	}
	return false
}
