package task

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/agent/buserr"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/sirupsen/logrus"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/app/repo"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/i18n"
	"github.com/google/uuid"
)

type ActionFunc func(*Task) error
type RollbackFunc func(*Task)

type NonRetryableError struct {
	Err error
}

func (e *NonRetryableError) Error() string {
	return e.Err.Error()
}

func (e *NonRetryableError) Unwrap() error {
	return e.Err
}

func MarkNonRetryable(err error) error {
	if err == nil || IsNonRetryable(err) {
		return err
	}
	return &NonRetryableError{Err: err}
}

func IsNonRetryable(err error) bool {
	var target *NonRetryableError
	return errors.As(err, &target)
}

type Task struct {
	TaskCtx context.Context

	Name      string
	TaskID    string
	Logger    *logrus.Logger
	SubTasks  []*SubTask
	Rollbacks []RollbackFunc
	logFile   *os.File
	taskRepo  repo.ITaskRepo
	Task      *model.Task
	ParentID  string
}

type SubTask struct {
	RootTask      *Task
	Name          string
	StepAlias     string
	Retry         int
	Timeout       time.Duration
	StopGrace     time.Duration
	RetryInterval time.Duration
	Action        ActionFunc
	Rollback      RollbackFunc
	Error         error
	IgnoreErr     bool
}

const (
	defaultSubTaskStopGrace     = 5 * time.Second
	defaultSubTaskRetryInterval = time.Second
)

type subTaskTimeoutError struct{}

func (*subTaskTimeoutError) Error() string {
	return "timeout!"
}

func (*subTaskTimeoutError) Unwrap() error {
	return context.DeadlineExceeded
}

type pendingAttemptError struct {
	cause        error
	done         <-chan error
	attemptTask  *Task
	rollback     RollbackFunc
	finalized    chan struct{}
	finalizeOnce sync.Once
}

func (e *pendingAttemptError) Error() string {
	return e.cause.Error()
}

func (e *pendingAttemptError) Unwrap() error {
	return e.cause
}

// IsPendingExecution reports whether Execute returned before a non-cooperative
// action exited. The task remains executing and owns its resources until its
// asynchronous finalization completes.
func IsPendingExecution(err error) bool {
	var pending *pendingAttemptError
	return errors.As(err, &pending)
}

// WaitPendingExecution waits for the asynchronous task finalizer when err is a
// pending execution error. Other errors are returned unchanged.
func WaitPendingExecution(err error) error {
	var pending *pendingAttemptError
	if !errors.As(err, &pending) {
		return err
	}
	<-pending.finalized
	return pending.cause
}

const (
	TaskInstall   = "TaskInstall"
	TaskUninstall = "TaskUninstall"
	TaskCreate    = "TaskCreate"
	TaskDelete    = "TaskDelete"
	TaskUpgrade   = "TaskUpgrade"
	TaskUpdate    = "TaskUpdate"
	TaskRestart   = "TaskRestart"
	TaskBackup    = "TaskBackup"
	TaskRecover   = "TaskRecover"
	TaskRollback  = "TaskRollback"
	TaskSync      = "TaskSync"
	TaskBuild     = "TaskBuild"
	TaskPull      = "TaskPull"
	TaskImport    = "TaskImport"
	TaskExport    = "TaskExport"
	TaskCommit    = "TaskCommit"
	TaskPush      = "TaskPush"
	TaskClean     = "TaskClean"
	TaskHandle    = "TaskHandle"
	TaskScan      = "TaskScan"
	TaskExec      = "TaskExec"
	TaskBatch     = "TaskBatch"
	TaskProtect   = "TaskProtect"
	TaskConvert   = "TaskConvert"
	TaskSwapSet   = "TaskSwapSet"
)

const (
	TaskScopeWebsite          = "Website"
	TaskScopeAI               = "AI"
	TaskScopeApp              = "App"
	TaskScopeRuntime          = "Runtime"
	TaskScopeDatabase         = "Database"
	TaskScopeCronjob          = "Cronjob"
	TaskScopeClam             = "Clam"
	TaskScopeSystem           = "System"
	TaskScopeAppStore         = "AppStore"
	TaskScopeSnapshot         = "Snapshot"
	TaskScopeContainer        = "Container"
	TaskScopeCompose          = "Compose"
	TaskScopeImage            = "Image"
	TaskScopeBackup           = "Backup"
	TaskScopeRuntimeExtension = "RuntimeExtension"
	TaskScopeCustomAppstore   = "CustomAppstore"
	TaskScopeTamper           = "Tamper"
	TaskScopeFileConvert      = "Convert"
	TaskScopeTask             = "Task"
)

