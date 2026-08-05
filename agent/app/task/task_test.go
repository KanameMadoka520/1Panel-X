package task

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/app/repo"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/i18n"
	"github.com/sirupsen/logrus"
)

type recordingTaskRepo struct {
	mu        sync.Mutex
	snapshots []model.Task
	updates   chan model.Task
}

func newRecordingTaskRepo() *recordingTaskRepo {
	return &recordingTaskRepo{updates: make(chan model.Task, 64)}
}

func (r *recordingTaskRepo) Save(_ context.Context, task *model.Task) error {
	r.record(task)
	return nil
}

func (r *recordingTaskRepo) GetFirst(_ ...repo.DBOption) (model.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.snapshots) == 0 {
		return model.Task{}, nil
	}
	return r.snapshots[len(r.snapshots)-1], nil
}

func (r *recordingTaskRepo) Page(_, _ int, _ ...repo.DBOption) (int64, []model.Task, error) {
	return 0, nil, nil
}

func (r *recordingTaskRepo) Update(_ context.Context, task *model.Task) error {
	r.record(task)
	return nil
}

func (r *recordingTaskRepo) UpdateRunningTaskToFailed() error {
	return nil
}

func (r *recordingTaskRepo) CountExecutingTask() (int64, error) {
	return 0, nil
}

func (r *recordingTaskRepo) Delete(_ ...repo.DBOption) error {
	return nil
}

func (r *recordingTaskRepo) DeleteAll() error {
	return nil
}

func (r *recordingTaskRepo) WithByID(string) repo.DBOption {
	return nil
}

func (r *recordingTaskRepo) WithByIDNotIn([]string) repo.DBOption {
	return nil
}

func (r *recordingTaskRepo) WithResourceID(uint) repo.DBOption {
	return nil
}

func (r *recordingTaskRepo) WithOperate(string) repo.DBOption {
	return nil
}

func (r *recordingTaskRepo) WithByStatus(string) repo.DBOption {
	return nil
}

func (r *recordingTaskRepo) record(task *model.Task) {
	snapshot := *task
	r.mu.Lock()
	r.snapshots = append(r.snapshots, snapshot)
	r.mu.Unlock()
	r.updates <- snapshot
}

func (r *recordingTaskRepo) latest() model.Task {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.snapshots) == 0 {
		return model.Task{}
	}
	return r.snapshots[len(r.snapshots)-1]
}

func discardTask(taskID string, ctx context.Context) *Task {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return &Task{
		TaskCtx: ctx,
		TaskID:  taskID,
		Logger:  logger,
		Task: &model.Task{
			ID:     taskID,
			Status: constant.StatusExecuting,
		},
	}
}

func TestSubTaskAttemptReceivesScopedContextAndCopiesModelBack(t *testing.T) {
	i18n.Init()
	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	root := discardTask("attempt-context-test", rootCtx)
	root.Task.ResourceID = 10

	var gotDeadline bool
	subTask := &SubTask{
		RootTask: root,
		Name:     "context propagation",
		Timeout:  200 * time.Millisecond,
		Action: func(attempt *Task) error {
			if attempt == root {
				return errors.New("action received the root task instead of an attempt copy")
			}
			if attempt.Task == root.Task {
				return errors.New("action received the shared task model")
			}
			deadline, ok := attempt.TaskCtx.Deadline()
			if !ok || time.Until(deadline) <= 0 {
				return errors.New("attempt context has no active deadline")
			}
			gotDeadline = true
			attempt.Task.ResourceID = 20
			return nil
		},
	}

	if err := subTask.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !gotDeadline {
		t.Fatal("action did not observe the attempt deadline")
	}
	if root.Task.ResourceID != 20 {
		t.Fatalf("root resource ID = %d, want copied value 20", root.Task.ResourceID)
	}
}

func TestSubTaskParentCancellationReachesAttemptBeforeRollback(t *testing.T) {
	i18n.Init()
	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	root := discardTask("attempt-cancel-test", rootCtx)
	started := make(chan struct{})
	result := make(chan error, 1)
	var active atomic.Int32
	var rollbackWhileActive atomic.Bool

	subTask := &SubTask{
		RootTask:  root,
		Name:      "canceled action",
		Timeout:   time.Minute,
		StopGrace: 200 * time.Millisecond,
		Action: func(attempt *Task) error {
			active.Add(1)
			close(started)
			<-attempt.TaskCtx.Done()
			active.Add(-1)
			return nil
		},
		Rollback: func(*Task) {
			if active.Load() != 0 {
				rollbackWhileActive.Store(true)
			}
		},
	}

	go func() {
		result <- subTask.Execute()
	}()
	<-started
	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Execute() error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Execute() did not return after cooperative cancellation")
	}
	if rollbackWhileActive.Load() {
		t.Fatal("rollback overlapped the canceled action")
	}
}

