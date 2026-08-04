package task

import (
	"errors"
	"io"
	"testing"
	"time"

	"github.com/1Panel-dev/1Panel/agent/i18n"
	"github.com/sirupsen/logrus"
)

func TestSubTaskNonRetryableErrorStopsRetries(t *testing.T) {
	i18n.Init()
	sentinel := errors.New("cleanup failed")
	actionCalls := 0
	rollbackCalls := 0
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	subTask := &SubTask{
		RootTask: &Task{TaskID: "non-retryable-test", Logger: logger},
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