func GetTaskName(resourceName, operate, scope string) string {
	return fmt.Sprintf("%s%s [%s]", i18n.GetMsgByKey(operate), i18n.GetMsgByKey(scope), resourceName)
}

func NewTaskWithOps(resourceName, operate, scope, taskID string, resourceID uint) (*Task, error) {
	return NewTask(GetTaskName(resourceName, operate, scope), operate, scope, taskID, resourceID)
}

func CheckTaskIsExecuting(name string) error {
	taskRepo := repo.NewITaskRepo()
	task, _ := taskRepo.GetFirst(taskRepo.WithByStatus(constant.StatusExecuting), repo.WithByName(name))
	if task.ID != "" {
		return buserr.New("TaskIsExecuting")
	}
	return nil
}

func CheckResourceTaskIsExecuting(operate, scope string, resourceID uint) bool {
	taskRepo := repo.NewITaskRepo()
	task, _ := taskRepo.GetFirst(
		taskRepo.WithByStatus(constant.StatusExecuting),
		taskRepo.WithResourceID(resourceID),
		taskRepo.WithOperate(operate),
		repo.WithByType(scope))
	return task.ID != ""
}

func CheckScopeTaskIsExecuting(scope string, resourceID uint) error {
	taskRepo := repo.NewITaskRepo()
	task, _ := taskRepo.GetFirst(
		taskRepo.WithByStatus(constant.StatusExecuting),
		taskRepo.WithResourceID(resourceID),
		repo.WithByType(scope),
	)
	if task.ID != "" {
		return buserr.New("TaskIsExecuting")
	}
	return nil
}

func NewTask(name, operate, taskScope, taskID string, resourceID uint) (*Task, error) {
	if taskID == "" {
		taskID = uuid.New().String()
	}
	logDir := path.Join(global.Dir.TaskDir, taskScope)
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		if err = os.MkdirAll(logDir, constant.DirPerm); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %w", err)
		}
	}
	logPath := path.Join(global.Dir.TaskDir, taskScope, taskID+".log")
	logger := logrus.New()
	logger.SetFormatter(&SimpleFormatter{})
	logFile, err := os.OpenFile(logPath, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, constant.FilePerm)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	logger.SetOutput(logFile)
	taskModel := &model.Task{
		ID:         taskID,
		Name:       name,
		Type:       taskScope,
		LogFile:    logPath,
		Status:     constant.StatusExecuting,
		ResourceID: resourceID,
		Operate:    operate,
	}
	taskRepo := repo.NewITaskRepo()
	ctx, cancel := context.WithCancel(context.Background())
	global.RegisterTaskCancel(taskID, cancel)
	task := &Task{TaskCtx: ctx, TaskID: taskID, Name: name, logFile: logFile, Logger: logger, taskRepo: taskRepo, Task: taskModel}
	return task, nil
}

func ReNewTaskWithOps(resourceName, operate, scope, taskID string, resourceID uint) (*Task, error) {
	return ReNewTask(GetTaskName(resourceName, operate, scope), operate, scope, taskID, resourceID)
}
func ReNewTask(name, operate, taskScope, taskID string, resourceID uint) (*Task, error) {
	taskRepo := repo.NewITaskRepo()
	taskItem, _ := taskRepo.GetFirst(taskRepo.WithByID(taskID))
	if taskItem.ID == "" {
		return NewTask(name, operate, taskScope, taskID, resourceID)
	}
	logDir := path.Join(global.Dir.TaskDir, taskScope)
	if _, err := os.Stat(logDir); err != nil {
		if err = os.MkdirAll(logDir, constant.DirPerm); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %w", err)
		}
	}

	logPath := path.Join(global.Dir.TaskDir, taskScope, taskID+".log")
	logger := logrus.New()
	logger.SetFormatter(&SimpleFormatter{})
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, constant.FilePerm)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	logger.SetOutput(logFile)
	logger.Print("\n --------------------------------------------------- \n")
	taskItem.Status = constant.StatusExecuting
	ctx, cancel := context.WithCancel(context.Background())
	global.RegisterTaskCancel(taskID, cancel)
	task := &Task{TaskCtx: ctx, TaskID: taskID, Name: name, logFile: logFile, Logger: logger, taskRepo: taskRepo, Task: &taskItem}
	task.updateTask(&taskItem)
	return task, nil
}