func TestSubTaskCooperativeTimeoutDoesNotOverlapRetriesOrRollback(t *testing.T) {
	i18n.Init()
	root := discardTask("cooperative-timeout-test", context.Background())
	var calls atomic.Int32
	var active atomic.Int32
	var maxActive atomic.Int32
	var rollbackCalls atomic.Int32
	var rollbackWhileActive atomic.Bool

	subTask := &SubTask{
		RootTask:      root,
		Name:          "cooperative timeout",
		Retry:         2,
		Timeout:       15 * time.Millisecond,
		StopGrace:     100 * time.Millisecond,
		RetryInterval: time.Millisecond,
		Action: func(attempt *Task) error {
			calls.Add(1)
			current := active.Add(1)
			updateAtomicMax(&maxActive, current)
			<-attempt.TaskCtx.Done()
			active.Add(-1)
			return attempt.TaskCtx.Err()
		},
		Rollback: func(*Task) {
			rollbackCalls.Add(1)
			if active.Load() != 0 {
				rollbackWhileActive.Store(true)
			}
		},
	}

	err := subTask.Execute()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Execute() error = %v, want deadline exceeded", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("action calls = %d, want 3", calls.Load())
	}
	if maxActive.Load() != 1 {
		t.Fatalf("maximum concurrent actions = %d, want 1", maxActive.Load())
	}
	if rollbackCalls.Load() != 1 {
		t.Fatalf("rollback calls = %d, want 1", rollbackCalls.Load())
	}
	if rollbackWhileActive.Load() {
		t.Fatal("rollback overlapped a timed-out action")
	}
}

func TestSubTaskDeadlineGraceSuccessDoesNotRetry(t *testing.T) {
	i18n.Init()
	root := discardTask("deadline-grace-success-test", context.Background())
	var calls atomic.Int32
	var rollbackCalls atomic.Int32

	subTask := &SubTask{
		RootTask:      root,
		Name:          "deadline grace success",
		Retry:         3,
		Timeout:       10 * time.Millisecond,
		StopGrace:     100 * time.Millisecond,
		RetryInterval: time.Millisecond,
		Action: func(attempt *Task) error {
			calls.Add(1)
			<-attempt.TaskCtx.Done()
			time.Sleep(5 * time.Millisecond)
			return nil
		},
		Rollback: func(*Task) {
			rollbackCalls.Add(1)
		},
	}

	if err := subTask.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, want success", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("action calls = %d, want 1", calls.Load())
	}
	if rollbackCalls.Load() != 0 {
		t.Fatalf("rollback calls = %d, want 0", rollbackCalls.Load())
	}
}

