package backupsync

import (
	"encoding/hex"
	"sync"
)

var stateBarrier sync.RWMutex

// AcquireMutation serializes an authoritative snapshot apply or enrollment
// namespace change with public-account execution.
func AcquireMutation() func() {
	stateBarrier.Lock()
	return stateBarrier.Unlock
}

// AcquireExecution keeps the validated public-account snapshot stable for the
// duration of one storage operation.
func AcquireExecution() func() {
	stateBarrier.RLock()
	return stateBarrier.RUnlock
}

func ValidIdentity(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