func (t *Task) AddSubTask(name string, action ActionFunc, rollback RollbackFunc) {
	subTask := &SubTask{RootTask: t, Name: name, Retry: 0, Timeout: 30 * time.Minute, Action: action, Rollback: rollback}
	t.SubTasks = append(t.SubTasks, subTask)
}

func (t *Task) AddSubTaskWithAlias(key string, action ActionFunc, rollback RollbackFunc) {
	subTask := &SubTask{RootTask: t, Name: i18n.GetMsgByKey(key), StepAlias: key, Retry: 0, Timeout: 30 * time.Minute, Action: action, Rollback: rollback}
	t.SubTasks = append(t.SubTasks, subTask)
}

func (t *Task) AddSubTaskWithOps(name string, action ActionFunc, rollback RollbackFunc, retry int, timeout time.Duration) {
	subTask := &SubTask{RootTask: t, Name: name, Retry: retry, Timeout: timeout, Action: action, Rollback: rollback}
	t.SubTasks = append(t.SubTasks, subTask)
}

func (t *Task) AddSubTaskWithAliasAndOps(key string, action ActionFunc, rollback RollbackFunc, retry int, timeout time.Duration) {
	subTask := &SubTask{RootTask: t, Name: i18n.GetMsgByKey(key), StepAlias: key, Retry: retry, Timeout: timeout, Action: action, Rollback: rollback}
	t.SubTasks = append(t.SubTasks, subTask)
}

func (t *Task) AddSubTaskWithIgnoreErr(name string, action ActionFunc) {
	subTask := &SubTask{RootTask: t, Name: name, Retry: 0, Timeout: 30 * time.Minute, Action: action, Rollback: nil, IgnoreErr: true}
	t.SubTasks = append(t.SubTasks, subTask)
}

func (s *SubTask) Execute() error {
	subTaskName := s.Name
	if s.Name == "" {
		subTaskName = i18n.GetMsgByKey("SubTask")
	}
	parentCtx := s.RootTask.TaskCtx
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	var err error
	for i := 0; i < s.Retry+1; i++ {
		if parentErr := parentCtx.Err(); parentErr != nil {
			return parentErr
		}
		if i > 0 {
			s.RootTask.Log(i18n.GetWithName("TaskRetry", strconv.Itoa(i)))
		}
		var ctx context.Context
		var cancel context.CancelFunc
		if s.Timeout == 0 {
			ctx, cancel = context.WithCancel(parentCtx)
		} else {
			ctx, cancel = context.WithTimeout(parentCtx, s.Timeout)
		}
		attemptTask := s.newAttemptTask(ctx)

		done := make(chan error, 1)
		go func() {
			done <- s.Action(attemptTask)
		}()

		var stopCause error
		select {
		case <-ctx.Done():
			stopCause = subTaskStopCause(parentCtx)
			cancel()
			timer := time.NewTimer(s.stopGrace())
			select {
			case err = <-done:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				s.adoptAttemptTask(attemptTask)
				if errors.Is(stopCause, context.Canceled) {
					s.logAttemptStop(subTaskName, stopCause)
				} else {
					if err != nil {
						s.logAttemptStop(subTaskName, stopCause)
					}
					stopCause = nil
				}
			case <-timer.C:
				s.logAttemptStop(subTaskName, stopCause)
				return &pendingAttemptError{
					cause:       stopCause,
					done:        done,
					attemptTask: attemptTask,
					rollback:    s.Rollback,
					finalized:   make(chan struct{}),
				}
			}
		case err = <-done:
			ctxErr := ctx.Err()
			cancel()
			s.adoptAttemptTask(attemptTask)
			if ctxErr != nil {
				stopCause = subTaskStopCause(parentCtx)
				if errors.Is(stopCause, context.Canceled) {
					s.logAttemptStop(subTaskName, stopCause)
				} else {
					if err != nil {
						s.logAttemptStop(subTaskName, stopCause)
					}
					stopCause = nil
				}
			}
		}

		if stopCause != nil {
			err = stopCause
			if errors.Is(stopCause, context.Canceled) {
				if s.Rollback != nil {
					s.Rollback(s.RootTask)
				}
				return err
			}
		} else {
			if err != nil {
				s.RootTask.Log(i18n.GetWithNameAndErr("SubTaskFailed", subTaskName, err))
				if IsNonRetryable(err) {
					if s.Rollback != nil {
						s.Rollback(s.RootTask)
					}
					return err
				}
				if err.Error() == i18n.GetMsgByKey("ErrShutDown") {
					return err
				}
			} else {
				s.RootTask.Log(i18n.GetWithName("SubTaskSuccess", subTaskName))
				return nil
			}
		}

		if i == s.Retry {
			if s.Rollback != nil {
				s.Rollback(s.RootTask)
			}
			break
		}
		timer := time.NewTimer(s.retryInterval())
		select {
		case <-timer.C:
		case <-parentCtx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			if s.Rollback != nil {
				s.Rollback(s.RootTask)
			}
			return parentCtx.Err()
		}
	}
	return err
}