func TestSubTaskDeadlineGracePreservesNonRetryableError(t *testing.T) {
	i18n.Init()
	root := discardTask("deadline-grace-non-retryable-test", context.Background())
	sentinel := errors.New("cleanup state is unknown")
	var calls atomic.Int32
	var rollbackCalls atomic.Int32

	subTask := &SubTask{
		RootTask:      root,
		Name:          "deadline grace non-retryable",
		Retry:         3,
		Timeout:       10 * time.Millisecond,
		StopGrace:     100 * time.Millisecond,
		RetryInterval: time.Millisecond,
		Action: func(attempt *Task) error {
			calls.Add(1)
			<-attempt.TaskCtx.Done()
			return MarkNonRetryable(sentinel)
		},
		Rollback: func(*Task) {
			rollbackCalls.Add(1)
		},
	}

	err := subTask.Execute()
	if !errors.Is(err, sentinel) || !IsNonRetryable(err) {
		t.Fatalf("Execute() error = %v, want preserved non-retryable sentinel", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("action calls = %d, want 1", calls.Load())
	}
	if rollbackCalls.Load() != 1 {
		t.Fatalf("rollback calls = %d, want 1", rollbackCalls.Load())
	}
}

func TestSubTaskOrdinaryErrorRetriesWithoutOverlap(t *testing.T) {
	i18n.Init()
	root := discardTask("ordinary-retry-test", context.Background())
	var calls atomic.Int32
	var active atomic.Int32
	var maxActive atomic.Int32
	var rollbackCalls atomic.Int32

	subTask := &SubTask{
		RootTask:      root,
		Name:          "ordinary retry",
		Retry:         2,
		Timeout:       time.Second,
		RetryInterval: time.Millisecond,
		Action: func(*Task) error {
			call := calls.Add(1)
			current := active.Add(1)
			updateAtomicMax(&maxActive, current)
			active.Add(-1)
			if call < 3 {
				return fmt.Errorf("attempt %d failed", call)
			}
			return nil
		},
		Rollback: func(*Task) {
			rollbackCalls.Add(1)
		},
	}

	if err := subTask.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("action calls = %d, want 3", calls.Load())
	}
	if maxActive.Load() != 1 {
		t.Fatalf("maximum concurrent actions = %d, want 1", maxActive.Load())
	}
	if rollbackCalls.Load() != 0 {
		t.Fatalf("rollback calls = %d, want 0", rollbackCalls.Load())
	}
}

func TestSubTaskNonRetryableErrorStopsRetries(t *testing.T) {
	i18n.Init()
	sentinel := errors.New("cleanup failed")
	actionCalls := 0
	rollbackCalls := 0
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	subTask := &SubTask{
		RootTask: &Task{TaskCtx: context.Background(), TaskID: "non-retryable-test", Logger: logger},
		Name:     "cleanup",
		Retry:    3,
		Timeout:  time.Second,
		Action: func(*Task) error {
			actionCalls++
			return MarkNonRetryable(sentinel)
		},
		Rollback: func(*Task) {
			rollbackCalls++
		},
	}

	startedAt := time.Now()
	err := subTask.Execute()
	if !errors.Is(err, sentinel) {
		t.Fatalf("Execute() error = %v, want sentinel", err)
	}
	if actionCalls != 1 {
		t.Fatalf("action calls = %d, want 1", actionCalls)
	}
	if rollbackCalls != 1 {
		t.Fatalf("rollback calls = %d, want 1", rollbackCalls)
	}
	if elapsed := time.Since(startedAt); elapsed >= time.Second {
		t.Fatalf("non-retryable action waited for retry delay: %s", elapsed)
	}
}

func TestMarkNonRetryableIsNilSafeAndIdempotent(t *testing.T) {
	if marked := MarkNonRetryable(nil); marked != nil {
		t.Fatalf("MarkNonRetryable(nil) = %v, want nil", marked)
	}

	sentinel := errors.New("sentinel")
	marked := MarkNonRetryable(sentinel)
	if !IsNonRetryable(marked) || !errors.Is(marked, sentinel) {
		t.Fatalf("marked error = %v, marker or unwrap missing", marked)
	}
	if markedAgain := MarkNonRetryable(marked); markedAgain != marked {
		t.Fatal("MarkNonRetryable added a second wrapper")
	}
}

func TestSubTaskKeepsTaskCancellationRegistered(t *testing.T) {
	i18n.Init()
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	taskID := "task-cancel-lifetime-test"
	global.RegisterTaskCancel(taskID, func() {})
	defer global.RemoveTaskCancel(taskID)

	subTask := &SubTask{
		RootTask: &Task{TaskCtx: context.Background(), TaskID: taskID, Logger: logger},
		Name:     "first step",
		Timeout:  time.Second,
		Action:   func(*Task) error { return nil },
	}
	if err := subTask.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if _, ok := global.LoadTaskCancel(taskID); !ok {
		t.Fatal("subtask removed the root task cancellation registration")
	}
}

func TestTaskDetachedAttemptFinalizesAfterTimeout(t *testing.T) {
	testDetachedAttemptFinalization(t, false)
}

func TestTaskDetachedAttemptFinalizesAfterParentCancellation(t *testing.T) {
	testDetachedAttemptFinalization(t, true)
}

