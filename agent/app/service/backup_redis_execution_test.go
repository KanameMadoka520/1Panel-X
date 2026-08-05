package service

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/task"
	"github.com/1Panel-dev/1Panel/agent/i18n"
)

func TestRedisRecoveryRollbackUsesFrozenModeWithoutReinspection(t *testing.T) {
	recoverErr := errors.New("synthetic recovery failure")
	calls := make([]string, 0, 2)
	apply := func(file string) error {
		calls = append(calls, file)
		if file == "requested.rdb" {
			return recoverErr
		}
		return nil
	}

	err := recoverRedisWithRollback("requested.rdb", "rollback.rdb", apply)
	if !errors.Is(err, recoverErr) {
		t.Fatalf("recoverRedisWithRollback() error = %v, want original recovery error", err)
	}
	if got := strings.Join(calls, ","); got != "requested.rdb,rollback.rdb" {
		t.Fatalf("recovery sequence = %q, want requested then rollback", got)
	}
}

func TestRedisRecoveryRollbackFailureIsNonRetryableAndRedacted(t *testing.T) {
	const sensitiveMarker = "provider-detail-must-not-leak"
	calls := 0
	err := recoverRedisWithRollback("requested.rdb", "rollback.rdb", func(string) error {
		calls++
		return errors.New(sensitiveMarker)
	})
	if err == nil || !task.IsNonRetryable(err) {
		t.Fatalf("rollback failure = %v, want non-retryable error", err)
	}
	if !errors.Is(err, errRedisRecoveryRollbackFailed) {
		t.Fatalf("rollback failure = %v, want rollback failure sentinel", err)
	}
	if strings.Contains(err.Error(), sensitiveMarker) {
		t.Fatalf("rollback failure leaked internal detail: %v", err)
	}
	if calls != 2 {
		t.Fatalf("recovery apply calls = %d, want 2", calls)
	}
}

func TestRedisRecoverySuccessDoesNotRunRollback(t *testing.T) {
	calls := 0
	if err := recoverRedisWithRollback("requested.rdb", "rollback.rdb", func(string) error {
		calls++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("recovery apply calls = %d, want 1", calls)
	}
}

func TestRedisParentTaskRegistersWithoutImmediateExecution(t *testing.T) {
	i18n.Init()
	tests := []struct {
		name     string
		schedule func(*task.Task, task.ActionFunc, func() error) error
		retry    int
		timeout  time.Duration
	}{
		{
			name: "backup",
			schedule: func(parent *task.Task, action task.ActionFunc, execute func() error) error {
				return scheduleRedisBackupTask(parent, parent, action, execute, func(error) {})
			},
			retry:   3,
			timeout: time.Hour,
		},
		{
			name: "recover",
			schedule: func(parent *task.Task, action task.ActionFunc, execute func() error) error {
				return scheduleRedisRecoverTask(parent, parent, action, execute)
			},
			retry:   0,
			timeout: 30 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent := &task.Task{}
			var actionCalls atomic.Int32
			var executeCalls atomic.Int32
			action := func(*task.Task) error {
				actionCalls.Add(1)
				return nil
			}
			execute := func() error {
				executeCalls.Add(1)
				return nil
			}

			if err := tt.schedule(parent, action, execute); err != nil {
				t.Fatalf("schedule parent Redis %s task: %v", tt.name, err)
			}
			if actionCalls.Load() != 0 || executeCalls.Load() != 0 {
				t.Fatalf("parent Redis %s executed immediately: action=%d execute=%d", tt.name, actionCalls.Load(), executeCalls.Load())
			}
			if len(parent.SubTasks) != 1 {
				t.Fatalf("parent Redis %s registered %d subtasks, want 1", tt.name, len(parent.SubTasks))
			}
			registered := parent.SubTasks[0]
			if registered.Retry != tt.retry || registered.Timeout != tt.timeout {
				t.Fatalf("parent Redis %s task options = retry:%d timeout:%s, want retry:%d timeout:%s", tt.name, registered.Retry, registered.Timeout, tt.retry, tt.timeout)
			}
			if err := registered.Action(parent); err != nil {
				t.Fatalf("execute registered Redis %s action: %v", tt.name, err)
			}
			if actionCalls.Load() != 1 || executeCalls.Load() != 0 {
				t.Fatalf("parent Redis %s execution counts = action:%d execute:%d, want 1 and 0", tt.name, actionCalls.Load(), executeCalls.Load())
			}
		})
	}
}