func (s *SubTask) newAttemptTask(ctx context.Context) *Task {
	attemptTask := *s.RootTask
	attemptTask.TaskCtx = ctx
	if s.RootTask.Task != nil {
		attemptModel := *s.RootTask.Task
		attemptTask.Task = &attemptModel
	}
	return &attemptTask
}

func (s *SubTask) adoptAttemptTask(attemptTask *Task) {
	if s.RootTask.Task != nil && attemptTask.Task != nil {
		*s.RootTask.Task = *attemptTask.Task
	}
}

func (s *SubTask) stopGrace() time.Duration {
	if s.StopGrace > 0 {
		return s.StopGrace
	}
	return defaultSubTaskStopGrace
}

func (s *SubTask) retryInterval() time.Duration {
	if s.RetryInterval > 0 {
		return s.RetryInterval
	}
	return defaultSubTaskRetryInterval
}

func (s *SubTask) logAttemptStop(subTaskName string, cause error) {
	if errors.Is(cause, context.Canceled) {
		s.RootTask.Log(i18n.GetWithNameAndErr("SubTaskFailed", subTaskName, cause))
		return
	}
	s.RootTask.Log(i18n.GetWithName("TaskTimeout", subTaskName))
}

func subTaskStopCause(parentCtx context.Context) error {
	if errors.Is(parentCtx.Err(), context.Canceled) {
		return context.Canceled
	}
	return &subTaskTimeoutError{}
}

func (t *Task) updateTask(task *model.Task) {
	_ = t.taskRepo.Update(context.Background(), task)
}

func (t *Task) Execute() error {
	cleanupOwned := true
	defer func() {
		if cleanupOwned {
			t.cleanupExecution()
		}
	}()
	if err := t.taskRepo.Save(context.Background(), t.Task); err != nil {
		return err
	}
	var err error
	t.Log(i18n.GetWithName("TaskStart", t.Name))
	for i := 0; i < len(t.SubTasks); i++ {
		subTask := t.SubTasks[i]
		t.Task.CurrentStep = subTask.StepAlias
		t.updateTask(t.Task)
		if err = subTask.Execute(); err == nil {
			if subTask.Rollback != nil {
				t.Rollbacks = append(t.Rollbacks, subTask.Rollback)
			}
		} else {
			var pending *pendingAttemptError
			if errors.As(err, &pending) {
				cleanupOwned = false
				t.finalizePendingAttempt(pending)
				return err
			}
			if subTask.IgnoreErr && !errors.Is(err, context.Canceled) {
				err = nil
				continue
			}
			t.Task.ErrorMsg = err.Error()
			if errors.Is(err, context.Canceled) {
				t.Task.Status = constant.StatusCanceled
			} else {
				t.Task.Status = constant.StatusFailed
			}
			for _, rollback := range t.Rollbacks {
				rollback(t)
			}
			t.updateTask(t.Task)
			break
		}
	}
	if t.Task.Status == constant.StatusExecuting {
		t.Task.Status = constant.StatusSuccess
	}
	t.Log("[TASK-END]")
	t.Task.EndAt = time.Now()
	t.updateTask(t.Task)
	return err
}