func testDetachedAttemptFinalization(t *testing.T, parentCancellation bool) {
	t.Helper()
	i18n.Init()
	taskID := "detached-timeout-test"
	expectedStatus := constant.StatusFailed
	if parentCancellation {
		taskID = "detached-cancellation-test"
		expectedStatus = constant.StatusCanceled
	}

	taskRepo := newRecordingTaskRepo()
	logPath := filepath.Join(t.TempDir(), taskID+".log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	logger := logrus.New()
	logger.SetFormatter(&SimpleFormatter{})
	logger.SetOutput(logFile)
	rootCtx, rootCancel := context.WithCancel(context.Background())
	global.RegisterTaskCancel(taskID, rootCancel)
	defer func() {
		rootCancel()
		global.RemoveTaskCancel(taskID)
		_ = logFile.Close()
	}()

	taskItem := &Task{
		TaskCtx:  rootCtx,
		TaskID:   taskID,
		Name:     "detached task",
		Logger:   logger,
		logFile:  logFile,
		taskRepo: taskRepo,
		Task: &model.Task{
			ID:      taskID,
			Name:    "detached task",
			LogFile: logPath,
			Status:  constant.StatusExecuting,
		},
	}

	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	started := make(chan struct{})
	var startedOnce sync.Once
	var calls atomic.Int32
	var active atomic.Int32
	var currentRollbacks atomic.Int32
	var previousRollbacks atomic.Int32
	var rollbackWhileActive atomic.Bool
	var orderMu sync.Mutex
	var rollbackOrder []string

	taskItem.AddSubTaskWithAlias("first", func(*Task) error { return nil }, func(*Task) {
		previousRollbacks.Add(1)
		if active.Load() != 0 {
			rollbackWhileActive.Store(true)
		}
		orderMu.Lock()
		rollbackOrder = append(rollbackOrder, "previous")
		orderMu.Unlock()
	})
	taskItem.AddSubTaskWithAliasAndOps("stuck", func(*Task) error {
		calls.Add(1)
		active.Add(1)
		startedOnce.Do(func() { close(started) })
		<-release
		active.Add(-1)
		return nil
	}, func(*Task) {
		currentRollbacks.Add(1)
		if active.Load() != 0 {
			rollbackWhileActive.Store(true)
		}
		orderMu.Lock()
		rollbackOrder = append(rollbackOrder, "current")
		orderMu.Unlock()
	}, 3, 15*time.Millisecond)
	stuck := taskItem.SubTasks[1]
	stuck.StopGrace = 15 * time.Millisecond
	stuck.RetryInterval = time.Millisecond
	if parentCancellation {
		stuck.Timeout = 0
	}

	result := make(chan error, 1)
	go func() {
		result <- taskItem.Execute()
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("non-cooperative action did not start")
	}
	if parentCancellation {
		rootCancel()
	}

	var executeErr error
	select {
	case executeErr = <-result:
	case <-time.After(time.Second):
		t.Fatal("Task.Execute() blocked on a non-cooperative action")
	}
	var pending *pendingAttemptError
	if !errors.As(executeErr, &pending) {
		t.Fatalf("Task.Execute() error = %T %v, want pendingAttemptError", executeErr, executeErr)
	}
	if !IsPendingExecution(executeErr) {
		t.Fatal("IsPendingExecution() = false, want true")
	}
	if parentCancellation {
		if !errors.Is(executeErr, context.Canceled) {
			t.Fatalf("Task.Execute() error = %v, want context.Canceled", executeErr)
		}
	} else if !errors.Is(executeErr, context.DeadlineExceeded) {
		t.Fatalf("Task.Execute() error = %v, want deadline exceeded", executeErr)
	}

	latest := taskRepo.latest()
	if latest.Status != constant.StatusExecuting {
		t.Fatalf("persisted status before action exit = %s, want %s", latest.Status, constant.StatusExecuting)
	}
	if !latest.EndAt.IsZero() || latest.ErrorMsg != "" {
		t.Fatalf("task finalized before action exit: end=%v error=%q", latest.EndAt, latest.ErrorMsg)
	}
	if calls.Load() != 1 {
		t.Fatalf("action calls before release = %d, want 1", calls.Load())
	}
	if currentRollbacks.Load() != 0 || previousRollbacks.Load() != 0 {
		t.Fatal("rollback ran before the detached action exited")
	}
	if _, ok := global.LoadTaskCancel(taskID); !ok {
		t.Fatal("task cancellation registration was removed before finalization")
	}
	if _, err := logFile.Stat(); err != nil {
		t.Fatalf("task log was closed before finalization: %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	if calls.Load() != 1 {
		t.Fatalf("detached action was retried: calls = %d", calls.Load())
	}

	releaseOnce.Do(func() { close(release) })
	final := waitForTaskSnapshot(t, taskRepo, func(snapshot model.Task) bool {
		return snapshot.Status == expectedStatus && !snapshot.EndAt.IsZero()
	})
	waitForCondition(t, time.Second, func() bool {
		_, ok := global.LoadTaskCancel(taskID)
		return !ok
	}, "task cancellation registration removal")

	if final.ErrorMsg == "" {
		t.Fatal("final task error message is empty")
	}
	if calls.Load() != 1 {
		t.Fatalf("action calls after finalization = %d, want 1", calls.Load())
	}
	if currentRollbacks.Load() != 1 || previousRollbacks.Load() != 1 {
		t.Fatalf("rollback calls current=%d previous=%d, want 1 each", currentRollbacks.Load(), previousRollbacks.Load())
	}
	if rollbackWhileActive.Load() {
		t.Fatal("rollback overlapped the detached action")
	}
	orderMu.Lock()
	order := append([]string(nil), rollbackOrder...)
	orderMu.Unlock()
	if strings.Join(order, ",") != "current,previous" {
		t.Fatalf("rollback order = %v, want current then previous", order)
	}
	if _, err := logFile.Stat(); err == nil {
		t.Fatal("task log remained open after finalization")
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logData), "[TASK-END]") {
		t.Fatal("task log does not contain [TASK-END]")
	}
}

func TestTaskExecuteToCompletionWaitsForDetachedActionFinalization(t *testing.T) {
	i18n.Init()
	taskID := "execute-to-completion-test"
	taskRepo := newRecordingTaskRepo()
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	ctx, cancel := context.WithCancel(context.Background())
	global.RegisterTaskCancel(taskID, cancel)
	defer func() {
		cancel()
		global.RemoveTaskCancel(taskID)
	}()

	taskItem := &Task{
		TaskCtx:  ctx,
		TaskID:   taskID,
		Name:     "execute to completion",
		Logger:   logger,
		taskRepo: taskRepo,
		Task: &model.Task{
			ID:     taskID,
			Name:   "execute to completion",
			Status: constant.StatusExecuting,
		},
	}
	release := make(chan struct{})
	started := make(chan struct{})
	var finalized atomic.Int32
	taskItem.AddSubTaskWithOps("stuck", func(*Task) error {
		close(started)
		<-release
		return nil
	}, func(*Task) {
		finalized.Add(1)
	}, 2, 10*time.Millisecond)
	taskItem.SubTasks[0].StopGrace = 10 * time.Millisecond

	result := make(chan error, 1)
	go func() {
		result <- taskItem.ExecuteToCompletion()
	}()
	<-started
	select {
	case err := <-result:
		t.Fatalf("ExecuteToCompletion() returned before action exit: %v", err)
	case <-time.After(40 * time.Millisecond):
	}
	if finalized.Load() != 0 {
		t.Fatal("rollback ran before action exit")
	}
	if latest := taskRepo.latest(); latest.Status != constant.StatusExecuting {
		t.Fatalf("status before action exit = %s, want %s", latest.Status, constant.StatusExecuting)
	}

	close(release)
	select {
	case err := <-result:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("ExecuteToCompletion() error = %v, want deadline exceeded", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ExecuteToCompletion() did not return after action exit")
	}
	if finalized.Load() != 1 {
		t.Fatalf("rollback calls = %d, want 1", finalized.Load())
	}
	if latest := taskRepo.latest(); latest.Status != constant.StatusFailed || latest.EndAt.IsZero() {
		t.Fatalf("final task = %+v, want finalized failure", latest)
	}
}

func waitForTaskSnapshot(t *testing.T, taskRepo *recordingTaskRepo, predicate func(model.Task) bool) model.Task {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case snapshot := <-taskRepo.updates:
			if predicate(snapshot) {
				return snapshot
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for task snapshot; latest = %+v", taskRepo.latest())
		}
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, predicate func() bool, description string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}

func updateAtomicMax(maximum *atomic.Int32, candidate int32) {
	for {
		current := maximum.Load()
		if candidate <= current || maximum.CompareAndSwap(current, candidate) {
			return
		}
	}
}
