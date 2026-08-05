package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSnapshotUploadContextInheritsParentCancellation(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	ctx, cancel := snapshotUploadContext(parent, time.Minute)
	defer cancel()

	cancelParent()
	select {
	case <-ctx.Done():
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatalf("context error = %v, want context.Canceled", ctx.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("snapshot upload context did not inherit parent cancellation")
	}
}

func TestSnapshotUploadContextAddsTimeout(t *testing.T) {
	ctx, cancel := snapshotUploadContext(context.Background(), 10*time.Millisecond)
	defer cancel()

	select {
	case <-ctx.Done():
		if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			t.Fatalf("context error = %v, want context.DeadlineExceeded", ctx.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("snapshot upload context did not enforce its timeout")
	}
}