// ExecuteToCompletion executes the task and, when an action outlives its stop
// grace, waits for that action and all task finalization to finish. Background
// workers should use this method before releasing locks, closing resources, or
// updating business records derived from the task result.
func (t *Task) ExecuteToCompletion() error {
	return WaitPendingExecution(t.Execute())
}

func (t *Task) finalizePendingAttempt(pending *pendingAttemptError) {
	finalizerTask := *t
	if t.Task != nil {
		finalizerModel := *t.Task
		finalizerTask.Task = &finalizerModel
	}
	finalizerTask.Rollbacks = append([]RollbackFunc(nil), t.Rollbacks...)

	pending.finalizeOnce.Do(func() {
		go func() {
			defer close(pending.finalized)
			defer finalizerTask.cleanupExecution()
			<-pending.done
			if pending.attemptTask.Task != nil {
				attemptModel := *pending.attemptTask.Task
				finalizerTask.Task = &attemptModel
			}
			if pending.rollback != nil {
				pending.rollback(&finalizerTask)
			}
			for _, rollback := range finalizerTask.Rollbacks {
				rollback(&finalizerTask)
			}
			finalizerTask.Task.ErrorMsg = pending.cause.Error()
			if errors.Is(pending.cause, context.Canceled) {
				finalizerTask.Task.Status = constant.StatusCanceled
			} else {
				finalizerTask.Task.Status = constant.StatusFailed
			}
			finalizerTask.Log("[TASK-END]")
			finalizerTask.Task.EndAt = time.Now()
			finalizerTask.updateTask(finalizerTask.Task)
		}()
	})
}

func (t *Task) cleanupExecution() {
	if t.logFile != nil {
		_ = t.logFile.Close()
	}
	global.RemoveTaskCancel(t.TaskID)
}

func (t *Task) DeleteLogFile() {
	_ = os.Remove(t.Task.LogFile)
}

func (t *Task) LogWithStatus(msg string, err error) {
	if err != nil {
		t.Logger.Print(i18n.GetWithNameAndErr("FailedStatus", msg, err))
	} else {
		t.Logger.Print(i18n.GetWithName("SuccessStatus", msg))
	}
}

func (t *Task) Log(msg string) {
	t.Logger.Print(msg)
}

func (t *Task) Logf(format string, v ...any) {
	t.Logger.Printf(format, v...)
}

func (t *Task) LogFailed(msg string) {
	t.Logger.Println(msg + i18n.GetMsgByKey("Failed"))
}

func (t *Task) LogFailedWithErr(msg string, err error) {
	t.Logger.Printf("%s %s : %s", msg, i18n.GetMsgByKey("Failed"), err.Error())
}

func (t *Task) LogSuccess(msg string) {
	t.Logger.Println(msg + " " + i18n.GetMsgByKey("Success"))
}
func (t *Task) LogSuccessF(format string, v ...any) {
	t.Logger.Println(fmt.Sprintf(format, v...) + i18n.GetMsgByKey("Success"))
}

func (t *Task) LogStart(msg string) {
	t.Logger.Printf("%s%s", i18n.GetMsgByKey("Start"), msg)
}

func (t *Task) LogWithOps(operate, msg string) {
	t.Logger.Printf("%s%s", i18n.GetMsgByKey(operate), msg)
}

func (t *Task) LogSuccessWithOps(operate, msg string) {
	t.Logger.Printf("%s%s%s", i18n.GetMsgByKey(operate), msg, i18n.GetMsgByKey("Success"))
}

func (t *Task) LogFailedWithOps(operate, msg string, err error) {
	t.Logger.Printf("%s%s : %s ", msg, i18n.GetMsgByKey("Failed"), err.Error())
}

func (t *Task) LogWithProgress(msg string, current int, total int) {
	const barWidth = 10
	filled := int(float64(current) / float64(total) * 100 / 10)
	if filled > barWidth {
		filled = barWidth
	}
	if filled < 0 {
		filled = 0
	}
	bar := strings.Repeat("=", filled) + strings.Repeat("-", barWidth-filled)
	t.Logger.Printf("%s [%s] %.2f%%", msg, bar, float64(current)/float64(total)*100)
}

type SimpleFormatter struct{}

func (f *SimpleFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	timestamp := entry.Time.Format("2006/01/02 15:04:05")
	message := fmt.Sprintf("%s %s\n", timestamp, entry.Message)
	return []byte(message), nil
}