func TestRedisStandaloneTaskExecutesOnceWithoutOverlap(t *testing.T) {
	i18n.Init()
	tests := []struct {
		name     string
		schedule func(*task.Task, task.ActionFunc, func() error, func(error)) error
	}{
		{
			name: "backup",
			schedule: func(itemTask *task.Task, action task.ActionFunc, execute func() error, complete func(error)) error {
				return scheduleRedisBackupTask(itemTask, nil, action, execute, complete)
			},
		},
		{
			name: "recover",
			schedule: func(itemTask *task.Task, action task.ActionFunc, execute func() error, _ func(error)) error {
				return scheduleRedisRecoverTask(itemTask, nil, action, execute)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			itemTask := &task.Task{}
			started := make(chan struct{}, 2)
			release := make(chan struct{})
			executeDone := make(chan struct{}, 2)
			completionDone := make(chan struct{}, 2)
			var actionCalls atomic.Int32
			var active atomic.Int32
			var maxActive atomic.Int32
			var executeCalls atomic.Int32
			var completionCalls atomic.Int32

			action := func(*task.Task) error {
				actionCalls.Add(1)
				current := active.Add(1)
				for {
					maximum := maxActive.Load()
					if current <= maximum || maxActive.CompareAndSwap(maximum, current) {
						break
					}
				}
				started <- struct{}{}
				<-release
				active.Add(-1)
				return nil
			}
			execute := func() error {
				executeCalls.Add(1)
				defer func() { executeDone <- struct{}{} }()
				if len(itemTask.SubTasks) != 1 {
					t.Errorf("standalone Redis %s executor saw %d subtasks, want 1", tt.name, len(itemTask.SubTasks))
					return nil
				}
				return itemTask.SubTasks[0].Action(itemTask)
			}
			complete := func(error) {
				completionCalls.Add(1)
				completionDone <- struct{}{}
			}

			returned := make(chan error, 1)
			go func() {
				returned <- tt.schedule(itemTask, action, execute, complete)
			}()

			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatalf("standalone Redis %s action did not start", tt.name)
			}
			select {
			case <-started:
				t.Errorf("standalone Redis %s started a concurrent duplicate action", tt.name)
			case <-time.After(100 * time.Millisecond):
			}
			close(release)

			select {
			case err := <-returned:
				if err != nil {
					t.Fatalf("schedule standalone Redis %s task: %v", tt.name, err)
				}
			case <-time.After(time.Second):
				t.Fatalf("standalone Redis %s scheduler did not return", tt.name)
			}
			select {
			case <-executeDone:
			case <-time.After(time.Second):
				t.Fatalf("standalone Redis %s executor did not finish", tt.name)
			}
			if tt.name == "backup" {
				select {
				case <-completionDone:
				case <-time.After(time.Second):
					t.Fatal("standalone Redis backup completion did not run")
				}
			}

			if len(itemTask.SubTasks) != 1 {
				t.Fatalf("standalone Redis %s registered %d subtasks, want 1", tt.name, len(itemTask.SubTasks))
			}
			if actionCalls.Load() != 1 || executeCalls.Load() != 1 || maxActive.Load() != 1 {
				t.Fatalf("standalone Redis %s counts = action:%d execute:%d max-concurrent:%d, want 1/1/1", tt.name, actionCalls.Load(), executeCalls.Load(), maxActive.Load())
			}
			if tt.name == "backup" && completionCalls.Load() != 1 {
				t.Fatalf("standalone Redis backup completion calls = %d, want 1", completionCalls.Load())
			}
			if tt.name == "recover" && completionCalls.Load() != 0 {
				t.Fatalf("standalone Redis recover completion calls = %d, want 0", completionCalls.Load())
			}
		})
	}
}

func TestRedisStandaloneBackupFinalizationWaitsForExecutorExit(t *testing.T) {
	i18n.Init()
	itemTask := &task.Task{}
	started := make(chan struct{})
	release := make(chan struct{})
	completed := make(chan error, 1)
	wantErr := errors.New("synthetic task result")
	var completionCalls atomic.Int32

	execute := func() error {
		close(started)
		<-release
		return wantErr
	}
	complete := func(err error) {
		completionCalls.Add(1)
		completed <- err
	}
	if err := scheduleRedisBackupTask(itemTask, nil, func(*task.Task) error { return nil }, execute, complete); err != nil {
		t.Fatalf("schedule Redis backup: %v", err)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("Redis backup executor did not start")
	}
	select {
	case err := <-completed:
		t.Fatalf("completion ran before executor exit: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if completionCalls.Load() != 0 {
		t.Fatalf("completion calls before executor exit = %d, want 0", completionCalls.Load())
	}

	close(release)
	select {
	case err := <-completed:
		if !errors.Is(err, wantErr) {
			t.Fatalf("completion error = %v, want %v", err, wantErr)
		}
	case <-time.After(time.Second):
		t.Fatal("completion did not run after executor exit")
	}
	if completionCalls.Load() != 1 {
		t.Fatalf("completion calls after executor exit = %d, want 1", completionCalls.Load())
	}
}
